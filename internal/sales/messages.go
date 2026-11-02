package sales

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy (Units, SaleNew).
const (
	codeInvalidInput = "INVALID_INPUT"
	codeNotFound     = "SALE_ENTRY_NOT_FOUND"
	codeUnitTaken    = "UNIT_NAME_TAKEN"
	codeUnitSold     = "UNIT_SOLD"
	codeUnitHasSale  = "UNIT_HAS_SALE"
	codeOverpaid     = "MORE_THAN_DUE"
)

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:     {BN: "এন্ট্রিটা পাওয়া যায়নি", EN: "That entry was not found"},
	codeUnitTaken:    {BN: "এই নামে আগে থেকেই একটা ইউনিট আছে", EN: "A unit with this name already exists"},
	codeUnitSold:     {BN: "এই ইউনিট আগেই বিক্রি হয়েছে", EN: "This unit is already sold"},
	codeUnitHasSale:  {BN: "বিক্রি হওয়া ইউনিট মোছা যায় না", EN: "A unit that has been sold can't be deleted"},
	codeOverpaid:     {BN: "বাকির চেয়ে বেশি টাকা নেওয়া যাবে না", EN: "You can't take more than what is owed"},
}

var fieldMessages = map[string]msg.Text{
	"unitName": {BN: "ইউনিটের নাম দিন", EN: "Enter the unit's name"},
	"unitType": {BN: "ধরন বেছে নিন", EN: "Choose the type"},
	"floor":    {BN: "তলা বেছে নিন", EN: "Choose the floor"},
	"size":     {BN: "সঠিক মাপ দিন", EN: "Enter a valid size"},
	"price":    {BN: "দাম দিন", EN: "Enter the price"},
	"name":     {BN: "ক্রেতার নাম দিন", EN: "Enter the buyer's name"},
	"phone":    {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712-345678)", EN: "Enter a valid mobile number (e.g. 01712-345678)"},
	"text":     {BN: "লেখাটা অনেক বড় হয়ে গেছে", EN: "This is too long"},
	"customer": {BN: "একজন কাস্টমার বাছুন", EN: "Choose a customer"},
	"unit":     {BN: "ইউনিট বেছে নিন", EN: "Choose a unit"},
	"date":     {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"down":     {BN: "ডাউনপেমেন্ট দামের সমান বা বেশি — পুরো টাকা দিলে \"একবারে পুরো দাম\" বাছুন", EN: "The down payment is the whole price or more — choose \"Full price at once\" instead"},
	"tooSmall": {BN: "কিস্তির টাকা খুব কম — কিস্তির সংখ্যা কমান", EN: "Installments come out too small — use fewer of them"},
	"count":    {BN: "কিস্তির সংখ্যা 1 থেকে 240", EN: "Use 1 to 240 installments"},
	"interval": {BN: "কত মাস পরপর, বেছে নিন", EN: "Choose how many months apart"},
	"firstDue": {BN: "বিক্রির তারিখের পরে হতে হবে", EN: "It must be after the sale date"},
	"account":  {BN: "অ্যাকাউন্ট বেছে নিন", EN: "Choose an account"},
	"amount":   {BN: "টাকার পরিমাণ দিন", EN: "Enter the amount"},
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
