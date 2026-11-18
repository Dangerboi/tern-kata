package model_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestPaneCalendarFormats(t *testing.T) {
	c := model.NewPaneCalendar("2026-11-18", "Europe/Berlin", "")
	now := time.Date(2026, 11, 18, 17, 30, 0, 0, time.UTC) // 18:30 in Berlin
	today := time.Date(2026, 11, 18, 17, 21, 9, 0, time.UTC)
	earlier := time.Date(2026, 11, 17, 17, 21, 9, 0, time.UTC)
	for _, tt := range []struct {
		name, got, want string
	}{
		{"stamp has no seconds and no Z", c.Stamp(today), "2026-11-18 18:21"},
		{"read today is the time", c.Read(today, now), "18:21"},
		{"read another day is the stamp", c.Read(earlier, now), "2026-11-17 18:21"},
		{"day", c.Day(today), "2026-11-18"},
		{"stamp unset", c.Stamp(time.Time{}), model.None},
		{"read unset", c.Read(time.Time{}, now), model.None},
		{"day unset", c.Day(time.Time{}), model.None},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
