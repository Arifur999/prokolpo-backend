package ratelimit

import (
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	w := New(3, 15*time.Minute)
	w.now = func() time.Time { return now }

	for range 3 {
		if w.Exceeded("01712345678") {
			t.Fatal("exceeded before the limit")
		}
		w.Hit("01712345678")
	}
	if !w.Exceeded("01712345678") {
		t.Fatal("not exceeded after 3 hits")
	}
	if w.Exceeded("01811111111") {
		t.Fatal("another key is affected")
	}

	now = now.Add(15*time.Minute + time.Second)
	if w.Exceeded("01712345678") {
		t.Fatal("still exceeded after the window passed")
	}
}

func TestAllowAndReset(t *testing.T) {
	w := New(1, time.Minute)
	if !w.Allow("k") {
		t.Fatal("first Allow refused")
	}
	if w.Allow("k") {
		t.Fatal("second Allow within the window accepted")
	}
	w.Reset("k")
	if !w.Allow("k") {
		t.Fatal("Allow refused after Reset")
	}
}
