package worker

import (
	"os"
	"path/filepath"
	"strings"
)

// CheckConfig runs worker.toml's strict load over content without touching worker.toml. The error names the problem
// the way LoadConfig does, with the file called worker.toml.
func CheckConfig(content string) error {
	dir, err := os.MkdirTemp("", "tern-kata-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "worker.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	if _, err := LoadConfig(path); err != nil {
		return stripPath(err, path)
	}
	return nil
}

type pathless struct{ msg string }

func (e pathless) Error() string { return e.msg }

func stripPath(err error, path string) error {
	return pathless{strings.ReplaceAll(err.Error(), path, "worker.toml")}
}
