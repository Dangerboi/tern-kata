package ompext

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A caller asks IsOmp first, then Argv: the table runs that composition on the program and args an agent starts with.
func TestTheExtensionArgsFollowTheProgramAndWhatTheAgentTrusts(t *testing.T) {
	const path = "/abs/tern-kata.ts"
	trusted := []string{"--trusted-extension", path}
	load := []string{"-e", path}
	for _, c := range []struct {
		name        string
		program     string
		args        []string
		trustsPaths bool
		want        []string
	}{
		{"plain omp", "omp", []string{"--fast"}, false, load},
		{"omp with no args", "omp", nil, false, load},
		{"omp by path", "/usr/local/bin/omp", nil, false, load},
		{"omp.exe", "omp.exe", nil, false, load},
		{"claude", "claude", []string{"--fast"}, false, nil},
		{"codex by path", "/usr/bin/codex", nil, false, nil},
		{"a wrapper named for omp", "omp-wrapper", nil, false, nil},
		{"no program", "", nil, false, nil},
		{"--trusted-extension in args", "omp", []string{"--trusted-extension", "/abs/guard.ts"}, false, trusted},
		{"--trusted-extension=path in args", "omp", []string{"--trusted-extension=/abs/guard.ts"}, false, trusted},
		{"an extensions list", "omp", []string{"--fast"}, true, trusted},
		{"--no-extensions keeps -e", "omp", []string{"--no-extensions"}, false, load},
		{"--no-extensions beside --trusted-extension", "omp", []string{"--no-extensions", "--trusted-extension", "/abs/guard.ts"}, false, trusted},
		{"--no-extensions beside an extensions list", "omp", []string{"--no-extensions"}, true, trusted},
	} {
		var got []string
		if IsOmp(c.program) {
			got = Argv(path, c.args, c.trustsPaths)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: extension args = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestInstallWritesTheExtensionOnlyWhenItsContentDiffers(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "omp")
	path, err := Install(folder)
	if err != nil || path != filepath.Join(folder, FileName) {
		t.Fatalf("path %q, %v", path, err)
	}
	if text, err := os.ReadFile(path); err != nil || !bytes.Equal(text, Source) {
		t.Fatalf("the file holds %d bytes, %v; want the embedded %d", len(text), err, len(Source))
	}
	// A current file is not rewritten: a read-only copy of the same content still installs.
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	_, err = Install(folder)
	_ = os.Chmod(folder, 0o700)
	if err != nil && os.Geteuid() != 0 {
		t.Fatalf("a current file was written again: %v", err)
	}
	// An older tern-kata's extension is replaced.
	if err := os.WriteFile(path, []byte("export default function () {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := Install(folder); err != nil || again != path {
		t.Fatalf("again %q, %v", again, err)
	}
	if text, err := os.ReadFile(path); err != nil || !bytes.Equal(text, Source) {
		t.Fatalf("after a second install the file holds %q, %v", text, err)
	}
}
