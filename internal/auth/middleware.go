package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

type claimsKey struct{}

// UserFrom returns the signed-in user's claims. Only valid behind RequireUser.
func UserFrom(ctx context.Context) token.Claims {
	claims, _ := ctx.Value(claimsKey{}).(token.Claims)
	return claims
}

// AccountGate says whether a signed token's account may still act: suspension and a password
// change take effect at once instead of when the token expires.
type AccountGate interface {
	Active(ctx context.Context, claims token.Claims) error
}

// RequireUser rejects requests without a valid Bearer access token, or whose account the gate
// refuses, and puts its claims in the context for the handler.
func RequireUser(issuer *token.Issuer, gate AccountGate, responder *httpx.Responder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				responder.Error(w, r, messages.Localize(messages.Fail(apperr.KindUnauthorized, codeUnauthorized), httpx.Lang(r)))
				return
			}
			claims, err := issuer.Parse(raw)
			if err != nil {
				responder.Error(w, r, messages.Localize(messages.Fail(apperr.KindUnauthorized, codeUnauthorized), httpx.Lang(r)))
				return
			}
			if err = gate.Active(r.Context(), claims); err != nil {
				responder.Error(w, r, messages.Localize(err, httpx.Lang(r)))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
		})
	}
}

// MaybeUserFrom returns the claims OptionalUser found, if any.
func MaybeUserFrom(ctx context.Context) (token.Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(token.Claims)
	return claims, ok
}

// OptionalUser attaches the caller's claims when they send a valid Bearer
// token and lets the request through either way, for routes that work for
// visitors and signed-in users alike.
func OptionalUser(issuer *token.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				if claims, err := issuer.Parse(raw); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
