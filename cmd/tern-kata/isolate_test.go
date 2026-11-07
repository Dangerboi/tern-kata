package main

import (
	"path/filepath"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// isolateConfig points the user's config, cache and home folders at fresh temp dirs on every OS (XDG_* on Unix, %AppData%,
// %LocalAppData% and %USERPROFILE% on Windows) and returns tern-kata's config folder under them.
func isolateConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	dir, err := worker.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
