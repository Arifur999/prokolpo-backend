package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// Profile is the account page (board Profile): who you are, when the password last changed
// and every project you are in.
type Profile struct {
	UserView
	PasswordChangedAt *time.Time       `json:"passwordChangedAt"`
	Projects          []ProfileProject `json:"projects"`
}

// ProfileProject is one project the account belongs to and its role there.
type ProfileProject struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Ownership string `json:"ownership"`
	Status    string `json:"status"`
	Role      string `json:"role"`
}

// Profile loads the account page.
func (s *Service) Profile(ctx context.Context, userID string) (Profile, error) {
	user, err := userByID(ctx, s.pool, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("load user: %w", err)
	}
	p := Profile{UserView: user.view(), Projects: []ProfileProject{}}
	if err = s.pool.QueryRow(ctx, `SELECT password_changed_at FROM users WHERE id = $1`, userID).Scan(&p.PasswordChangedAt); err != nil {
		return Profile{}, fmt.Errorf("load password date: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.kind, p.ownership, p.status, m.role
		FROM project_members m JOIN projects p ON p.id = m.project_id
		WHERE m.user_id = $1 AND p.deleted_at IS NULL
		ORDER BY m.role = 'OWNER' DESC, p.created_at`, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pp ProfileProject
		if err = rows.Scan(&pp.ID, &pp.Name, &pp.Kind, &pp.Ownership, &pp.Status, &pp.Role); err != nil {
			return Profile{}, fmt.Errorf("scan project: %w", err)
		}
		p.Projects = append(p.Projects, pp)
	}
	return p, rows.Err()
}

// ProfileInput is the "আপনার তথ্য" card after validation.
type ProfileInput struct {
	FirstName   string
	LastName    string
	CompanyName *string
	Email       *string
}

type profileRequest struct {
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	CompanyName string `json:"companyName"`
	Email       string `json:"email"`
}

// validate applies the registration form's rules, except that the last name may be blank.
func (req profileRequest) validate(lang string) (ProfileInput, error) {
	var fields []apperr.FieldError
	add := func(path, key string) {
		fields = append(fields, apperr.FieldError{Path: path, Message: fieldMessages[key].In(lang)})
	}
	in := ProfileInput{FirstName: strings.TrimSpace(req.FirstName), LastName: strings.TrimSpace(req.LastName)}
	switch n := utf8.RuneCountInString(in.FirstName); {
	case n == 0:
		add("firstName", "firstMissing")
	case n < 2:
		add("firstName", "firstShort")
	case n > maxNameLen:
		add("firstName", "firstLong")
	}
	if utf8.RuneCountInString(in.LastName) > maxNameLen {
		add("lastName", "lastLong")
	}
	if company := strings.TrimSpace(req.CompanyName); company != "" {
		switch n := utf8.RuneCountInString(company); {
		case n < 2:
			add("companyName", "companyShort")
		case n > maxCompanyLen:
			add("companyName", "companyLong")
		}
		in.CompanyName = &company
	}
	if email := strings.TrimSpace(req.Email); email != "" {
		if !isEmail(email) {
			add("email", "emailBad")
		}
		lower := strings.ToLower(email)
		in.Email = &lower
	}
	if len(fields) > 0 {
		return ProfileInput{}, messages.Fail(apperr.KindInvalid, codeInvalidInput, fields...)
	}
	return in, nil
}

// UpdateProfile saves the account's name, company and email.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in ProfileInput, lang string) (UserView, error) {
	user, err := scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET first_name = $2, last_name = $3, company_name = $4, email = $5
		WHERE id = $1 RETURNING `+userColumns, userID, in.FirstName, in.LastName, in.CompanyName, in.Email))
	if isUniqueViolation(err) {
		return UserView{}, duplicateError(false, true, lang)
	}
	if err != nil {
		return UserView{}, fmt.Errorf("update profile: %w", err)
	}
	return user.view(), nil
}

// SetLocale remembers the language the account reads the app in.
func (s *Service) SetLocale(ctx context.Context, userID, locale string) error {
	if locale != "bn" && locale != "en" {
		return messages.Fail(apperr.KindInvalid, codeInvalidInput)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET locale = $2 WHERE id = $1`, userID, locale); err != nil {
		return fmt.Errorf("set locale: %w", err)
	}
	return nil
}
