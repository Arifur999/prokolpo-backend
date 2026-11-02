package books

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

const someID = "01a19c75-2be8-7a14-92ae-79565523f112"

func paths(err error) []string {
	e, ok := apperr.As(err)
	if !ok {
		return nil
	}
	out := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		out[i] = f.Path
	}
	return out
}

func TestExpenseBodyNeedsAmountCategoryAndParty(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		body expenseBody
		want []string
	}{
		{"blank", expenseBody{}, []string{"amount", "categoryId"}},
		{"supplier without id", expenseBody{CategoryID: someID, Amount: 10, PartyKind: "SUPPLIER"}, []string{"partyId"}},
		{"paid now without account", expenseBody{CategoryID: someID, Amount: 10, PaidNow: true}, []string{"accountId"}},
		{"bad quantity", expenseBody{CategoryID: someID, Amount: 10, Quantity: "1.23456"}, []string{"quantity"}},
		{"fine", expenseBody{CategoryID: someID, Amount: 475000, Quantity: "5", Unit: "টন"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := tt.body.validate(httpx.LangBangla, today)
			if diff := cmp.Diff(tt.want, paths(err)); diff != "" {
				t.Errorf("paths (-want +got):\n%s", diff)
			}
			if err == nil && !in.Date.Equal(today) {
				t.Errorf("blank date = %v, want today", in.Date)
			}
		})
	}
}

func TestSupplierPhoneIsNormalized(t *testing.T) {
	ns, err := supplierBody{Name: "করিম রড স্টোর", Phone: "+880 1712-345678", Supplies: []string{"রড", "রড", "নেই"}}.validate(httpx.LangBangla)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if *ns.Phone != "01712345678" || !cmp.Equal(ns.Supplies, []string{"রড"}) {
		t.Errorf("phone %q supplies %v", *ns.Phone, ns.Supplies)
	}
}

func TestTransferNeedsTwoAccounts(t *testing.T) {
	_, err := transferBody{From: someID, To: someID, Amount: 5}.validate(httpx.LangBangla, time.Now())
	if diff := cmp.Diff([]string{"toAccountId"}, paths(err)); diff != "" {
		t.Error(diff)
	}
}

func TestWeeksStartOnSunday(t *testing.T) {
	friday := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if got := weekStart(friday).Format(time.DateOnly); got != "2026-09-27" {
		t.Errorf("weekStart = %s", got)
	}
}
