package support

import (
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

func TestValidateNormalises(t *testing.T) {
	req, err := requestBody{Name: " তানিয়া রহমান ", Phone: "01755-889900", Email: "Tania@Example.com", Topic: "login", Source: "forgot", Message: "পাসওয়ার্ড মনে নেই, ইমেইলও দেওয়া নেই"}.validate("bn")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if req.Name != "তানিয়া রহমান" || req.Phone != "01755889900" || *req.Email != "tania@example.com" || req.Source != "forgot" {
		t.Errorf("request = %+v", req)
	}
}

func TestValidateReportsEveryField(t *testing.T) {
	_, err := requestBody{Name: "ত", Phone: "123", Email: "nope", Topic: "billing", Message: "help"}.validate("bn")
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	got := map[string]bool{}
	for _, f := range e.Fields {
		got[f.Path] = true
	}
	for _, path := range []string{"name", "phone", "email", "topic", "message"} {
		if !got[path] {
			t.Errorf("no error for %s", path)
		}
	}
}

func TestValidateDefaultsUnknownSource(t *testing.T) {
	req, err := requestBody{Name: "করিম", Phone: "01712345678", Topic: "other", Source: "evil", Message: "দশ অক্ষরের বেশি লেখা"}.validate("en")
	if err != nil || req.Source != "public" {
		t.Errorf("source = %q, err %v; want public", req.Source, err)
	}
}
