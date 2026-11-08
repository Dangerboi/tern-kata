//go:build !unix

package docbanksetup

import (
	"io/fs"
	"os"

	"go.kenn.io/kit/safefileio"
)

// privateWords is how setup names the access a key file it writes gets.
const privateWords = "readable only by you, with no inherited access"

// privateProblem is kata's reason to refuse an open key file on Windows: an access list that is not protected from inheritance
// or that admits anyone but the current user, the system and administrators; empty when kata accepts it.
func privateProblem(f *os.File, _ fs.FileInfo) string {
	if safefileio.ValidatePrivateCurrentUserFile(f) != nil {
		return "must be a private current-user file"
	}
	return ""
}
