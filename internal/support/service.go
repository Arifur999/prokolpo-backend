package support

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/ratelimit"
)

const codeTooMany = "TOO_MANY_REQUESTS"

// Three requests an hour from one number is plenty for a real problem and
// keeps the inbox usable if someone scripts the public form.
const (
	maxRequestsPerPhone = 3
	requestWindow       = time.Hour
)

// Receipt is what the sender is told: the number to quote on WhatsApp.
type Receipt struct {
	Reference string `json:"reference"`
	Phone     string `json:"phone"`
}

// Service stores support requests.
type Service struct {
	pool    *pgxpool.Pool
	perHour *ratelimit.Window
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, perHour: ratelimit.New(maxRequestsPerPhone, requestWindow)}
}

// Create stores a request and returns its reference (S-1045).
func (s *Service) Create(ctx context.Context, req Request) (Receipt, error) {
	if !s.perHour.Allow(req.Phone) {
		return Receipt{}, messages.Fail(apperr.KindTooManyRequests, codeTooMany)
	}
	var number int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO support_requests (user_id, name, phone, email, topic, source, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING number`,
		req.UserID, req.Name, req.Phone, req.Email, req.Topic, req.Source, req.Message).Scan(&number)
	if err != nil {
		return Receipt{}, fmt.Errorf("insert support request: %w", err)
	}
	return Receipt{Reference: "S-" + strconv.Itoa(number), Phone: req.Phone}, nil
}
