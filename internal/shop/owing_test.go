package shop

import "testing"

func TestOwingDatesTheOldestCreditNotYetPaid(t *testing.T) {
	people := owing([]entry{
		{personID: "rahim", name: "রহিম কনস্ট্রাকশন", kind: "CREDIT", date: "2026-09-02", amount: 5000},
		{personID: "selim", name: "সেলিম মিস্ত্রি", kind: "CREDIT", date: "2026-09-03", amount: 7800},
		{personID: "rahim", name: "রহিম কনস্ট্রাকশন", kind: "CREDIT", date: "2026-09-12", amount: 18500},
		{personID: "rahim", name: "রহিম কনস্ট্রাকশন", kind: "PAYMENT", date: "2026-09-20", amount: 5000},
		{personID: "jamal", name: "জামাল প্লাম্বার", kind: "CREDIT", date: "2026-09-26", amount: 5500},
		{personID: "jamal", name: "জামাল প্লাম্বার", kind: "PAYMENT", date: "2026-09-30", amount: 5500},
	})
	if len(people) != 2 {
		t.Fatalf("people = %+v, want rahim and selim", people)
	}
	r := people[0]
	if r.Name != "রহিম কনস্ট্রাকশন" || r.Due != 18500 || *r.Since != "2026-09-12" || *r.Last != "2026-09-12" {
		t.Errorf("rahim = %+v since %s", r, *r.Since)
	}
	if people[1].Due != 7800 || *people[1].Since != "2026-09-03" {
		t.Errorf("selim = %+v", people[1])
	}
}
