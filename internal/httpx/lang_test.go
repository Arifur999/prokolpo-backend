package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLang(t *testing.T) {
	tests := map[string]string{"": LangBangla, "bn": LangBangla, "en": LangEnglish, "en-US,bn;q=0.8": LangEnglish, "bn-BD,en;q=0.5": LangBangla, "fr": LangBangla}
	for header, want := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		if header != "" {
			r.Header.Set("Accept-Language", header)
		}
		if got := Lang(r); got != want {
			t.Errorf("Lang(%q) = %q, want %q", header, got, want)
		}
	}
}
