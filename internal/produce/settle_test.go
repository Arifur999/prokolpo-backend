package produce

import "testing"

func TestSettleClearsABuyersOldestSalesFirst(t *testing.T) {
	sales := []Sale{
		{ID: "s1", BuyerID: "mamun", Buyer: "মামুন", Date: "2026-08-24", Amount: 295_680, paid: 283_680},
		{ID: "s2", BuyerID: "hanif", Buyer: "হানিফ", Date: "2026-08-25", Amount: 168_560, paid: 168_560},
		{ID: "s3", BuyerID: "mamun", Buyer: "মামুন", Date: "2026-09-01", Amount: 50_000},
		{ID: "s4", BuyerID: "hanif", Buyer: "হানিফ", Date: "2026-10-02", Amount: 157_500, paid: 120_000},
	}
	dues := settle(sales, map[string]int64{"mamun": 20_000})

	lefts := []int64{0, 0, 42_000, 37_500}
	for i, want := range lefts {
		if sales[i].Left != want {
			t.Errorf("sale %s left = %d, want %d", sales[i].ID, sales[i].Left, want)
		}
	}
	if len(dues) != 2 {
		t.Fatalf("dues = %+v, want two buyers", dues)
	}
	if dues[0].Buyer != "মামুন" || dues[0].Amount != 42_000 || dues[0].Since != "2026-09-01" {
		t.Errorf("first due = %+v, want মামুন 42,000 since 2026-09-01", dues[0])
	}
	if dues[1].Buyer != "হানিফ" || dues[1].Amount != 37_500 {
		t.Errorf("second due = %+v, want হানিফ 37,500", dues[1])
	}
}

func TestSellableFollowsTheFarmType(t *testing.T) {
	cases := []struct {
		farm, product, unit string
		want                bool
	}{
		{"POULTRY", "BROILER", "KG", true},
		{"POULTRY", "EGG", "TRAY", true},
		{"POULTRY", "MILK", "LITRE", false},
		{"CATTLE", "MILK", "LITRE", true},
		{"FISH", "FISH", "PIECE", false},
		{"CROP", "OTHER", "KG", true},
	}
	for _, c := range cases {
		if got := sellable(c.farm, c.product, c.unit); got != c.want {
			t.Errorf("sellable(%s, %s, %s) = %v, want %v", c.farm, c.product, c.unit, got, c.want)
		}
	}
}
