package produce

import (
	"testing"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

func TestSaleBodyRefusesAmountsThatDoNotFit(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	for name, b := range map[string]saleBody{
		"rounds to nothing": {Product: "MILK", Quantity: 0.4, Unit: "LITRE", Rate: 1, Buyer: "দোকান", Date: "2026-10-02"},
		"overflows":         {Product: "MILK", Quantity: 1e9, Unit: "LITRE", Rate: maxAmount, Buyer: "দোকান", Date: "2026-10-02"},
	} {
		if _, err := b.validate(httpx.LangBangla, today); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := saleBody{Product: "MILK", Quantity: 0.5, Unit: "LITRE", Rate: 3, Buyer: "দোকান", Date: "2026-10-02"}
	if in, err := ok.validate(httpx.LangBangla, today); err != nil || in.Amount != 2 {
		t.Fatalf("half a litre at 3 = %+v, %v; want 2 taka", in, err)
	}
}
