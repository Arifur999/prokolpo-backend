package partners

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (PartnerList).
const (
	codeInvalidInput   = "INVALID_INPUT"
	codeNotFound       = "PARTNER_NOT_FOUND"
	codePartnerExists  = "PARTNER_EXISTS"
	codeNotPartnership = "NOT_PARTNERSHIP"
	codeNoPartners     = "NO_PARTNERS"
)

var messages = msg.Catalog{
	codeInvalidInput:   {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:       {BN: "পার্টনার পাওয়া যায়নি", EN: "Partner not found"},
	codePartnerExists:  {BN: "এই নম্বরের মানুষ আগে থেকেই পার্টনার", EN: "This number already belongs to a partner"},
	codeNotPartnership: {BN: "শুধু যৌথ প্রজেক্টে পার্টনার থাকে", EN: "Only joint projects have partners"},
	codeNoPartners:     {BN: "আগে পার্টনার যোগ করুন", EN: "Add partners first"},
}

var fieldMessages = map[string]msg.Text{
	"phone":     {BN: "সঠিক মোবাইল নম্বর দিন — 11 অঙ্ক, 01 দিয়ে শুরু", EN: "Enter a valid mobile number — 11 digits starting with 01"},
	"name":      {BN: "পার্টনারের নাম লিখুন", EN: "Enter the partner's name"},
	"share":     {BN: "শেয়ার 0 থেকে 100 এর মধ্যে দিন — যেমন 10", EN: "Enter a share between 0 and 100 — e.g. 10"},
	"shares":    {BN: "সবার শেয়ার মিলে 100% হতে হবে", EN: "Everyone's shares must add up to 100%"},
	"committed": {BN: "সঠিক টাকার পরিমাণ দিন", EN: "Enter a valid amount"},
	"date":      {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"amount":    {BN: "মোট টাকার পরিমাণ দিন", EN: "Enter the total amount"},
	"reason":    {BN: "কেন লাগবে লিখুন — পার্টনাররা এটা দেখবেন", EN: "Say what it is for — the partners will see this"},
	"account":   {BN: "অ্যাকাউন্ট বেছে নিন", EN: "Choose an account"},
	"call":      {BN: "এই কলে এই পার্টনারের কোনো ভাগ নেই", EN: "This partner has no part in that call"},
	"note":      {BN: "লেখাটা অনেক বড় হয়ে গেছে", EN: "This is too long"},
}

// Localize puts a coded error into lang.
func Localize(err error, lang string) error { return messages.Localize(err, lang) }

func notFound() error { return messages.Fail(apperr.KindNotFound, codeNotFound) }

func invalid(lang string, pairs ...string) error {
	fields := make([]apperr.FieldError, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		fields = append(fields, apperr.FieldError{Path: pairs[i], Message: fieldMessages[pairs[i+1]].In(lang)})
	}
	return messages.Fail(apperr.KindInvalid, codeInvalidInput, fields...)
}
