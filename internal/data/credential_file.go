package data

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.kenn.io/kit/safefileio"
)

// readTokenFile is the token in a credential file, checked as kata checks one: an absolute (or ~/) path, owned by the current user,
// a regular file closed to group and others, at most 64 KiB, not empty. A failure is worded as kata words it. With optional set, a
// file that does not exist holds no token.
func readTokenFile(path string, optional bool) (string, error) {
	if optional {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
	}
	fail := func(why string) (string, error) {
		return "", fmt.Errorf("selected credential is unavailable: file:%s: %s", path, why)
	}
	resolved := path
	if resolved == "~" || strings.HasPrefix(resolved, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fail("cannot resolve home")
		}
		resolved = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(resolved, "~"), "/"))
	}
	if !filepath.IsAbs(resolved) {
		return fail("must use an absolute path or ~/ path")
	}
	f, err := safefileio.OpenCurrentUserFile(resolved)
	if err != nil {
		return fail("cannot read credential file")
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fail("not a readable regular file")
	}
	if runtime.GOOS == "windows" {
		if err := safefileio.ValidatePrivateCurrentUserFile(f); err != nil {
			return fail("must be a private current-user file")
		}
	} else if info.Mode().Perm()&0o077 != 0 {
		return fail("group/world-readable; use chmod 600")
	}
	if info.Size() > 64<<10 {
		return fail("exceeds 64 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, 64<<10+1))
	if err != nil || len(data) > 64<<10 {
		return fail("cannot read credential file")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return fail("credential file is empty")
	}
	return token, nil
}
