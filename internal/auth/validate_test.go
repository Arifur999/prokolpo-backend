package auth

import (
	"strings"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

func validRequest() registerRequest {
	return registerRequest{FirstName: " আব্দুল ", LastName: "করিম", Email: "Abdul.Karim@Example.com", Phone: "+880 1712-345678", Password: "karim2026", AcceptTerms: true}
}

func TestRegisterValidateNormalises(t *testing.T) {
	in, err := validRequest().validate("bn")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if in.FirstName != "আব্দুল" || in.Phone != "01712345678" || in.Email == nil || *in.Email != "abdul.karim@example.com" || in.CompanyName != nil {
		t.Errorf("normalised input = %+v", in)
	}
}

func TestRegisterValidateReportsEveryField(t *testing.T) {
	req := registerRequest{FirstName: "আ", CompanyName: "ক", Email: "not-an-email", Phone: "12345", Password: "short"}
	_, err := req.validate("bn")
	e, ok := apperr.As(err)
	if !ok || e.Code != codeInvalidInput {
		t.Fatalf("err = %v, want INVALID_INPUT", err)
	}
	got := map[string]string{}
	for _, f := range e.Fields {
		got[f.Path] = f.Message
	}
	want := map[string]string{
		"companyName": "কোম্পানির নাম কমপক্ষে ২ অক্ষরের হতে হবে",
		"firstName":   "প্রথম নাম কমপক্ষে ২ অক্ষরের হতে হবে",
		"lastName":    "শেষ নাম দিন",
		"email":       "সঠিক ইমেইল দিন",
		"phone":       "সঠিক মোবাইল নম্বর দিন (যেমন 01712345678)",
		"password":    "পাসওয়ার্ড কমপক্ষে ৮ অক্ষরের এবং অক্ষর ও সংখ্যা মিলিয়ে হতে হবে",
		"acceptTerms": "শর্তাবলীতে সম্মতি দিন",
	}
	for path, msg := range want {
		if got[path] != msg {
			t.Errorf("%s: message %q, want %q", path, got[path], msg)
		}
	}
}

func TestRegisterValidateCapsLengths(t *testing.T) {
	req := validRequest()
	req.FirstName = strings.Repeat("ক", 31)
	req.CompanyName = strings.Repeat("ক", 81)
	_, err := req.validate("bn")
	e, _ := apperr.As(err)
	paths := map[string]bool{}
	for _, f := range e.Fields {
		paths[f.Path] = true
	}
	if !paths["firstName"] || !paths["companyName"] || len(paths) != 2 {
		t.Errorf("fields = %+v, want firstName and companyName", e.Fields)
	}
}

func TestRegisterValidateSpeaksEnglish(t *testing.T) {
	req := validRequest()
	req.LastName = ""
	_, err := req.validate("en")
	e, _ := apperr.As(err)
	if len(e.Fields) != 1 || e.Fields[0].Message != "Enter your last name" {
		t.Errorf("fields = %+v, want the English last-name message", e.Fields)
	}
}

func TestParseIdentifier(t *testing.T) {
	if id, ok := parseIdentifier("০১৭১২৩৪৫৬৭৮"); !ok || id.phone != "01712345678" {
		t.Errorf("phone identifier = %+v, %v", id, ok)
	}
	if id, ok := parseIdentifier(" Karim@Example.com "); !ok || id.email != "karim@example.com" {
		t.Errorf("email identifier = %+v, %v", id, ok)
	}
	if _, ok := parseIdentifier("karim"); ok {
		t.Error("parseIdentifier accepted a bare name")
	}
}

func TestMaskEmail(t *testing.T) {
	tests := map[string]string{"tania.rahman@example.com": "ta•••@example.com", "a@x.com": "a•••@x.com"}
	for in, want := range tests {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
