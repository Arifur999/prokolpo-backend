package support

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Handler serves /api/v1/support.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the support HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes. optionalUser attaches the sender's account when
// they are signed in; the form also works signed out.
func (h *Handler) Register(mux *http.ServeMux, optionalUser func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/support/requests", optionalUser(http.HandlerFunc(h.create)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	lang := httpx.Lang(r)
	var body requestBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.responder.Error(w, r, err)
		return
	}
	req, err := body.validate(lang)
	if err != nil {
		h.responder.Error(w, r, messages.Localize(err, lang))
		return
	}
	if claims, ok := auth.MaybeUserFrom(r.Context()); ok {
		req.UserID = &claims.UserID
		req.Source = "app"
	}
	receipt, err := h.svc.Create(r.Context(), req)
	if err != nil {
		h.responder.Error(w, r, messages.Localize(err, lang))
		return
	}
	h.responder.JSON(w, r, http.StatusCreated, "", receipt)
}
