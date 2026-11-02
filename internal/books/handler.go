package books

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Handler serves a project's books under /api/v1/projects/{id}.
type Handler struct {
	svc       *Service
	responder *httpx.Responder
}

// NewHandler returns the books HTTP handlers.
func NewHandler(svc *Service, responder *httpx.Responder) *Handler {
	return &Handler{svc: svc, responder: responder}
}

// Register mounts the routes; every one needs a signed-in member of the project.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	route := func(method, path string, fn http.HandlerFunc) {
		mux.Handle(method+" /api/v1/projects/{id}"+path, requireUser(fn))
	}
	route("GET", "/overview", h.overview)
	route("GET", "/categories", h.categories)
	route("POST", "/categories", h.createCategory)

	route("GET", "/accounts", h.accounts)
	route("POST", "/accounts", h.createAccount)
	route("GET", "/accounts/ledger", h.ledger)
	route("POST", "/transfers", h.transfer)

	route("GET", "/suppliers", h.suppliers)
	route("POST", "/suppliers", h.createSupplier)
	route("GET", "/suppliers/{sid}", h.supplierLedger)
	route("POST", "/suppliers/{sid}/payments", h.paySupplier)

	route("GET", "/contractors", h.contractors)
	route("POST", "/contractors", h.createContractor)
	route("GET", "/contractors/{cid}", h.contractorLedger)
	route("PATCH", "/contractors/{cid}/status", h.contractorStatus)
	route("POST", "/contractors/{cid}/extras", h.addExtra)
	route("POST", "/contractors/{cid}/payments", h.payContractor)
	route("GET", "/attendance", h.attendance)
	route("PUT", "/attendance", h.saveAttendance)

	route("GET", "/expenses", h.expenses)
	route("POST", "/expenses", h.createExpense)
	route("GET", "/expenses/{eid}", h.expense)
	route("PUT", "/expenses/{eid}", h.updateExpense)
	route("POST", "/expenses/{eid}/delete", h.deleteExpense)
	route("POST", "/expenses/{eid}/restore", h.restoreExpense)
	route("POST", "/payments/{pid}/delete", h.deletePayment)
}

// call is what every handler does: who is asking, about which project, in which language.
type call struct {
	user, project, lang string
}

func (h *Handler) call(r *http.Request) call {
	return call{user: auth.UserFrom(r.Context()).UserID, project: r.PathValue("id"), lang: httpx.Lang(r)}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	lang := httpx.Lang(r)
	h.responder.Error(w, r, project.Localize(messages.Localize(err, lang), lang))
}

// reply sends data, or the error localized.
func (h *Handler) reply(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.JSON(w, r, status, "", data)
}

// decode reads and validates a body; on failure it has already answered.
func decode[B, T any](h *Handler, w http.ResponseWriter, r *http.Request, validate func(B) (T, error)) (T, bool) {
	var body B
	var zero T
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return zero, false
	}
	v, err := validate(body)
	if err != nil {
		h.fail(w, r, err)
		return zero, false
	}
	return v, true
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	o, err := h.svc.Overview(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, o, err)
}

func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	list, err := h.svc.Categories(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body categoryBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	cat, err := h.svc.CreateCategory(r.Context(), c.user, c.project, c.lang, body)
	h.reply(w, r, http.StatusCreated, cat, err)
}

func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	list, err := h.svc.Accounts(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	na, ok := decode(h, w, r, func(b accountBody) (NewAccount, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	a, err := h.svc.CreateAccount(r.Context(), c.user, c.project, c.lang, na)
	h.reply(w, r, http.StatusCreated, a, err)
}

func (h *Handler) ledger(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	list, err := h.svc.Ledger(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	t, ok := decode(h, w, r, func(b transferBody) (NewTransfer, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.Transfer(r.Context(), c.user, c.project, t))
}

func (h *Handler) suppliers(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	list, err := h.svc.Suppliers(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	ns, ok := decode(h, w, r, func(b supplierBody) (NewSupplier, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	s, err := h.svc.CreateSupplier(r.Context(), c.user, c.project, c.lang, ns)
	h.reply(w, r, http.StatusCreated, s, err)
}

func (h *Handler) supplierLedger(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	d, err := h.svc.SupplierLedger(r.Context(), c.user, c.project, r.PathValue("sid"))
	h.reply(w, r, http.StatusOK, d, err)
}

func (h *Handler) pay(w http.ResponseWriter, r *http.Request, to Party) {
	c := h.call(r)
	p, ok := decode(h, w, r, func(b paymentBody) (NewPayment, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.Pay(r.Context(), c.user, c.project, to, p))
}

func (h *Handler) paySupplier(w http.ResponseWriter, r *http.Request) {
	h.pay(w, r, Party{Table: "suppliers", ID: r.PathValue("sid")})
}

func (h *Handler) payContractor(w http.ResponseWriter, r *http.Request) {
	h.pay(w, r, Party{Table: "contractors", ID: r.PathValue("cid")})
}

func (h *Handler) contractors(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	list, err := h.svc.Contractors(r.Context(), c.user, c.project)
	h.reply(w, r, http.StatusOK, list, err)
}

func (h *Handler) createContractor(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	nc, ok := decode(h, w, r, func(b contractorBody) (NewContractor, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	k, err := h.svc.CreateContractor(r.Context(), c.user, c.project, c.lang, nc)
	h.reply(w, r, http.StatusCreated, k, err)
}

func (h *Handler) contractorLedger(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	d, err := h.svc.ContractorLedger(r.Context(), c.user, c.project, r.PathValue("cid"))
	h.reply(w, r, http.StatusOK, d, err)
}

func (h *Handler) contractorStatus(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	var body struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.SetContractorStatus(r.Context(), c.user, c.project, r.PathValue("cid"), body.Status))
}

func (h *Handler) addExtra(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	ne, ok := decode(h, w, r, func(b extraBody) (NewExtra, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusCreated, nil, h.svc.AddExtra(r.Context(), c.user, c.project, r.PathValue("cid"), ne))
}

func (h *Handler) attendance(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	day := h.svc.today()
	if q := r.URL.Query().Get("week"); q != "" {
		if d, err := time.Parse(time.DateOnly, q); err == nil {
			day = d
		}
	}
	start, teams, err := h.svc.AttendanceWeek(r.Context(), c.user, c.project, day)
	h.reply(w, r, http.StatusOK, map[string]any{"weekStart": start, "today": h.svc.today().Format(time.DateOnly), "teams": teams}, err)
}

func (h *Handler) saveAttendance(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	cells, ok := decode(h, w, r, func(b attendanceBody) ([]AttendanceCell, error) { return b.validate(c.lang) })
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.SaveAttendance(r.Context(), c.user, c.project, cells))
}

// Expense list paging: 10 a page, as board ExpenseList shows.
const (
	defaultPageSize = 10
	maxPageSize     = 100
)

func (h *Handler) expenses(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	q := r.URL.Query()
	f := ExpenseFilter{CategoryID: q.Get("category"), PhaseID: q.Get("phase"), PartyID: q.Get("party"), Paid: q.Get("paid"), Query: q.Get("q")}
	if d, err := time.Parse(time.DateOnly, q.Get("from")); err == nil {
		f.From = &d
	}
	if d, err := time.Parse(time.DateOnly, q.Get("to")); err == nil {
		f.To = &d
	}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Page = max(f.Page, 1)
	if f.Limit < 1 || f.Limit > maxPageSize {
		f.Limit = defaultPageSize
	}
	page, err := h.svc.Expenses(r.Context(), c.user, c.project, f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.responder.List(w, r, "", page, httpx.NewMeta(f.Page, f.Limit, page.Summary.Count))
}

func (h *Handler) createExpense(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	in, ok := decode(h, w, r, func(b expenseBody) (ExpenseInput, error) { return b.validate(c.lang, h.svc.today()) })
	if !ok {
		return
	}
	e, err := h.svc.CreateExpense(r.Context(), c.user, c.project, in)
	h.reply(w, r, http.StatusCreated, e, err)
}

func (h *Handler) expense(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	e, err := h.svc.Expense(r.Context(), c.user, c.project, r.PathValue("eid"))
	h.reply(w, r, http.StatusOK, e, err)
}

func (h *Handler) updateExpense(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	in, ok := decode(h, w, r, func(b expenseBody) (ExpenseInput, error) {
		b.PaidNow = false
		return b.validate(c.lang, h.svc.today())
	})
	if !ok {
		return
	}
	e, err := h.svc.UpdateExpense(r.Context(), c.user, c.project, r.PathValue("eid"), in)
	h.reply(w, r, http.StatusOK, e, err)
}

type reasonBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) reason(w http.ResponseWriter, r *http.Request) (*string, bool) {
	c := h.call(r)
	return decode(h, w, r, func(b reasonBody) (*string, error) {
		f := &fields{lang: c.lang}
		reason := f.text("reason", b.Reason, maxNoteLen)
		return reason, f.err()
	})
}

func (h *Handler) deleteExpense(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	reason, ok := h.reason(w, r)
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.DeleteExpense(r.Context(), c.user, c.project, r.PathValue("eid"), reason))
}

func (h *Handler) restoreExpense(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	h.reply(w, r, http.StatusOK, nil, h.svc.RestoreExpense(r.Context(), c.user, c.project, r.PathValue("eid")))
}

func (h *Handler) deletePayment(w http.ResponseWriter, r *http.Request) {
	c := h.call(r)
	reason, ok := h.reason(w, r)
	if !ok {
		return
	}
	h.reply(w, r, http.StatusOK, nil, h.svc.DeletePayment(r.Context(), c.user, c.project, r.PathValue("pid"), reason))
}
