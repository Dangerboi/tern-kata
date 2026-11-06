package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStaleOldFilesMatchesInAFolderWithGlobCharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin[1]")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "tern-kata.exe.old")
	for _, name := range []string{"tern-kata.exe.old-1", "tern-kata.exe.old-22", "tern-kata.exe.old", "tern-kata.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := staleOldFiles(old)
	want := []string{filepath.Join(dir, "tern-kata.exe.old-1"), filepath.Join(dir, "tern-kata.exe.old-22")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("staleOldFiles = %q, want %q", got, want)
	}
}
