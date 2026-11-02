package books

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// The units on board ExpenseNew.
var units = []string{"টন", "বস্তা", "কেজি", "সিএফটি", "পিস", "ফুট", "বর্গফুট", "লিটার", "ট্রিপ", "দিন"}

// Up to 11 digits and 3 decimals, as numeric(14,3) holds.
var quantityPattern = regexp.MustCompile(`^\d{1,11}(\.\d{1,3})?$`)

// ExpenseInput is a validated bill.
type ExpenseInput struct {
	CategoryID   string
	PhaseID      *string
	SupplierID   *string
	ContractorID *string
	Date         time.Time
	Quantity     *string
	Unit         *string
	Rate         *int64
	Amount       int64
	Description  *string
	// Create only: pay the whole bill now from AccountID.
	PaidNow   bool
	AccountID *string
	Reason    *string
}

type expenseBody struct {
	CategoryID  string `json:"categoryId"`
	PhaseID     string `json:"phaseId"`
	PartyKind   string `json:"partyKind"`
	PartyID     string `json:"partyId"`
	Date        string `json:"date"`
	Quantity    string `json:"quantity"`
	Unit        string `json:"unit"`
	Rate        *int64 `json:"rate"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	PaidNow     bool   `json:"paidNow"`
	AccountID   string `json:"accountId"`
	Reason      string `json:"reason"`
}

func (b expenseBody) validate(lang string, today time.Time) (ExpenseInput, error) {
	f := &fields{lang: lang}
	in := ExpenseInput{
		CategoryID:  b.CategoryID,
		Date:        f.date("date", b.Date, today),
		Amount:      f.amount("amount", b.Amount, "amount"),
		Description: f.text("description", b.Description, maxNoteLen),
		PaidNow:     b.PaidNow,
		Reason:      f.text("reason", b.Reason, maxNoteLen),
		PhaseID:     f.ref("phaseId", b.PhaseID, "phase"),
	}
	if !isID(b.CategoryID) {
		f.add("categoryId", "category")
	}
	switch b.PartyKind {
	case "SUPPLIER":
		in.SupplierID = f.ref("partyId", b.PartyID, "party")
		if in.SupplierID == nil {
			f.add("partyId", "party")
		}
	case "CONTRACTOR":
		in.ContractorID = f.ref("partyId", b.PartyID, "party")
		if in.ContractorID == nil {
			f.add("partyId", "party")
		}
	}
	if b.Quantity != "" {
		if !quantityPattern.MatchString(b.Quantity) {
			f.add("quantity", "quantity")
		}
		in.Quantity = &b.Quantity
		if slices.Contains(units, b.Unit) {
			in.Unit = &b.Unit
		}
	}
	if b.Rate != nil && *b.Rate > 0 {
		in.Rate = b.Rate
	}
	if b.PaidNow {
		in.AccountID = f.ref("accountId", b.AccountID, "account")
		if in.AccountID == nil {
			f.add("accountId", "account")
		}
	}
	return in, f.err()
}

// checkRefs makes sure every id the bill points at belongs to this project.
func checkRefs(ctx context.Context, q db, projectID string, in ExpenseInput) error {
	refs := []struct {
		table string
		id    *string
	}{
		{"expense_categories", &in.CategoryID},
		{"project_phases", in.PhaseID},
		{"suppliers", in.SupplierID},
		{"contractors", in.ContractorID},
		{"money_accounts", in.AccountID},
	}
	for _, r := range refs {
		if r.id == nil {
			continue
		}
		ok, err := exists(ctx, q, r.table, projectID, *r.id)
		if err != nil {
			return fmt.Errorf("check %s: %w", r.table, err)
		}
		if !ok {
			return notFound()
		}
	}
	return nil
}

// CreateExpense records a bill. Paid now, it brings a payment of the same amount from the
// chosen account, so the party's due does not move; otherwise the amount joins their due.
func (s *Service) CreateExpense(ctx context.Context, userID, projectID string, in ExpenseInput) (Expense, error) {
	var e Expense
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		if err := checkRefs(ctx, tx, projectID, in); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO expenses (project_id, category_id, phase_id, supplier_id, contractor_id, date, quantity, unit, rate, amount, description, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7::numeric, $8, $9, $10, $11, $12) RETURNING id`,
			projectID, in.CategoryID, in.PhaseID, in.SupplierID, in.ContractorID, in.Date, in.Quantity, in.Unit, in.Rate, in.Amount, in.Description, userID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert expense: %w", err)
		}
		if in.PaidNow {
			_, err = tx.Exec(ctx, `
				INSERT INTO payments (project_id, account_id, supplier_id, contractor_id, expense_id, paid_with_expense, date, amount, created_by)
				VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)`,
				projectID, in.AccountID, in.SupplierID, in.ContractorID, id, in.Date, in.Amount, userID)
			if err != nil {
				return fmt.Errorf("insert payment with expense: %w", err)
			}
		}
		if e, err = expenseByID(ctx, tx, projectID, id, false); err != nil {
			return err
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "expense", EntityID: id, Summary: expenseSummary(e)})
	})
	return e, err
}

// Expense returns one live bill, for the edit page.
func (s *Service) Expense(ctx context.Context, userID, projectID, expenseID string) (Expense, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return Expense{}, err
	}
	return expenseByID(ctx, s.pool, projectID, expenseID, false)
}

// UpdateExpense changes a bill and logs what changed and why. Payments made against it stay
// as they were; a bill paid on the spot keeps its payment equal to the bill.
func (s *Service) UpdateExpense(ctx context.Context, userID, projectID, expenseID string, in ExpenseInput) (Expense, error) {
	var after Expense
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		// Locked first, so a delete in between can't leave the bill and its payment apart.
		if isID(expenseID) {
			if _, err := tx.Exec(ctx, `SELECT 1 FROM expenses WHERE id = $1 AND project_id = $2 FOR UPDATE`, expenseID, projectID); err != nil {
				return fmt.Errorf("lock expense: %w", err)
			}
		}
		before, err := expenseByID(ctx, tx, projectID, expenseID, false)
		if err != nil {
			return err
		}
		in.AccountID = nil
		err = checkRefs(ctx, tx, projectID, in)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE expenses SET category_id = $3, phase_id = $4, supplier_id = $5, contractor_id = $6, date = $7, quantity = $8::numeric,
			       unit = $9, rate = $10, amount = $11, description = $12
			WHERE id = $1 AND project_id = $2 AND deleted_at IS NULL`,
			expenseID, projectID, in.CategoryID, in.PhaseID, in.SupplierID, in.ContractorID, in.Date, in.Quantity, in.Unit, in.Rate, in.Amount, in.Description)
		if err != nil {
			return fmt.Errorf("update expense: %w", err)
		}
		_, err = tx.Exec(ctx, `UPDATE payments SET amount = $2, supplier_id = $3, contractor_id = $4, date = $5
			WHERE expense_id = $1 AND paid_with_expense AND deleted_at IS NULL`, expenseID, in.Amount, in.SupplierID, in.ContractorID, in.Date)
		if err != nil {
			return fmt.Errorf("update payment with expense: %w", err)
		}
		if after, err = expenseByID(ctx, tx, projectID, expenseID, false); err != nil {
			return err
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "UPDATE", Entity: "expense", EntityID: expenseID,
			Summary: expenseSummary(after), Changes: expenseChanges(before, after), Reason: in.Reason,
		})
	})
	return after, err
}

// DeleteExpense moves a bill out of the books. Its on-the-spot payment goes with it; later
// payments stay and become an advance with the party.
func (s *Service) DeleteExpense(ctx context.Context, userID, projectID, expenseID string, reason *string) error {
	return s.setExpenseDeleted(ctx, userID, projectID, expenseID, true, reason)
}

// RestoreExpense brings a deleted bill back, with its on-the-spot payment.
func (s *Service) RestoreExpense(ctx context.Context, userID, projectID, expenseID string) error {
	return s.setExpenseDeleted(ctx, userID, projectID, expenseID, false, nil)
}

func (s *Service) setExpenseDeleted(ctx context.Context, userID, projectID, expenseID string, deleted bool, reason *string) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		e, err := expenseByID(ctx, tx, projectID, expenseID, true)
		if err != nil {
			return err
		}
		if (e.DeletedAt != nil) == deleted {
			return nil
		}
		at := s.now()
		var when *time.Time
		var by *string
		action := "RESTORE"
		if deleted {
			when, by, action = &at, &userID, "DELETE"
		}
		if _, err := tx.Exec(ctx, `UPDATE expenses SET deleted_at = $2, deleted_by = $3 WHERE id = $1`, expenseID, when, by); err != nil {
			return fmt.Errorf("delete expense: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET deleted_at = $2, deleted_by = $3 WHERE expense_id = $1 AND paid_with_expense`, expenseID, when, by); err != nil {
			return fmt.Errorf("delete payment with expense: %w", err)
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: action, Entity: "expense", EntityID: expenseID, Summary: expenseSummary(e), Reason: reason})
	})
}

// expenseSummary is the one-line description the activity page shows.
func expenseSummary(e Expense) string {
	return fmt.Sprintf("%s · %s · ৳ %d", e.CategoryName, e.Date, e.Amount)
}

// expenseChanges lists the fields an edit changed, in display form.
func expenseChanges(before, after Expense) []activity.Change {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	num := func(p *int64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatInt(*p, 10)
	}
	candidates := []activity.Change{
		{Field: "category", Before: before.CategoryName, After: after.CategoryName},
		{Field: "quantity", Before: str(before.Quantity) + " " + str(before.Unit), After: str(after.Quantity) + " " + str(after.Unit)},
		{Field: "rate", Before: num(before.Rate), After: num(after.Rate)},
		{Field: "amount", Before: strconv.FormatInt(before.Amount, 10), After: strconv.FormatInt(after.Amount, 10)},
		{Field: "party", Before: str(before.PartyName), After: str(after.PartyName)},
		{Field: "phase", Before: str(before.PhaseName), After: str(after.PhaseName)},
		{Field: "date", Before: before.Date, After: after.Date},
		{Field: "description", Before: str(before.Description), After: str(after.Description)},
	}
	changes := []activity.Change{}
	for _, c := range candidates {
		if c.Before != c.After {
			changes = append(changes, c)
		}
	}
	return changes
}
