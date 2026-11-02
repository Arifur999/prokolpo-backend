package project

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Handler serves /api/v1/projects.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the project HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; every one needs a signed-in user.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, requireUser(fn)) }
	route("GET /api/v1/projects", h.list)
	route("POST /api/v1/projects", h.create)
	route("GET /api/v1/projects/deleted", h.deleted)
	route("GET /api/v1/projects/{id}", h.get)
	route("DELETE /api/v1/projects/{id}", h.delete)
	route("POST /api/v1/projects/{id}/restore", h.restore)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.responder.Error(w, r, messages.Localize(err, httpx.Lang(r)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", projects)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	lang := httpx.Lang(r)
	var body createBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	np, err := body.validate(lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.svc.Create(r.Context(), auth.UserFrom(r.Context()).UserID, cashAccountName.In(lang), np)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusCreated, "", p)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", p)
}

func (h *Handler) deleted(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.Deleted(r.Context(), auth.UserFrom(r.Context()).UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", projects)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).UserID, r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Restore(r.Context(), auth.UserFrom(r.Context()).UserID, r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}
