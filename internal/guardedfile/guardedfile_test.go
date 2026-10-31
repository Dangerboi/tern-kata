package guardedfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteRefusesAChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "f.toml")

	missing, err := Read(path)
	if err != nil || missing.Text() != "" {
		t.Fatalf("missing file: text %q, err %v", missing.Text(), err)
	}
	if err := Write(path, missing, []byte("one\n")); err != nil {
		t.Fatalf("write to a missing file: %v", err)
	}

	tok, err := Read(path)
	if err != nil || tok.Text() != "one\n" {
		t.Fatalf("read: text %q, err %v", tok.Text(), err)
	}
	if err := os.WriteFile(path, []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, tok, []byte("two\n")); !errors.Is(err, ErrChanged) {
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
	if err := Write(gone, none, []byte("y")); !errors.Is(err, ErrChanged) {
		t.Fatalf("write over a new file = %v, want ErrChanged", err)
	}

	fresh, _ := Read(path)
	if err := Write(path, fresh, []byte("three\n")); err != nil {
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
					err = Write(path, tok, []byte(tok.Text()+"x"))
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
