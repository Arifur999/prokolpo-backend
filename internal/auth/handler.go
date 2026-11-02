package auth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Handler serves /api/v1/auth.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the auth HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes. requireUser guards the ones that need a signed-in user.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	mux.HandleFunc("GET /api/v1/auth/registration", h.registration)
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.Handle("GET /api/v1/auth/me", requireUser(http.HandlerFunc(h.me)))
	mux.HandleFunc("POST /api/v1/auth/password/options", h.resetOptions)
	mux.HandleFunc("POST /api/v1/auth/password/email", h.sendResetLink)
	mux.HandleFunc("POST /api/v1/auth/password/check", h.checkResetLink)
	mux.HandleFunc("POST /api/v1/auth/password/reset", h.resetPassword)
	mux.HandleFunc("POST /api/v1/auth/password/code", h.resetWithCode)
	mux.HandleFunc("POST /api/v1/auth/invite", h.checkInvite)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.responder.Error(w, r, messages.Localize(err, httpx.Lang(r)))
}

func (h *Handler) registration(w http.ResponseWriter, r *http.Request) {
	mode, err := h.svc.RegistrationMode(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", map[string]string{"mode": mode})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, err := req.validate(httpx.Lang(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	session, err := h.svc.Register(r.Context(), in, clientMeta(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusCreated, welcome(httpx.Lang(r)), session)
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
	Remember   bool   `json:"remember"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Identifier) == "" || req.Password == "" {
		h.fail(w, r, h.required(r, "identifier", req.Identifier, "password", req.Password))
		return
	}
	session, err := h.svc.Login(r.Context(), req.Identifier, req.Password, req.Remember, clientMeta(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", session)
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	session, err := h.svc.Refresh(r.Context(), req.RefreshToken, clientMeta(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", session)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, err := h.svc.Me(r.Context(), UserFrom(r.Context()).UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", user)
}

type phoneRequest struct {
	Phone string `json:"phone"`
}

func (h *Handler) resetOptions(w http.ResponseWriter, r *http.Request) {
	var req phoneRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	opts, err := h.svc.ResetOptions(r.Context(), req.Phone, httpx.Lang(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", opts)
}

func (h *Handler) sendResetLink(w http.ResponseWriter, r *http.Request) {
	var req phoneRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.SendResetLink(r.Context(), req.Phone, httpx.Lang(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}

type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (h *Handler) checkResetLink(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	info, err := h.svc.CheckResetLink(r.Context(), req.Token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", info)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.Password, httpx.Lang(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}

type codeResetRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (h *Handler) resetWithCode(w http.ResponseWriter, r *http.Request) {
	var req codeResetRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.ResetWithCode(r.Context(), req.Phone, req.Code, req.Password, httpx.Lang(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", nil)
}

func (h *Handler) checkInvite(w http.ResponseWriter, r *http.Request) {
	var req phoneRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	invite, err := h.svc.CheckInvite(r.Context(), req.Phone, clientIP(r), httpx.Lang(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", invite)
}

// required reports which of the named fields are blank.
func (h *Handler) required(r *http.Request, pairs ...string) error {
	lang := httpx.Lang(r)
	var fields []apperr.FieldError
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			fields = append(fields, apperr.FieldError{Path: pairs[i], Message: fieldMessages["required"].In(lang)})
		}
	}
	return messages.Fail(apperr.KindInvalid, codeInvalidInput, fields...)
}

func welcome(lang string) string {
	if lang == httpx.LangEnglish {
		return "Welcome! Your account is ready"
	}
	return "স্বাগতম! আপনার অ্যাকাউন্ট তৈরি হয়েছে"
}

// clientMeta records where a session started, for the sessions list. The
// frontend server forwards the browser's address in X-Forwarded-For; it is
// shown to people, never trusted for access decisions.
func clientMeta(r *http.Request) ClientMeta {
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	return ClientMeta{UserAgent: ua, IP: clientIP(r)}
}

func clientIP(r *http.Request) string {
	first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ",")
	if addr, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
		return addr.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String()
	}
	return ""
}
