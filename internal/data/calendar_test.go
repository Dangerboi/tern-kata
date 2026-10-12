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
	mkdir := func(t *testing.T, root, dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// zoneFile makes the zoneinfo file for name under root and returns its root-relative path.
	zoneFile := func(t *testing.T, root, dir, name string) string {
		t.Helper()
		rel := filepath.Join(dir, "zoneinfo", name)
		mkdir(t, root, filepath.Dir(rel))
		if err := os.WriteFile(filepath.Join(root, rel), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return rel
	}
	link := func(t *testing.T, root, from, target string) {
		t.Helper()
		mkdir(t, root, filepath.Dir(from))
		if err := os.Symlink(target, filepath.Join(root, from)); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, tz         string
		setup            func(t *testing.T, root string)
		date, zone, note string
	}{
		{"none", "", func(*testing.T, string) {}, "2026-10-12", "UTC", "time zone unknown, using UTC"},
		{"TZ with colon", ":Asia/Tokyo", func(*testing.T, string) {}, "2026-10-13", "Asia/Tokyo", ""},
		{"zone link", "", func(t *testing.T, root string) {
			zoneFile(t, root, "usr/share", "America/New_York")
			link(t, root, "etc/localtime", "../usr/share/zoneinfo/America/New_York")
		}, "2026-10-12", "America/New_York", ""},
		{"link chain", "", func(t *testing.T, root string) {
			zoneFile(t, root, "nix/store/abc", "America/New_York")
			link(t, root, "etc/static/localtime", "../../nix/store/abc/zoneinfo/America/New_York")
			link(t, root, "etc/localtime", "static/localtime")
		}, "2026-10-12", "America/New_York", ""},
		{"TZ beats link", "Asia/Tokyo", func(t *testing.T, root string) {
			zoneFile(t, root, "usr/share", "America/New_York")
			link(t, root, "etc/localtime", "../usr/share/zoneinfo/America/New_York")
		}, "2026-10-13", "Asia/Tokyo", ""},
		{"timezone file", "", func(t *testing.T, root string) {
			mkdir(t, root, "etc")
			if err := os.WriteFile(filepath.Join(root, "etc/timezone"), []byte("Asia/Tokyo\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "2026-10-13", "Asia/Tokyo", ""},
		{"unloadable", "Nowhere/Land", func(*testing.T, string) {}, "2026-10-12", "UTC", `time zone "Nowhere/Land" not found, using UTC`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			c.setup(t, root)
			cal := calendar{Now: now, Env: func(string) string { return c.tz }, Zone: func() string { return unixZone(root) }}
			date, zone, note := cal.Today()
			if date != c.date || zone != c.zone || note != c.note {
				t.Errorf("got %q %q %q, want %q %q %q", date, zone, note, c.date, c.zone, c.note)
			}
		})
	}
}
