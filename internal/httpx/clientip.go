package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP is the browser's address: the frontend server forwards it in X-Forwarded-For.
// The last entry is the one the hop in front of the API wrote; earlier ones are whatever the
// caller claimed. It is shown to people and logged, never trusted for access decisions.
func ClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	last := forwarded[strings.LastIndex(forwarded, ",")+1:]
	if addr, err := netip.ParseAddr(strings.TrimSpace(last)); err == nil {
		return addr.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String()
	}
	return ""
}
