// Package filelock is an exclusive advisory lock on an open file, held by this process until released and dropped by the
// operating system when the process ends. Separate opens of one file contend, in one process or across processes.
package filelock

import (
	"errors"
	"os"
	"time"
)

// ErrLocked is a try-lock finding the file held by another lock.
var ErrLocked = errors.New("locked")

// ErrTimeout is Wait giving up because the file stayed held.
var ErrTimeout = errors.New("timed out waiting for a file lock")

const pollEvery = 5 * time.Millisecond

// Wait takes f exclusively, retrying until timeout has passed and then returning ErrTimeout.
func Wait(f *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := TryLock(f)
		if !errors.Is(err, ErrLocked) {
			return err
		}
		if !time.Now().Before(deadline) {
			return ErrTimeout
		}
		time.Sleep(pollEvery)
	}
}
