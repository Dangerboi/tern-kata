package model_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestRelativeTimeNeverReadsNegative(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"unset", time.Time{}, "-"},
		{"ahead of the clock", now.Add(12 * time.Second), "0s ago"},
		{"seconds", now.Add(-12 * time.Second), "12s ago"},
		{"days", now.Add(-72 * time.Hour), "3d ago"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.RelativeTime(tt.at, now); got != tt.want {
				t.Fatalf("RelativeTime = %q, want %q", got, tt.want)
			}
		})
	}
}
