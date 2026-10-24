//go:build !unix

package data

import (
	"errors"
	"os"
)

var errWrongOwner = errors.New("credential file owned by another user")

// openCredentialFile opens path; kata's ownership and private-file checks of Windows are not mirrored here.
func openCredentialFile(path string) (*os.File, error) { return os.Open(path) }

func privateCredentialFile(os.FileInfo) bool { return true }
