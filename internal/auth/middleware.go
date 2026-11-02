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

// RequireUser rejects requests without a valid Bearer access token and puts
// its claims in the context for the handler.
func RequireUser(issuer *token.Issuer, responder *httpx.Responder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				responder.Error(w, r, localize(fail(apperr.KindUnauthorized, codeUnauthorized), httpx.Lang(r)))
				return
			}
			claims, err := issuer.Parse(raw)
			if err != nil {
				responder.Error(w, r, localize(fail(apperr.KindUnauthorized, codeUnauthorized), httpx.Lang(r)))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
		})
	}
}
