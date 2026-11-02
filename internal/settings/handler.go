package settings

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

// Handler serves /api/v1/projects/{id}/settings.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the settings HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; every one needs a signed-in user.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}/settings"+path, requireUser(fn))
	}
	route("GET", "", h.load)
	route("PUT", "/general", h.saveGeneral)
	route("PUT", "/phases", h.savePhases)
	route("POST", "/categories", h.addCategory)
	route("PATCH", "/categories/{cid}", h.updateCategory)
	route("DELETE", "/categories/{cid}", h.deleteCategory)
	route("PUT", "/receipt", h.saveReceipt)
	route("POST", "/support", h.grantSupport)
	route("DELETE", "/support", h.revokeSupport)
	route("POST", "/owner", h.transfer)
	route("GET", "/export", h.export)
}

type call struct {
	user, project, lang string
}

func (h *Handler) call(r *http.Request) call {
	return call{user: auth.UserFrom(r.Context()).UserID, project: r.PathValue("id"), lang: httpx.Lang(r)}
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		h.responder.Error(w, r, Localize(err, httpx.Lang(r)))
		return
	}
	h.responder.JSON(w, r, http.StatusOK, "", data)
}

// decode reads a JSON body and validates it; on any problem it has already answered.
func decode[B, In any](h *Handler, w http.ResponseWriter, r *http.Request, validate func(B) (In, error)) (In, bool) {
	var body B
	var zero In
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, nil, err)
		return zero, false
	}
	in, err := validate(body)
	if err != nil {
		h.reply(w, r, nil, err)
		return zero, false
	}
	return in, true
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	st, err := h.svc.Load(r.Context(), c.user, c.project)
	h.reply(w, r, st, err)
}

func (h *Handler) saveGeneral(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	if in, ok := decode(h, w, r, func(b generalBody) (GeneralInput, error) { return b.validate(c.lang) }); ok {
		h.reply(w, r, nil, h.svc.SaveGeneral(r.Context(), c.user, c.project, in))
	}
}

func (h *Handler) savePhases(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	validate := func(b struct {
		Phases []phaseBody `json:"phases"`
	},
	) ([]PhaseInput, error) {
		return validatePhases(b.Phases, c.lang)
	}
	if in, ok := decode(h, w, r, validate); ok {
		h.reply(w, r, nil, h.svc.SavePhases(r.Context(), c.user, c.project, in))
	}
}

func (h *Handler) addCategory(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	if in, ok := decode(h, w, r, func(b categoryBody) (CategoryInput, error) { return b.validate(c.lang) }); ok {
		cat, err := h.svc.AddCategory(r.Context(), c.user, c.project, c.lang, in)
		h.reply(w, r, cat, err)
	}
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	if in, ok := decode(h, w, r, func(b categoryBody) (CategoryInput, error) { return b.validate(c.lang) }); ok {
		h.reply(w, r, nil, h.svc.UpdateCategory(r.Context(), c.user, c.project, r.PathValue("cid"), c.lang, in))
	}
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	h.reply(w, r, nil, h.svc.DeleteCategory(r.Context(), c.user, c.project, r.PathValue("cid")))
}

func (h *Handler) saveReceipt(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	if in, ok := decode(h, w, r, func(b receiptBody) (ReceiptInput, error) { return b.validate(c.lang) }); ok {
		h.reply(w, r, nil, h.svc.SaveReceipt(r.Context(), c.user, c.project, in))
	}
}

func (h *Handler) grantSupport(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body struct {
		Hours int `json:"hours"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, nil, err)
		return
	}
	h.reply(w, r, nil, h.svc.GrantSupport(r.Context(), c.user, c.project, c.lang, body.Hours))
}

func (h *Handler) revokeSupport(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	h.reply(w, r, nil, h.svc.RevokeSupport(r.Context(), c.user, c.project))
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body struct {
		UserID string `json:"userId"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.reply(w, r, nil, err)
		return
	}
	h.reply(w, r, nil, h.svc.TransferOwnership(r.Context(), c.user, c.project, body.UserID))
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	sheets, err := h.svc.Export(r.Context(), c.user, c.project)
	h.reply(w, r, sheets, err)
}
