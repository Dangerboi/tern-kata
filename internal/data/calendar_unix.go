//go:build !windows

package data

import (
	"os"
	"path/filepath"
	"strings"
)

func osZone() string { return unixZone("/") }

// unixZone reads the system zone under root: the /etc/localtime link target after "zoneinfo/", else /etc/timezone.
func unixZone(root string) string {
	if target, err := os.Readlink(filepath.Join(root, "etc/localtime")); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			return zone
		}
	}
	raw, _ := os.ReadFile(filepath.Join(root, "etc/timezone"))
	return strings.TrimSpace(string(raw))
}
