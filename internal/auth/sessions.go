package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

const (
	rememberedSessionTTL = 30 * 24 * time.Hour
	shortSessionTTL      = 24 * time.Hour
	// Two tabs can refresh with the same token at once; a reuse this soon after
	// rotation is that race, not theft, so it fails without revoking the family.
	reuseGrace = 10 * time.Second
)

func newFamily() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// issueSession signs an access token and stores a new refresh token in family.
func (s *Service) issueSession(ctx context.Context, q db, user User, family string, remember bool, meta ClientMeta) (Session, error) {
	access, accessExpires, err := s.issuer.Issue(user.ID, user.Role())
	if err != nil {
		return Session{}, err
	}
	ttl := shortSessionTTL
	if remember {
		ttl = rememberedSessionTTL
	}
	raw, hash := token.NewOpaque()
	refreshExpires := time.Now().Add(ttl)
	if err := insertRefresh(ctx, q, newRefresh{
		UserID: user.ID, FamilyID: family, Hash: hash, Remember: remember,
		UserAgent: meta.UserAgent, IP: meta.IP, ExpiresAt: refreshExpires,
	}); err != nil {
		return Session{}, err
	}
	return Session{
		User: user.view(), AccessToken: access, AccessTokenExpiresAt: accessExpires,
		RefreshToken: raw, RefreshTokenExpiresAt: refreshExpires,
	}, nil
}

// Refresh trades a refresh token for a new pair. Each token works once; a
// token presented again after its grace period means it leaked, so every
// session descended from the same login is revoked.
func (s *Service) Refresh(ctx context.Context, raw string, meta ClientMeta) (Session, error) {
	var session Session
	var revokedReason error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		row, err := refreshForUpdate(ctx, tx, token.Hash(raw))
		if errors.Is(err, errNotFound) {
			revokedReason = fail(apperr.KindUnauthorized, codeSessionInvalid)
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case row.RevokedAt != nil, time.Now().After(row.ExpiresAt):
			revokedReason = fail(apperr.KindUnauthorized, codeSessionInvalid)
			return nil
		case row.RotatedAt != nil:
			revokedReason = fail(apperr.KindUnauthorized, codeSessionInvalid)
			if time.Since(*row.RotatedAt) <= reuseGrace {
				return nil
			}
			s.log.WarnContext(ctx, "refresh token reused; revoking its family", "user_id", row.UserID)
			return revokeFamily(ctx, tx, row.FamilyID)
		}

		user, err := userByID(ctx, tx, row.UserID)
		if err != nil {
			return err
		}
		if user.IsSuspended() {
			revokedReason = fail(apperr.KindForbidden, codeAccountSuspended)
			return revokeFamily(ctx, tx, row.FamilyID)
		}
		if rotErr := markRotated(ctx, tx, row.ID); rotErr != nil {
			return rotErr
		}
		session, err = s.issueSession(ctx, tx, user, row.FamilyID, row.Remember, meta)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("refresh: %w", err)
	}
	if revokedReason != nil {
		return Session{}, revokedReason
	}
	return session, nil
}

// Logout ends the session the refresh token belongs to. An unknown token is
// already logged out, so it is not an error.
func (s *Service) Logout(ctx context.Context, raw string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		row, err := refreshForUpdate(ctx, tx, token.Hash(raw))
		if errors.Is(err, errNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return revokeFamily(ctx, tx, row.FamilyID)
	})
}
