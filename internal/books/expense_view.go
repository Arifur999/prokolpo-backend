package books

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Expense is one bill as the list and the edit page show it.
type Expense struct {
	ID           string  `json:"id"`
	Date         string  `json:"date"`
	CategoryID   string  `json:"categoryId"`
	CategoryName string  `json:"categoryName"`
	PhaseID      *string `json:"phaseId"`
	PhaseName    *string `json:"phaseName"`
	SupplierID   *string `json:"supplierId"`
	ContractorID *string `json:"contractorId"`
	PartyName    *string `json:"partyName"`
	Quantity     *string `json:"quantity"`
	Unit         *string `json:"unit"`
	Rate         *int64  `json:"rate"`
	Amount       int64   `json:"amount"`
	Description  *string `json:"description"`
	HasMemo      bool    `json:"hasMemo"`
	PaidNow      bool    `json:"paidNow"`
	PaidAccount  *string `json:"paidAccount"`
	// Payments made later against this bill (not the one made with it).
	PaidOnBill int64      `json:"paidOnBill"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
}

const expenseSelect = `
	SELECT e.id, to_char(e.date, 'YYYY-MM-DD'), e.category_id, c.name AS category, e.phase_id, ph.name, e.supplier_id, e.contractor_id,
	       coalesce(s.name, k.name), e.quantity::text, e.unit, e.rate, e.amount AS amount, e.description, e.memo_path IS NOT NULL,
	       pn.id IS NOT NULL AS paid, pa.name,
	       coalesce((SELECT sum(amount) FROM payments x WHERE x.expense_id = e.id AND NOT x.paid_with_expense AND x.deleted_at IS NULL), 0),
	       u.first_name || ' ' || u.last_name, e.created_at, e.updated_at, e.deleted_at
	FROM expenses e
	JOIN expense_categories c ON c.id = e.category_id
	JOIN users u ON u.id = e.created_by
	LEFT JOIN project_phases ph ON ph.id = e.phase_id
	LEFT JOIN suppliers s ON s.id = e.supplier_id
	LEFT JOIN contractors k ON k.id = e.contractor_id
	LEFT JOIN payments pn ON pn.expense_id = e.id AND pn.paid_with_expense AND pn.deleted_at IS NULL
	LEFT JOIN money_accounts pa ON pa.id = pn.account_id`

func scanExpense(row pgx.Row) (Expense, error) {
	var e Expense
	err := row.Scan(&e.ID, &e.Date, &e.CategoryID, &e.CategoryName, &e.PhaseID, &e.PhaseName, &e.SupplierID, &e.ContractorID,
		&e.PartyName, &e.Quantity, &e.Unit, &e.Rate, &e.Amount, &e.Description, &e.HasMemo, &e.PaidNow, &e.PaidAccount,
		&e.PaidOnBill, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt)
	return e, err
}

func expenseByID(ctx context.Context, q db, projectID, id string, withDeleted bool) (Expense, error) {
	if !isID(id) {
		return Expense{}, notFound()
	}
	where := ` WHERE e.id = $1 AND e.project_id = $2 AND e.deleted_at IS NULL`
	if withDeleted {
		where = ` WHERE e.id = $1 AND e.project_id = $2`
	}
	e, err := scanExpense(q.QueryRow(ctx, expenseSelect+where, id, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Expense{}, notFound()
	}
	return e, err
}

// ExpenseFilter narrows the expense list; zero values mean "all".
type ExpenseFilter struct {
	From, To   *time.Time
	CategoryID string
	PhaseID    string
	PartyID    string
	// "credit" for bills not paid on the spot, "cash" for the ones that were.
	Paid  string
	Query string
	Page  int
	Limit int
}

// where builds the filter's SQL condition and arguments, after $1 (the project).
func (f ExpenseFilter) where() (string, []any) {
	var conds []string
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args)+1)
	}
	if f.From != nil {
		conds = append(conds, "e.date >= "+arg(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "e.date <= "+arg(*f.To))
	}
	if isID(f.CategoryID) {
		conds = append(conds, "e.category_id = "+arg(f.CategoryID))
	}
	if isID(f.PhaseID) {
		conds = append(conds, "e.phase_id = "+arg(f.PhaseID))
	}
	if isID(f.PartyID) {
		p := arg(f.PartyID)
		conds = append(conds, "(e.supplier_id = "+p+" OR e.contractor_id = "+p+")")
	}
	switch f.Paid {
	case "credit":
		conds = append(conds, "pn.id IS NULL")
	case "cash":
		conds = append(conds, "pn.id IS NOT NULL")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := arg("%" + q + "%")
		conds = append(conds, "(e.description ILIKE "+p+" OR c.name ILIKE "+p+" OR s.name ILIKE "+p+" OR k.name ILIKE "+p+")")
	}
	if len(conds) == 0 {
		return "", args
	}
	return " AND " + strings.Join(conds, " AND "), args
}

// ExpenseSummary is the cards above the list, for the whole filtered set.
type ExpenseSummary struct {
	Count       int     `json:"count"`
	Total       int64   `json:"total"`
	Credit      int64   `json:"credit"`
	Cash        int64   `json:"cash"`
	TopCategory *string `json:"topCategory"`
	TopAmount   int64   `json:"topAmount"`
}

// ExpensePage is one page of the list with the summary of everything that matched.
type ExpensePage struct {
	Items   []Expense      `json:"items"`
	Summary ExpenseSummary `json:"summary"`
}

// Expenses lists bills newest first.
func (s *Service) Expenses(ctx context.Context, userID, projectID string, f ExpenseFilter) (ExpensePage, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return ExpensePage{}, err
	}
	cond, args := f.where()
	base := ` WHERE e.project_id = $1 AND e.deleted_at IS NULL` + cond
	all := append([]any{projectID}, args...)

	var sum ExpenseSummary
	err := s.pool.QueryRow(ctx, `
		WITH f AS (`+expenseSelect+base+`)
		SELECT count(*), coalesce(sum(amount), 0),
		       coalesce(sum(amount) FILTER (WHERE NOT paid), 0), coalesce(sum(amount) FILTER (WHERE paid), 0),
		       (SELECT cat FROM (SELECT category AS cat, sum(amount) AS t FROM f GROUP BY 1 ORDER BY 2 DESC LIMIT 1) top),
		       coalesce((SELECT max(t) FROM (SELECT sum(amount) AS t FROM f GROUP BY category) x), 0)
		FROM f`,
		all...).Scan(&sum.Count, &sum.Total, &sum.Credit, &sum.Cash, &sum.TopCategory, &sum.TopAmount)
	if err != nil {
		return ExpensePage{}, fmt.Errorf("summarise expenses: %w", err)
	}

	limitArg := "$" + strconv.Itoa(len(all)+1)
	offsetArg := "$" + strconv.Itoa(len(all)+2)
	rows, err := s.pool.Query(ctx, expenseSelect+base+` ORDER BY e.date DESC, e.created_at DESC LIMIT `+limitArg+` OFFSET `+offsetArg,
		append(all, f.Limit, (f.Page-1)*f.Limit)...)
	if err != nil {
		return ExpensePage{}, fmt.Errorf("list expenses: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Expense, error) { return scanExpense(row) })
	return ExpensePage{Items: items, Summary: sum}, err
}
