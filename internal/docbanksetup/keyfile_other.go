//go:build !unix

package docbanksetup

import "io/fs"

// Windows has no owner or permission bits to compare here; kata checks the file's access list itself.
func ownedByCurrentUser(fs.FileInfo) bool { return true }

func groupOrWorldAccessible(fs.FileInfo) bool { return false }
