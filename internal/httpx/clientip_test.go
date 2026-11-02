package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustsTheLastHop(t *testing.T) {
	for _, tc := range []struct{ forwarded, want string }{
		{"", "192.0.2.1"},
		{"203.0.113.7", "203.0.113.7"},
		{"6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"203.0.113.7, not-an-ip", "192.0.2.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if tc.forwarded != "" {
			r.Header.Set("X-Forwarded-For", tc.forwarded)
		}
		if got := ClientIP(r); got != tc.want {
			t.Errorf("X-Forwarded-For %q: got %q, want %q", tc.forwarded, got, tc.want)
		}
	}
}
