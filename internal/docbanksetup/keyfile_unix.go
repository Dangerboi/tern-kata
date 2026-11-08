//go:build unix

package docbanksetup

import (
	"io/fs"
	"os"
	"syscall"
)

// privateWords is how setup names the access a key file it writes gets.
const privateWords = "mode 0600"

// privateProblem is kata's reason to refuse an open key file on Unix: not owned by the effective user, or any group or other
// permission bit set; empty when kata accepts it.
func privateProblem(_ *os.File, info fs.FileInfo) string {
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int64(st.Uid) != int64(os.Geteuid()) {
		return "must be owned by the daemon user"
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "group/world-readable; use chmod 600"
	}
	return ""
}
