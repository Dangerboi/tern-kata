//go:build !windows

package atomicfile

import "os"

// rename moves oldpath over newpath once; only Windows needs retries.
func rename(oldpath, newpath string) error {
	return retryRename(func() error { return os.Rename(oldpath, newpath) }, func(error) bool { return false })
}
