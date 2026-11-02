package books

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// TeamWeek is one daily team's row in the attendance grid.
type TeamWeek struct {
	ContractorID string          `json:"contractorId"`
	Name         string          `json:"name"`
	Work         string          `json:"work"`
	DailyWage    int64           `json:"dailyWage"`
	TeamSize     int16           `json:"teamSize"`
	Due          int64           `json:"due"`
	Days         []AttendanceDay `json:"days"`
}

// weekStart is the Sunday on or before day: the books' weeks run Sunday to Saturday.
func weekStart(day time.Time) time.Time { return day.AddDate(0, 0, -int(day.Weekday())) }

// AttendanceWeek returns every daily team with its days in the week holding day.
func (s *Service) AttendanceWeek(ctx context.Context, userID, projectID string, day time.Time) (string, []TeamWeek, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return "", nil, err
	}
	start := weekStart(day)
	rows, err := s.pool.Query(ctx, contractorSelect+` AND c.pay_type = 'DAILY' ORDER BY c.created_at`, projectID)
	if err != nil {
		return "", nil, fmt.Errorf("list teams: %w", err)
	}
	teams, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Contractor, error) { return scanContractor(row) })
	if err != nil {
		return "", nil, err
	}
	out := make([]TeamWeek, 0, len(teams))
	for i := range teams {
		c := &teams[i]
		days, err := attendanceOf(ctx, s.pool, c.ID, start, start.AddDate(0, 0, 6))
		if err != nil {
			return "", nil, err
		}
		out = append(out, TeamWeek{ContractorID: c.ID, Name: c.Name, Work: c.Work, DailyWage: *c.DailyWage, TeamSize: *c.TeamSize, Due: c.Due, Days: days})
	}
	return start.Format(time.DateOnly), out, nil
}

// AttendanceCell is one edited cell; a headcount of 0 clears the day.
type AttendanceCell struct {
	ContractorID string
	Day          time.Time
	Headcount    int16
	HalfDay      bool
}

type attendanceBody struct {
	Cells []struct {
		ContractorID string `json:"contractorId"`
		Day          string `json:"day"`
		Headcount    int16  `json:"headcount"`
		HalfDay      bool   `json:"halfDay"`
	} `json:"cells"`
}

// maxCells bounds one save: a week of a large crew list.
const maxCells = 7 * 60

func (b attendanceBody) validate(lang string) ([]AttendanceCell, error) {
	f := &fields{lang: lang}
	if len(b.Cells) > maxCells {
		f.add("cells", "headcount")
		return nil, f.err()
	}
	cells := make([]AttendanceCell, 0, len(b.Cells))
	for i, c := range b.Cells {
		path := fmt.Sprintf("cells.%d", i)
		day, err := time.Parse(time.DateOnly, c.Day)
		if err != nil {
			f.add(path+".day", "date")
		}
		if c.Headcount < 0 || c.Headcount > 99 {
			f.add(path+".headcount", "headcount")
		}
		if !isID(c.ContractorID) {
			f.add(path+".contractorId", "party")
		}
		cells = append(cells, AttendanceCell{ContractorID: c.ContractorID, Day: day, Headcount: c.Headcount, HalfDay: c.HalfDay})
	}
	return cells, f.err()
}

// SaveAttendance writes the changed cells. A new day takes the team's current wage; a
// day already written keeps the wage it had, so a raise never rewrites past weeks.
func (s *Service) SaveAttendance(ctx context.Context, userID, projectID string, cells []AttendanceCell) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		for _, c := range cells {
			if c.Headcount == 0 {
				if _, err := tx.Exec(ctx, `DELETE FROM attendance WHERE contractor_id = $1 AND day = $2 AND project_id = $3`, c.ContractorID, c.Day, projectID); err != nil {
					return fmt.Errorf("clear attendance: %w", err)
				}
				continue
			}
			tag, err := tx.Exec(ctx, `
				INSERT INTO attendance (project_id, contractor_id, day, headcount, half_day, rate, updated_by)
				SELECT $1, c.id, $3, $4, $5, c.daily_wage, $6 FROM contractors c
				WHERE c.id = $2 AND c.project_id = $1 AND c.pay_type = 'DAILY'
				ON CONFLICT (contractor_id, day) DO UPDATE
				SET headcount = EXCLUDED.headcount, half_day = EXCLUDED.half_day, updated_by = EXCLUDED.updated_by, updated_at = now()`,
				projectID, c.ContractorID, c.Day, c.Headcount, c.HalfDay, userID)
			if err != nil {
				return fmt.Errorf("save attendance: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return notFound()
			}
		}
		if len(cells) == 0 {
			return nil
		}
		return logActivity(ctx, tx, activity{
			projectID: projectID, actorID: userID, action: "UPDATE", entity: "attendance", entityID: cells[0].ContractorID,
			summary: strconv.Itoa(len(cells)),
		})
	})
}
