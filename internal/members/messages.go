package members

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (Members).
const (
	codeInvalidInput  = "INVALID_INPUT"
	codeNotFound      = "MEMBER_NOT_FOUND"
	codeAlreadyMember = "ALREADY_MEMBER"
	codeInviteExists  = "INVITE_EXISTS"
	codeOwnerFixed    = "OWNER_FIXED"
)

var messages = msg.Catalog{
	codeInvalidInput:  {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:      {BN: "মেম্বার পাওয়া যায়নি — হয়তো আগেই সরানো হয়েছে", EN: "Member not found — they may already have been removed"},
	codeAlreadyMember: {BN: "এই নম্বর দিয়ে একজন আগে থেকেই এই প্রজেক্টে আছেন", EN: "Someone with this number is already on this project"},
	codeInviteExists:  {BN: "এই নম্বরে আগেই আমন্ত্রণ দেওয়া আছে", EN: "This number has already been invited"},
	codeOwnerFixed:    {BN: "মালিকের রোল বদলানো বা তাঁকে সরানো যায় না", EN: "The owner's role can't be changed and the owner can't be removed"},
}

var fieldMessages = map[string]msg.Text{
	"phone": {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712345678)", EN: "Enter a valid mobile number (e.g. 01712345678)"},
	"role":  {BN: "রোল বেছে নিন", EN: "Choose a role"},
}

// Localize puts a coded error into lang.
func Localize(err error, lang string) error { return messages.Localize(err, lang) }

func notFound() error { return messages.Fail(apperr.KindNotFound, codeNotFound) }

func invalid(lang, path, key string) error {
	return messages.Fail(apperr.KindInvalid, codeInvalidInput, apperr.FieldError{Path: path, Message: fieldMessages[key].In(lang)})
}
