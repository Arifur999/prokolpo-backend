// Package server assembles the API's routes and middleware into one handler.
package server

import (
	"log/slog"
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/health"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/status"
	"github.com/Arifur999/prokolpo-backend/internal/support"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// Deps are the handlers' dependencies, built once in app.Run.
type Deps struct {
	Log     *slog.Logger
	DB      health.Pinger
	Auth    *auth.Service
	Support *support.Service
	Status  status.Source
	Issuer  *token.Issuer
}

// New returns the API's root handler. Every route lives under /api/v1.
func New(deps Deps) http.Handler {
	responder := httpx.NewResponder(deps.Log)
	healthHandler := health.NewHandler(deps.DB, deps.Log, responder)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", healthHandler.Check)
	mux.HandleFunc("GET /api/v1/status", status.Handler(deps.Status, responder))
	auth.NewHandler(deps.Auth, responder).Register(mux, auth.RequireUser(deps.Issuer, responder))
	support.NewHandler(deps.Support, responder).Register(mux, auth.OptionalUser(deps.Issuer))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responder.Error(w, r, apperr.NotFound("This endpoint does not exist."))
	})

	// The request id goes on first, so the log line and any panic report carry it.
	routes := status.BlockDuringMaintenance(deps.Status, deps.Issuer, responder)(mux)
	return httpx.WithRequestID(httpx.RecoverPanics(deps.Log, httpx.LogRequests(deps.Log, routes)))
}
