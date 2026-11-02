package staff

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCodesMatchTheRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	// RFC 6238 appendix B, SHA-1, last six digits.
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for unix, want := range cases {
		if got := codeAt(secret, stepOf(time.Unix(unix, 0))); got != want {
			t.Errorf("code at %d = %s, want %s", unix, got, want)
		}
	}
}

func TestACodeCountsOneStepEitherSide(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0)
	before := codeAt(secret, stepOf(now)-1)
	if matchStep(secret, before, now) != stepOf(now)-1 {
		t.Error("the previous step's code was refused")
	}
	if matchStep(secret, codeAt(secret, stepOf(now)-2), now) != 0 {
		t.Error("a code two steps old was accepted")
	}
	if matchStep(secret, "12345", now) != 0 {
		t.Error("a short code was accepted")
	}
}

func TestBackupCodesReadTheSameWithOrWithoutTheDash(t *testing.T) {
	code, err := newBackupCode()
	if err != nil || len(code) != 9 || code[4] != '-' || strings.ContainsAny(code, "01OI") {
		t.Fatalf("code = %q, %v", code, err)
	}
	if !bytes.Equal(hashBackup(code), hashBackup(strings.ToLower(strings.ReplaceAll(code, "-", "")))) {
		t.Error("the dashless lower-case form hashed differently")
	}
	if digitsOnly("১২৩ ৪৫৬") != "123456" {
		t.Error("Bangla digits were not read")
	}
}

func TestSecretsSurviveSealing(t *testing.T) {
	s, err := newSealer([]byte("a-test-secret-that-is-long-enough-32"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.seal([]byte("totp-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := s.open(sealed); err != nil || string(plain) != "totp-secret" {
		t.Fatalf("open = %q, %v", plain, err)
	}
	if !strings.HasPrefix(otpauthURI([]byte("12345678901234567890"), "01700112233"), "otpauth://totp/") {
		t.Error("the app link has the wrong scheme")
	}
}
