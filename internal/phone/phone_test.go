package phone

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"01712345678", "01712345678"},
		{"+880 1712-345678", "01712345678"},
		{"8801712345678", "01712345678"},
		{"০১৭১২ ৩৪৫৬৭৮", "01712345678"},
		{"01312345678", "01312345678"},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, in := range []string{"", "0171234567", "017123456789", "01212345678", "02712345678", "abc"} {
		if _, err := Normalize(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Normalize(%q) err = %v, want ErrInvalid", in, err)
		}
	}
}
