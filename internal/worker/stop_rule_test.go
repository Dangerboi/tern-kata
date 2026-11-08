package worker

import (
	"errors"
	"testing"
)

// A stopping Worker refuses the commands that would start or continue work and answers the rest.
func TestCommandsRefusedWhileStopping(t *testing.T) {
	for _, c := range []struct {
		command, mode string
		refused       bool
	}{
		{cmdRun, "", false},
		{cmdRun, StopIdle, true},
		{cmdRun, StopNow, true},
		{cmdResume, "", false},
		{cmdResume, StopIdle, true},
		{cmdResume, StopNow, true},
		{cmdAbandon, "", false},
		{cmdAbandon, StopIdle, false},
		{cmdAbandon, StopNow, true},
		{cmdStatus, StopNow, false},
		{cmdRunDetail, StopNow, false},
		{cmdStop, StopNow, false},
		{cmdIdentity, StopNow, false},
	} {
		err := refusedWhileStopping(c.command, c.mode)
		if c.refused != (err != nil) || err != nil && !errors.Is(err, errStopping) {
			t.Errorf("%s during mode %q: got %v, want refused=%v", c.command, c.mode, err, c.refused)
		}
	}
}
