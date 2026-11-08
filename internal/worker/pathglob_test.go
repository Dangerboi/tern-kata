package worker

import (
	"os"
	"path/filepath"
	"testing"
)

// A state folder named with glob characters must still yield its pin files and session files.
func TestListingsSurviveGlobCharactersInTheStatePath(t *testing.T) {
	state := filepath.Join(t.TempDir(), "proj[1]*?", "state")
	dirs := Dirs{State: state}
	for _, uid := range []string{"a", "b"} {
		if err := saveJSON(dirs.Pin(uid), pin{ProjectUID: uid}); err != nil {
			t.Fatal(err)
		}
	}
	if got := dirs.pinFiles(); len(got) != 2 {
		t.Fatalf("pin files = %v, want 2", got)
	}
	sess := filepath.Join(state, "s[1]")
	if err := os.MkdirAll(sess, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sess, "2026_abc.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := sessionIn(sess, ""); got != "abc" {
		t.Fatalf("sessionIn(unknown) = %q", got)
	}
	if got := sessionIn(sess, "abc"); got != "abc" {
		t.Fatalf("sessionIn(abc) = %q", got)
	}
}
