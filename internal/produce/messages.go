package produce

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (FarmSales).
const (
	codeInvalidInput = "INVALID_INPUT"
	codeNotFound     = "PRODUCE_ENTRY_NOT_FOUND"
	codeNotFarm      = "NOT_A_FARM"
	codeOverpaid     = "MORE_THAN_DUE"
)

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:     {BN: "এন্ট্রিটা পাওয়া যায়নি", EN: "That entry was not found"},
	codeNotFarm:      {BN: "উৎপাদন বিক্রি শুধু খামারের প্রজেক্টে লেখা যায়", EN: "Produce sales are only for farm projects"},
	codeOverpaid:     {BN: "বাকির চেয়ে বেশি নেওয়া যাবে না", EN: "You can't take more than what is owed"},
}

var fieldMessages = map[string]msg.Text{
	"product":  {BN: "কী বিক্রি করলেন, বেছে নিন", EN: "Choose what you sold"},
	"unit":     {BN: "একক বেছে নিন", EN: "Choose the unit"},
	"batch":    {BN: "ব্যাচটা পাওয়া যায়নি", EN: "That batch was not found"},
	"quantity": {BN: "পরিমাণ দিন", EN: "Enter the quantity"},
	"rate":     {BN: "দর দিন", EN: "Enter the rate"},
	"buyer":    {BN: "ক্রেতার নাম দিন", EN: "Enter the buyer's name"},
	"date":     {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"received": {BN: "যত টাকার বিক্রি তার বেশি পাওয়া যায় না", EN: "You can't receive more than the sale is worth"},
	"account":  {BN: "অ্যাকাউন্ট বেছে নিন", EN: "Choose an account"},
	"amount":   {BN: "টাকার পরিমাণ দিন", EN: "Enter the amount"},
	"count":    {BN: "সঠিক সংখ্যা দিন", EN: "Enter a valid number"},
	"ended":    {BN: "শেষের তারিখ শুরুর আগে হতে পারে না", EN: "The end date can't be before the start"},
	"text":     {BN: "লেখাটা অনেক বড় হয়ে গেছে", EN: "This is too long"},
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
