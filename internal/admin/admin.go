// Package admin serves the admin panel's pages: the overview, users, projects, the support
// inbox, announcements and the admin log. Every route sits behind a live panel session.
package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/staff"
)

// Confirmer checks a staff member's app code before a sensitive action.
type Confirmer interface {
	Confirm(ctx context.Context, userID, code string, meta staff.Meta) error
}

// ResetLinks emails an account its password-reset link.
type ResetLinks interface {
	SendResetLink(ctx context.Context, rawPhone, lang string) error
}

// Service reads and changes things across every account.
type Service struct {
	pool    *pgxpool.Pool
	confirm Confirmer
	links   ResetLinks
	now     func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool, confirm Confirmer, links ResetLinks) *Service {
	return &Service{pool: pool, confirm: confirm, links: links, now: time.Now}
}

func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// Handler serves the panel under /api/v1/admin.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the panel's HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; guard is auth.RequireUser followed by the panel session check.
func (h *Handler) Register(mux *http.ServeMux, guard func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/admin"+path, guard(fn))
	}
	route("GET", "/overview", h.overview)
	route("GET", "/badges", h.badges)
	h.registerUsers(route)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		lang := httpx.Lang(r)
		h.responder.Error(w, r, staff.Localize(Localize(err, lang), lang))
		return
	}
	h.responder.JSON(w, r, status, "", data)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.Overview(r.Context(), auth.UserFrom(r.Context()).UserID)
	h.reply(w, r, http.StatusOK, o, err)
}

// Badges are the counts the panel's menu shows; asking for them also keeps the session alive.
type Badges struct {
	OpenSupport int `json:"openSupport"`
}

func (h *Handler) badges(w http.ResponseWriter, r *http.Request) {
	var b Badges
	err := h.svc.pool.QueryRow(r.Context(), `SELECT count(*) FROM support_requests WHERE status = 'open'`).Scan(&b.OpenSupport)
	h.reply(w, r, http.StatusOK, b, err)
}
