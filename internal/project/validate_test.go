package project

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

func validBuilding() createBody {
	return createBody{
		Name: "মিরপুর ৬ তলা ভবন", Kind: KindBuilding, Ownership: OwnershipPartnership,
		Location: " মিরপুর-১০, ঢাকা ", StartDate: "2026-10-03", Budget: new(int64(25_000_000)), Floors: new(int16(6)),
		Categories: []string{"রড", "সিমেন্ট"}, Phases: []string{"জমি ও নকশা", "১ম তলা"},
	}
}

func fieldPaths(t *testing.T, err error) []string {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != apperr.KindInvalid {
		t.Fatalf("err = %v, want an invalid-input error", err)
	}
	paths := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		paths[i] = f.Path
	}
	return paths
}

func TestValidateKeepsOnlyTheKindsDetails(t *testing.T) {
	body := validBuilding()
	body.Batches = new(int16(3))
	body.FarmType = "POULTRY"
	body.BusinessType = "দোকান"

	np, err := body.validate(httpx.LangBangla)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if np.Batches != nil || np.FarmType != nil || np.BusinessType != nil {
		t.Errorf("building kept another kind's details: %+v", np)
	}
	if *np.Location != "মিরপুর-১০, ঢাকা" || *np.Floors != 6 {
		t.Errorf("location %q floors %d", *np.Location, *np.Floors)
	}
}

func TestValidateReportsEveryBadField(t *testing.T) {
	tests := []struct {
		name string
		edit func(*createBody)
		want []string
	}{
		{"short name", func(b *createBody) { b.Name = "ab" }, []string{"name"}},
		{"unknown kind and ownership", func(b *createBody) { b.Kind = "CASTLE"; b.Ownership = "" }, []string{"kind", "ownership"}},
		{"bad date and budget", func(b *createBody) { b.StartDate = "03/10/2026"; b.Budget = new(int64(-1)) }, []string{"startDate", "budget"}},
		{"too many floors", func(b *createBody) { b.Floors = new(int16(31)) }, []string{"floors"}},
		{"repeated category", func(b *createBody) { b.Categories = []string{"রড", " রড"} }, []string{"categories"}},
		{"blank phase", func(b *createBody) { b.Phases = []string{"ছাদ", ""} }, []string{"phases"}},
		{"farm without a type", func(b *createBody) { b.Kind = KindFarm }, []string{"farmType"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := validBuilding()
			tt.edit(&body)
			_, err := body.validate(httpx.LangEnglish)
			if diff := cmp.Diff(tt.want, fieldPaths(t, err)); diff != "" {
				t.Errorf("field paths (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateNameMessagesFollowTheBoard(t *testing.T) {
	body := validBuilding()
	body.Name = "  "
	_, err := body.validate(httpx.LangBangla)
	e, _ := apperr.As(err)
	if got := e.Fields[0].Message; got != "প্রজেক্টের নাম দিন" {
		t.Errorf("blank name message = %q", got)
	}
}
