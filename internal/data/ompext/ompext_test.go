package ompext

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTheExtensionFlagFollowsWhatTheAgentTrusts(t *testing.T) {
	const path = "/abs/tern-kata.ts"
	trusted := []string{"--trusted-extension", path}
	for _, c := range []struct {
		name        string
		args        []string
		trustsPaths bool
		want        []string
	}{
		{"plain omp", []string{"--fast"}, false, []string{"-e", path}},
		{"no args", nil, false, []string{"-e", path}},
		{"--trusted-extension in args", []string{"--trusted-extension", "/abs/guard.ts"}, false, trusted},
		{"--trusted-extension=path in args", []string{"--trusted-extension=/abs/guard.ts"}, false, trusted},
		{"an extensions list", []string{"--fast"}, true, trusted},
		{"--no-extensions keeps -e", []string{"--no-extensions"}, false, []string{"-e", path}},
		{"--no-extensions beside --trusted-extension", []string{"--no-extensions", "--trusted-extension", "/abs/guard.ts"}, false, trusted},
		{"--no-extensions beside an extensions list", []string{"--no-extensions"}, true, trusted},
	} {
		if got := Argv(path, c.args, c.trustsPaths); !slices.Equal(got, c.want) {
			t.Errorf("%s: Argv = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOnlyOmpIsOmp(t *testing.T) {
	for program, want := range map[string]bool{
		"omp": true, "/opt/omp/omp": true, "omp.exe": true,
		"claude": false, "omp-wrapper": false, "": false,
	} {
		if got := IsOmp(program); got != want {
			t.Errorf("IsOmp(%q) = %v, want %v", program, got, want)
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
