package data

import (
	"errors"
	"fmt"
	"os"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
)

// writePrivateFile puts secret in file for its owner alone (as kata creates its own credential files), written to a temporary
// file that is synced and renamed into place: a kill part way leaves no file, never an empty one a reader would take as written.
func writePrivateFile(file, secret string) error {
	if err := atomicfile.Write(file, []byte(secret+"\n"), atomicfile.Options{Mode: 0o600, Private: true, Check: syncFile}); err != nil {
		return fmt.Errorf("write private file %s: %w", file, err)
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
