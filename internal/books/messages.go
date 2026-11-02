package books

import (
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend can branch on. The Bangla text is the design's copy.
const (
	codeInvalidInput = "INVALID_INPUT"
	codeNotFound     = "ENTRY_NOT_FOUND"
	codeNameTaken    = "NAME_TAKEN"
	codeSameAccount  = "SAME_ACCOUNT"
)

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:     {BN: "এন্ট্রিটা পাওয়া যায়নি — হয়তো মুছে ফেলা হয়েছে", EN: "That entry was not found — it may have been deleted"},
	codeNameTaken:    {BN: "এই নামে আগেই একজন আছে", EN: "Someone with this name is already here"},
	codeNotImage:     {BN: "শুধু JPG বা PNG ছবি দেওয়া যাবে", EN: "Only JPG or PNG photos"},
	codeSameAccount:  {BN: "একই অ্যাকাউন্টে সরানো যাবে না", EN: "You can't move money to the same account"},
}

// Field messages follow the boards (ExpenseNew, SupplierAdd, ContractorAdd, Accounts).
var fieldMessages = map[string]msg.Text{
	"amount":          {BN: "টাকার পরিমাণ দিন", EN: "Enter the amount"},
	"category":        {BN: "খাত বেছে নিন", EN: "Choose a category"},
	"date":            {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"party":           {BN: "সাপ্লায়ার বা কন্ট্রাক্টর বেছে নিন", EN: "Choose the supplier or contractor"},
	"phase":           {BN: "ধাপটা পাওয়া যায়নি", EN: "That phase was not found"},
	"account":         {BN: "অ্যাকাউন্ট বেছে নিন", EN: "Choose an account"},
	"quantity":        {BN: "সঠিক পরিমাণ দিন", EN: "Enter a valid quantity"},
	"text":            {BN: "লেখাটা অনেক বড় হয়ে গেছে", EN: "This is too long"},
	"supplierName":    {BN: "সাপ্লায়ারের নাম দিন", EN: "Enter the supplier's name"},
	"supplierShort":   {BN: "নাম কমপক্ষে ২ অক্ষরের হতে হবে", EN: "The name needs at least 2 characters"},
	"supplierTaken":   {BN: "এই নামে আগেই একজন সাপ্লায়ার আছে", EN: "A supplier with this name already exists"},
	"phone":           {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712345678)", EN: "Enter a valid mobile number (e.g. 01712345678)"},
	"contractorName":  {BN: "নাম দিন", EN: "Enter a name"},
	"contractorTaken": {BN: "এই নামে আগেই একজন আছে", EN: "Someone with this name already exists"},
	"contract":        {BN: "চুক্তির টাকা দিন", EN: "Enter the contract amount"},
	"wage":            {BN: "দৈনিক মজুরি দিন", EN: "Enter the daily wage"},
	"team":            {BN: "কয়জন, 1 থেকে 50 এর মধ্যে লিখুন", EN: "Enter a team size from 1 to 50"},
	"work":            {BN: "কী কাজ করেন, বেছে নিন", EN: "Choose the kind of work"},
	"payType":         {BN: "কীভাবে টাকা দেবেন, বেছে নিন", EN: "Choose how they are paid"},
	"purpose":         {BN: "কিসের জন্য, বেছে নিন", EN: "Choose what it is for"},
	"title":           {BN: "কী কাজ, লিখুন", EN: "Describe the work"},
	"accountName":     {BN: "অ্যাকাউন্টের নাম দিন", EN: "Enter the account name"},
	"accountTaken":    {BN: "এই নামে আগেই একটা অ্যাকাউন্ট আছে", EN: "An account with this name already exists"},
	"accountKind":     {BN: "অ্যাকাউন্টের ধরন বেছে নিন", EN: "Choose the account type"},
	"toAccount":       {BN: "একই অ্যাকাউন্টে সরানো যাবে না", EN: "You can't move money to the same account"},
	"headcount":       {BN: "0 থেকে 99 জন লিখুন", EN: "Enter 0 to 99 people"},
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

// nameTaken is the invalid-input error for a duplicate name, on the name field.
func nameTaken(lang, key string) error {
	f := &fields{lang: lang}
	f.add("name", key)
	return f.err()
}
