//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION, which package syscall does not export.
const errorSharingViolation syscall.Errno = 32

// rename moves oldpath over newpath, retrying while another process holds the target open. Go opens files on Windows
// without FILE_SHARE_DELETE, so a reader of the target makes the rename fail with access denied or a sharing violation
// until it closes the file.
func rename(oldpath, newpath string) error {
	return retryRename(func() error { return os.Rename(oldpath, newpath) }, isSharingError)
}

func isSharingError(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errorSharingViolation)
}
