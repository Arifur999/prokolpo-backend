package members

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a project's team under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the members HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; every one needs a signed-in member, and the changes the owner.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/members", h.team)
	route("GET", "/members/lookup", h.lookup)
	route("POST", "/members", h.invite)
	route("PATCH", "/members/{uid}", h.changeRole)
	route("DELETE", "/members/{uid}", h.remove)
	route("POST", "/members/{uid}/reset-code", h.resetCode)
	route("DELETE", "/invites/{iid}", h.cancelInvite)
}

type call struct {
	user, project, lang string
}

func (h *Handler) call(r *http.Request) call {
	return call{user: auth.UserFrom(r.Context()).UserID, project: r.PathValue("id"), lang: httpx.Lang(r)}
}

// reply sends data, or the error localized.
func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		lang := httpx.Lang(r)
		h.responder.Error(w, r, project.Localize(messages.Localize(err, lang), lang))
		return
	}
	h.responder.JSON(w, r, status, "", data)
}

func (h *Handler) team(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	t, err := h.svc.Team(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, t, err)
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	l, err := h.svc.Lookup(r.Context(), c.user, c.project, r.URL.Query().Get("phone"), c.lang)
	h.reply(w, r, http.StatusOK, l, err)
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body inviteBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, 0, nil, err)
		return
	}
	in, err := body.validate(c.lang)
	if err != nil {
		h.reply(w, r, 0, nil, err)
		return
	}
	added, err := h.svc.Invite(r.Context(), c.user, c.project, in)
	h.reply(w, r, http.StatusCreated, added, err)
}

type roleBody struct {
	Role string `json:"role"`
}

func (h *Handler) changeRole(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body roleBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, 0, nil, err)
		return
	}
	err := h.svc.ChangeRole(r.Context(), c.user, c.project, r.PathValue("uid"), body.Role, c.lang)
	h.reply(w, r, http.StatusOK, nil, err)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.Remove(r.Context(), c.user, c.project, r.PathValue("uid")))
}

func (h *Handler) resetCode(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	rc, err := h.svc.IssueResetCode(r.Context(), c.user, c.project, r.PathValue("uid"))
	h.reply(w, r, http.StatusCreated, rc, err)
}

func (h *Handler) cancelInvite(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.CancelInvite(r.Context(), c.user, c.project, r.PathValue("iid")))
}
