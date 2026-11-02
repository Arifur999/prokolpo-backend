package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// UsersPerPage is how many accounts a page of the list holds (board AdminUsers).
const UsersPerPage = 50

// UserFilter is one of the list's chips: ALL, NEW (this week), NOEMAIL, SUSPENDED or STAFF.
type UserFilter struct {
	Kind  string
	Query string
	Page  int
}

// UserRow is one account in the list. Note is the line under the name: why it is suspended,
// or the open support request.
type UserRow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Company    *string    `json:"company"`
	Phone      string     `json:"phone"`
	Email      *string    `json:"email"`
	Locale     string     `json:"locale"`
	Status     string     `json:"status"`
	StaffRole  *string    `json:"staffRole"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastActive *time.Time `json:"lastActive"`
	Owned      int        `json:"owned"`
	Member     int        `json:"member"`
	Suspended  *string    `json:"suspendedReason"`
	OpenTicket *int       `json:"openTicket"`
}

// UserPage is a page of the list with every chip's count.
type UserPage struct {
	Rows   []UserRow      `json:"rows"`
	Counts map[string]int `json:"counts"`
	Total  int            `json:"total"`
	Page   int            `json:"page"`
	Pages  int            `json:"pages"`
}

// userWhere picks the accounts for a chip and a search ($1 is a week ago, $2 the search, $3 its digits).
const userWhere = `
	CASE $4::text
		WHEN 'NEW' THEN u.created_at > $1 AND u.staff_role IS NULL
		WHEN 'NOEMAIL' THEN u.email IS NULL AND u.staff_role IS NULL
		WHEN 'SUSPENDED' THEN u.status = 'suspended'
		WHEN 'STAFF' THEN u.staff_role IS NOT NULL
		ELSE true
	END
	AND ($2 = '' OR lower(u.first_name || ' ' || u.last_name || ' ' || coalesce(u.company_name, '') || ' ' || coalesce(u.email::text, '')) LIKE '%' || lower($2) || '%'
	     OR (length($3) >= 3 AND u.phone LIKE '%' || $3 || '%'))`

// Users lists accounts, most recently active first.
func (s *Service) Users(ctx context.Context, f UserFilter) (UserPage, error) {
	weekAgo := s.now().AddDate(0, 0, -7)
	query := strings.TrimSpace(f.Query)
	digits := digitsOf(query)
	p := UserPage{Page: max(f.Page, 1)}
	var all, fresh, noEmail, suspended, staffCount int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE created_at > $1 AND staff_role IS NULL),
		       count(*) FILTER (WHERE email IS NULL AND staff_role IS NULL),
		       count(*) FILTER (WHERE status = 'suspended'), count(*) FILTER (WHERE staff_role IS NOT NULL)
		FROM users`, weekAgo).Scan(&all, &fresh, &noEmail, &suspended, &staffCount)
	if err != nil {
		return UserPage{}, fmt.Errorf("count users: %w", err)
	}
	p.Counts = map[string]int{"ALL": all, "NEW": fresh, "NOEMAIL": noEmail, "SUSPENDED": suspended, "STAFF": staffCount}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM users u WHERE `+userWhere, weekAgo, query, digits, f.Kind).Scan(&p.Total)
	if err != nil {
		return UserPage{}, fmt.Errorf("count matching users: %w", err)
	}
	p.Pages = max(1, (p.Total+UsersPerPage-1)/UsersPerPage)
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, nullif(u.company_name, ''), u.phone, u.email::text, u.locale, u.status, u.staff_role,
		       u.created_at, coalesce(u.last_login_at, u.created_at),
		       (SELECT count(*) FROM projects p WHERE p.owner_id = u.id AND p.deleted_at IS NULL),
		       (SELECT count(*) FROM project_members m JOIN projects p ON p.id = m.project_id
		        WHERE m.user_id = u.id AND p.owner_id <> u.id AND p.deleted_at IS NULL),
		       u.suspended_reason,
		       (SELECT r.number FROM support_requests r WHERE r.status = 'open' AND (r.user_id = u.id OR r.phone = u.phone)
		        ORDER BY r.created_at DESC LIMIT 1)
		FROM users u WHERE `+userWhere+`
		ORDER BY coalesce(u.last_login_at, u.created_at) DESC, u.id
		LIMIT $5 OFFSET $6`, weekAgo, query, digits, f.Kind, UsersPerPage, (p.Page-1)*UsersPerPage)
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	if p.Rows, err = pgx.CollectRows(rows, pgx.RowToStructByPos[UserRow]); err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	return p, nil
}

// digitsOf keeps a search's digits (Bangla ones as Latin) so a phone number matches however typed.
func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '০' && r <= '৯':
			b.WriteRune('0' + (r - '০'))
		}
	}
	return b.String()
}
