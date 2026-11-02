package httpx

import (
	"net/http"
	"strings"
)

// Languages the API answers in. Bangla is the default.
const (
	LangBangla  = "bn"
	LangEnglish = "en"
)

// Lang picks the response language from Accept-Language, which the frontend
// sets from the user's choice. Anything that does not start with "en" is Bangla.
func Lang(r *http.Request) string {
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(first)), "en") {
		return LangEnglish
	}
	return LangBangla
}
