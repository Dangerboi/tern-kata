package guardedfile

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestWriteRefusesAChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "f.toml")

	missing, err := Read(path)
	if err != nil || missing.Text() != "" {
		t.Fatalf("missing file: text %q, err %v", missing.Text(), err)
	}
	if err := Write(path, 0o700, missing, []byte("one\n")); err != nil {
		t.Fatalf("write to a missing file: %v", err)
	}

	tok, err := Read(path)
	if err != nil || tok.Text() != "one\n" {
		t.Fatalf("read: text %q, err %v", tok.Text(), err)
	}
	if err := os.WriteFile(path, []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, 0o700, tok, []byte("two\n")); !errors.Is(err, ErrChanged) {
		t.Fatalf("write after a change = %v, want ErrChanged", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "other\n" {
		t.Errorf("a refused write left %q", got)
	}

	// A file that appeared after a read of a missing one is a change too.
	gone := filepath.Join(t.TempDir(), "g.toml")
	none, _ := Read(gone)
	if err := os.WriteFile(gone, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(gone, 0o700, none, []byte("y")); !errors.Is(err, ErrChanged) {
		t.Fatalf("write over a new file = %v, want ErrChanged", err)
	}

	fresh, _ := Read(path)
	if err := Write(path, 0o700, fresh, []byte("three\n")); err != nil {
		t.Fatalf("write from a fresh read: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "three\n" {
		t.Errorf("file = %q", got)
	}
}

func TestReadReportsAnUnreadableFile(t *testing.T) {
	if _, err := Read(t.TempDir()); err == nil {
		t.Fatal("reading a folder gave no error")
	}
}

func TestConcurrentWritersLoseNoChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	const writers, each = 8, 15
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				for {
					tok, err := Read(path)
					if err != nil {
						errs <- err
						return
					}
					err = Write(path, 0o700, tok, []byte(tok.Text()+"x"))
					if errors.Is(err, ErrChanged) {
						continue
					}
					if err != nil {
						errs <- err
						return
					}
					break
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if len(got) != writers*each {
		t.Errorf("file holds %d changes, want %d", len(got), writers*each)
	}
}

func TestUpdateRunsTheChangeAgainOnWhatAnotherWriterLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var seen []string
	err := Update(path, 0o700, func(tok Token) ([]byte, error) {
		seen = append(seen, tok.Text())
		if len(seen) == 1 {
			if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return []byte(tok.Text() + "three\n"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one\n", "one\ntwo\n"}; !slices.Equal(seen, want) {
		t.Errorf("change saw %q, want %q", seen, want)
	}
	if got, _ := os.ReadFile(path); string(got) != "one\ntwo\nthree\n" {
		t.Errorf("file = %q", got)
	}
}

func TestUpdateOfAFileThatKeepsChangingGivesUpWrappingErrChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	calls := 0
	err := Update(path, 0o700, func(Token) ([]byte, error) {
		calls++
		if err := os.WriteFile(path, []byte(strings.Repeat("x", calls)), 0o600); err != nil {
			t.Fatal(err)
		}
		return []byte("mine"), nil
	})
	if !errors.Is(err, ErrChanged) || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want ErrChanged naming the file", err)
	}
	if got, _ := os.ReadFile(path); string(got) == "mine" {
		t.Error("the change was written over the other writer's file")
	}
}

func TestUpdateReturnsTheChangesErrorAndWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	boom := errors.New("boom")
	if err := Update(path, 0o700, func(Token) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a file was written")
	}
}

func TestWriteGoesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "views.toml")
	link := filepath.Join(dir, "config", "views.toml")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	tok, err := Read(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(link, 0o700, tok, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	if got, _ := os.ReadFile(real); string(got) != "two\n" {
		t.Errorf("the target holds %q, want the write", got)
	}
	if _, err := os.Lstat(link + ".lock"); err == nil {
		t.Error("the lock was made beside the link, not the target")
	}

	// A dangling link: the write creates the target where the link points.
	dangling := filepath.Join(dir, "config", "lanes.toml")
	target := filepath.Join(dir, "dotfiles", "lanes.toml")
	if err := os.Symlink(target, dangling); err != nil {
		t.Fatal(err)
	}
	tok, _ = Read(dangling)
	if err := Write(dangling, 0o700, tok, []byte("lane\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "lane\n" {
		t.Errorf("the dangling link's target holds %q", got)
	}
	if info, err := os.Lstat(dangling); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the dangling link was replaced: %v, %v", info, err)
	}
}
