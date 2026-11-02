package shop

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a shop's day book under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the day book HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; members read, owners and managers write.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/shop", h.book)
	route("PUT", "/shop/days", h.saveDay)
	route("POST", "/shop/khata/{pid}/payments", h.collect)
}

type caller struct {
	user, project, lang string
}

func (h *Handler) who(r *http.Request) caller {
	return caller{user: auth.UserFrom(r.Context()).UserID, project: r.PathValue("id"), lang: httpx.Lang(r)}
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		lang := httpx.Lang(r)
		h.responder.Error(w, r, project.Localize(messages.Localize(err, lang), lang))
		return
	}
	h.responder.JSON(w, r, status, "", data)
}

// decode reads and validates a body; on failure it has already answered.
func decode[B, T any](h *Handler, w http.ResponseWriter, r *http.Request, validate func(B) (T, error)) (T, bool) {
	var body B
	var zero T
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, 0, nil, err)
		return zero, false
	}
	v, err := validate(body)
	if err != nil {
		h.reply(w, r, 0, nil, err)
		return zero, false
	}
	return v, true
}

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	b, err := h.svc.Book(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, b, err)
}

func (h *Handler) saveDay(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b dayBody) (DayInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.SaveDay(r.Context(), c.user, c.project, c.lang, in))
}

func (h *Handler) collect(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b collectBody) (CollectInput, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.Collect(r.Context(), c.user, c.project, r.PathValue("pid"), c.lang, in))
}
