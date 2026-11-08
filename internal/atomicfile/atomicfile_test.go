package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRetryRenameRetriesRetryableErrorsThenSucceeds(t *testing.T) {
	retryable := errors.New("busy")
	calls := 0
	err := retryRename(func() error {
		calls++
		if calls < 3 {
			return retryable
		}
		return nil
	}, func(err error) bool { return errors.Is(err, retryable) })
	if err != nil || calls != 3 {
		t.Fatalf("err = %v, calls = %d, want nil after 3", err, calls)
	}
}

func TestRetryRenameGivesUpAfterBound(t *testing.T) {
	retryable := errors.New("busy")
	calls := 0
	err := retryRename(func() error { calls++; return retryable }, func(error) bool { return true })
	if !errors.Is(err, retryable) || calls != renameAttempts {
		t.Fatalf("err = %v, calls = %d, want %d attempts", err, calls, renameAttempts)
	}
}

func TestRetryRenameDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	boom := errors.New("boom")
	err := retryRename(func() error { calls++; return boom }, func(error) bool { return false })
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err = %v, calls = %d, want one attempt", err, calls)
	}
}

func TestWriteReplacesAFileWhoseNameIsAsLongAsTheFilesystemAllows(t *testing.T) {
	path := filepath.Join(t.TempDir(), strings.Repeat("a", 255))
	if err := Write(path, []byte("body"), Options{Mode: 0o600}); err != nil {
		t.Fatalf("Write = %v; want a 255-byte name to work", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "body" {
		t.Fatalf("file = %q, %v", got, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("%d entries left in the folder; want only the target", len(entries))
	}
}

func TestWriteKeepsASymlinkAndWritesBesideItsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "repo", "worker.toml")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(dir, "mid.toml")
	link := filepath.Join(dir, "worker.toml")
	if err := os.Symlink(real, mid); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("mid.toml", link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("new"), Options{Mode: 0o600}); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{link, mid} {
		if info, err := os.Lstat(l); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is no longer a symlink: %v %v", l, info, err)
		}
	}
	if got, _ := os.ReadFile(real); string(got) != "new" {
		t.Fatalf("target holds %q, want new", got)
	}
}

func TestResolveLinkNamesTheLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.Symlink(b, a)
	_ = os.Symlink(a, b)
	if _, err := ResolveLink(a); err == nil || !strings.Contains(err.Error(), a) {
		t.Fatalf("want an error naming %s, got %v", a, err)
	}
}
