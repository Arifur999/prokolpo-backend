// Package msg holds user-facing messages in Bangla and English and turns coded
// errors into the request's language.
package msg

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Text is one message in both languages.
type Text struct {
	BN string
	EN string
}

// In returns the text for lang (httpx.LangBangla or httpx.LangEnglish).
func (t Text) In(lang string) string {
	if lang == httpx.LangEnglish {
		return t.EN
	}
	return t.BN
}

// Catalog maps error codes to their messages.
type Catalog map[string]Text

// Fail builds a coded error. Its message is English until Localize sets the
// request's language, so logs read the same whoever triggered it.
func (c Catalog) Fail(kind apperr.Kind, code string, fields ...apperr.FieldError) error {
	return &apperr.Error{Kind: kind, Code: code, Message: c[code].EN, Fields: fields}
}

// Localize swaps a coded error's message for lang. Errors without a code, or
// with a code this catalog does not know, pass through unchanged.
func (c Catalog) Localize(err error, lang string) error {
	e, ok := apperr.As(err)
	if !ok || e.Code == "" {
		return err
	}
	text, ok := c[e.Code]
	if !ok {
		return err
	}
	out := *e
	out.Message = text.In(lang)
	return &out
}
