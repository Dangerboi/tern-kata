package worker

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A process that exits while a descendant still holds its output pipes makes Wait return ErrWaitDelay; the exit is the
// process's own, so a clean exit is recorded as code 0 and a failure keeps its code.
func TestAnExitWithADescendantHoldingTheOutputIsRecordedByItsOwnStatus(t *testing.T) {
	for _, tc := range []struct {
		script string
		want   int
	}{
		{"sleep 5 & exit 0", 0},
		{"sleep 5 & exit 3", 3},
	} {
		t.Run(tc.script, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", tc.script)
			cmd.WaitDelay = 100 * time.Millisecond
			var out strings.Builder
			cmd.Stdout = &out // a Go writer: Wait copies through a pipe the descendant keeps open
			err := cmd.Run()
			if code, signal := exitStatus(cmd, err); code != tc.want || signal != "" {
				t.Fatalf("exit = %d %q, want %d (wait error %v)", code, signal, tc.want, err)
			}
		})
	}
}
