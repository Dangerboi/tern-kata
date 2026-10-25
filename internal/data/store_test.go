package data

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const (
	uidOne = "01JABCDEFGHJKMNPQRSTVWXYZ0"
	uidTwo = "01JABCDEFGHJKMNPQRSTVWXYZ1"
)

func newStore(t *testing.T) (attachmentStore, string) {
	t.Helper()
	root := t.TempDir()
	return attachmentStore{root: root}, root
}

func put(t *testing.T, s attachmentStore, uid, source string, taken ...string) (string, string) {
	t.Helper()
	name, path, err := s.put(context.Background(), uid, source, filepath.Base(source), taken)
	if err != nil {
		t.Fatal(err)
	}
	return name, path
}

func TestStoreMakesItsFolderAndGitignoreOnFirstUse(t *testing.T) {
	s, root := newStore(t)
	_, path := put(t, s, uidOne, writeFile(t, t.TempDir(), "a.txt", "x"))
	if want := filepath.Join(root, ".kata-attachments", uidOne, "a.txt"); path != want {
		t.Fatalf("stored at %s, want %s", path, want)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".kata-attachments", ".gitignore")); string(raw) != "*\n" {
		t.Fatalf(".gitignore holds %q", raw)
	}
}

func TestStoreRewritesAnEmptyGitignoreAndKeepsANonEmptyOne(t *testing.T) {
	s, root := newStore(t)
	source := writeFile(t, t.TempDir(), "a.txt", "x")
	ignore := filepath.Join(root, ".kata-attachments", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(ignore), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Dir(ignore), ".gitignore", "")
	put(t, s, uidOne, source)
	if raw, _ := os.ReadFile(ignore); string(raw) != "*\n" {
		t.Fatalf("empty .gitignore became %q", raw)
	}
	writeFile(t, filepath.Dir(ignore), ".gitignore", "mine\n")
	put(t, s, uidOne, source)
	if raw, _ := os.ReadFile(ignore); string(raw) != "mine\n" {
		t.Fatalf("non-empty .gitignore became %q", raw)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(ignore), ".gitignore*")); len(left) != 1 {
		t.Fatalf("stray files %v", left)
	}
}

func TestStorePicksTheLowestFreeName(t *testing.T) {
	s, _ := newStore(t)
	dir := t.TempDir()
	for _, c := range []struct {
		file  string
		taken []string
		want  string
	}{
		{"shot.png", nil, "shot.png"},
		{"shot.png", nil, "shot-2.png"}, // the first copy is on disk
		{"shot.png", []string{"shot-3.png"}, "shot-4.png"},
		{"shot.png", []string{"shot-4.png", "shot-5.png"}, "shot-3.png"}, // the lowest free number, whatever is taken above it
		{".env", nil, ".env"},
		{".env", nil, ".env-2"},
		{"Makefile", nil, "Makefile"},
		{"Makefile", nil, "Makefile-2"},
	} {
		if name, _ := put(t, s, uidOne, writeFile(t, dir, c.file, "x"), c.taken...); name != c.want {
			t.Fatalf("%s with %v stored as %s, want %s", c.file, c.taken, name, c.want)
		}
	}
}

func TestStoreNeverOverwritesAndLeavesNoPartialCopy(t *testing.T) {
	s, root := newStore(t)
	source := writeFile(t, t.TempDir(), "a.txt", "new")
	_, first := put(t, s, uidOne, writeFile(t, t.TempDir(), "a.txt", "old"))
	if _, second := put(t, s, uidOne, source); second == first {
		t.Fatal("overwrote the existing copy")
	}
	if raw, _ := os.ReadFile(first); string(raw) != "old" {
		t.Fatalf("existing copy now holds %q", raw)
	}
	dir := filepath.Join(root, ".kata-attachments", uidOne)
	before, _ := filepath.Glob(filepath.Join(dir, "*"))
	// A source that cannot be read leaves nothing behind.
	if _, _, err := s.put(context.Background(), uidOne, filepath.Join(t.TempDir(), "missing"), "missing", nil); err == nil {
		t.Fatal("copied a missing file")
	}
	if after, _ := filepath.Glob(filepath.Join(dir, "*")); !slices.Equal(before, after) {
		t.Fatalf("store folder went from %v to %v", before, after)
	}
}

func TestStoreRemoveReportsDeletedGoneAndLeft(t *testing.T) {
	s, root := newStore(t)
	_, mine := put(t, s, uidOne, writeFile(t, t.TempDir(), "a.txt", "x"))
	_, theirs := put(t, s, uidTwo, writeFile(t, t.TempDir(), "a.txt", "x"))
	outside := writeFile(t, t.TempDir(), "a.txt", "x")
	folder := filepath.Join(root, ".kata-attachments", uidOne)
	for _, c := range []struct {
		what, path string
		want       model.FileFate
	}{
		{"outside the store", outside, model.FileLeft},
		{"another Issue's folder", theirs, model.FileLeft},
		{"the Issue's folder itself", folder, model.FileLeft},
		{"a relative path", "a.txt", model.FileLeft},
		{"a path climbing out", filepath.Join(folder, "..", uidTwo, "a.txt"), model.FileLeft},
		{"the copy", mine, model.FileDeleted},
		{"the copy again", mine, model.FileGone},
	} {
		if got, err := s.remove(uidOne, c.path); err != nil || got != c.want {
			t.Fatalf("%s: %q, %v, want %q", c.what, got, err, c.want)
		}
	}
	for _, path := range []string{outside, theirs} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was deleted: %v", path, err)
		}
	}
}

func TestStoreRemoveReportsADeleteThatFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory this user cannot write to")
	}
	s, root := newStore(t)
	_, path := put(t, s, uidOne, writeFile(t, t.TempDir(), "a.txt", "x"))
	folder := filepath.Join(root, ".kata-attachments", uidOne)
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	if got, err := s.remove(uidOne, path); err == nil || got != model.FileLeft {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestStoreRefusesAnIssueUIDThatIsNotAULID(t *testing.T) {
	s, root := newStore(t)
	victim := writeFile(t, root, "victim.txt", "keep")
	source := writeFile(t, t.TempDir(), "a.txt", "x")
	for _, uid := range []string{"../x", "a/b", "../../..", "", "U1", uidOne + "/.."} {
		if _, _, err := s.put(context.Background(), uid, source, "a.txt", nil); err == nil || !strings.Contains(err.Error(), "not a ULID") {
			t.Fatalf("put with UID %q: %v, want a refusal", uid, err)
		}
		if err := s.ensure(uid); err == nil {
			t.Fatalf("ensure with UID %q went ahead", uid)
		}
		if got, err := s.remove(uid, victim); err == nil || got != model.FileLeft {
			t.Fatalf("remove with UID %q: %q, %v, want the file left with an error", uid, got, err)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("the file outside the store was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".kata-attachments")); !os.IsNotExist(err) {
		t.Fatalf("the store was created for a bad UID: %v", err)
	}
}
