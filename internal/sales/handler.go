package sales

import (
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a project's sales books under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the sales HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; members read, owners and managers write.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/units", h.units)
	route("POST", "/units", h.addUnits)
	route("PUT", "/units/{uid}", h.updateUnit)
	route("DELETE", "/units/{uid}", h.deleteUnit)
	route("GET", "/customers", h.customers)
	route("POST", "/customers", h.addCustomer)
	route("GET", "/customers/{cid}", h.customer)
	route("GET", "/sales", h.sales)
	route("POST", "/sales", h.createSale)
	route("GET", "/sales/{sid}", h.sale)
	route("POST", "/sales/{sid}/payments", h.collect)
	route("GET", "/sale-payments/{pid}/receipt", h.receipt)
	route("POST", "/sale-payments/{pid}/printed", h.printed)
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

func (h *Handler) units(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	list, err := h.svc.Units(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) addUnits(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b unitsBody) ([]UnitInput, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	n, err := h.svc.AddUnits(r.Context(), c.user, c.project, in)
	h.reply(w, r, http.StatusCreated, map[string]int{"added": n}, err)
}

func (h *Handler) updateUnit(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b unitBody) (UnitInput, error) {
		f := &fields{lang: c.lang}
		u := b.validate(f, "")
		return u, f.err()
	})
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.UpdateUnit(r.Context(), c.user, c.project, r.PathValue("uid"), in))
}

func (h *Handler) deleteUnit(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.DeleteUnit(r.Context(), c.user, c.project, r.PathValue("uid")))
}

func (h *Handler) customers(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	list, err := h.svc.Customers(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) addCustomer(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b customerBody) (CustomerInput, error) {
		f := &fields{lang: c.lang}
		v := b.validate(f, "")
		return v, f.err()
	})
	if !ok {
		return
	}
	cu, err := h.svc.AddCustomer(r.Context(), c.user, c.project, in)
	h.reply(w, r, http.StatusCreated, cu, err)
}

func (h *Handler) customer(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	cu, err := h.svc.Customer(r.Context(), c.user, c.project, r.PathValue("cid"))
	h.reply(w, r, http.StatusOK, cu, err)
}

func (h *Handler) sales(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	list, err := h.svc.Sales(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) createSale(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b saleBody) (SaleInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	sold, err := h.svc.CreateSale(r.Context(), c.user, c.project, c.lang, in)
	h.reply(w, r, http.StatusCreated, sold, err)
}

func (h *Handler) sale(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	d, err := h.svc.Sale(r.Context(), c.user, c.project, r.PathValue("sid"))
	h.reply(w, r, http.StatusOK, d, err)
}

func (h *Handler) collect(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	in, ok := decode(h, w, r, func(b collectBody) (CollectInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	sold, err := h.svc.Collect(r.Context(), c.user, c.project, r.PathValue("sid"), c.lang, in)
	h.reply(w, r, http.StatusCreated, sold, err)
}

func (h *Handler) receipt(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	rc, err := h.svc.Receipt(r.Context(), c.user, c.project, r.PathValue("pid"))
	h.reply(w, r, http.StatusOK, rc, err)
}

func (h *Handler) printed(w http.ResponseWriter, r *http.Request) {
	c := h.who(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.Printed(r.Context(), c.user, c.project, r.PathValue("pid")))
}
