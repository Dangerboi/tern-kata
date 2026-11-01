package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// renameFile is os.Rename; a test replaces it to make one rename fail.
var renameFile = os.Rename

// swapBinary replaces the running target on Windows, where a running .exe can be renamed but not overwritten or deleted. It
// removes target.old, or renames it aside to target.old-<unix nanoseconds> when a running process still holds it, renames
// target to .old and fresh to target, and renames .old back when that last rename fails. It then deletes the .old-* files that
// no process holds; the others stay for a later update.
func swapBinary(target, fresh string) error {
	old := target + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		aside := fmt.Sprintf("%s-%d", old, time.Now().UnixNano())
		if err := renameFile(old, aside); err != nil {
			return fmt.Errorf("cannot move the held %s aside: %w", old, err)
		}
	}
	if err := renameFile(target, old); err != nil {
		return err
	}
	if err := renameFile(fresh, target); err != nil {
		if back := renameFile(old, target); back != nil {
			return fmt.Errorf("%w; and %s could not be restored: %v", err, target, back)
		}
		return err
	}
	stale, _ := filepath.Glob(old + "-*")
	for _, path := range stale {
		_ = os.Remove(path)
	}
	return nil
}
