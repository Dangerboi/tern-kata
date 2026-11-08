// Package filelock is an exclusive advisory lock on an open file, held by this process until released and dropped by the
// operating system when the process ends. Separate opens of one file contend, in one process or across processes.
package filelock

import (
	"errors"
	"os"
	"time"
)

var ErrLocked = errors.New("locked")

var ErrTimeout = errors.New("timed out waiting for a file lock")

const pollEvery = 5 * time.Millisecond

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
