// Package token issues the API's credentials: short-lived signed access tokens
// and random opaque tokens (refresh tokens, reset links) stored only as hashes.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Roles carried in an access token.
const (
	RoleUser  = "user"
	RoleStaff = "staff"
)

// ErrInvalid covers every access token we refuse: bad signature, wrong
// algorithm, expired, or malformed. Callers answer 401 either way.
var ErrInvalid = errors.New("invalid access token")

// Claims is what the API learns from a valid access token.
type Claims struct {
	UserID string
	Role   string
}

type accessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Issuer signs and verifies access tokens with one HMAC secret.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewIssuer returns an Issuer whose tokens live for ttl.
func NewIssuer(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl, now: time.Now}
}

// Issue returns a signed access token for userID and when it expires.
func (is *Issuer) Issue(userID, role string) (string, time.Time, error) {
	now := is.now()
	expires := now.Add(is.ttl)
	claims := accessClaims{
		Role:      role,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(is.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expires, nil
}

// Parse verifies raw and returns its claims. Only HS256 is accepted, so a
// token cannot pick its own algorithm.
func (is *Issuer) Parse(raw string) (Claims, error) {
	var claims accessClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return is.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(is.now),
	)
	if err != nil || claims.Subject == "" {
		return Claims{}, ErrInvalid
	}
	return Claims{UserID: claims.Subject, Role: claims.Role}, nil
}

// NewOpaque returns a random URL-safe token and the hash to store. The raw
// value goes to the client once and is never kept.
func NewOpaque() (raw string, hash []byte) {
	var b [32]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	raw = base64.RawURLEncoding.EncodeToString(b[:])
	return raw, Hash(raw)
}

// Hash is how an opaque token is looked up: its SHA-256. The token already
// carries 256 random bits, so a slow hash adds nothing.
func Hash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
