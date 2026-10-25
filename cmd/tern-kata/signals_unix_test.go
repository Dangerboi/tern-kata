//go:build unix

package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestASignalEndsTheCommandsContextAndNamesItself(t *testing.T) {
	ctx, received, stop := withSignals(context.Background(), syscall.SIGHUP)
	defer stop()
	if received() != nil {
		t.Fatal("a signal reported before any came")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGHUP did not end the context")
	}
	if got := received(); got != syscall.SIGHUP {
		t.Fatalf("received %v, want hangup", got)
	}
	if status := exitStatus(interrupted{received(), ctx.Err()}); status != 129 {
		t.Fatalf("exit %d, want 129 (128 + SIGHUP)", status)
	}
}
