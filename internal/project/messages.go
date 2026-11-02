package project

import "github.com/Arifur999/prokolpo-backend/internal/msg"

// Error codes the frontend can branch on. The Bangla text is the design's copy.
const (
	codeInvalidInput  = "INVALID_INPUT"
	codeNotFound      = "PROJECT_NOT_FOUND"
	codeOwnerOnly     = "OWNER_ONLY"
	codeRestoreClosed = "RESTORE_WINDOW_CLOSED"
	codeReadOnly      = "READ_ONLY_ROLE"
)

var messages = msg.Catalog{
	codeInvalidInput:  {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:      {BN: "প্রজেক্টটা পাওয়া যায়নি", EN: "Project not found"},
	codeOwnerOnly:     {BN: "এটা শুধু প্রজেক্টের মালিক করতে পারেন", EN: "Only the project's owner can do this"},
	codeReadOnly:      {BN: "আপনি এই প্রজেক্টে শুধু দেখতে পারেন — এন্ট্রি করতে মালিককে বলুন", EN: "You can only view this project — ask the owner for entry access"},
	codeRestoreClosed: {BN: "30 দিন পার হয়ে গেছে — এই প্রজেক্ট আর ফেরানো যাবে না", EN: "More than 30 days have passed — this project can no longer be restored"},
}

var fieldMessages = map[string]msg.Text{
	"nameMissing":  {BN: "প্রজেক্টের নাম দিন", EN: "Enter a project name"},
	"nameShort":    {BN: "নাম কমপক্ষে ৩ অক্ষরের হতে হবে", EN: "The name needs at least 3 characters"},
	"nameLong":     {BN: "নাম ৮০ অক্ষরের বেশি হতে পারবে না", EN: "The name can be at most 80 characters"},
	"kind":         {BN: "কিসের প্রজেক্ট, বেছে নিন", EN: "Choose what the project is for"},
	"ownership":    {BN: "মালিকানা বেছে নিন", EN: "Choose the ownership"},
	"location":     {BN: "ঠিকানা 120 অক্ষরের মধ্যে লিখুন", EN: "Keep the location under 120 characters"},
	"description":  {BN: "বিবরণ 500 অক্ষরের মধ্যে লিখুন", EN: "Keep the description under 500 characters"},
	"startDate":    {BN: "সঠিক তারিখ দিন", EN: "Enter a valid date"},
	"budget":       {BN: "সঠিক টাকার পরিমাণ দিন", EN: "Enter a valid amount"},
	"farmType":     {BN: "খামারের ধরন বেছে নিন", EN: "Choose the type of farm"},
	"count":        {BN: "সংখ্যাটা ঠিক নেই", EN: "This number is out of range"},
	"landArea":     {BN: "জমির সঠিক পরিমাণ দিন", EN: "Enter a valid land area"},
	"landUnit":     {BN: "জমির একক বেছে নিন", EN: "Choose a land unit"},
	"businessType": {BN: "ব্যবসার ধরন 60 অক্ষরের মধ্যে লিখুন", EN: "Keep the business type under 60 characters"},
	"list":         {BN: "তালিকায় ভুল আছে — খালি, একই নাম দুবার বা 60 অক্ষরের বেশি", EN: "This list has a blank, repeated or over-long (60+) entry"},
}

// cashAccountName names the account every new project starts with.
var cashAccountName = msg.Text{BN: "নগদ ক্যাশ", EN: "Cash in hand"}
