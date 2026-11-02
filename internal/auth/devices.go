package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// Device is one login that is still open ("যেখানে লগইন করা আছে"): its newest refresh token.
type Device struct {
	ID        string    `json:"id"`
	UserAgent *string   `json:"userAgent"`
	IP        *string   `json:"ip"`
	LastSeen  time.Time `json:"lastSeen"`
	Current   bool      `json:"current"`
}

// Devices lists the account's open logins, most recently used first; current marks the one
// the request came from.
func (s *Service) Devices(ctx context.Context, userID, current string) ([]Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT family_id::text, user_agent, host(ip), created_at FROM (
			SELECT DISTINCT ON (family_id) family_id, user_agent, ip, created_at
			FROM refresh_tokens
			WHERE user_id = $1 AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > now()
			ORDER BY family_id, created_at DESC
		) d ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		var d Device
		if err = rows.Scan(&d.ID, &d.UserAgent, &d.IP, &d.LastSeen); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		d.Current = d.ID == current
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// EndDevice signs out one of the account's other logins. The current one logs out instead.
func (s *Service) EndDevice(ctx context.Context, userID, current, deviceID string) error {
	if deviceID == current {
		return messages.Fail(apperr.KindInvalid, codeInvalidInput)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND family_id::text = $2 AND revoked_at IS NULL`, userID, deviceID)
	if err != nil {
		return fmt.Errorf("end device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return messages.Fail(apperr.KindNotFound, codeDeviceGone)
	}
	return nil
}

// EndOtherDevices signs out every login but the current one and says how many it ended.
func (s *Service) EndOtherDevices(ctx context.Context, userID, current string) (int, error) {
	var ended int
	err := s.pool.QueryRow(ctx, `
		WITH ended AS (
			UPDATE refresh_tokens SET revoked_at = now()
			WHERE user_id = $1 AND family_id::text <> $2 AND revoked_at IS NULL
			RETURNING family_id)
		SELECT count(DISTINCT family_id) FROM ended`, userID, current).Scan(&ended)
	if err != nil {
		return 0, fmt.Errorf("end other devices: %w", err)
	}
	return ended, nil
}
