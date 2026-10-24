package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// Page keys work from every non-text screen of the Federation Page, and wait for a write that is out.
func TestPageKeysWorkFromEveryFederationScreenButNotWhileAWriteIsOut(t *testing.T) {
	enrolled, run := running(t)
	leaving := step(t, leavePreviewed(t, leavingPane(t)), key("enter"))
	browsing := step(t, enrollPane(t), key("b"))
	recovered := step(t, enrolled, model.FederationEnrolled{Attempt: run.Attempt, Err: errors.New("join failed"), Recovery: recovery("join", false)}).State
	done := step(t, enrolled, model.FederationEnrolled{Attempt: run.Attempt}).State

	idle := map[string]driver.State{
		"leave preview":     leavePreviewed(t, leavingPane(t)),
		"local choice":      step(t, enrollPane(t), key("n")).State,
		"hub choice":        atHubStep(t, enrollPane(t), rowReplica),
		"hub Project":       atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta),
		"enroll preview":    previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta)),
		"browse":            browsing.State,
		"recovery":          recovered,
		"enrollment result": done,
	}
	for name, s := range idle {
		if got := step(t, s, key("1")).State.Model.Page; got != model.PageIssues {
			t.Errorf("%s: 1 leaves page %v, want the Issues Page", name, got)
		}
	}
	for name, s := range map[string]driver.State{"leave": leaving.State, "enrollment": enrolled} {
		out := step(t, s, key("1"))
		if out.State.Model.Page != model.PageFederation || len(out.Cmds) != 0 {
			t.Errorf("%s out: 1 gave page %v and %#v, want it to wait", name, out.State.Model.Page, out.Cmds)
		}
	}
}
