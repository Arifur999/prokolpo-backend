package settings

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (Settings).
const (
	codeInvalidInput  = "INVALID_INPUT"
	codePhaseLocked   = "PHASE_HAS_ENTRIES"
	codeCategoryInUse = "CATEGORY_HAS_ENTRIES"
	codeNotFound      = "SETTING_NOT_FOUND"
	codeNotMember     = "NOT_A_MEMBER"
)

var messages = msg.Catalog{
	codeInvalidInput:  {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codePhaseLocked:   {BN: "খরচ লেখা হয়ে গেছে এমন ধাপ মুছা যায় না — নাম বদলানো যায়", EN: "A phase with entries can't be deleted — you can rename it"},
	codeCategoryInUse: {BN: "এই খাতে খরচ আছে — মুছা যাবে না, লুকাতে পারেন", EN: "This head has expenses — it can't be deleted, but you can hide it"},
	codeNotFound:      {BN: "পাওয়া যায়নি — হয়তো আগেই মুছে ফেলা হয়েছে", EN: "Not found — it may already have been deleted"},
	codeNotMember:     {BN: "নতুন মালিককে আগে এই প্রজেক্টের মেম্বার হতে হবে", EN: "The new owner has to be a member of this project first"},
}

var fieldMessages = map[string]msg.Text{
	"nameShort":   {BN: "নাম কমপক্ষে ৩ অক্ষরের হতে হবে", EN: "The name needs at least 3 characters"},
	"nameLong":    {BN: "নাম ৮০ অক্ষরের বেশি হতে পারবে না", EN: "The name can be at most 80 characters"},
	"location":    {BN: "ঠিকানা 120 অক্ষরের মধ্যে লিখুন", EN: "Keep the location under 120 characters"},
	"description": {BN: "বিবরণ 500 অক্ষরের মধ্যে লিখুন", EN: "Keep the description under 500 characters"},
	"date":        {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"status":      {BN: "অবস্থা বেছে নিন", EN: "Choose a status"},
	"phaseName":   {BN: "কোনো ধাপের নাম খালি রাখা যাবে না, আর একই নাম দুবার নয়", EN: "Every phase needs a name, and no name twice"},
	"phases":      {BN: "ধাপের তালিকা ঠিক নেই", EN: "The list of phases isn't right"},
	"budget":      {BN: "সঠিক টাকার পরিমাণ দিন", EN: "Enter a valid amount"},
	"category":    {BN: "খাতের নাম দিন (60 অক্ষরের মধ্যে)", EN: "Enter the head's name (up to 60 characters)"},
	"catTaken":    {BN: "এই নামে খাত আগেই আছে", EN: "A head with this name already exists"},
	"company":     {BN: "নাম 80 অক্ষরের মধ্যে লিখুন", EN: "Keep the name under 80 characters"},
	"address":     {BN: "ঠিকানা 160 অক্ষরের মধ্যে লিখুন", EN: "Keep the address under 160 characters"},
	"phone":       {BN: "ফোন নম্বর 30 অক্ষরের মধ্যে লিখুন", EN: "Keep the phone under 30 characters"},
	"prefix":      {BN: "2–5টা ইংরেজি অক্ষর দিন", EN: "Use 2–5 English capital letters"},
	"choice":      {BN: "একটা বেছে নিন", EN: "Pick one"},
	"footer":      {BN: "নিচের লেখা 160 অক্ষরের মধ্যে লিখুন", EN: "Keep the footer under 160 characters"},
	"hours":       {BN: "24 ঘণ্টা, 3 দিন বা 7 দিন বেছে নিন", EN: "Pick 24 hours, 3 days or 7 days"},
}

// Localize puts this package's coded errors, and the project package's (not found,
// owner only), into lang.
func Localize(err error, lang string) error {
	return project.Localize(messages.Localize(err, lang), lang)
}

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

func notFound() error { return messages.Fail(apperr.KindNotFound, codeNotFound) }
