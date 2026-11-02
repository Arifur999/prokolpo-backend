package partners

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/members"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// What the new-partner dialog says about a number.
const (
	LookupPartner = "PARTNER"
	LookupMember  = "MEMBER"
	LookupAccount = "ACCOUNT"
	LookupNone    = "NONE"
)

// Lookup is who a number belongs to: a partner already, a team member, an account, or nobody.
type Lookup struct {
	Status string  `json:"status"`
	Name   *string `json:"name"`
	Role   *string `json:"role"`
}

// Lookup tells the owner who a number belongs to before they make it a partner.
func (s *Service) Lookup(ctx context.Context, userID, projectID, rawPhone, lang string) (Lookup, error) {
	p, err := phone.Normalize(rawPhone)
	if err != nil {
		return Lookup{}, invalid(lang, "phone", "phone")
	}
	if _, ownerErr := s.access.Owner(ctx, userID, projectID); ownerErr != nil {
		return Lookup{}, ownerErr
	}
	return lookup(ctx, s.pool, projectID, p)
}

func lookup(ctx context.Context, q db, projectID, p string) (Lookup, error) {
	var l Lookup
	err := q.QueryRow(ctx, `
		SELECT CASE
		         WHEN EXISTS (SELECT 1 FROM partners pt WHERE pt.project_id = $1 AND pt.phone = $2) THEN 'PARTNER'
		         WHEN m.role IS NOT NULL THEN 'MEMBER'
		         WHEN u.id IS NOT NULL THEN 'ACCOUNT'
		         ELSE 'NONE' END,
		       coalesce((SELECT pt.name FROM partners pt WHERE pt.project_id = $1 AND pt.phone = $2), u.first_name || ' ' || u.last_name),
		       m.role
		FROM (SELECT 1) one
		LEFT JOIN users u ON u.phone = $2
		LEFT JOIN project_members m ON m.project_id = $1 AND m.user_id = u.id`, projectID, p).Scan(&l.Status, &l.Name, &l.Role)
	if err != nil {
		return Lookup{}, fmt.Errorf("look up phone: %w", err)
	}
	return l, nil
}

type addBody struct {
	Phone     string             `json:"phone"`
	Name      string             `json:"name"`
	Share     float64            `json:"share"`
	Committed int64              `json:"committed"`
	JoinedOn  string             `json:"joinedOn"`
	Shares    map[string]float64 `json:"shares"`
	Viewer    bool               `json:"viewer"`
}

// NewPartner is a validated new partner with everyone's shares after they join.
type NewPartner struct {
	Phone     string
	Name      string
	Share     int64 // tenths of a percent
	Committed int64
	JoinedOn  time.Time
	Shares    map[string]int64
	Viewer    bool
}

func (b addBody) validate(lang string, today time.Time) (NewPartner, error) {
	var fields []string
	p, err := phone.Normalize(b.Phone)
	if err != nil {
		fields = append(fields, "phone", "phone")
	}
	name := strings.TrimSpace(b.Name)
	if n := len([]rune(name)); n < 2 || n > 60 {
		fields = append(fields, "name", "name")
	}
	share := toTenths(b.Share)
	if share <= 0 || share > 1000 {
		fields = append(fields, "share", "share")
	}
	if b.Committed < 0 || b.Committed > maxAmount {
		fields = append(fields, "committed", "committed")
	}
	joined, err := time.Parse(time.DateOnly, b.JoinedOn)
	if err != nil || joined.After(today) {
		fields = append(fields, "joinedOn", "date")
	}
	shares := make(map[string]int64, len(b.Shares))
	total := share
	for id, s := range b.Shares {
		t := toTenths(s)
		if t <= 0 {
			fields = append(fields, "shares", "shares")
		}
		shares[id] = t
		total += t
	}
	if total != 1000 {
		fields = append(fields, "shares", "shares")
	}
	if len(fields) > 0 {
		return NewPartner{}, invalid(lang, fields...)
	}
	return NewPartner{Phone: p, Name: name, Share: share, Committed: b.Committed, JoinedOn: joined, Shares: shares, Viewer: b.Viewer}, nil
}

// AddPartner brings in a partner, setting everyone's new share in the same step, and puts
// a number not yet on the team there as a viewer when asked.
func (s *Service) AddPartner(ctx context.Context, userID, projectID, lang string, in NewPartner) (Partner, error) {
	var id string
	err := s.joint(ctx, func() error { _, err := s.access.Owner(ctx, userID, projectID); return err }, projectID, func(tx pgx.Tx) error {
		current, err := partnersOf(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(current) != len(in.Shares) {
			return invalid(lang, "shares", "shares")
		}
		changes := make([]activity.Change, 0, len(current))
		for i := range current {
			p := &current[i]
			next, ok := in.Shares[p.ID]
			if !ok {
				return invalid(lang, "shares", "shares")
			}
			if next == toTenths(p.Share) {
				continue
			}
			if _, updateErr := tx.Exec(ctx, `UPDATE partners SET share = $2 WHERE id = $1`, p.ID, float64(next)/10); updateErr != nil {
				return fmt.Errorf("update share: %w", updateErr)
			}
			changes = append(changes, activity.Change{Field: p.Name, Before: percent(toTenths(p.Share)), After: percent(next)})
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO partners (project_id, name, phone, share, committed, joined_on, color, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, (SELECT coalesce(max(color) + 1, 0) FROM partners WHERE project_id = $1), $7)
			ON CONFLICT (project_id, phone) DO NOTHING RETURNING id`,
			projectID, in.Name, in.Phone, float64(in.Share)/10, in.Committed, in.JoinedOn, userID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return messages.Fail(apperr.KindConflict, codePartnerExists)
		}
		if err != nil {
			return fmt.Errorf("insert partner: %w", err)
		}
		if in.Viewer {
			if err := seatViewer(ctx, tx, userID, projectID, in.Phone); err != nil {
				return err
			}
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "partner", EntityID: id, Summary: in.Name,
			Changes: changes, Details: map[string]string{"share": percent(in.Share), "phone": in.Phone},
		})
	})
	if err != nil {
		return Partner{}, err
	}
	return s.partner(ctx, projectID, id)
}

// seatViewer puts a partner's number on the team as a viewer unless it is there or invited already.
func seatViewer(ctx context.Context, tx pgx.Tx, actorID, projectID, p string) error {
	_, err := members.Join(ctx, tx, actorID, projectID, members.NewInvite{Phone: p, Role: project.RoleViewer})
	if e, ok := apperr.As(err); ok && e.Kind == apperr.KindConflict {
		return nil
	}
	return err
}

func (s *Service) partner(ctx context.Context, projectID, id string) (Partner, error) {
	rows, err := s.pool.Query(ctx, partnerSelect+` WHERE p.project_id = $1 AND p.id = $2`, projectID, id)
	if err != nil {
		return Partner{}, fmt.Errorf("load partner: %w", err)
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Partner])
	if errors.Is(err, pgx.ErrNoRows) {
		return Partner{}, notFound()
	}
	return p, err
}

// percent writes tenths of a percent as "12.5%" or "40%".
func percent(tenths int64) string {
	if tenths%10 == 0 {
		return fmt.Sprintf("%d%%", tenths/10)
	}
	return fmt.Sprintf("%d.%d%%", tenths/10, tenths%10)
}
