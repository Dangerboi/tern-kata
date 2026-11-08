//go:build !windows

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// TryLock takes f exclusively without waiting. It returns ErrLocked when another lock holds it.
func TryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrLocked
	}
	return err
}

// Unlock drops f's lock.
func Unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
