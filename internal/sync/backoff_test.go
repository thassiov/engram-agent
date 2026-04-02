package sync

import (
	"testing"
	"time"
)

func TestBackoff_FirstNextApproximatelyBase(t *testing.T) {
	base := 10 * time.Second
	max := 2 * time.Minute
	b := newBackoff(base, max)

	d := b.next()
	// With ±25% jitter: range is [7.5s, 12.5s].
	low := time.Duration(float64(base) * 0.75)
	high := time.Duration(float64(base) * 1.25)
	if d < low || d > high {
		t.Errorf("first next() = %v, want in [%v, %v]", d, low, high)
	}
}

func TestBackoff_SuccessiveCallsDouble(t *testing.T) {
	base := 1 * time.Second
	max := 1 * time.Hour
	b := newBackoff(base, max)

	// After the first call, current doubles to 2s.
	b.next()
	// After second call, current should be 4s before the call.
	// We check that the second next() value is roughly 2x the base.
	d := b.next()
	low := time.Duration(float64(2*base) * 0.75)
	high := time.Duration(float64(2*base) * 1.25)
	if d < low || d > high {
		t.Errorf("second next() = %v, want in [%v, %v]", d, low, high)
	}
}

func TestBackoff_NeverExceedsMax(t *testing.T) {
	base := 1 * time.Second
	max := 4 * time.Second
	b := newBackoff(base, max)

	// Call many times; values should never far exceed max (with jitter max+25%).
	ceiling := time.Duration(float64(max) * 1.25)
	for i := 0; i < 20; i++ {
		d := b.next()
		if d > ceiling {
			t.Errorf("next() = %v exceeds ceiling %v at call %d", d, ceiling, i)
		}
	}
}

func TestBackoff_ResetReturnsToBase(t *testing.T) {
	base := 5 * time.Second
	max := 2 * time.Minute
	b := newBackoff(base, max)

	// Advance the backoff several times.
	for i := 0; i < 5; i++ {
		b.next()
	}
	// Reset, then next() should be approximately base again.
	b.reset()
	d := b.next()
	low := time.Duration(float64(base) * 0.75)
	high := time.Duration(float64(base) * 1.25)
	if d < low || d > high {
		t.Errorf("after reset, next() = %v, want in [%v, %v]", d, low, high)
	}
}
