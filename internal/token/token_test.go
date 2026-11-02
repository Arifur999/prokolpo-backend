package token

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte("test-secret-that-is-long-enough-32b")

func TestIssueAndParse(t *testing.T) {
	is := NewIssuer(secret, 15*time.Minute)
	raw, expires, err := is.Issue("user-1", RoleUser)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if d := time.Until(expires); d < 14*time.Minute || d > 15*time.Minute {
		t.Errorf("expires in %s, want about 15m", d)
	}
	claims, err := is.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != "user-1" || claims.Role != RoleUser {
		t.Errorf("claims = %+v", claims)
	}
}

func TestParseRejects(t *testing.T) {
	is := NewIssuer(secret, 15*time.Minute)
	good, _, err := is.Issue("user-1", RoleUser)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	expired := NewIssuer(secret, time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Minute) }
	old, _, err := expired.Issue("user-1", RoleUser)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	otherKey, _, err := NewIssuer([]byte("another-secret-also-32-bytes-long!"), time.Minute).Issue("user-1", RoleUser)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}

	for name, raw := range map[string]string{"expired": old, "other key": otherKey, "alg none": none, "garbage": "abc", "tampered": good + "x"} {
		if _, err := is.Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Parse err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestNewOpaque(t *testing.T) {
	a, ha := NewOpaque()
	b, _ := NewOpaque()
	if a == b {
		t.Error("two opaque tokens are equal")
	}
	if len(a) < 40 {
		t.Errorf("token %q is too short", a)
	}
	if !bytes.Equal(Hash(a), ha) {
		t.Error("Hash(raw) does not match the returned hash")
	}
}
