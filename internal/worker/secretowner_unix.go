//go:build !windows

package worker

import (
	"io/fs"
	"os"
	"syscall"
)

// effectiveUID is the user the worker runs as; a test may replace it to stand for another user.
var effectiveUID = os.Geteuid

// foreignOwned reports whether the file belongs to a user other than the one the worker runs as (ADR-0010: a secret
// file is owned by the user).
func foreignOwned(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) != effectiveUID()
}
