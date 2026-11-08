//go:build unix

package docbanksetup

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByCurrentUser reports whether info says the file belongs to the effective user, which is how kata checks a secret file.
func ownedByCurrentUser(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return !ok || int64(st.Uid) == int64(os.Geteuid())
}

// groupOrWorldAccessible reports whether any group or other permission bit is set.
func groupOrWorldAccessible(info fs.FileInfo) bool { return info.Mode().Perm()&0o077 != 0 }
