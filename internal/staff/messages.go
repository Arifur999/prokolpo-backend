package staff

import (
	"strconv"
	"strings"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend branches on. The Bangla text is the design's copy (AdminTwoFactor).
const (
	codeNotStaff     = "NOT_STAFF"
	codeAlreadyOn    = "TWO_FACTOR_ON"
	codeNoSetup      = "TWO_FACTOR_OFF"
	codeWrongSetup   = "WRONG_SETUP_CODE"
	codeWrongCode    = "WRONG_CODE"
	codeWrongBackup  = "WRONG_BACKUP_CODE"
	codeLocked       = "TWO_FACTOR_LOCKED"
	codeSessionEnded = "ADMIN_SESSION_ENDED"
)

var messages = msg.Catalog{
	codeNotStaff:     {BN: "এই পাতা শুধু প্রকল্পের স্টাফদের জন্য", EN: "This page is for Prokolpo staff only"},
	codeAlreadyOn:    {BN: "দুই ধাপের যাচাই আগেই চালু আছে", EN: "Two-step verification is already on"},
	codeNoSetup:      {BN: "আগে দুই ধাপের যাচাই চালু করুন", EN: "Set up two-step verification first"},
	codeWrongSetup:   {BN: "কোড মেলেনি — অ্যাপে এখন যে কোড দেখাচ্ছে সেটা দিন। বারবার না মিললে ফোনের সময় \"স্বয়ংক্রিয়\" আছে কিনা দেখুন।", EN: "That code didn't match — enter the one the app shows now. If it keeps failing, check the phone's clock is set automatically."},
	codeWrongCode:    {BN: "কোড মেলেনি — আর {left} বার চেষ্টা করা যাবে। অ্যাপে এখন যে কোড দেখাচ্ছে সেটা দিন।", EN: "That code didn't match — {left} tries left. Enter the code the app shows now."},
	codeWrongBackup:  {BN: "কোড মেলেনি — আর {left} বার চেষ্টা করা যাবে। কোডটা আগে ব্যবহার হয়ে থাকতে পারে।", EN: "That code didn't match — {left} tries left. It may have been used already."},
	codeLocked:       {BN: "পরপর 5 বার ভুল হয়েছে — 15 মিনিট পর আবার চেষ্টা করুন। সুপার অ্যাডমিনদের জানানো হয়েছে।", EN: "Five wrong codes in a row — try again in 15 minutes. The super admins have been told."},
	codeSessionEnded: {BN: "অ্যাডমিন সেশন শেষ — আবার কোড দিন", EN: "The admin session has ended — enter a code again"},
}

// Localize puts a coded error into lang, with the tries left filled in.
func Localize(err error, lang string) error {
	err = messages.Localize(err, lang)
	e, ok := apperr.As(err)
	if !ok {
		return err
	}
	for _, f := range e.Fields {
		if f.Path == "left" {
			out := *e
			out.Message = strings.ReplaceAll(e.Message, "{left}", f.Message)
			return &out
		}
	}
	return err
}

// wrongCode says how many tries are left; the count goes into the message and its details.
func wrongCode(backup bool, left int) error {
	code := codeWrongCode
	if backup {
		code = codeWrongBackup
	}
	return messages.Fail(apperr.KindInvalid, code, apperr.FieldError{Path: "left", Message: strconv.Itoa(left)})
}
