package staff

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined over HMAC-SHA1; authenticator apps expect it
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	stepSeconds = 30
	codeDigits  = 6
	// A code from the step before or after still counts, for a phone clock a little off.
	skew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newSecret is 20 random bytes, as RFC 4226 recommends for HMAC-SHA1.
func newSecret() ([]byte, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate totp secret: %w", err)
	}
	return secret, nil
}

// codeAt is the 6-digit code for one 30-second step (RFC 6238 over RFC 4226).
func codeAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps are positive Unix times / 30
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

func stepOf(t time.Time) int64 { return t.Unix() / stepSeconds }

// matchStep returns the step a code belongs to near now, or 0 when it matches none.
func matchStep(secret []byte, code string, now time.Time) int64 {
	if len(code) != codeDigits {
		return 0
	}
	current := stepOf(now)
	for d := -skew; d <= skew; d++ {
		step := current + int64(d)
		if subtle.ConstantTimeCompare([]byte(codeAt(secret, step)), []byte(code)) == 1 {
			return step
		}
	}
	return 0
}

// groupedKey is the secret in groups of four, as people type it into an app.
func groupedKey(secret []byte) string {
	raw := b32.EncodeToString(secret)
	var parts []string
	for i := 0; i < len(raw); i += 4 {
		parts = append(parts, raw[i:min(i+4, len(raw))])
	}
	return strings.Join(parts, " ")
}

// otpauthURI is what the QR code holds for the authenticator app.
func otpauthURI(secret []byte, account string) string {
	const issuer = "প্রকল্প অ্যাডমিন"
	q := url.Values{}
	q.Set("secret", b32.EncodeToString(secret))
	q.Set("issuer", issuer)
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}
