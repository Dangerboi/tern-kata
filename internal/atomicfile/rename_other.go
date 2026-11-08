//go:build !windows

package atomicfile

import "os"

// rename moves oldpath over newpath once; only Windows needs retries.
var rename = os.Rename
