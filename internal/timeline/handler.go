package timeline

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// pageSize is how many entries one "আরও আগের" press loads.
const pageSize = 50

// Handler serves a project's activity under /api/v1/projects/{id}/activity.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the activity HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; both need a signed-in member of the project.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/projects/{id}/activity", requireUser(http.HandlerFunc(h.feed)))
	mux.Handle("GET /api/v1/projects/{id}/activity/week", requireUser(http.HandlerFunc(h.week)))
}

// filterFrom reads the query. Filters it cannot read are left off rather than refused:
// a stale link should still show the feed.
func (h *Handler) filterFrom(r *http.Request) Filter {
	q := r.URL.Query()
	f := Filter{ActorID: q.Get("actor"), Limit: pageSize}
	if ValidAction(q.Get("action")) {
		f.Action = q.Get("action")
	}
	if days, err := strconv.Atoi(q.Get("days")); err == nil && days > 0 && days <= 366 {
		since := daysBack(h.svc.now(), days)
		f.Since = &since
	}
	if before, err := time.Parse(time.RFC3339Nano, q.Get("before")); err == nil {
		f.Before = &before
	}
	if f.ActorID != "" && !isID(f.ActorID) {
		f.ActorID = ""
	}
	return f
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Feed(r.Context(), auth.UserFrom(r.Context()).UserID, r.PathValue("id"), h.filterFrom(r))
	if err != nil {
		h.responder.Error(w, r, project.Localize(err, httpx.Lang(r)))
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", items)
}

type weekReply struct {
	From   string  `json:"from"`
	Counts []Count `json:"counts"`
}

func (h *Handler) week(w http.ResponseWriter, r *http.Request) {
	counts, from, err := h.svc.Week(r.Context(), auth.UserFrom(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		h.responder.Error(w, r, project.Localize(err, httpx.Lang(r)))
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", weekReply{From: from.Format(time.DateOnly), Counts: counts})
}

// daysBack is the start of the day days-1 days before t in Bangladesh, so "7 days"
// is today and the six before it.
func daysBack(t time.Time, days int) time.Time {
	y, m, d := t.In(dhaka).Date()
	return time.Date(y, m, d-days+1, 0, 0, 0, 0, dhaka)
}
