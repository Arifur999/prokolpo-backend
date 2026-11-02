package books

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

var accountKinds = []string{"CASH", "BANK", "MOBILE"}

// movementsCTE is every movement of money through a project's accounts ($1 is the
// project): starting balances, payments out, and both legs of each transfer. Partner
// deposits and installments join it when the sales books arrive.
const movementsCTE = `
WITH movements AS (
	SELECT a.id::text AS ref, 'OPENING' AS kind, a.id AS account_id, (a.created_at AT TIME ZONE 'Asia/Dhaka')::date AS date,
	       a.created_at, a.opening_balance AS amount, NULL::text AS party, NULL::text AS title, NULL::text AS note, NULL::text AS other
	FROM money_accounts a WHERE a.project_id = $1 AND a.opening_balance <> 0
	UNION ALL
	SELECT p.id::text, CASE WHEN p.paid_with_expense AND p.supplier_id IS NULL AND p.contractor_id IS NULL THEN 'BUY' ELSE 'PAYMENT' END,
	       p.account_id, p.date, p.created_at, -p.amount, coalesce(s.name, c.name), coalesce(e.description, ec.name), p.note, NULL
	FROM payments p
	LEFT JOIN suppliers s ON s.id = p.supplier_id
	LEFT JOIN contractors c ON c.id = p.contractor_id
	LEFT JOIN expenses e ON e.id = p.expense_id
	LEFT JOIN expense_categories ec ON ec.id = e.category_id
	WHERE p.project_id = $1 AND p.deleted_at IS NULL
	UNION ALL
	SELECT t.id::text, 'TRANSFER_OUT', t.from_account_id, t.date, t.created_at, -t.amount, NULL, NULL, t.note, b.name
	FROM transfers t JOIN money_accounts b ON b.id = t.to_account_id WHERE t.project_id = $1
	UNION ALL
	SELECT t.id::text, 'TRANSFER_IN', t.to_account_id, t.date, t.created_at, t.amount, NULL, NULL, t.note, b.name
	FROM transfers t JOIN money_accounts b ON b.id = t.from_account_id WHERE t.project_id = $1
)`

// Account is one money account with its balance today.
type Account struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Number    *string    `json:"number"`
	Opening   int64      `json:"opening"`
	Balance   int64      `json:"balance"`
	LastDate  *time.Time `json:"lastDate"`
	LastMove  *int64     `json:"lastMove"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Movement is one line of the accounts ledger, with that account's balance after it.
type Movement struct {
	Ref         string    `json:"ref"`
	Kind        string    `json:"kind"`
	AccountID   string    `json:"accountId"`
	AccountName string    `json:"accountName"`
	Date        string    `json:"date"`
	Amount      int64     `json:"amount"`
	Party       *string   `json:"party"`
	Title       *string   `json:"title"`
	Note        *string   `json:"note"`
	Other       *string   `json:"otherAccount"`
	Balance     int64     `json:"balance"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Accounts lists the project's accounts, oldest first, with balances.
func (s *Service) Accounts(ctx context.Context, userID, projectID string) ([]Account, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	return accountsOf(ctx, s.pool, projectID)
}

func accountsOf(ctx context.Context, q db, projectID string) ([]Account, error) {
	rows, err := q.Query(ctx, movementsCTE+`,
		latest AS (
			SELECT DISTINCT ON (account_id) account_id, date, amount FROM movements
			WHERE kind <> 'OPENING' ORDER BY account_id, date DESC, created_at DESC
		)
		SELECT a.id, a.name, a.kind, a.number, a.opening_balance,
		       a.opening_balance + coalesce((SELECT sum(amount) FROM movements m WHERE m.account_id = a.id AND m.kind <> 'OPENING'), 0),
		       l.date, l.amount, a.created_at
		FROM money_accounts a LEFT JOIN latest l ON l.account_id = a.id
		WHERE a.project_id = $1 ORDER BY a.created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) {
		var a Account
		err := row.Scan(&a.ID, &a.Name, &a.Kind, &a.Number, &a.Opening, &a.Balance, &a.LastDate, &a.LastMove, &a.CreatedAt)
		return a, err
	})
}

// Ledger lists every movement, newest first, each with its account's running balance. An
// opening balance counts first and lists last, whatever its date.
func (s *Service) Ledger(ctx context.Context, userID, projectID string) ([]Movement, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, movementsCTE+`
		SELECT m.ref, m.kind, m.account_id, a.name, to_char(m.date, 'YYYY-MM-DD'), m.amount, m.party, m.title, m.note, m.other,
		       sum(m.amount) OVER (PARTITION BY m.account_id ORDER BY m.kind <> 'OPENING', m.date, m.created_at, m.ref ROWS UNBOUNDED PRECEDING),
		       m.created_at
		FROM movements m JOIN money_accounts a ON a.id = m.account_id
		ORDER BY m.kind = 'OPENING', m.date DESC, m.created_at DESC, m.ref DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Movement, error) {
		var m Movement
		err := row.Scan(&m.Ref, &m.Kind, &m.AccountID, &m.AccountName, &m.Date, &m.Amount, &m.Party, &m.Title, &m.Note, &m.Other, &m.Balance, &m.CreatedAt)
		return m, err
	})
}

// NewAccount is a validated new money account.
type NewAccount struct {
	Name    string
	Kind    string
	Number  *string
	Opening int64
}

type accountBody struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Number  string `json:"number"`
	Opening int64  `json:"opening"`
}

func (b accountBody) validate(lang string) (NewAccount, error) {
	f := &fields{lang: lang}
	na := NewAccount{Kind: b.Kind, Opening: b.Opening}
	if name := f.text("name", b.Name, 60); name == nil {
		f.add("name", "accountName")
	} else {
		na.Name = *name
	}
	if !slices.Contains(accountKinds, b.Kind) {
		f.add("kind", "accountKind")
	}
	na.Number = f.text("number", b.Number, 60)
	if b.Opening < 0 || b.Opening > maxAmount {
		f.add("opening", "amount")
	}
	return na, f.err()
}

// CreateAccount adds a money account; its opening balance starts its ledger.
func (s *Service) CreateAccount(ctx context.Context, userID, projectID, lang string, na NewAccount) (Account, error) {
	var id string
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		err := tx.QueryRow(ctx, `INSERT INTO money_accounts (project_id, name, kind, number, opening_balance) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			projectID, na.Name, na.Kind, na.Number, na.Opening).Scan(&id)
		if isUniqueViolation(err) {
			return nameTaken(lang, "accountTaken")
		}
		if err != nil {
			return fmt.Errorf("insert account: %w", err)
		}
		return logActivity(ctx, tx, activity{projectID: projectID, actorID: userID, action: "CREATE", entity: "account", entityID: id, summary: na.Name})
	})
	if err != nil {
		return Account{}, err
	}
	accounts, err := accountsOf(ctx, s.pool, projectID)
	if err != nil {
		return Account{}, err
	}
	for _, a := range accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, notFound()
}
