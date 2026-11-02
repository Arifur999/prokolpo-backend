package msg

import (
	"errors"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

var catalog = Catalog{"PHONE_TAKEN": {BN: "এই নম্বর দিয়ে আগেই অ্যাকাউন্ট খোলা হয়েছে", EN: "An account already uses this mobile number"}}

func TestFailAndLocalize(t *testing.T) {
	err := catalog.Fail(apperr.KindConflict, "PHONE_TAKEN")
	if e, _ := apperr.As(err); e.Message != catalog["PHONE_TAKEN"].EN || e.Code != "PHONE_TAKEN" {
		t.Fatalf("Fail = %+v", e)
	}
	if e, _ := apperr.As(catalog.Localize(err, httpx.LangBangla)); e.Message != catalog["PHONE_TAKEN"].BN {
		t.Errorf("Localize(bn) message = %q", e.Message)
	}
	if e, _ := apperr.As(catalog.Localize(err, httpx.LangEnglish)); e.Message != catalog["PHONE_TAKEN"].EN {
		t.Errorf("Localize(en) message = %q", e.Message)
	}
}

func TestLocalizeLeavesOtherErrorsAlone(t *testing.T) {
	plain := errors.New("boom")
	if got := catalog.Localize(plain, httpx.LangBangla); !errors.Is(got, plain) {
		t.Error("an uncoded error was changed")
	}
	unknown := &apperr.Error{Kind: apperr.KindInvalid, Code: "OTHER", Message: "as is"}
	if got, _ := apperr.As(catalog.Localize(unknown, httpx.LangBangla)); got.Message != "as is" {
		t.Error("an unknown code was changed")
	}
}
