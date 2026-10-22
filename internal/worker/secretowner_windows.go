//go:build windows

package worker

import "io/fs"

// effectiveUID has no meaning on Windows, where files carry no owner id and no mode bits to check.
var effectiveUID = func() int { return 0 }

// foreignOwned is false on Windows: access to a secret file is the folder's ACL, which the worker does not read.
func foreignOwned(fs.FileInfo) bool { return false }
