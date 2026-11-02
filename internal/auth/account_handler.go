package auth

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// registerAccount adds the signed-in account's own pages (board Profile) under /me.
func (h *Handler) registerAccount(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, requireUser(fn)) }
	route("GET /api/v1/me/profile", h.profile)
	route("PATCH /api/v1/me/profile", h.updateProfile)
	route("PUT /api/v1/me/locale", h.setLocale)
	route("POST /api/v1/me/password", h.changePassword)
	route("POST /api/v1/me/phone", h.changePhone)
	route("POST /api/v1/me/delete", h.deleteAccount)
	route("GET /api/v1/me/devices", h.devices)
	route("DELETE /api/v1/me/devices/{id}", h.endDevice)
	route("POST /api/v1/me/devices/end-others", h.endOtherDevices)
}

func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Profile(r.Context(), UserFrom(r.Context()).UserID)
	h.reply(w, r, p, err)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, err := req.validate(httpx.Lang(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), UserFrom(r.Context()).UserID, in, httpx.Lang(r))
	h.reply(w, r, user, err)
}

func (h *Handler) setLocale(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Locale string `json:"locale"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	h.reply(w, r, nil, h.svc.SetLocale(r.Context(), UserFrom(r.Context()).UserID, req.Locale))
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
		LogoutOthers    bool   `json:"logoutOthers"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	session, err := h.svc.ChangePassword(r.Context(), UserFrom(r.Context()), req.CurrentPassword, req.Password, req.LogoutOthers, clientMeta(r), httpx.Lang(r))
	h.reply(w, r, session, err)
}

func (h *Handler) changePhone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.svc.ChangePhone(r.Context(), UserFrom(r.Context()).UserID, req.Phone, req.Password, httpx.Lang(r))
	h.reply(w, r, user, err)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	h.reply(w, r, nil, h.svc.DeleteAccount(r.Context(), UserFrom(r.Context()).UserID, req.Password, httpx.Lang(r)))
}

func (h *Handler) devices(w http.ResponseWriter, r *http.Request) {
	c := UserFrom(r.Context())
	devices, err := h.svc.Devices(r.Context(), c.UserID, c.SessionID)
	h.reply(w, r, devices, err)
}

func (h *Handler) endDevice(w http.ResponseWriter, r *http.Request) {
	c := UserFrom(r.Context())
	h.reply(w, r, nil, h.svc.EndDevice(r.Context(), c.UserID, c.SessionID, r.PathValue("id")))
}

func (h *Handler) endOtherDevices(w http.ResponseWriter, r *http.Request) {
	c := UserFrom(r.Context())
	ended, err := h.svc.EndOtherDevices(r.Context(), c.UserID, c.SessionID)
	h.reply(w, r, map[string]int{"ended": ended}, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", data)
}
