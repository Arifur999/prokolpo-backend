package password

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		pw   string
		weak bool
	}{
		{"karim2026", false},
		{"পাসওয়ার্ড12", false},
		{"short1", true},
		{"lettersonly", true},
		{"12345678", true},
	}
	for _, tt := range tests {
		err := Validate(tt.pw)
		if gotWeak := errors.Is(err, ErrWeak); gotWeak != tt.weak {
			t.Errorf("Validate(%q) = %v, want weak=%v", tt.pw, err, tt.weak)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	encoded, err := Hash("karim2026")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if ok, verr := Verify("karim2026", encoded); verr != nil || !ok {
		t.Errorf("Verify(right password) = %v, %v; want true", ok, verr)
	}
	if ok, verr := Verify("karim2027", encoded); verr != nil || ok {
		t.Errorf("Verify(wrong password) = %v, %v; want false", ok, verr)
	}
	other, err := Hash("karim2026")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if other == encoded {
		t.Error("two hashes of one password are equal; salt is not random")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := Verify("x", "$bcrypt$whatever"); err == nil {
		t.Error("Verify accepted a hash it did not write")
	}
}
