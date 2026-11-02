package sales

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

var unitTypes = []string{"FLAT", "SHOP", "PARKING", "PLOT", "OTHER"}

const maxAmount = 1_000_000_000_000

// Unit is something the project sells, with its sale if it has one.
type Unit struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Floor    int    `json:"floor"`
	Position int    `json:"position"`
	Size     *int   `json:"size"`
	Price    int64  `json:"price"`
	Sale     *Sale  `json:"sale"`
}

// Units lists the project's units by floor, top floor first, each with its sale.
func (s *Service) Units(ctx context.Context, userID, projectID string) ([]Unit, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, type, floor, position, size, price FROM units
		WHERE project_id = $1 ORDER BY floor DESC, position, created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	units, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Unit, error) {
		var u Unit
		scanErr := row.Scan(&u.ID, &u.Name, &u.Type, &u.Floor, &u.Position, &u.Size, &u.Price)
		return u, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	sold, err := loadSales(ctx, s.pool, projectID, saleFilter{}, s.today())
	if err != nil {
		return nil, err
	}
	byUnit := make(map[string]*Sale, len(sold))
	for i := range sold {
		byUnit[sold[i].UnitID] = &sold[i]
	}
	for i := range units {
		units[i].Sale = byUnit[units[i].ID]
	}
	return units, nil
}

// UnitInput is a validated unit.
type UnitInput struct {
	Name  string
	Type  string
	Floor int
	Size  *int
	Price int64
}

type unitBody struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Floor int    `json:"floor"`
	Size  int    `json:"size"`
	Price int64  `json:"price"`
}

func (b unitBody) validate(f *fields, prefix string) UnitInput {
	u := UnitInput{Name: strings.TrimSpace(b.Name), Type: b.Type, Floor: b.Floor, Price: b.Price}
	if n := len([]rune(u.Name)); n == 0 || n > 40 {
		f.add(prefix+"name", "unitName")
	}
	if !slices.Contains(unitTypes, b.Type) {
		f.add(prefix+"type", "unitType")
	}
	if b.Floor < 0 || b.Floor > 60 {
		f.add(prefix+"floor", "floor")
	}
	if b.Size < 0 || b.Size > 1_000_000 {
		f.add(prefix+"size", "size")
	} else if b.Size > 0 {
		u.Size = &b.Size
	}
	if b.Price <= 0 || b.Price > maxAmount {
		f.add(prefix+"price", "price")
	}
	return u
}

type unitsBody struct {
	Units []unitBody `json:"units"`
}

func (b unitsBody) validate(lang string) ([]UnitInput, error) {
	f := &fields{lang: lang}
	if len(b.Units) == 0 || len(b.Units) > 200 {
		f.add("units", "unitName")
	}
	out := make([]UnitInput, len(b.Units))
	for i, u := range b.Units {
		out[i] = u.validate(f, fmt.Sprintf("units.%d.", i))
	}
	return out, f.err()
}

// AddUnits adds one unit, or a whole block of flats at once. Names already used are
// skipped in a block; a single unit with a used name is refused.
func (s *Service) AddUnits(ctx context.Context, userID, projectID string, in []UnitInput) (int, error) {
	added := 0
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		for _, u := range in {
			var id string
			err := tx.QueryRow(ctx, `
				INSERT INTO units (project_id, name, type, floor, position, size, price, created_by)
				VALUES ($1, $2, $3, $4, (SELECT coalesce(max(position), 0) + 1 FROM units WHERE project_id = $1 AND floor = $4), $5, $6, $7)
				ON CONFLICT (project_id, (lower(name))) DO NOTHING RETURNING id`,
				projectID, u.Name, u.Type, u.Floor, u.Size, u.Price, userID).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				if len(in) == 1 {
					return messages.Fail(apperr.KindConflict, codeUnitTaken)
				}
				continue
			}
			if err != nil {
				return fmt.Errorf("insert unit: %w", err)
			}
			added++
		}
		summary := in[0].Name
		if added > 1 {
			summary = strconv.Itoa(added)
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "unit", EntityID: projectID, Summary: summary,
			Details: map[string]string{"count": strconv.Itoa(added)},
		})
	})
	return added, err
}

// UpdateUnit changes a unit's details; a sold unit keeps its sale.
func (s *Service) UpdateUnit(ctx context.Context, userID, projectID, unitID string, u UnitInput) error {
	if !isID(unitID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE units SET name = $3, type = $4, floor = $5, size = $6, price = $7 WHERE id = $1 AND project_id = $2`,
			unitID, projectID, u.Name, u.Type, u.Floor, u.Size, u.Price)
		if isUniqueViolation(err) {
			return messages.Fail(apperr.KindConflict, codeUnitTaken)
		}
		if err != nil {
			return fmt.Errorf("update unit: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return notFound()
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "UPDATE", Entity: "unit", EntityID: unitID, Summary: u.Name})
	})
}

// DeleteUnit removes a unit that was never sold.
func (s *Service) DeleteUnit(ctx context.Context, userID, projectID, unitID string) error {
	if !isID(unitID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		var sold bool
		err := tx.QueryRow(ctx, `SELECT name, EXISTS (SELECT 1 FROM sales WHERE unit_id = u.id) FROM units u WHERE id = $1 AND project_id = $2`,
			unitID, projectID).Scan(&name, &sold)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("load unit: %w", err)
		}
		if sold {
			return messages.Fail(apperr.KindConflict, codeUnitHasSale)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM units WHERE id = $1`, unitID); err != nil {
			return fmt.Errorf("delete unit: %w", err)
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "DELETE", Entity: "unit", EntityID: unitID, Summary: name})
	})
}
