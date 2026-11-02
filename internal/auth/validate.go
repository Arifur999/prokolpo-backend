package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
)

// Length caps from the registration form (design board Register).
const (
	maxNameLen    = 30
	maxCompanyLen = 80
)

// RegisterInput is the registration form after validation and normalisation.
type RegisterInput struct {
	FirstName   string
	LastName    string
	CompanyName *string
	Email       *string
	Phone       string
	Password    string
	Locale      string
}

type registerRequest struct {
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	CompanyName string `json:"companyName"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Password    string `json:"password"`
	AcceptTerms bool   `json:"acceptTerms"`
}

// validate applies the form's rules (design board RegisterStates) and returns
// every problem at once, in the request's language.
func (req registerRequest) validate(lang string) (RegisterInput, error) {
	var fields []apperr.FieldError
	add := func(path, key string) {
		fields = append(fields, apperr.FieldError{Path: path, Message: fieldMessages[key].In(lang)})
	}

	in := RegisterInput{
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		Password:  req.Password,
		Locale:    lang,
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
	switch n := utf8.RuneCountInString(in.FirstName); {
	case n == 0:
		add("firstName", "firstMissing")
	case n < 2:
		add("firstName", "firstShort")
	case n > maxNameLen:
		add("firstName", "firstLong")
	}
	switch n := utf8.RuneCountInString(in.LastName); {
	case n == 0:
		add("lastName", "lastMissing")
	case n > maxNameLen:
		add("lastName", "lastLong")
	}
	if email := strings.TrimSpace(req.Email); email != "" {
		if !isEmail(email) {
			add("email", "emailBad")
		}
		lower := strings.ToLower(email)
		in.Email = &lower
	}
	if p, err := phone.Normalize(req.Phone); err != nil {
		add("phone", "phoneBad")
	} else {
		in.Phone = p
	}
	if password.Validate(req.Password) != nil {
		add("password", "passwordWeak")
	}
	if !req.AcceptTerms {
		add("acceptTerms", "termsMissing")
	}
	if len(fields) > 0 {
		return RegisterInput{}, messages.Fail(apperr.KindInvalid, codeInvalidInput, fields...)
	}
	return in, nil
}

// isEmail accepts a bare address (no display name) with a dotted domain.
func isEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	_, domain, _ := strings.Cut(s, "@")
	return strings.Contains(domain, ".")
}

// identifier is what the login box holds: a phone number or an email.
type identifier struct {
	phone string
	email string
}

func parseIdentifier(raw string) (identifier, bool) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "@") {
		if !isEmail(raw) {
			return identifier{}, false
		}
		return identifier{email: strings.ToLower(raw)}, true
	}
	p, err := phone.Normalize(raw)
	if err != nil {
		return identifier{}, false
	}
	return identifier{phone: p}, true
}

// key is what rate limits count against.
func (id identifier) key() string {
	if id.phone != "" {
		return "phone:" + id.phone
	}
	return "email:" + id.email
}
