// Package settings is the owner's project settings page (boards Settings, SettingsPhases,
// SettingsReceipt, SettingsSupport, SettingsSupportGrant, SettingsDelete): the project's
// details, its phases and heads, receipt printing, support access, ownership and the backup.
// Everyone in the project can read it; only the owner changes it.
package settings

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Service reads and changes a project's settings.
type Service struct {
	pool    *pgxpool.Pool
	members *project.Service
	now     func() time.Time
}

// NewService returns a Service.
func NewService(pool *pgxpool.Pool, members *project.Service) *Service {
	return &Service{pool: pool, members: members, now: time.Now}
}

// Settings is the whole page.
type Settings struct {
	Role       string     `json:"role"`
	General    General    `json:"general"`
	Phases     []Phase    `json:"phases"`
	Categories []Category `json:"categories"`
	Receipt    Receipt    `json:"receipt"`
	Support    *Grant     `json:"support"`
	Members    []Member   `json:"members"`
	Counts     Counts     `json:"counts"`
}

// General is the "প্রজেক্টের তথ্য" tab; kind and ownership are fixed at creation.
type General struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Ownership    string  `json:"ownership"`
	FarmType     *string `json:"farmType"`
	Floors       *int16  `json:"floors"`
	BusinessType *string `json:"businessType"`
	Location     *string `json:"location"`
	Description  *string `json:"description"`
	StartDate    string  `json:"startDate"`
	Status       string  `json:"status"`
	Partners     int     `json:"partners"`
}

// Phase is one row of "তলা ও ধাপ". Locked phases have entries and can't be deleted.
type Phase struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Budget *int64 `json:"budget"`
	Spent  int64  `json:"spent"`
	Locked bool   `json:"locked"`
}

// Category is one row of "খরচের খাত".
type Category struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Entries int    `json:"entries"`
	Spent   int64  `json:"spent"`
	Budget  *int64 `json:"budget"`
	Hidden  bool   `json:"hidden"`
}

// Receipt is what deposit and installment receipts print ("রশিদ ও প্রিন্ট").
type Receipt struct {
	Company string `json:"company"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Prefix  string `json:"prefix"`
	Lang    string `json:"lang"`
	Paper   string `json:"paper"`
	Footer  string `json:"footer"`
	// Next is the number the next receipt will get.
	Next int `json:"next"`
}

// Grant is the support access that is open now.
type Grant struct {
	StartsAt     time.Time  `json:"startsAt"`
	EndsAt       time.Time  `json:"endsAt"`
	GrantedBy    string     `json:"grantedBy"`
	LastOpenedBy *string    `json:"lastOpenedBy"`
	LastOpenedAt *time.Time `json:"lastOpenedAt"`
}

// Member is someone the project could be handed to.
type Member struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	// Share is their partner share in percent, when they are a partner of a joint project.
	Share *float64 `json:"share"`
}

// Counts is what deleting the project would take away from everyone.
type Counts struct {
	Expenses int `json:"expenses"`
	Sales    int `json:"sales"`
	Partners int `json:"partners"`
	Receipts int `json:"receipts"`
	Members  int `json:"members"`
}

// Load reads the page for any member of the project.
func (s *Service) Load(ctx context.Context, userID, projectID string) (Settings, error) {
	m, err := s.members.Member(ctx, userID, projectID)
	if err != nil {
		return Settings{}, err
	}
	st := Settings{Role: m.Role}
	steps := []func(context.Context, string, *Settings) error{s.loadGeneral, s.loadPhases, s.loadCategories, s.loadSupport, s.loadMembers}
	for _, step := range steps {
		if stepErr := step(ctx, projectID, &st); stepErr != nil {
			return Settings{}, stepErr
		}
	}
	return st, nil
}

func (s *Service) loadGeneral(ctx context.Context, projectID string, st *Settings) error {
	g, r, c := &st.General, &st.Receipt, &st.Counts
	err := s.pool.QueryRow(ctx, `
		SELECT p.name, p.kind, p.ownership, p.farm_type, p.floors, p.business_type, p.location, p.description,
		       to_char(p.start_date, 'YYYY-MM-DD'), p.status,
		       coalesce(p.receipt_company, nullif(o.company_name, ''), o.first_name || ' ' || o.last_name),
		       coalesce(p.receipt_address, p.location, ''), coalesce(p.receipt_phone, o.phone),
		       p.receipt_prefix, p.receipt_lang, p.receipt_paper, coalesce(p.receipt_footer, ''), p.receipt_seq + 1,
		       (SELECT count(*) FROM partners x WHERE x.project_id = p.id),
		       (SELECT count(*) FROM expenses x WHERE x.project_id = p.id AND x.deleted_at IS NULL),
		       (SELECT count(*) FROM sales x WHERE x.project_id = p.id) + (SELECT count(*) FROM produce_sales x WHERE x.project_id = p.id),
		       p.receipt_seq,
		       (SELECT count(*) FROM project_members x WHERE x.project_id = p.id)
		FROM projects p JOIN users o ON o.id = p.owner_id WHERE p.id = $1`, projectID).Scan(
		&g.Name, &g.Kind, &g.Ownership, &g.FarmType, &g.Floors, &g.BusinessType, &g.Location, &g.Description,
		&g.StartDate, &g.Status,
		&r.Company, &r.Address, &r.Phone, &r.Prefix, &r.Lang, &r.Paper, &r.Footer, &r.Next,
		&g.Partners, &c.Expenses, &c.Sales, &c.Receipts, &c.Members)
	if err != nil {
		return fmt.Errorf("load project settings: %w", err)
	}
	c.Partners = g.Partners
	return nil
}

func (s *Service) loadPhases(ctx context.Context, projectID string, st *Settings) error {
	rows, err := s.pool.Query(ctx, `
		SELECT ph.id, ph.name, ph.budget,
		       coalesce((SELECT sum(e.amount) FROM expenses e WHERE e.phase_id = ph.id AND e.deleted_at IS NULL), 0)::bigint,
		       ph.setup OR EXISTS (SELECT 1 FROM expenses e WHERE e.phase_id = ph.id)
		                OR EXISTS (SELECT 1 FROM produce_sales x WHERE x.phase_id = ph.id)
		FROM project_phases ph WHERE ph.project_id = $1 ORDER BY ph.position, ph.created_at`, projectID)
	if err != nil {
		return fmt.Errorf("list phases: %w", err)
	}
	st.Phases, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Phase])
	if err != nil {
		return fmt.Errorf("scan phases: %w", err)
	}
	return nil
}

func (s *Service) loadCategories(ctx context.Context, projectID string, st *Settings) error {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name,
		       (SELECT count(*) FROM expenses e WHERE e.category_id = c.id AND e.deleted_at IS NULL)::int,
		       coalesce((SELECT sum(e.amount) FROM expenses e WHERE e.category_id = c.id AND e.deleted_at IS NULL), 0)::bigint,
		       c.budget, c.hidden
		FROM expense_categories c WHERE c.project_id = $1 ORDER BY c.position, c.created_at`, projectID)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	st.Categories, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Category])
	if err != nil {
		return fmt.Errorf("scan categories: %w", err)
	}
	return nil
}

func (s *Service) loadMembers(ctx context.Context, projectID string, st *Settings) error {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, m.role,
		       (SELECT x.share::float8 FROM partners x WHERE x.project_id = m.project_id AND x.phone = u.phone LIMIT 1)
		FROM project_members m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.role <> 'OWNER'
		ORDER BY m.role = 'MANAGER' DESC, m.created_at`, projectID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	st.Members, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
	if err != nil {
		return fmt.Errorf("scan members: %w", err)
	}
	return nil
}
