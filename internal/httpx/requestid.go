package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader carries the request's id in both directions, so a log line
// can be matched to what the frontend saw.
const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID returns the id the middleware gave this request, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithRequestID tags each request with an id: the caller's, if it sent a
// sane one, otherwise a fresh random one.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !isSaneID(id) {
			id = newID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// isSaneID accepts short ids of URL-safe characters, so a client cannot push
// arbitrary text into our logs.
func isSaneID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !isIDChar(c) {
			return false
		}
	}
	return true
}

func isIDChar(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	default:
		return c == '-' || c == '_'
	}
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b[:])
}
