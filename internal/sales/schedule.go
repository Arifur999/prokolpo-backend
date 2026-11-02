package sales

import "time"

// Terms are how a sale is paid: a down payment on the day, then Installments equal parts
// every IntervalMonths from FirstDue, the last part taking what rounding left over.
type Terms struct {
	Price          int64
	Down           int64
	Installments   int
	IntervalMonths int
	FirstDue       time.Time
}

// installmentFor is the even part of what is left after the down payment, rounded down
// to a hundred taka (the last installment makes up the difference).
func installmentFor(price, down int64, n int) int64 {
	if n <= 0 {
		return 0
	}
	return (price - down) / int64(n) / 100 * 100
}

// Row is one installment with what has been paid toward it.
type Row struct {
	No     int       `json:"no"`
	Due    time.Time `json:"due"`
	Amount int64     `json:"amount"`
	Paid   int64     `json:"paid"`
	// PaidOn is when it was paid in full; LastPaidOn when anything last went toward it.
	PaidOn     *time.Time `json:"paidOn"`
	LastPaidOn *time.Time `json:"lastPaidOn"`
}

// Paid is an installment payment as the schedule fills it: oldest installment first.
type Paid struct {
	Date   time.Time
	Amount int64
}

// Schedule lays out the installments and fills them with payments in date order.
func Schedule(t Terms, payments []Paid) []Row {
	base := installmentFor(t.Price, t.Down, t.Installments)
	rows := make([]Row, t.Installments)
	for k := range rows {
		amount := base
		if k == t.Installments-1 {
			amount = t.Price - t.Down - base*int64(t.Installments-1)
		}
		rows[k] = Row{No: k + 1, Due: addMonths(t.FirstDue, k*t.IntervalMonths), Amount: amount}
	}
	for _, p := range payments {
		left := p.Amount
		for k := range rows {
			if left == 0 {
				break
			}
			r := &rows[k]
			need := r.Amount - r.Paid
			if need <= 0 {
				continue
			}
			take := min(need, left)
			r.Paid += take
			left -= take
			on := p.Date
			r.LastPaidOn = &on
			if r.Paid == r.Amount {
				r.PaidOn = &on
			}
		}
	}
	return rows
}

// addMonths moves a date by whole months, keeping the day or the month's last day.
func addMonths(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last), 0, 0, 0, 0, time.UTC)
}

// Standing is where a sale is today: what came in, what is left, the next installment and
// the oldest one overdue.
type Standing struct {
	Received  int64      `json:"received"`
	Due       int64      `json:"due"`
	PaidCount int        `json:"paidCount"`
	Next      *RowDue    `json:"next"`
	Late      *RowLate   `json:"late"`
	LateCount int        `json:"lateCount"`
	LateSum   int64      `json:"lateSum"`
	Status    string     `json:"status"`
	LastPaid  *time.Time `json:"lastPaid"`
}

// RowDue is the next installment to pay and what is left of it.
type RowDue struct {
	No     int       `json:"no"`
	Date   time.Time `json:"date"`
	Amount int64     `json:"amount"`
}

// RowLate is the oldest overdue installment and how many days it is late.
type RowLate struct {
	RowDue
	Days int `json:"days"`
}

// Unit standings: booked while money is owed, sold once it is all in.
const (
	StatusAvailable = "AVAILABLE"
	StatusBooked    = "BOOKED"
	StatusSold      = "SOLD"
)

// Stand sums a sale's schedule as of today.
func Stand(t Terms, rows []Row, today time.Time) Standing {
	s := Standing{Received: t.Down}
	for i := range rows {
		r := &rows[i]
		s.Received += r.Paid
		if r.LastPaidOn != nil && (s.LastPaid == nil || r.LastPaidOn.After(*s.LastPaid)) {
			s.LastPaid = r.LastPaidOn
		}
		if r.Paid >= r.Amount {
			s.PaidCount++
			continue
		}
		left := r.Amount - r.Paid
		if s.Next == nil {
			s.Next = &RowDue{No: r.No, Date: r.Due, Amount: left}
		}
		if r.Due.Before(today) {
			s.LateCount++
			s.LateSum += left
			if s.Late == nil {
				s.Late = &RowLate{No: r.No, Date: r.Due, Amount: left, Days: int(today.Sub(r.Due).Hours() / 24)}
			}
		}
	}
	s.Due = t.Price - s.Received
	s.Status = StatusBooked
	if s.Due <= 0 {
		s.Status = StatusSold
	}
	return s
}
