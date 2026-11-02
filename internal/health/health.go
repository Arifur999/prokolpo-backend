// Package health reports whether the API and its database can serve traffic.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// pingTimeout keeps a stuck database from hanging the probe; orchestrators
// treat a slow answer as a failed one anyway.
const pingTimeout = 2 * time.Second

// Pinger is what the check needs from the database pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler serves the health probe.
type Handler struct {
	db        Pinger
	log       *slog.Logger
	responder *httpx.Responder
}

// NewHandler returns a health probe backed by db.
func NewHandler(db Pinger, log *slog.Logger, responder *httpx.Responder) *Handler {
	return &Handler{db: db, log: log, responder: responder}
}

type status struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// Check answers 200 when the database responds and 503 when it does not.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(ctx, "health check: database unreachable", "err", err)
		w.Header().Set("Cache-Control", "no-store")
		h.responder.JSON(w, r, http.StatusServiceUnavailable, "Database is unreachable.", status{Status: "degraded", Database: "down"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.responder.JSON(w, r, http.StatusOK, "OK", status{Status: "ok", Database: "up"})
}
