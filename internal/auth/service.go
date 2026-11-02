package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/mail"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/ratelimit"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// Registration modes stored in app_settings.
const (
	modeOpen       = "open"
	modeInviteOnly = "invite_only"
	modeClosed     = "closed"
)

const (
	// Five wrong passwords in 15 minutes lock the identifier for the rest of
	// the window (Admin › সিকিউরিটি shows the same rule).
	maxLoginFailures   = 5
	loginFailureWindow = 15 * time.Minute
)

// dummyHash is verified against when the account does not exist, so a wrong
// phone number takes as long as a wrong password.
var dummyHash = mustHash("timing-equaliser-1")

func mustHash(pw string) string {
	h, err := password.Hash(pw)
	if err != nil {
		panic(err) // crypto/rand failed at startup; nothing can work
	}
	return h
}

// ClientMeta describes where a request came from, for the sessions list.
type ClientMeta struct {
	UserAgent string
	IP        string
}

// Service implements the auth use cases.
type Service struct {
	pool          *pgxpool.Pool
	issuer        *token.Issuer
	mailer        mail.Sender
	appURL        string
	log           *slog.Logger
	loginFailures *ratelimit.Window
	resetSends    *ratelimit.Window
}

// NewService wires the auth service. appURL is the frontend origin used in emailed links.
func NewService(pool *pgxpool.Pool, issuer *token.Issuer, mailer mail.Sender, appURL string, log *slog.Logger) *Service {
	return &Service{
		pool: pool, issuer: issuer, mailer: mailer, appURL: appURL, log: log,
		loginFailures: ratelimit.New(maxLoginFailures, loginFailureWindow),
		resetSends:    ratelimit.New(1, time.Minute),
	}
}

// RegistrationMode reports whether new accounts can be opened.
func (s *Service) RegistrationMode(ctx context.Context) (string, error) {
	return registrationMode(ctx, s.pool)
}

// Register opens an account and signs it in.
func (s *Service) Register(ctx context.Context, in RegisterInput, meta ClientMeta) (Session, error) {
	mode, err := registrationMode(ctx, s.pool)
	if err != nil {
		return Session{}, fmt.Errorf("read registration mode: %w", err)
	}
	// Invite-only needs project invitations, which arrive with the members
	// feature; until then it behaves like closed.
	if mode != modeOpen {
		return Session{}, fail(apperr.KindForbidden, codeRegistrationClosed)
	}

	phoneTaken, emailTaken, err := phoneOrEmailTaken(ctx, s.pool, in.Phone, in.Email)
	if err != nil {
		return Session{}, fmt.Errorf("check duplicates: %w", err)
	}
	if dupErr := duplicateError(phoneTaken, emailTaken, in.Locale); dupErr != nil {
		return Session{}, dupErr
	}

	hash, err := password.Hash(in.Password)
	if err != nil {
		return Session{}, err
	}

	var session Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		user, txErr := insertUser(ctx, tx, newUser{
			FirstName: in.FirstName, LastName: in.LastName, CompanyName: in.CompanyName,
			Phone: in.Phone, Email: in.Email, PasswordHash: hash, Locale: in.Locale,
		})
		if txErr != nil {
			return txErr
		}
		session, txErr = s.issueSession(ctx, tx, user, newFamily(), true, meta)
		return txErr
	})
	if isUniqueViolation(err) {
		// Lost a race with another sign-up for the same phone or email.
		return Session{}, duplicateError(true, false, in.Locale)
	}
	if err != nil {
		return Session{}, fmt.Errorf("register: %w", err)
	}
	return session, nil
}

func duplicateError(phoneTaken, emailTaken bool, lang string) error {
	var fields []apperr.FieldError
	if phoneTaken {
		fields = append(fields, apperr.FieldError{Path: "phone", Message: messages[codePhoneTaken].in(lang)})
	}
	if emailTaken {
		fields = append(fields, apperr.FieldError{Path: "email", Message: messages[codeEmailTaken].in(lang)})
	}
	switch {
	case phoneTaken:
		return fail(apperr.KindConflict, codePhoneTaken, fields...)
	case emailTaken:
		return fail(apperr.KindConflict, codeEmailTaken, fields...)
	default:
		return nil
	}
}

// Login checks the password and opens a session. remember keeps it for 30 days instead of one.
func (s *Service) Login(ctx context.Context, rawID, pw string, remember bool, meta ClientMeta) (Session, error) {
	id, ok := parseIdentifier(rawID)
	if !ok {
		return Session{}, fail(apperr.KindUnauthorized, codeInvalidCredentials)
	}
	if s.loginFailures.Exceeded(id.key()) {
		return Session{}, fail(apperr.KindTooManyRequests, codeTooManyAttempts)
	}

	user, err := s.findByIdentifier(ctx, id)
	if err != nil && !errors.Is(err, errNotFound) {
		return Session{}, fmt.Errorf("find user: %w", err)
	}
	hash := dummyHash
	if err == nil {
		hash = user.PasswordHash
	}
	match, verr := password.Verify(pw, hash)
	if verr != nil {
		return Session{}, fmt.Errorf("verify password: %w", verr)
	}
	if err != nil || !match {
		s.loginFailures.Hit(id.key())
		return Session{}, fail(apperr.KindUnauthorized, codeInvalidCredentials)
	}
	// Only after the right password, so suspension does not reveal which numbers have accounts.
	if user.IsSuspended() {
		return Session{}, fail(apperr.KindForbidden, codeAccountSuspended)
	}
	s.loginFailures.Reset(id.key())

	var session Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if txErr := touchLastLogin(ctx, tx, user.ID); txErr != nil {
			return txErr
		}
		var txErr error
		session, txErr = s.issueSession(ctx, tx, user, newFamily(), remember, meta)
		return txErr
	})
	if err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	return session, nil
}

func (s *Service) findByIdentifier(ctx context.Context, id identifier) (User, error) {
	if id.phone != "" {
		return userByPhone(ctx, s.pool, id.phone)
	}
	return userByEmail(ctx, s.pool, id.email)
}

// Me returns the signed-in account.
func (s *Service) Me(ctx context.Context, userID string) (UserView, error) {
	user, err := userByID(ctx, s.pool, userID)
	if errors.Is(err, errNotFound) {
		return UserView{}, fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	if err != nil {
		return UserView{}, fmt.Errorf("load user: %w", err)
	}
	if user.IsSuspended() {
		return UserView{}, fail(apperr.KindForbidden, codeAccountSuspended)
	}
	return user.view(), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
