package produce

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a farm's sales under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the farm sales HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; members read, owners and managers write.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/produce", h.book)
	route("POST", "/produce", h.createSale)
	route("POST", "/produce/buyers/{bid}/payments", h.collect)
	route("PUT", "/batches/{phid}", h.updateBatch)
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

func (h *Handler) createSale(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b saleBody) (SaleInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	saved, err := h.svc.CreateSale(r.Context(), c.user, c.project, c.lang, in)
	h.reply(w, r, http.StatusCreated, saved, err)
}

func (h *Handler) collect(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b collectBody) (CollectInput, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.Collect(r.Context(), c.user, c.project, r.PathValue("bid"), c.lang, in))
}

func (h *Handler) updateBatch(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b batchBody) (BatchInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.UpdateBatch(r.Context(), c.user, c.project, r.PathValue("phid"), in))
}
