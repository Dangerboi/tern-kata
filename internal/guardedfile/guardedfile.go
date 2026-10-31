// Package guardedfile reads a local file with a token and writes it only while it still matches that token, so a write made
// from an old read cannot overwrite another writer's change.
package guardedfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/filelock"
)

// ErrChanged is a write finding the file no longer as the read it was given. Nothing was written; read again.
var ErrChanged = errors.New("file changed since it was read")

// Token is what a read saw: the file's text, empty for a missing file. Callers treat it as opaque.
type Token struct{ text string }

// TokenOf is the token of a file holding text.
func TokenOf(text string) Token { return Token{text: text} }

// Text is the file's text at the read.
func (t Token) Text() string { return t.text }

// Read reads path. A missing file reads as empty text. Any other failure is the error, with no token, so nothing can write
// from it.
func Read(path string) (Token, error) {
	text, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Token{}, err
	}
	return Token{text: string(text)}, nil
}

// lockWait bounds how long a write waits for another writer's lock before giving up with an error.
const lockWait = 10 * time.Second

// Write replaces path with next, creating the file and its folder (with dirMode) when missing, unless the file is no longer
// the text the token holds: then it writes nothing and returns ErrChanged. The compare and the rename happen under an
// exclusive lock on path+".lock", so another process writing through this package cannot slip between them; a lock held
// longer than lockWait is an error. A writer that bypasses this package is not excluded.
func Write(path string, dirMode fs.FileMode, token Token, next []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := filelock.Wait(lock, lockWait); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	defer filelock.Unlock(lock)
	return atomicfile.Write(path, next, atomicfile.Options{Mode: 0o600, DirMode: dirMode, Check: func(string) error {
		disk, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			disk = nil
		case err != nil:
			return err
		}
		if string(disk) != token.text {
			return ErrChanged
		}
		return nil
	}})
}
