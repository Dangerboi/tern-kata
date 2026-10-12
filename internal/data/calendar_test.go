//go:build !windows

package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCalendarToday(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC) }
	root := t.TempDir()
	etc := filepath.Join(root, "etc")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	link := func(target string) {
		os.Remove(filepath.Join(etc, "localtime"))
		if err := os.Symlink(target, filepath.Join(etc, "localtime")); err != nil {
			t.Fatal(err)
		}
	}
	file := func(body string) {
		if err := os.WriteFile(filepath.Join(etc, "timezone"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, tz         string
		setup            func()
		date, zone, note string
	}{
		{"none", "", func() {}, "2026-10-12", "UTC", "time zone unknown, using UTC"},
		{"TZ with colon", ":Asia/Tokyo", func() {}, "2026-10-13", "Asia/Tokyo", ""},
		{"zone link", "", func() { link("/usr/share/zoneinfo/America/New_York") }, "2026-10-12", "America/New_York", ""},
		{"TZ beats link", "Asia/Tokyo", func() {}, "2026-10-13", "Asia/Tokyo", ""},
		{"timezone file", "", func() { os.Remove(filepath.Join(etc, "localtime")); file("Asia/Tokyo\n") }, "2026-10-13", "Asia/Tokyo", ""},
		{"unloadable", "Nowhere/Land", func() {}, "2026-10-12", "UTC", `time zone "Nowhere/Land" not found, using UTC`},
	}
	for _, c := range cases {
		c.setup()
		cal := calendar{Now: now, Env: func(string) string { return c.tz }, Zone: func() string { return unixZone(root) }}
		date, zone, note := cal.Today()
		if date != c.date || zone != c.zone || note != c.note {
			t.Errorf("%s: got %q %q %q, want %q %q %q", c.name, date, zone, note, c.date, c.zone, c.note)
		}
	}
}
