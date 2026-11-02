package shop

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (BizSales).
const (
	codeInvalidInput = "INVALID_INPUT"
	codeNotFound     = "SHOP_ENTRY_NOT_FOUND"
	codeOverpaid     = "MORE_THAN_DUE"
	codeEmptyDay     = "EMPTY_DAY"
)

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:     {BN: "এন্ট্রিটা পাওয়া যায়নি", EN: "That entry was not found"},
	codeOverpaid:     {BN: "খাতায় বাকির চেয়ে বেশি নেওয়া যাবে না", EN: "You can't take more than the credit book shows"},
	codeEmptyDay:     {BN: "অন্তত একটা টাকার ঘর পূরণ করুন", EN: "Fill in at least one amount"},
}

var fieldMessages = map[string]msg.Text{
	"date":     {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"amount":   {BN: "টাকার পরিমাণ দিন", EN: "Enter the amount"},
	"account":  {BN: "অ্যাকাউন্ট বেছে নিন", EN: "Choose an account"},
	"name":     {BN: "বাকিতে যিনি নিলেন তাঁর নাম দিন", EN: "Enter the name of who took it on credit"},
	"unknown":  {BN: "এই নামে খাতায় কেউ নেই", EN: "Nobody by this name is in the credit book"},
	"overpaid": {BN: "খাতায় বাকির চেয়ে বেশি শোধ লেখা হয়েছে", EN: "This is more than they owe"},
	"text":     {BN: "লেখাটা অনেক বড় হয়ে গেছে", EN: "This is too long"},
	"tooMany":  {BN: "একদিনে 50 জনের বেশি লেখা যায় না", EN: "Up to 50 people a day"},
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

func invalid(lang, path, key string) error {
	f := &fields{lang: lang}
	f.add(path, key)
	return f.err()
}
