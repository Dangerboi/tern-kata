package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestParseDateInput(t *testing.T) {
	const today = "2026-10-08"
	for _, tt := range []struct{ in, today, want string }{
		{"", today, ""},
		{"  ", today, ""},
		{"2026-10-24", "", "2026-10-24"},
		{"Today", today, "2026-10-08"},
		{"tomorrow", today, "2026-10-09"},
		{"+0d", today, "2026-10-08"},
		{"+3d", today, "2026-10-11"},
		{"+2W", today, "2026-10-22"},
		{"+30d", "2026-12-20", "2027-01-19"},
		{" 2024-02-29 ", today, "2024-02-29"},
	} {
		if got, err := model.ParseDateInput(tt.in, tt.today); err != nil || got != tt.want {
			t.Errorf("%q = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"2026-02-30", "2026-1-5", "26-10-12", "+d", "+3", "-1d", "+1m", "next week", "2026-10-24T09:00"} {
		if got, err := model.ParseDateInput(in, today); err == nil {
			t.Errorf("%q read as %q, want an error", in, got)
		}
	}
	if _, err := model.ParseDateInput("today", ""); err == nil {
		t.Error("today with no Pane date was read")
	}
}
