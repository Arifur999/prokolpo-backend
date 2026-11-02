package sales

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestScheduleSplitsEvenlyAndTheLastTakesTheRest(t *testing.T) {
	terms := Terms{Price: 5_400_000, Down: 3_360_000, Installments: 24, IntervalMonths: 1, FirstDue: day("2026-01-31")}
	rows := Schedule(terms, nil)
	if rows[0].Amount != 85_000 || rows[23].Amount != 85_000 {
		t.Fatalf("amounts = %d, %d; want 85,000 each", rows[0].Amount, rows[23].Amount)
	}
	if got := rows[1].Due.Format(time.DateOnly); got != "2026-02-28" {
		t.Fatalf("second due = %s, want the end of February", got)
	}

	odd := Schedule(Terms{Price: 1_000_000, Down: 0, Installments: 3, IntervalMonths: 3, FirstDue: day("2026-05-10")}, nil)
	if odd[0].Amount != 333_300 || odd[2].Amount != 333_400 || odd[2].Due.Format(time.DateOnly) != "2026-11-10" {
		t.Fatalf("rows = %+v", odd)
	}
}

func TestPaymentsFillTheOldestInstallmentFirst(t *testing.T) {
	terms := Terms{Price: 400_000, Down: 100_000, Installments: 3, IntervalMonths: 1, FirstDue: day("2026-08-05")}
	rows := Schedule(terms, []Paid{{Date: day("2026-08-04"), Amount: 150_000}})
	if rows[0].Paid != 100_000 || rows[0].PaidOn == nil || rows[1].Paid != 50_000 || rows[1].PaidOn != nil {
		t.Fatalf("rows = %+v", rows)
	}

	s := Stand(terms, rows, day("2026-10-03"))
	if s.Received != 250_000 || s.Due != 150_000 || s.PaidCount != 1 || s.Status != StatusBooked {
		t.Fatalf("standing = %+v", s)
	}
	if s.Next == nil || s.Next.No != 2 || s.Next.Amount != 50_000 {
		t.Fatalf("next = %+v", s.Next)
	}
	if s.LastPaid == nil || !s.LastPaid.Equal(day("2026-08-04")) {
		t.Fatalf("last paid = %v", s.LastPaid)
	}
	if s.Late == nil || s.Late.No != 2 || s.Late.Days != 28 || s.LateCount != 1 || s.LateSum != 50_000 {
		t.Fatalf("late = %+v (count %d, sum %d)", s.Late, s.LateCount, s.LateSum)
	}
}

func TestAFullPriceSaleIsSold(t *testing.T) {
	terms := Terms{Price: 5_200_000, Down: 5_200_000}
	s := Stand(terms, Schedule(terms, nil), day("2026-10-03"))
	if s.Status != StatusSold || s.Due != 0 || s.Next != nil {
		t.Fatalf("standing = %+v", s)
	}
}
