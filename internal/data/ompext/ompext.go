// Package ompext holds tern-kata's omp extension, tern-kata.ts, and the one rule for loading it: the Pane's agents and the
// worker's runs both ask here whether a program is omp, which flag loads the extension, and where the file goes. Its Bun
// tests sit beside it.
package ompext

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
)

// Source is tern-kata.ts: in an omp agent it names the session to `kata close` (KATA_TRANSCRIPT_AGENT and
// KATA_TRANSCRIPT_SESSION_ID) and, given an Agent address, delivers the messages written to it.
//
//go:embed tern-kata.ts
var Source []byte

// FileName is the name the extension is written under; omp loads it by path.
const FileName = "tern-kata.ts"

// Program is the name of omp, the only program the extension is for.
const Program = "omp"

// IsOmp is true when program (a name or a path, with or without .exe) is omp.
func IsOmp(program string) bool {
	base := filepath.Base(program)
	return strings.TrimSuffix(base, filepath.Ext(base)) == Program
}

// Argv is the omp flag and path that load the extension at path. omp refuses -e beside --trusted-extension, so an agent that
// trusts paths (args hold --trusted-extension, or the caller's `extensions` list does: trustsPaths) gets one more of those; omp
// keeps either under --no-extensions. omp refuses a relative trusted path, so path must be absolute.
func Argv(path string, args []string, trustsPaths bool) []string {
	if trustsPaths || slices.ContainsFunc(args, func(a string) bool {
		flag, _, _ := strings.Cut(a, "=")
		return flag == "--trusted-extension"
	}) {
		return []string{"--trusted-extension", path}
	}
	return []string{"-e", path}
}

// Install puts the extension in folder and returns its absolute path. A file of other content (an older tern-kata's) is
// replaced; one already current is left alone. An agent already running keeps the extension it loaded.
func Install(folder string) (string, error) {
	path, err := filepath.Abs(filepath.Join(folder, FileName))
	if err != nil {
		return "", err
	}
	if held, err := os.ReadFile(path); err == nil && bytes.Equal(held, Source) {
		return path, nil
	}
	if err := atomicfile.Write(path, Source, atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return "", err
	}
	return path, nil
}
