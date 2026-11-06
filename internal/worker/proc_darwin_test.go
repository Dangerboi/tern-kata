//go:build darwin

package worker

import (
	"os"
	"testing"
)

func TestStartTicksOfALiveProcessAreStable(t *testing.T) {
	a := startTicks(os.Getpid())
	if a == 0 || a != startTicks(os.Getpid()) {
		t.Fatalf("startTicks = %d", a)
	}
}
