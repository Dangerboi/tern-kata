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
		{OpRun, "", false},
		{OpRun, StopIdle, true},
		{OpRun, StopNow, true},
		{OpResume, "", false},
		{OpResume, StopIdle, true},
		{OpResume, StopNow, true},
		{OpAbandon, "", false},
		{OpAbandon, StopIdle, false},
		{OpAbandon, StopNow, true},
		{OpStatus, StopNow, false},
		{OpRunDetail, StopNow, false},
		{OpStop, StopNow, false},
		{OpIdentity, StopNow, false},
	} {
		err := refusedWhileStopping(c.command, c.mode)
		if c.refused != (err != nil) || err != nil && !errors.Is(err, errStopping) {
			t.Errorf("%s during mode %q: got %v, want refused=%v", c.command, c.mode, err, c.refused)
		}
	}
}
