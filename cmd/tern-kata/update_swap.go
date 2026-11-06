package main

import (
	"os"
	"path/filepath"
	"strings"
)

// staleOldFiles lists the files beside old whose names start with old's base name and "-", the set swapBinary moves held .old
// binaries to. It matches the prefix itself so a glob character in the install path is taken literally.
func staleOldFiles(old string) []string {
	entries, err := os.ReadDir(filepath.Dir(old))
	if err != nil {
		return nil
	}
	prefix := filepath.Base(old) + "-"
	var stale []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			stale = append(stale, filepath.Join(filepath.Dir(old), entry.Name()))
		}
	}
	return stale
}
