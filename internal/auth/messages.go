package auth

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy.
const (
	codeInvalidInput       = "INVALID_INPUT"
	codePhoneTaken         = "PHONE_TAKEN"
	codeEmailTaken         = "EMAIL_TAKEN"
	codeRegistrationClosed = "REGISTRATION_CLOSED"
	codeInvalidCredentials = "INVALID_CREDENTIALS" //nolint:gosec // an error code name, not a credential
	codeAccountSuspended   = "ACCOUNT_SUSPENDED"
	codeTooManyAttempts    = "TOO_MANY_ATTEMPTS"
	codeSessionInvalid     = "SESSION_INVALID"
	codeUnauthorized       = "UNAUTHORIZED"
	codeAccountNotFound    = "ACCOUNT_NOT_FOUND"
	codeNoEmail            = "NO_EMAIL_ON_ACCOUNT"
	codeResetThrottled     = "RESET_RATE_LIMITED"
	codeResetLinkInvalid   = "RESET_LINK_INVALID"
)

type text struct{ bn, en string }

var messages = map[string]text{
	codeInvalidInput:       {"কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", "Some details need fixing — check the highlighted fields"},
	codePhoneTaken:         {"এই নম্বর দিয়ে আগেই অ্যাকাউন্ট খোলা হয়েছে", "An account already uses this mobile number"},
	codeEmailTaken:         {"এই ইমেইল আগেই ব্যবহার হয়েছে", "This email is already in use"},
	codeRegistrationClosed: {"কিছুদিনের জন্য নতুন রেজিস্ট্রেশন বন্ধ", "New registrations are paused for now"},
	codeInvalidCredentials: {"মোবাইল নম্বর/ইমেইল বা পাসওয়ার্ড মেলেনি", "Mobile number/email or password is incorrect"},
	codeAccountSuspended:   {"আপনার অ্যাকাউন্ট সাময়িকভাবে বন্ধ আছে — সাপোর্টে যোগাযোগ করুন", "Your account is temporarily suspended — please contact support"},
	codeTooManyAttempts:    {"অনেকবার ভুল হয়েছে — 15 মিনিট পরে আবার চেষ্টা করুন", "Too many attempts — try again in 15 minutes"},
	codeSessionInvalid:     {"লগইনের মেয়াদ শেষ — আবার লগইন করুন", "Your session has ended — please log in again"},
	codeUnauthorized:       {"আগে লগইন করুন", "Please log in first"},
	codeAccountNotFound:    {"এই নম্বরে কোনো অ্যাকাউন্ট নেই — নতুন অ্যাকাউন্ট খুলতে পারেন", "No account uses this number — you can create one"},
	codeNoEmail:            {"আপনার অ্যাকাউন্টে কোনো ইমেইল দেওয়া নেই", "Your account has no email address"},
	codeResetThrottled:     {"1 মিনিট পর আবার পাঠাতে পারবেন", "You can send another link in a minute"},
	codeResetLinkInvalid:   {"লিংকের মেয়াদ শেষ বা আগেই ব্যবহার হয়েছে — নতুন লিংক নিন", "This link has expired or was already used — request a new one"},
}

// Field messages for registration and reset validation.
var fieldMessages = map[string]text{
	"companyShort": {"কোম্পানির নাম কমপক্ষে ২ অক্ষরের হতে হবে", "Company name must be at least 2 characters"},
	"firstMissing": {"প্রথম নাম দিন", "Enter your first name"},
	"firstShort":   {"প্রথম নাম কমপক্ষে ২ অক্ষরের হতে হবে", "First name must be at least 2 characters"},
	"lastMissing":  {"শেষ নাম দিন", "Enter your last name"},
	"emailBad":     {"সঠিক ইমেইল দিন", "Enter a valid email"},
	"phoneBad":     {"সঠিক মোবাইল নম্বর দিন (যেমন 01712345678)", "Enter a valid mobile number (e.g. 01712345678)"},
	"passwordWeak": {"পাসওয়ার্ড কমপক্ষে ৮ অক্ষরের এবং অক্ষর ও সংখ্যা মিলিয়ে হতে হবে", "Password must be at least 8 characters with letters and numbers"},
	"termsMissing": {"শর্তাবলীতে সম্মতি দিন", "Please accept the terms"},
	"required":     {"এই ঘরটা পূরণ করুন", "This field is required"},
}

func (t text) in(lang string) string {
	if lang == httpx.LangEnglish {
		return t.en
	}
	return t.bn
}

// fail builds a coded error whose message is filled in per request language by localize.
func fail(kind apperr.Kind, code string, fields ...apperr.FieldError) error {
	return &apperr.Error{Kind: kind, Code: code, Message: messages[code].en, Fields: fields}
}

// localize swaps a coded error's message for the request's language.
func localize(err error, lang string) error {
	e, ok := apperr.As(err)
	if !ok || e.Code == "" {
		return err
	}
	msg, ok := messages[e.Code]
	if !ok {
		return err
	}
	out := *e
	out.Message = msg.in(lang)
	return &out
}
