package partners

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/members"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a joint project's partner books under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the partners HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; members read, the owner adds partners and calls, writers take deposits.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/partners", h.board)
	route("GET", "/partners/lookup", h.lookup)
	route("POST", "/partners", h.add)
	route("GET", "/partners/{pid}", h.detail)
	route("POST", "/partners/{pid}/deposits", h.deposit)
	route("POST", "/capital-calls", h.createCall)
	route("GET", "/partner-deposits/{did}/receipt", h.receipt)
	route("POST", "/partner-deposits/{did}/printed", h.printed)
}

type caller struct {
	user, project, lang string
}

func (h *Handler) who(r *http.Request) caller {
	return caller{user: auth.UserFrom(r.Context()).UserID, project: r.PathValue("id"), lang: httpx.Lang(r)}
}

// reply sends data, or the error localized by whichever package coded it.
func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		lang := httpx.Lang(r)
		h.responder.Error(w, r, project.Localize(members.Localize(messages.Localize(err, lang), lang), lang))
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

func (h *Handler) board(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	b, err := h.svc.Board(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, b, err)
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	l, err := h.svc.Lookup(r.Context(), c.user, c.project, r.URL.Query().Get("phone"), c.lang)
	h.reply(w, r, http.StatusOK, l, err)
}

func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b addBody) (NewPartner, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	p, err := h.svc.AddPartner(r.Context(), c.user, c.project, c.lang, in)
	h.reply(w, r, http.StatusCreated, p, err)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	d, err := h.svc.Detail(r.Context(), c.user, c.project, r.PathValue("pid"))
	h.reply(w, r, http.StatusOK, d, err)
}

func (h *Handler) deposit(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b depositBody) (NewDeposit, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	d, err := h.svc.Deposit(r.Context(), c.user, c.project, r.PathValue("pid"), c.lang, in)
	h.reply(w, r, http.StatusCreated, d, err)
}

func (h *Handler) createCall(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b callBody) (NewCall, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	cl, err := h.svc.CreateCall(r.Context(), c.user, c.project, in)
	h.reply(w, r, http.StatusCreated, cl, err)
}

func (h *Handler) receipt(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	rc, err := h.svc.Receipt(r.Context(), c.user, c.project, r.PathValue("did"))
	h.reply(w, r, http.StatusOK, rc, err)
}

func (h *Handler) printed(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.Printed(r.Context(), c.user, c.project, r.PathValue("did")))
}
