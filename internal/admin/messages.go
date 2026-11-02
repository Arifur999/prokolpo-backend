package admin

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend branches on. The Bangla text is the design's copy (AdminUserDetail).
const (
	codeInvalidInput  = "INVALID_INPUT"
	codeNotFound      = "ADMIN_NOT_FOUND"
	codeNotAllowed    = "STAFF_ROLE_NOT_ALLOWED"
	codeNoLink        = "NO_RESET_LINK"
	codeSuspendedUser = "USER_SUSPENDED"
	codeStaffAccount  = "STAFF_ACCOUNT"
	codeStaffReset    = "STAFF_RESET"
)

var messages = msg.Catalog{
	codeInvalidInput:  {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:      {BN: "খুঁজে পাওয়া যায়নি", EN: "Not found"},
	codeNotAllowed:    {BN: "আপনার ভূমিকায় এই কাজটা করা যায় না", EN: "Your staff role can't do this"},
	codeNoLink:        {BN: "ইমেইল দেওয়া নেই বা অ্যাকাউন্ট বন্ধ — রিসেট লিংক পাঠানো যাবে না", EN: "No email, or the account is suspended — a reset link can't be sent"},
	codeSuspendedUser: {BN: "অ্যাকাউন্ট বন্ধ আছে", EN: "The account is suspended"},
	codeStaffReset:    {BN: "স্টাফের পাসওয়ার্ড কোড দিয়ে বদলানো যায় না — রিসেট লিংক পাঠান", EN: "A staff password can't be reset with a code — send a reset link"},
	codeStaffAccount:  {BN: "স্টাফের অ্যাকাউন্ট এখান থেকে বন্ধ করা যায় না", EN: "Staff accounts can't be suspended here"},
}

var fieldMessages = map[string]msg.Text{
	"reason": {BN: "কারণ লিখুন — কমপক্ষে 10 অক্ষর", EN: "Give a reason — at least 10 characters"},
	"text":   {BN: "লেখাটা ঠিক নেই বা অনেক বড়", EN: "This is empty or too long"},
	"phone":  {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712-345678)", EN: "Enter a valid mobile number (e.g. 01712-345678)"},
	"same":   {BN: "এটাই এখনকার নম্বর", EN: "That is the current number"},
	"taken":  {BN: "এই নম্বরে আরেকটা অ্যাকাউন্ট আছে — আগে সেটা দেখুন", EN: "Another account has this number — look at it first"},
}

// Localize puts a coded error into lang.
func Localize(err error, lang string) error { return messages.Localize(err, lang) }

func notFound() error { return messages.Fail(apperr.KindNotFound, codeNotFound) }

// fields collects field errors in the request's language.
type fields struct {
	lang string
	list []apperr.FieldError
}

func (f *fields) add(path, key string) {
	f.list = append(f.list, apperr.FieldError{Path: path, Message: fieldMessages[key].In(f.lang)})
}

func (f *fields) err() error {
	if len(f.list) == 0 {
		return nil
	}
	return messages.Fail(apperr.KindInvalid, codeInvalidInput, f.list...)
}
