package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestWriteReplacesAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.toml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), Options{Mode: 0o600}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content %q", got)
	}
	if got := names(t, dir); len(got) != 1 {
		t.Fatalf("files %v", got)
	}
	if filepath.Separator == '/' {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", info.Mode())
		}
	}
}

func TestWriteCreatesFolderWithMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "deeper", "a")
	if err := Write(path, []byte("x"), Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		t.Fatal(err)
	}
	if filepath.Separator == '/' {
		if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
			t.Fatalf("folder mode %v", info.Mode())
		}
	}
	if err := Write(filepath.Join(t.TempDir(), "missing", "a"), []byte("x"), Options{Mode: 0o600}); err == nil {
		t.Fatal("a missing folder without DirMode must fail")
	}
}

func TestCheckFailureKeepsOldFileAndSeesTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	refuse := errors.New("refused")
	var seen string
	err := Write(path, []byte("new"), Options{Mode: 0o600, Check: func(tmp string) error {
		raw, _ := os.ReadFile(tmp)
		seen = string(raw)
		return refuse
	}})
	if err != refuse {
		t.Fatalf("err %v", err)
	}
	var re *RenameError
	if errors.As(err, &re) {
		t.Fatal("a check failure is not a rename failure")
	}
	if seen != "new" {
		t.Fatalf("check saw %q", seen)
	}
	if got, _ := os.ReadFile(path); string(got) != "old" || len(names(t, dir)) != 1 {
		t.Fatalf("old file or folder changed: %q %v", got, names(t, dir))
	}
}

func TestRenameFailureIsDistinguishableAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(path, []byte("x"), Options{Mode: 0o600})
	var re *RenameError
	if !errors.As(err, &re) {
		t.Fatalf("err %v, want RenameError", err)
	}
	if got := names(t, dir); len(got) != 1 || got[0] != "target" {
		t.Fatalf("files %v", got)
	}
}
