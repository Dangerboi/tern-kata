package driver_test

import (
	"testing"
)

func TestAFederationRunDoesNotStartWhileADaemonSwitchIsOut(t *testing.T) {
	daemons, _ := switchOut(t)

	enroll := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta))
	enroll.Model.Daemons = daemons
	out := step(t, enroll, key("enter"))
	if len(out.Cmds) != 0 || out.State.Model.Federation.Enrolling {
		t.Fatalf("Enter with a switch out asked %#v (enrolling %v), want no run", out.Cmds, out.State.Model.Federation.Enrolling)
	}
	wantShown(t, "the preview", out.State, "daemon switch is still connecting")

	leave := leavePreviewed(t, leavingPane(t))
	leave.Model.Daemons = daemons
	out = step(t, leave, key("enter"))
	if _, ok := askedLeave(out.Cmds); ok || out.State.Model.Federation.Leaving {
		t.Fatalf("Enter with a switch out asked %#v (leaving %v), want no run", out.Cmds, out.State.Model.Federation.Leaving)
	}
	wantShown(t, "the leave preview", out.State, "daemon switch is still connecting")
}
