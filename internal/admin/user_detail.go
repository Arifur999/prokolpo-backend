package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UserDetail is what staff may see about one account (board AdminUserDetail) — never the
// money inside its projects.
type UserDetail struct {
	ID              string        `json:"id"`
	FirstName       string        `json:"firstName"`
	LastName        string        `json:"lastName"`
	Company         *string       `json:"company"`
	Phone           string        `json:"phone"`
	Email           *string       `json:"email"`
	Locale          string        `json:"locale"`
	Status          string        `json:"status"`
	StaffRole       *string       `json:"staffRole"`
	CreatedAt       time.Time     `json:"createdAt"`
	LastActive      *time.Time    `json:"lastActive"`
	PasswordChanged *time.Time    `json:"passwordChanged"`
	InvitedBy       *string       `json:"invitedBy"`
	InvitedTo       *string       `json:"invitedTo"`
	SuspendedAt     *time.Time    `json:"suspendedAt"`
	SuspendedBy     *string       `json:"suspendedBy"`
	SuspendedReason *string       `json:"suspendedReason"`
	Projects        []UserProject `json:"projects"`
	Devices         []Device      `json:"devices"`
	History         []Event       `json:"history"`
	Notes           []Note        `json:"notes"`
	Ticket          *Ticket       `json:"ticket"`
}

// UserProject is a project the account belongs to, with its owner and how busy it is.
type UserProject struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Ownership string     `json:"ownership"`
	Owner     string     `json:"owner"`
	OwnerID   string     `json:"ownerId"`
	Members   int        `json:"members"`
	Role      string     `json:"role"`
	LastWork  *time.Time `json:"lastWork"`
}

// Device is a signed-in browser: one refresh-token family still alive.
type Device struct {
	UserAgent *string   `json:"userAgent"`
	IP        *string   `json:"ip"`
	LastUsed  time.Time `json:"lastUsed"`
}

// Event is one line of the security history: a sign-in, a wrong password, a password change,
// a support request, an admin action, or the account being opened.
type Event struct {
	Kind    string            `json:"kind"`
	Text    string            `json:"text"`
	Details map[string]string `json:"details"`
	At      time.Time         `json:"at"`
}

// Note is a staff member's private note on the account.
type Note struct {
	By   string    `json:"by"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

// Ticket is the account's open support request, if it has one.
type Ticket struct {
	ID     string    `json:"id"`
	Number int       `json:"number"`
	Topic  string    `json:"topic"`
	At     time.Time `json:"at"`
}

// User returns one account for the panel.
func (s *Service) User(ctx context.Context, userID string) (UserDetail, error) {
	if !isID(userID) {
		return UserDetail{}, notFound()
	}
	var d UserDetail
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.first_name, u.last_name, nullif(u.company_name, ''), u.phone, u.email::text, u.locale, u.status, u.staff_role,
		       u.created_at, u.last_login_at, u.password_changed_at, u.suspended_at, u.suspended_reason,
		       (SELECT s.first_name || ' ' || s.last_name FROM users s WHERE s.id = u.suspended_by),
		       i.inviter, i.project
		FROM users u
		LEFT JOIN LATERAL (
			SELECT w.first_name || ' ' || w.last_name AS inviter, p.name AS project
			FROM project_invites v JOIN users w ON w.id = v.invited_by JOIN projects p ON p.id = v.project_id
			WHERE v.phone = u.phone AND v.accepted_at IS NOT NULL ORDER BY v.accepted_at LIMIT 1
		) i ON true
		WHERE u.id = $1`, userID).Scan(
		&d.ID, &d.FirstName, &d.LastName, &d.Company, &d.Phone, &d.Email, &d.Locale, &d.Status, &d.StaffRole,
		&d.CreatedAt, &d.LastActive, &d.PasswordChanged, &d.SuspendedAt, &d.SuspendedReason, &d.SuspendedBy, &d.InvitedBy, &d.InvitedTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, notFound()
	}
	if err != nil {
		return UserDetail{}, fmt.Errorf("load user: %w", err)
	}
	steps := []func(context.Context, *UserDetail) error{s.userProjects, s.userDevices, s.userHistory, s.userNotes, s.userTicket}
	for _, step := range steps {
		if err := step(ctx, &d); err != nil {
			return UserDetail{}, err
		}
	}
	return d, nil
}

func (s *Service) userProjects(ctx context.Context, d *UserDetail) error {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.kind, p.ownership, o.first_name || ' ' || o.last_name, o.id,
		       (SELECT count(*) FROM project_members x WHERE x.project_id = p.id), m.role,
		       (SELECT max(a.created_at) FROM activity_log a WHERE a.project_id = p.id)
		FROM project_members m JOIN projects p ON p.id = m.project_id JOIN users o ON o.id = p.owner_id
		WHERE m.user_id = $1 AND p.deleted_at IS NULL ORDER BY m.created_at`, d.ID)
	if err != nil {
		return fmt.Errorf("user projects: %w", err)
	}
	d.Projects, err = pgx.CollectRows(rows, pgx.RowToStructByPos[UserProject])
	return err
}

func (s *Service) userDevices(ctx context.Context, d *UserDetail) error {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (family_id) user_agent, host(ip), created_at
		FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > now()
		ORDER BY family_id, created_at DESC`, d.ID)
	if err != nil {
		return fmt.Errorf("user devices: %w", err)
	}
	d.Devices, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Device])
	return err
}

// userHistory merges the account's sign-ins, password changes, support requests and what
// staff did to it, newest first.
func (s *Service) userHistory(ctx context.Context, d *UserDetail) error {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, text, details, at FROM (
			SELECT CASE WHEN ok THEN 'login' ELSE 'fail' END AS kind, '' AS text,
			       jsonb_build_object('userAgent', coalesce(user_agent, ''), 'ip', coalesce(ip, '')) AS details, created_at AS at
			FROM login_events WHERE user_id = $1
			UNION ALL
			SELECT 'support', '', jsonb_build_object('number', number::text, 'topic', topic, 'message', left(message, 160), 'source', source), created_at
			FROM support_requests WHERE user_id = $1 OR phone = $2
			UNION ALL
			SELECT 'admin', a.action, a.details || jsonb_build_object('actor', coalesce(w.first_name || ' ' || w.last_name, '')), a.created_at
			FROM admin_audit a LEFT JOIN users w ON w.id = a.actor_id
			WHERE a.target_type = 'user' AND a.target_id = $1::text AND a.action NOT LIKE 'login%' AND a.action <> '2fa.enable'
			UNION ALL
			SELECT 'password', '', '{}'::jsonb, password_changed_at FROM users WHERE id = $1 AND password_changed_at IS NOT NULL
			UNION ALL
			SELECT 'join', '', '{}'::jsonb, created_at FROM users WHERE id = $1
		) h ORDER BY at DESC LIMIT 30`, d.ID, d.Phone)
	if err != nil {
		return fmt.Errorf("user history: %w", err)
	}
	d.History, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
	return err
}

func (s *Service) userNotes(ctx context.Context, d *UserDetail) error {
	rows, err := s.pool.Query(ctx, `
		SELECT coalesce(w.first_name || ' ' || w.last_name, ''), n.body, n.created_at
		FROM staff_notes n LEFT JOIN users w ON w.id = n.author_id WHERE n.user_id = $1 ORDER BY n.created_at DESC`, d.ID)
	if err != nil {
		return fmt.Errorf("user notes: %w", err)
	}
	d.Notes, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Note])
	return err
}

func (s *Service) userTicket(ctx context.Context, d *UserDetail) error {
	var t Ticket
	err := s.pool.QueryRow(ctx, `
		SELECT id, number, topic, created_at FROM support_requests
		WHERE status = 'open' AND (user_id = $1 OR phone = $2) ORDER BY created_at DESC LIMIT 1`, d.ID, d.Phone).Scan(&t.ID, &t.Number, &t.Topic, &t.At)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("user ticket: %w", err)
	}
	d.Ticket = &t
	return nil
}
