package status

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/msg"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

const codeMaintenance = "MAINTENANCE"

var messages = msg.Catalog{
	codeMaintenance: {BN: "কিছুক্ষণের জন্য কাজ চলছে — একটু পরে আবার চেষ্টা করুন", EN: "We're doing some maintenance — please try again a little later"},
}

// Paths that keep working during maintenance: probes, this status, and what
// staff need to sign in and switch maintenance off again.
var openDuringMaintenance = []string{
	"/api/v1/health",
	"/api/v1/status",
	"/api/v1/auth/login",
	"/api/v1/auth/refresh",
	"/api/v1/auth/me",
}

// Source is what the handler and middleware need: the current status.
type Source interface {
	Current(ctx context.Context) (Status, error)
}

// Handler serves GET /api/v1/status.
func Handler(svc Source, responder *httpx.Responder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, err := svc.Current(r.Context())
		if err != nil {
			responder.Error(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		responder.JSON(w, r, http.StatusOK, "", current)
	}
}

// BlockDuringMaintenance answers 503 with code MAINTENANCE while maintenance
// is on, except on the open paths and for staff. If the flag cannot be read,
// requests go through: a settings hiccup must not take the app down.
func BlockDuringMaintenance(svc Source, issuer *token.Issuer, responder *httpx.Responder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			current, err := svc.Current(r.Context())
			if err != nil || !current.Maintenance.Enabled || slices.Contains(openDuringMaintenance, r.URL.Path) || isStaff(issuer, r) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Retry-After", "300")
			responder.Error(w, r, messages.Localize(messages.Fail(apperr.KindUnavailable, codeMaintenance), httpx.Lang(r)))
		})
	}
}

func isStaff(issuer *token.Issuer, r *http.Request) bool {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	claims, err := issuer.Parse(raw)
	return err == nil && claims.Role == token.RoleStaff
}
