// Package phone normalises Bangladeshi mobile numbers to the 11-digit form
// the app stores and shows: 01XXXXXXXXX.
package phone

import (
	"errors"
	"strings"
)

// ErrInvalid reports input that is not a Bangladeshi mobile number.
var ErrInvalid = errors.New("not a Bangladeshi mobile number")

// Normalize accepts the ways people type a number (+880 1712-345678,
// ০১৭১২ ৩৪৫৬৭৮, 8801712345678) and returns 01712345678.
func Normalize(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '০' && r <= '৯':
			b.WriteRune('0' + (r - '০'))
		}
	}
	digits := b.String()
	if strings.HasPrefix(digits, "880") {
		digits = digits[2:]
	}
	if !isMobile(digits) {
		return "", ErrInvalid
	}
	return digits, nil
}

// isMobile checks 01 + an operator digit 3-9 + 8 more digits.
func isMobile(d string) bool {
	if len(d) != 11 || d[0] != '0' || d[1] != '1' || d[2] < '3' || d[2] > '9' {
		return false
	}
	for i := 3; i < len(d); i++ {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
	}
	return true
}
