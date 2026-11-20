package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// `p` on an archived job names what it would do were the job live: pause an enabled one, resume a disabled one.
func TestPOnAnArchivedJobRefusesWithTheVerbItWouldHaveUsed(t *testing.T) {
	for _, tc := range []struct {
		name string
		job  model.CronJob
		want string
	}{
		{"enabled", archived(guarded("J1", "Old", "EV1")), "Cannot pause the job: it is archived; restore it first."},
		{"disabled", archived(disabled(guarded("J1", "Old", "EV1"))), "Cannot resume the job: it is archived; restore it first."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := archivedJobsPane(t, tc.job)
			out := step(t, s, key("p"))
			if len(out.Cmds) != 0 || out.State.Model.Notice != tc.want {
				t.Errorf("cmds %#v, notice %q, want %q", out.Cmds, out.State.Model.Notice, tc.want)
			}
		})
	}
}
