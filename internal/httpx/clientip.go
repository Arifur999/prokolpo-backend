package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP is the browser's address: the frontend server forwards it in X-Forwarded-For.
// It is shown to people and logged, never trusted for access decisions.
func ClientIP(r *http.Request) string {
	first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ",")
	if addr, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
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
