package admin

import (
	"github.com/Arifur999/prokolpo-backend/internal/msg"
)

// Error codes the frontend branches on.
const (
	codeInvalidInput = "INVALID_INPUT"
	codeNotFound     = "ADMIN_NOT_FOUND"
)

var messages = msg.Catalog{
	codeInvalidInput: {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codeNotFound:     {BN: "খুঁজে পাওয়া যায়নি", EN: "Not found"},
}

// Localize puts a coded error into lang.
func Localize(err error, lang string) error { return messages.Localize(err, lang) }
