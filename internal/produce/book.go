package produce

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Batch is one batch with what was spent on it and what it sold. Weight is the kilos of
// produce sold by weight (birds, fish, crops) and WeightSales what they fetched.
type Batch struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	StartedOn   *string `json:"startedOn"`
	EndedOn     *string `json:"endedOn"`
	HeadCount   *int    `json:"headCount"`
	Losses      int     `json:"losses"`
	Note        *string `json:"note"`
	Cost        int64   `json:"cost"`
	Sales       int64   `json:"sales"`
	Weight      float64 `json:"weight"`
	WeightSales int64   `json:"weightSales"`
}

// Sale is one sale with what is still owed on it.
type Sale struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	PhaseID   *string   `json:"phaseId"`
	Product   string    `json:"product"`
	Unit      string    `json:"unit"`
	Quantity  float64   `json:"quantity"`
	Rate      int64     `json:"rate"`
	Amount    int64     `json:"amount"`
	Left      int64     `json:"left"`
	BuyerID   string    `json:"buyerId"`
	Buyer     string    `json:"buyer"`
	CreatedAt time.Time `json:"createdAt"`
	paid      int64
}

// Due is what one buyer owes, since their oldest unpaid sale.
type Due struct {
	BuyerID string `json:"buyerId"`
	Buyer   string `json:"buyer"`
	Amount  int64  `json:"amount"`
	Since   string `json:"since"`
}

// Buyer is someone who has bought from the farm, most recent first.
type Buyer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Book is everything the farm sales page shows. SetupCost is what was spent outside any
// batch (the shed, the pond) that batch profits pay back.
type Book struct {
	FarmType  string  `json:"farmType"`
	Batches   []Batch `json:"batches"`
	SetupCost int64   `json:"setupCost"`
	Sales     []Sale  `json:"sales"`
	Dues      []Due   `json:"dues"`
	Buyers    []Buyer `json:"buyers"`
}

// Book returns the farm's sales page.
func (s *Service) Book(ctx context.Context, userID, projectID string) (Book, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Book{}, err
	}
	return loadBook(ctx, s.pool, projectID)
}

func loadBook(ctx context.Context, q db, projectID string) (Book, error) {
	var b Book
	err := q.QueryRow(ctx, `
		SELECT coalesce(p.farm_type, ''),
		       coalesce((SELECT sum(amount) FROM expenses e WHERE e.project_id = p.id AND e.phase_id IS NULL AND e.deleted_at IS NULL), 0)
		FROM projects p WHERE p.id = $1`, projectID).Scan(&b.FarmType, &b.SetupCost)
	if err != nil {
		return Book{}, fmt.Errorf("load farm: %w", err)
	}
	if b.Batches, err = batchesOf(ctx, q, projectID); err != nil {
		return Book{}, err
	}
	if b.Sales, err = salesOf(ctx, q, projectID); err != nil {
		return Book{}, err
	}
	pool, err := receivedLater(ctx, q, projectID)
	if err != nil {
		return Book{}, err
	}
	b.Dues = settle(b.Sales, pool)
	slices.Reverse(b.Sales)
	if b.Buyers, err = buyersOf(ctx, q, projectID); err != nil {
		return Book{}, err
	}
	return b, nil
}

func batchesOf(ctx context.Context, q db, projectID string) ([]Batch, error) {
	rows, err := q.Query(ctx, `
		SELECT ph.id, ph.name, to_char(ph.started_on, 'YYYY-MM-DD'), to_char(ph.ended_on, 'YYYY-MM-DD'), ph.head_count, ph.losses, ph.note,
		       coalesce((SELECT sum(e.amount) FROM expenses e WHERE e.phase_id = ph.id AND e.deleted_at IS NULL), 0),
		       coalesce(s.amount, 0), coalesce(s.weight, 0), coalesce(s.weight_amount, 0)
		FROM project_phases ph
		LEFT JOIN LATERAL (
			SELECT sum(amount) AS amount,
			       sum(quantity) FILTER (WHERE unit = 'KG' AND product <> 'OTHER')::float8 AS weight,
			       sum(amount) FILTER (WHERE unit = 'KG' AND product <> 'OTHER') AS weight_amount
			FROM produce_sales WHERE phase_id = ph.id
		) s ON true
		WHERE ph.project_id = $1 ORDER BY ph.position, ph.created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list batches: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Batch, error) {
		var b Batch
		err := row.Scan(&b.ID, &b.Name, &b.StartedOn, &b.EndedOn, &b.HeadCount, &b.Losses, &b.Note, &b.Cost, &b.Sales, &b.Weight, &b.WeightSales)
		return b, err
	})
}

// salesOf lists the sales oldest first, each with the money taken on it.
func salesOf(ctx context.Context, q db, projectID string) ([]Sale, error) {
	rows, err := q.Query(ctx, `
		SELECT s.id, to_char(s.date, 'YYYY-MM-DD'), s.phase_id, s.product, s.unit, s.quantity::float8, s.rate, s.amount,
		       s.buyer_id, b.name, s.created_at,
		       coalesce((SELECT sum(r.amount) FROM produce_receipts r WHERE r.sale_id = s.id), 0)
		FROM produce_sales s JOIN produce_buyers b ON b.id = s.buyer_id
		WHERE s.project_id = $1 ORDER BY s.date, s.created_at, s.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list produce sales: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Sale, error) {
		var x Sale
		err := row.Scan(&x.ID, &x.Date, &x.PhaseID, &x.Product, &x.Unit, &x.Quantity, &x.Rate, &x.Amount, &x.BuyerID, &x.Buyer, &x.CreatedAt, &x.paid)
		return x, err
	})
}

// receivedLater is what each buyer paid after their sales, against what they owed.
func receivedLater(ctx context.Context, q db, projectID string) (map[string]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT buyer_id, sum(amount) FROM produce_receipts
		WHERE project_id = $1 AND sale_id IS NULL GROUP BY buyer_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("sum buyer payments: %w", err)
	}
	pool := map[string]int64{}
	var buyer string
	var amount int64
	_, err = pgx.ForEachRow(rows, []any{&buyer, &amount}, func() error {
		pool[buyer] = amount
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sum buyer payments: %w", err)
	}
	return pool, nil
}

func buyersOf(ctx context.Context, q db, projectID string) ([]Buyer, error) {
	rows, err := q.Query(ctx, `
		SELECT b.id, b.name FROM produce_buyers b
		WHERE b.project_id = $1
		ORDER BY (SELECT max(s.created_at) FROM produce_sales s WHERE s.buyer_id = b.id) DESC NULLS LAST, b.name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list buyers: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Buyer])
}

// settle works out what is still owed on each sale (sales oldest first): money taken on
// a sale pays that sale, and what a buyer paid later clears their oldest sales first.
// It returns each buyer's due, the longest-owing first.
func settle(sales []Sale, later map[string]int64) []Due {
	pool := maps.Clone(later)
	dues := []Due{}
	at := map[string]int{}
	for i := range sales {
		x := &sales[i]
		left := max(x.Amount-x.paid, 0)
		take := min(left, pool[x.BuyerID])
		if take > 0 {
			pool[x.BuyerID] -= take
		}
		x.Left = left - take
		if x.Left == 0 {
			continue
		}
		j, ok := at[x.BuyerID]
		if !ok {
			j = len(dues)
			at[x.BuyerID] = j
			dues = append(dues, Due{BuyerID: x.BuyerID, Buyer: x.Buyer, Since: x.Date})
		}
		dues[j].Amount += x.Left
	}
	return dues
}
