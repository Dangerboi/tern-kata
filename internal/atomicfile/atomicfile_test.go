package atomicfile

import (
	"errors"
	"testing"
)

func TestRetryRenameRetriesRetryableErrorsThenSucceeds(t *testing.T) {
	retryable := errors.New("busy")
	calls := 0
	err := retryRename(func() error {
		calls++
		if calls < 3 {
			return retryable
		}
		return nil
	}, func(err error) bool { return errors.Is(err, retryable) })
	if err != nil || calls != 3 {
		t.Fatalf("err = %v, calls = %d, want nil after 3", err, calls)
	}
}

func TestRetryRenameGivesUpAfterBound(t *testing.T) {
	retryable := errors.New("busy")
	calls := 0
	err := retryRename(func() error { calls++; return retryable }, func(error) bool { return true })
	if !errors.Is(err, retryable) || calls != renameAttempts {
		t.Fatalf("err = %v, calls = %d, want %d attempts", err, calls, renameAttempts)
	}
}

func TestRetryRenameDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	boom := errors.New("boom")
	err := retryRename(func() error { calls++; return boom }, func(error) bool { return false })
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err = %v, calls = %d, want one attempt", err, calls)
	}
}
