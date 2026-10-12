//go:build !windows

package data

import (
	"os"
	"path/filepath"
	"strings"
)

func osZone() string { return unixZone("/") }

// unixZone reads the system zone under root: the whole /etc/localtime link chain, whose end sits after "zoneinfo/", else /etc/timezone.
func unixZone(root string) string {
	if target, err := filepath.EvalSymlinks(filepath.Join(root, "etc/localtime")); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			return zone
		}
	}
	raw, _ := os.ReadFile(filepath.Join(root, "etc/timezone"))
	return strings.TrimSpace(string(raw))
}
