// Package status reports platform-wide state the frontend must know before
// anything else — today, whether the app is in maintenance — and enforces it.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cacheFor bounds how often every request hits the database for this flag;
// switching maintenance on takes effect within this long.
const cacheFor = 10 * time.Second

// Maintenance is the app_settings "maintenance" value.
type Maintenance struct {
	Enabled bool       `json:"enabled"`
	Until   *time.Time `json:"until"`
}

// Status is what GET /api/v1/status returns.
type Status struct {
	Maintenance Maintenance `json:"maintenance"`
}

// Service reads the status, cached briefly.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time

	mu       sync.Mutex
	cached   Status
	cachedAt time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

// Current returns the status, from cache when it is fresh.
func (s *Service) Current(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cachedAt.IsZero() && s.now().Sub(s.cachedAt) < cacheFor {
		return s.cached, nil
	}
	maintenance, err := s.readMaintenance(ctx)
	if err != nil {
		return Status{}, err
	}
	s.cached, s.cachedAt = Status{Maintenance: maintenance}, s.now()
	return s.cached, nil
}

func (s *Service) readMaintenance(ctx context.Context) (Maintenance, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = 'maintenance'`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Maintenance{}, nil
	}
	if err != nil {
		return Maintenance{}, fmt.Errorf("read maintenance setting: %w", err)
	}
	var m Maintenance
	if err := json.Unmarshal(raw, &m); err != nil {
		return Maintenance{}, fmt.Errorf("decode maintenance setting: %w", err)
	}
	return m, nil
}
