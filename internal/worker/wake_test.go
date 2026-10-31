package worker

import (
	"testing"
	"time"
)

// A waiting job whose Issue instant already passed (the Project was full when it came) never becomes the loop's next
// wake: a wake in the past spins the loop.
func TestSoonerSkipsAnInstantBehindNowAndKeepsTheEarliestAheadOfIt(t *testing.T) {
	now := time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC)
	past, soon, later := now.Add(-time.Minute), now.Add(time.Minute), now.Add(time.Hour)
	for _, tc := range []struct {
		name           string
		wake, at, want time.Time
	}{
		{"past instant without a wake", time.Time{}, past, time.Time{}},
		{"past instant keeps the wake", later, past, later},
		{"instant at now is not ahead", time.Time{}, now, time.Time{}},
		{"future instant without a wake", time.Time{}, later, later},
		{"earlier future instant wins", later, soon, soon},
		{"later future instant loses", soon, later, soon},
		{"no instant", soon, time.Time{}, soon},
	} {
		if got := sooner(tc.wake, tc.at, now); !got.Equal(tc.want) {
			t.Errorf("%s: sooner = %v, want %v", tc.name, got, tc.want)
		}
	}
}
