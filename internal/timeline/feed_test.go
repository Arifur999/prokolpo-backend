package timeline

import (
	"testing"
	"time"
)

func TestWeekStartIsSundayInDhaka(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"saturday evening", time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC), "2026-09-27"},
		{"sunday just after midnight in Dhaka", time.Date(2026, 10, 3, 18, 30, 0, 0, time.UTC), "2026-10-04"},
		{"sunday noon", time.Date(2026, 10, 4, 6, 0, 0, 0, time.UTC), "2026-10-04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekStart(tt.at).Format(time.DateOnly); got != tt.want {
				t.Errorf("weekStart(%v) = %s, want %s", tt.at, got, tt.want)
			}
		})
	}
}

func TestDaysBackCountsToday(t *testing.T) {
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if got := daysBack(at, 7).Format(time.DateOnly); got != "2026-09-27" {
		t.Errorf("daysBack(7) = %s, want 2026-09-27", got)
	}
}
