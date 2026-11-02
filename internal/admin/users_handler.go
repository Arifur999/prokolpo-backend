package admin

import (
	"net/http"
	"strconv"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/staff"
)

func (h *Handler) registerUsers(route func(method, path string, fn http.HandlerFunc)) {
	route("GET", "/users", h.users)
	route("GET", "/users/{uid}", h.user)
	route("POST", "/users/{uid}/reset-link", h.resetLink)
	route("POST", "/users/{uid}/reset-code", h.resetCode)
	route("POST", "/users/{uid}/logout-all", h.logoutAll)
	route("POST", "/users/{uid}/phone", h.changePhone)
	route("POST", "/users/{uid}/suspend", h.suspend(true))
	route("POST", "/users/{uid}/unsuspend", h.suspend(false))
	route("POST", "/users/{uid}/notes", h.addNote)
}

func actor(r *http.Request) Actor {
	return Actor{ID: auth.UserFrom(r.Context()).UserID, Lang: httpx.Lang(r), Meta: staff.Meta{IP: httpx.ClientIP(r), UserAgent: r.UserAgent()}}
}

// body reads a JSON body into B; on failure it has already answered.
func body[B any](h *Handler, w http.ResponseWriter, r *http.Request) (B, bool) {
	var b B
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		h.reply(w, r, 0, nil, err)
		return b, false
	}
	return b, true
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page")) // a missing or odd page is page 1
	p, err := h.svc.Users(r.Context(), UserFilter{Kind: q.Get("filter"), Query: q.Get("q"), Page: page})
	h.reply(w, r, http.StatusOK, p, err)
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.User(r.Context(), r.PathValue("uid"))
	h.reply(w, r, http.StatusOK, d, err)
}

func (h *Handler) resetLink(w http.ResponseWriter, r *http.Request) {
	h.reply(w, r, http.StatusOK, nil, h.svc.SendResetLink(r.Context(), actor(r), r.PathValue("uid")))
}

type noteBody struct {
	Note string `json:"note"`
	Body string `json:"body"`
}

func (h *Handler) resetCode(w http.ResponseWriter, r *http.Request) {
	b, ok := body[noteBody](h, w, r)
	if !ok {
		return
	}
	code, err := h.svc.IssueResetCode(r.Context(), actor(r), r.PathValue("uid"), b.Note)
	h.reply(w, r, http.StatusCreated, code, err)
}

func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	h.reply(w, r, http.StatusOK, nil, h.svc.LogoutEverywhere(r.Context(), actor(r), r.PathValue("uid")))
}

type sensitiveBody struct {
	Phone  string `json:"phone"`
	Reason string `json:"reason"`
	Code   string `json:"code"`
}

func (h *Handler) changePhone(w http.ResponseWriter, r *http.Request) {
	b, ok := body[sensitiveBody](h, w, r)
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.ChangePhone(r.Context(), actor(r), r.PathValue("uid"), b.Phone, b.Reason, b.Code))
}

func (h *Handler) suspend(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, ok := body[sensitiveBody](h, w, r)
		if !ok {
			return
		}
		h.reply(w, r, http.StatusOK, nil, h.svc.SetSuspended(r.Context(), actor(r), r.PathValue("uid"), on, b.Reason, b.Code))
	}
}

func (h *Handler) addNote(w http.ResponseWriter, r *http.Request) {
	b, ok := body[noteBody](h, w, r)
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.AddNote(r.Context(), actor(r), r.PathValue("uid"), b.Body))
}
