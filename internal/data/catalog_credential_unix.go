//go:build unix

package data

import (
	"errors"
	"os"
	"syscall"
)

var errWrongOwner = errors.New("credential file owned by another user")

// openCredentialFile opens path as kata does: without blocking on a pipe, and only when the daemon's user owns it.
func openCredentialFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int64(st.Uid) != int64(os.Geteuid()) {
		_ = f.Close()
		return nil, errWrongOwner
	}
	return f, nil
}

// privateCredentialFile is whether no group or other bits are set.
func privateCredentialFile(info os.FileInfo) bool { return info.Mode().Perm()&0o077 == 0 }
