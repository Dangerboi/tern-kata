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
		{"unset", time.Time{}, "—"},
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

func TestAgeIsCompactWithNoSpace(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		at   time.Time
		want string
	}{
		{time.Time{}, model.None},
		{now.Add(5 * time.Second), "0s"},
		{now.Add(-7 * time.Second), "7s"},
		{now.Add(-4 * time.Minute), "4m"},
		{now.Add(-2 * time.Hour), "2h"},
		{now.Add(-72 * time.Hour), "3d"},
		{now.Add(-8 * 24 * time.Hour), "1w"},
	} {
		if got := model.Age(tt.at, now); got != tt.want {
			t.Errorf("Age(%v) = %q, want %q", tt.at, got, tt.want)
		}
	}
}
