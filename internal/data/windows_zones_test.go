package data

import (
	"testing"
	"time"
)

func TestWindowsIANAZone(t *testing.T) {
	for key, want := range map[string]string{
		"W. Europe Standard Time": "Europe/Berlin",
		"Pacific Standard Time":   "America/Los_Angeles",
		"UTC":                     "Etc/UTC",
	} {
		if got, ok := windowsIANAZone(key); !ok || got != want {
			t.Errorf("windowsIANAZone(%q) = %q, %v; want %q", key, got, ok, want)
		}
	}
	for _, key := range []string{"No Such Standard Time", "", "# CLDR release-47"} {
		if got, ok := windowsIANAZone(key); ok {
			t.Errorf("windowsIANAZone(%q) = %q, true; want not found", key, got)
		}
	}
}

func TestCalendarUnknownWindowsKeyFallsBackToUTC(t *testing.T) {
	c := calendar{
		Now: func() time.Time { return time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC) },
		Env: func(string) string { return "" },
		Zone: func() string {
			zone, _ := windowsIANAZone("No Such Standard Time")
			return zone
		},
	}
	date, zone, note := c.Today()
	if date != "2026-10-12" || zone != "UTC" || note == "" {
		t.Errorf("Today() = %q, %q, %q; want UTC with a note", date, zone, note)
	}
}
