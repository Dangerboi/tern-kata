// Package atomicfile replaces a file by writing a temporary file beside it and renaming that over the target, so a failed
// write leaves the old file untouched and no temporary file behind.
//
// It is deliberately not durable: it never syncs the file or its folder, so after a power loss the old or the new file may
// be found, and a rename that returned may not have reached the disk. The worker journal's save (ADR-0006) needs the
// stronger contract (fsync, rename, folder sync, with its own handling of Windows) and stays separate in
// internal/worker; do not merge the two.
package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// renameAttempts and renameDelay bound the Windows rename retry to about one second.
const (
	renameAttempts = 20
	renameDelay    = 50 * time.Millisecond
)

func retryRename(rename func() error, retryable func(error) bool) error {
	err := rename()
	for i := 1; err != nil && i < renameAttempts && retryable(err); i++ {
		time.Sleep(renameDelay)
		err = rename()
	}
	return err
}

type Options struct {
	// Mode is the file's mode. It is not applied on Windows, which has no such modes.
	Mode fs.FileMode
	// DirMode, when non-zero, creates the target's folder (and parents) with this mode when it is missing.
	DirMode fs.FileMode
	// Check, when set, runs on the finished temporary file before the rename; an error from it stops the write and is
	// returned as it is.
	Check func(tmp string) error
}

// Write puts data at path. On any failure the temporary file is removed and the old file is untouched; after a failed rename the
// old file may still be there, or another writer may have put its own file there first.
func Write(path string, data []byte, opts Options) error {
	dir := filepath.Dir(path)
	if opts.DirMode != 0 {
		if err := os.MkdirAll(dir, opts.DirMode); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := fill(tmp, data, opts.Mode); err != nil {
		_ = os.Remove(name)
		return err
	}
	if opts.Check != nil {
		if err := opts.Check(name); err != nil {
			_ = os.Remove(name)
			return err
		}
	}
	if err := rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func fill(tmp *os.File, data []byte, mode fs.FileMode) error {
	var err error
	if filepath.Separator == '/' {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		_, err = tmp.Write(data)
	}
	return errors.Join(err, tmp.Close())
}
