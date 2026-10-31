package worker

import (
	"errors"
	"testing"
)

// One table says what every worker command does in every stop mode.
func TestEveryCommandAgainstEveryStopMode(t *testing.T) {
	commands := []string{cmdStatus, cmdRunDetail, cmdStop, cmdRun, cmdResume, cmdAbandon}
	// refused[command] lists the stop modes ("" is serving) in which the command is refused for stopping.
	refused := map[string][]string{
		cmdStatus:    nil,
		cmdRunDetail: nil,
		cmdStop:      nil,
		cmdRun:       {StopIdle, StopNow},
		cmdResume:    {StopIdle, StopNow},
		cmdAbandon:   {StopNow},
	}
	for _, command := range commands {
		for _, mode := range []string{"", StopIdle, StopNow} {
			want := false
			for _, m := range refused[command] {
				want = want || m == mode
			}
			err := refusedWhileStopping(command, mode)
			if want != (err != nil) || err != nil && !errors.Is(err, errStopping) {
				t.Errorf("%s during mode %q: refused with %v, want refused=%v with %q", command, mode, err, want, errStopping)
			}
		}
	}
}
