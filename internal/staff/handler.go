package staff

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// SessionHeader carries the panel session the frontend keeps in a cookie.
const SessionHeader = "X-Admin-Session"

// Handler serves the second step under /api/v1/admin.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the second-step HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; each needs a signed-in staff member.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/admin"+path, requireUser(h.staffOnly(fn)))
	}
	route("GET", "/2fa", h.status)
	route("POST", "/2fa/setup", h.setup)
	route("POST", "/2fa/enable", h.enable)
	route("POST", "/2fa/verify", h.verify)
	route("GET", "/session", h.session)
	route("POST", "/session/end", h.end)
}

func (h *Handler) staffOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFrom(r.Context()).Role != token.RoleStaff {
			h.reply(w, r, 0, nil, messages.Fail(apperr.KindForbidden, codeNotStaff))
			return
		}
		next(w, r)
	}
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		h.responder.Error(w, r, Localize(err, httpx.Lang(r)))
		return
	}
	h.responder.JSON(w, r, status, "", data)
}

func meta(r *http.Request) Meta { return Meta{IP: httpx.ClientIP(r), UserAgent: r.UserAgent()} }

type codeBody struct {
	Code   string `json:"code"`
	Backup bool   `json:"backup"`
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context(), auth.UserFrom(r.Context()).UserID)
	h.reply(w, r, http.StatusOK, st, err)
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Begin(r.Context(), auth.UserFrom(r.Context()).UserID)
	h.reply(w, r, http.StatusOK, out, err)
}

func (h *Handler) enable(w http.ResponseWriter, r *http.Request) {
	var b codeBody
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		h.reply(w, r, 0, nil, err)
		return
	}
	out, err := h.svc.Enable(r.Context(), auth.UserFrom(r.Context()).UserID, b.Code, meta(r))
	h.reply(w, r, http.StatusOK, out, err)
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var b codeBody
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		h.reply(w, r, 0, nil, err)
		return
	}
	out, err := h.svc.Verify(r.Context(), auth.UserFrom(r.Context()).UserID, b.Code, b.Backup, meta(r))
	h.reply(w, r, http.StatusOK, out, err)
}

// session answers whether the panel session is still alive, keeping it alive if so.
func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Touch(r.Context(), auth.UserFrom(r.Context()).UserID, r.Header.Get(SessionHeader))
	h.reply(w, r, http.StatusOK, nil, err)
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request) {
	h.reply(w, r, http.StatusOK, nil, h.svc.End(r.Context(), r.Header.Get(SessionHeader)))
}

// RequireSession lets a request through only for staff with a live panel session; the
// admin panel's routes sit behind it (and behind auth.RequireUser).
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return h.staffOnly(func(w http.ResponseWriter, r *http.Request) {
		if err := h.svc.Touch(r.Context(), auth.UserFrom(r.Context()).UserID, r.Header.Get(SessionHeader)); err != nil {
			h.reply(w, r, 0, nil, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
