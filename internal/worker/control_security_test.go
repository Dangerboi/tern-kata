//go:build !windows

package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// A state folder open to other users is tightened and the control socket is the user's alone.
func TestStartingTightensTheStateFolderAndTheSocket(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(h.dirs.State, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(h.dirs.State, 0o755); err != nil {
		t.Fatal(err)
	}
	h.start(worker.Hooks{})
	if info, err := os.Stat(h.dirs.State); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("state folder mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(h.dirs.Socket()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
}

// A state folder that is a symbolic link is refused before anything is listened on.
func TestAStateFolderThatIsASymlinkIsRefused(t *testing.T) {
	h := newHarness(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	cfg, err := worker.LoadConfig(h.dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	dirs := h.dirs
	dirs.State = link
	w, err := worker.New(worker.Options{Config: cfg, Dirs: dirs, Kata: h.kata.Kata, Clock: h.clock})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Run = %v; want a symbolic link refusal", err)
	}
	if _, err := os.Stat(filepath.Join(real, "worker.sock")); err == nil {
		t.Fatal("a socket was created behind the link")
	}
}

// A run uid that is not a ULID is refused and no file is read for it.
func TestAControlRequestWithAPathAsRunUIDIsRefused(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	for _, op := range []string{"run-detail", "resume", "abandon"} {
		resp := h.ask(worker.Request{Op: op, Run: "../x"})
		if resp.OK || resp.Error == "" {
			t.Fatalf("%s with ../x answered %+v; want an error", op, resp)
		}
	}
	if _, err := os.Stat(filepath.Join(h.dirs.State, "x")); err == nil {
		t.Fatal("a path outside the runs folder was touched")
	}
}
