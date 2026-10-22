package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ErrWorkerRunning is what a second worker on the same machine gets (ADR-0006: one worker, one worker.lock).
var ErrWorkerRunning = errors.New("another tern-kata worker is already running on this machine")

// Lock is the held worker.lock. The operating system releases it when the process ends, however it ends.
type Lock struct{ file *os.File }

// AcquireLock takes path exclusively without waiting and records this process id in it for `worker status`.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		holder, _ := os.ReadFile(path)
		_ = file.Close()
		if errors.Is(err, errLocked) {
			if len(holder) > 0 {
				return nil, fmt.Errorf("%w (process %s)", ErrWorkerRunning, string(holder))
			}
			return nil, ErrWorkerRunning
		}
		return nil, err
	}
	if err := file.Truncate(0); err == nil {
		_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	return &Lock{file: file}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	unlockFile(l.file)
	return l.file.Close()
}
