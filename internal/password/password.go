// Package password hashes and checks passwords with argon2id and enforces the
// app's password rule.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Parameters follow the OWASP argon2id baseline (19 MiB, 2 passes, 1 lane).
const (
	memoryKiB = 19 * 1024
	passes    = 2
	lanes     = 1
	saltLen   = 16
	keyLen    = 32
	minLength = 8
	maxLength = 128
)

// ErrWeak reports a password that breaks the rule: 8+ characters with both a letter and a digit.
var ErrWeak = errors.New("password must be at least 8 characters with letters and digits")

// errMalformedHash means a stored hash is not one this package wrote.
var errMalformedHash = errors.New("malformed password hash")

// Validate checks the rule shown on the registration form.
func Validate(pw string) error {
	n := len([]rune(pw))
	if n < minLength || n > maxLength {
		return ErrWeak
	}
	var hasLetter, hasDigit bool
	for _, r := range pw {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeak
	}
	return nil
}

// Hash returns an encoded argon2id hash that carries its own parameters and salt.
func Hash(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, passes, memoryKiB, lanes, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memoryKiB, passes, lanes, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify reports whether pw matches encoded, in constant time.
func Verify(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errMalformedHash
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) != keyLen {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(pw), salt, time, memory, threads, keyLen)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
