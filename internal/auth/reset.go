package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/mail"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

const resetLinkTTL = 30 * time.Minute

// ResetOptions is step two of "forgot password": which ways this account can use.
type ResetOptions struct {
	Name        string  `json:"name"`
	MaskedEmail *string `json:"maskedEmail"`
	CanUseEmail bool    `json:"canUseEmail"`
	// The owner-issued code arrives with project members; always false until then.
	CanUseOwnerCode bool `json:"canUseOwnerCode"`
}

// ResetOptions looks the account up by phone, as the design's first step does.
func (s *Service) ResetOptions(ctx context.Context, rawPhone, lang string) (ResetOptions, error) {
	user, err := s.userForReset(ctx, rawPhone, lang)
	if err != nil {
		return ResetOptions{}, err
	}
	opts := ResetOptions{Name: user.FirstName + " " + user.LastName}
	if user.Email != nil {
		masked := maskEmail(*user.Email)
		opts.MaskedEmail = &masked
		opts.CanUseEmail = true
	}
	return opts, nil
}

// SendResetLink emails a single-use link, voiding any earlier ones.
func (s *Service) SendResetLink(ctx context.Context, rawPhone, lang string) error {
	user, err := s.userForReset(ctx, rawPhone, lang)
	if err != nil {
		return err
	}
	if user.Email == nil {
		return fail(apperr.KindInvalid, codeNoEmail)
	}
	if !s.resetSends.Allow(user.ID) {
		return fail(apperr.KindTooManyRequests, codeResetThrottled)
	}

	raw, hash := token.NewOpaque()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if txErr := voidResetLinks(ctx, tx, user.ID); txErr != nil {
			return txErr
		}
		return insertResetLink(ctx, tx, user.ID, hash, time.Now().Add(resetLinkTTL))
	})
	if err != nil {
		return fmt.Errorf("store reset link: %w", err)
	}
	link := s.appURL + "/reset-password?token=" + url.QueryEscape(raw)
	if err := s.mailer.Send(ctx, resetEmail(user, link)); err != nil {
		return fmt.Errorf("send reset email: %w", err)
	}
	return nil
}

// CheckResetLink tells the reset page whether to show the form or the expired state.
func (s *Service) CheckResetLink(ctx context.Context, raw string) error {
	_, _, err := usableResetLink(ctx, s.pool, token.Hash(raw), false)
	if errors.Is(err, errNotFound) {
		return fail(apperr.KindInvalid, codeResetLinkInvalid)
	}
	return err
}

// ResetPassword sets a new password from a link and signs the account out everywhere.
func (s *Service) ResetPassword(ctx context.Context, raw, newPassword, lang string) error {
	if password.Validate(newPassword) != nil {
		return fail(apperr.KindInvalid, codeInvalidInput, apperr.FieldError{Path: "password", Message: fieldMessages["passwordWeak"].in(lang)})
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	var invalid bool
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		linkID, userID, txErr := usableResetLink(ctx, tx, token.Hash(raw), true)
		if errors.Is(txErr, errNotFound) {
			invalid = true
			return nil
		}
		if txErr != nil {
			return txErr
		}
		return applyReset(ctx, tx, linkID, userID, hash)
	})
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if invalid {
		return fail(apperr.KindInvalid, codeResetLinkInvalid)
	}
	return nil
}

// applyReset sets the password, spends the link, voids the rest and signs the account out everywhere.
func applyReset(ctx context.Context, tx pgx.Tx, linkID, userID, hash string) error {
	if err := updatePassword(ctx, tx, userID, hash); err != nil {
		return err
	}
	if err := markResetLinkUsed(ctx, tx, linkID); err != nil {
		return err
	}
	if err := voidResetLinks(ctx, tx, userID); err != nil {
		return err
	}
	return revokeAllForUser(ctx, tx, userID)
}

func (s *Service) userForReset(ctx context.Context, rawPhone, lang string) (User, error) {
	p, err := phone.Normalize(rawPhone)
	if err != nil {
		return User{}, fail(apperr.KindInvalid, codeInvalidInput, apperr.FieldError{Path: "phone", Message: fieldMessages["phoneBad"].in(lang)})
	}
	user, err := userByPhone(ctx, s.pool, p)
	if errors.Is(err, errNotFound) {
		return User{}, fail(apperr.KindNotFound, codeAccountNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}

// maskEmail keeps the first two characters of the local part: ta•••@example.com.
func maskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return email
	}
	runes := []rune(local)
	if len(runes) > 2 {
		runes = runes[:2]
	}
	return string(runes) + "•••@" + domain
}

// resetEmail is the copy from the design board ResetEmail, in the account's language.
func resetEmail(user User, link string) mail.Message {
	name := user.FirstName + " " + user.LastName
	if user.Locale == "en" {
		return mail.Message{
			To:      *user.Email,
			Subject: "Your Prokolpo password reset link",
			Body: fmt.Sprintf("Dear %s,\n\nWe received a request to reset the password for your Prokolpo account (%s). Open this link to set a new password:\n\n%s\n\n"+
				"The link works for 30 minutes and only once.\n\nIf you did not ask for this, you can ignore this email; your password stays the same. "+
				"The Prokolpo team never asks for your password by email or phone.\n\nProkolpo — every taka of your project, in one place", name, user.Phone, link),
		}
	}
	return mail.Message{
		To:      *user.Email,
		Subject: "আপনার প্রকল্প অ্যাকাউন্টের পাসওয়ার্ড বদলের লিংক",
		Body: fmt.Sprintf("প্রিয় %s,\n\nআপনার প্রকল্প অ্যাকাউন্টের (%s) পাসওয়ার্ড বদলানোর অনুরোধ পেয়েছি। নিচের লিংক খুলে নতুন পাসওয়ার্ড দিন:\n\n%s\n\n"+
			"লিংকটা 30 মিনিট আর একবারই কাজ করবে।\n\nআপনি এই অনুরোধ না করে থাকলে কিছু করার দরকার নেই — পাসওয়ার্ড আগের মতোই থাকবে। "+
			"প্রকল্পের টিম কখনো ইমেইল বা ফোনে আপনার পাসওয়ার্ড চায় না।\n\nপ্রকল্প — প্রজেক্টের প্রতিটা টাকার হিসাব, এক জায়গায়", name, user.Phone, link),
	}
}
