// Package support takes help requests from the public support page and from
// inside the app; the admin panel's inbox works through them.
package support

import (
	"net/mail"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
)

var (
	topics  = []string{"login", "project", "members", "other"}
	sources = []string{"public", "forgot", "suspended", "app"}
)

const (
	minMessageLen = 10
	maxMessageLen = 2000
	maxNameLen    = 60
)

const codeInvalidInput = "INVALID_INPUT"

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeTooMany:      {BN: "এই নম্বর থেকে অনেক অনুরোধ এসেছে — কিছুক্ষণ পরে আবার চেষ্টা করুন বা WhatsApp করুন", EN: "Too many requests from this number — try again later or message us on WhatsApp"},
}

// Field messages follow the Support board's copy.
var fieldMessages = map[string]msg.Text{
	"name":    {BN: "আপনার নাম লিখুন", EN: "Enter your name"},
	"phone":   {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712-345678)", EN: "Enter a valid mobile number (e.g. 01712-345678)"},
	"email":   {BN: "সঠিক ইমেইল দিন", EN: "Enter a valid email"},
	"topic":   {BN: "কী নিয়ে, বেছে নিন", EN: "Choose what it is about"},
	"message": {BN: "কী সমস্যা হচ্ছে, একটু লিখুন — কমপক্ষে 10 অক্ষর", EN: "Tell us a little about the problem — at least 10 characters"},
}

// Request is a validated support request ready to store.
type Request struct {
	UserID  *string
	Name    string
	Phone   string
	Email   *string
	Topic   string
	Source  string
	Message string
}

type requestBody struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Topic   string `json:"topic"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// validate checks every field at once and reports problems in lang. An unknown
// source is not the sender's mistake, so it falls back to "public".
func (b requestBody) validate(lang string) (Request, error) {
	var fields []apperr.FieldError
	add := func(path string) {
		fields = append(fields, apperr.FieldError{Path: path, Message: fieldMessages[path].In(lang)})
	}

	req := Request{
		Name:    strings.TrimSpace(b.Name),
		Topic:   b.Topic,
		Source:  b.Source,
		Message: strings.TrimSpace(b.Message),
	}
	if n := utf8.RuneCountInString(req.Name); n < 2 || n > maxNameLen {
		add("name")
	}
	if p, err := phone.Normalize(b.Phone); err != nil {
		add("phone")
	} else {
		req.Phone = p
	}
	if email := strings.TrimSpace(b.Email); email != "" {
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			add("email")
		}
		lower := strings.ToLower(email)
		req.Email = &lower
	}
	if !slices.Contains(topics, req.Topic) {
		add("topic")
	}
	if !slices.Contains(sources, req.Source) {
		req.Source = "public"
	}
	if n := utf8.RuneCountInString(req.Message); n < minMessageLen || n > maxMessageLen {
		add("message")
	}
	if len(fields) > 0 {
		return Request{}, messages.Fail(apperr.KindInvalid, codeInvalidInput, fields...)
	}
	return req, nil
}
