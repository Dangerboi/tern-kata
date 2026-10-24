package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func askedLeave(cmds []model.Cmd) (model.LeaveFederation, bool) {
	for _, cmd := range cmds {
		if leave, ok := cmd.(model.LeaveFederation); ok {
			return leave, true
		}
	}
	return model.LeaveFederation{}, false
}

// leavingPane is the Federation Page listing acme (project 5) and zeta (project 6), acting as dana.
func leavingPane(t *testing.T) driver.State {
	t.Helper()
	acme, zeta := healthySpoke("acme"), troubledSpoke("zeta")
	acme.ProjectID, zeta.ProjectID = 5, 6
	s := federationListed(t, acme, zeta)
	s.Model.Actor = "dana"
	return s
}

// leavePreviewed presses x on the selected spoke.
func leavePreviewed(t *testing.T, s driver.State) driver.State {
	t.Helper()
	out := step(t, s, key("x"))
	if out.State.Model.Federation.Screen != model.FederationLeavePreview || len(out.Cmds) != 0 {
		t.Fatalf("x gave screen %v and %#v, want the leave preview and no daemon call", out.State.Model.Federation.Screen, out.Cmds)
	}
	return out.State
}

func TestXOpensTheLeavePreviewOfTheSelectedSpokeAndSendsNothing(t *testing.T) {
	s := leavePreviewed(t, leavingPane(t))
	wantText(t, "the preview", wireTextOf(t, s), "Leave Federation", "local spoke project: acme", "hub: hub.example:7474", "hub project ID: 12",
		"local project: keep (detach) — standalone with all issues", "local-only: false",
		"will revoke the enrollment(s) on the hub, then detach locally (keep the project standalone)")
	if text := wireTextOf(t, s); strings.Contains(text, "skipping hub revoke") {
		t.Errorf("the default preview warns of a local-only leave: %s", text)
	}
	if mode := s.Model.CurrentMode(); mode != model.ModeFederationLeave {
		t.Errorf("mode %q, want the leave preview's", mode)
	}
	wantText(t, "the footer", footerText(t, s), "↵", "confirm", "d", "keep / archive", "l", "local-only", "back")

	second := leavePreviewed(t, step(t, leavingPane(t), key("j")).State)
	wantText(t, "the second spoke's preview", wireTextOf(t, second), "local spoke project: zeta", "hub project ID: 7")

	detail := step(t, leavingPane(t), key("enter")).State
	if got := leavePreviewed(t, detail); !strings.Contains(wireTextOf(t, got), "local spoke project: acme") {
		t.Error("x in a detail did not preview the shown spoke")
	}

	empty := federationListed(t)
	if got := step(t, empty, key("x")); got.State.Model.Federation.Screen != model.FederationList || len(got.Cmds) != 0 {
		t.Errorf("x on an empty list gave screen %v and %#v", got.State.Model.Federation.Screen, got.Cmds)
	}
}

func TestDAndLToggleTheLeaveAndTheFooterAndPreviewFollow(t *testing.T) {
	s := leavePreviewed(t, leavingPane(t))
	archive := step(t, s, key("d")).State
	wantText(t, "archiving", wireTextOf(t, archive), "local project: archive — remove the replica (reversible via kata projects restore)",
		"will revoke the enrollment(s) on the hub, then archive the local replica")
	if back := step(t, archive, key("d")).State; !strings.Contains(wireTextOf(t, back), "keep (detach)") {
		t.Error("a second d did not switch back to detaching")
	}

	local := step(t, s, key("l")).State
	wantText(t, "local-only", wireTextOf(t, local), "local-only: true",
		"local-only: skipping hub revoke; the enrollment token stays valid until manually revoked", "local teardown only: detach locally (keep the project standalone)")
	if strings.Contains(wireTextOf(t, local), "will revoke") {
		t.Error("a local-only preview still says it will revoke")
	}
	both := step(t, local, key("d")).State
	wantText(t, "local-only archive", wireTextOf(t, both), "local teardown only: archive the local replica")
	if off := step(t, both, key("l")).State; !strings.Contains(wireTextOf(t, off), "local-only: false") {
		t.Error("a second l did not turn local-only off")
	}
}

func TestEscAndBackspaceCancelThePreviewToTheListWithTheCursorKept(t *testing.T) {
	for _, leave := range []string{"escape", "backspace"} {
		s := leavePreviewed(t, step(t, leavingPane(t), key("j")).State)
		back := step(t, s, key(leave))
		if back.State.Model.Federation.Screen != model.FederationList || back.State.Model.Page != model.PageFederation || len(back.Cmds) != 0 {
			t.Errorf("%s gave screen %v on page %v with %#v", leave, back.State.Model.Federation.Screen, back.State.Model.Page, back.Cmds)
		}
		if back.State.Model.SelectedSpoke().ProjectName != "zeta" {
			t.Errorf("%s moved the cursor to %+v", leave, back.State.Model.SelectedSpoke())
		}
		// The next preview starts from the defaults again.
		if again := leavePreviewed(t, back.State); strings.Contains(wireTextOf(t, again), "local project: archive") {
			t.Errorf("%s: the next preview kept the earlier choice", leave)
		}
	}
	toggled := step(t, step(t, leavePreviewed(t, leavingPane(t)), key("l")).State, key("d")).State
	list := step(t, toggled, key("escape")).State
	if again := leavePreviewed(t, list); strings.Contains(wireTextOf(t, again), "local-only: true") {
		t.Error("a cancelled preview's toggles came back")
	}
}

func TestEnterRunsTheLeaveWithTheChoicesAndTheSpokesIdentity(t *testing.T) {
	s := step(t, leavePreviewed(t, leavingPane(t)), key("d")).State
	s = step(t, s, key("l")).State
	out := step(t, s, key("enter"))
	leave, ok := askedLeave(out.Cmds)
	if !ok {
		t.Fatalf("Enter asked %#v, want a leave", out.Cmds)
	}
	want := model.FederationLeaveDraft{ProjectID: 5, ProjectName: "acme", HubURL: "https://hub.example:7474", HubProjectID: 12, InstanceUID: "INSTANCE-1",
		Archive: true, Actor: "dana", LocalOnly: true}
	if leave.Workspace != "/repo" || leave.Draft != want {
		t.Errorf("leave %+v, want workspace /repo and %+v", leave, want)
	}
	if !out.State.Model.Federation.Leaving || out.State.Model.Federation.Screen != model.FederationLeavePreview {
		t.Error("the preview is not marked as running")
	}
	wantText(t, "while leaving", wireTextOf(t, out.State), "leaving...")

	// An insecure binding is passed on to the hub step.
	zeta := step(t, leavePreviewed(t, step(t, leavingPane(t), key("j")).State), key("enter"))
	if again, _ := askedLeave(zeta.Cmds); !again.Draft.AllowInsecure || again.Draft.ProjectID != 6 || again.Draft.Archive {
		t.Errorf("zeta's leave %+v, want project 6, insecure, detaching", again.Draft)
	}
}

func TestASecondEnterWhileTheLeaveRunsAsksNothingAndEscDropsItsAnswer(t *testing.T) {
	out := step(t, leavePreviewed(t, leavingPane(t)), key("enter"))
	leave, _ := askedLeave(out.Cmds)
	if again := step(t, out.State, key("enter")); len(again.Cmds) != 0 {
		t.Errorf("Enter while leaving asked %#v", again.Cmds)
	}

	cancelled := step(t, out.State, key("escape")).State
	late := step(t, cancelled, model.FederationLeft{Attempt: leave.Attempt, Outcome: model.FederationLeaveOutcome{Draft: leave.Draft}})
	if late.State.Model.Federation.Screen != model.FederationList || len(late.Cmds) != 0 {
		t.Errorf("an answer after esc gave screen %v and %#v, want the list and no read", late.State.Model.Federation.Screen, late.Cmds)
	}

	// The next run is a new attempt, and the earlier answer is still dropped.
	next := step(t, leavePreviewed(t, late.State), key("enter"))
	newer, _ := askedLeave(next.Cmds)
	if newer.Attempt == leave.Attempt {
		t.Fatal("two runs share an attempt number")
	}
	stale := step(t, next.State, model.FederationLeft{Attempt: leave.Attempt, Err: errors.New("old")})
	if stale.State.Model.Federation.LeaveErr != nil || !stale.State.Model.Federation.Leaving {
		t.Error("an answer to an older run changed the preview")
	}
}

func TestAFailedLeaveKeepsThePreviewWithKatasErrorAndEnterTriesAgain(t *testing.T) {
	for name, failure := range map[string]string{
		"preflight":   "spoke: leave preflight failed: archive refused: 2 open issues",
		"hub":         "hub revoke failed: unauthorized; retry with local-only to tear down locally, then revoke on the hub later",
		"foreign":     "no active enrollment matches this spoke's instance UID, but enrollment(s) #3, #4 still authorize the hub project — revoke the right one with `kata federation revoke ID` on the hub, or retry with local-only to tear down locally without revoking",
		"provider":    "provider cleanup cannot be discarded; turn off local-only and retry leave",
		"leave":       "spoke: leave failed: project has changed",
		"no instance": "spoke instance UID is not loaded; refresh federation status before leaving",
	} {
		s := step(t, leavePreviewed(t, leavingPane(t)), key("l")).State
		out := step(t, s, key("enter"))
		leave, _ := askedLeave(out.Cmds)
		failed := step(t, out.State, model.FederationLeft{Attempt: leave.Attempt, Err: errors.New(failure)})
		if failed.State.Model.Federation.Screen != model.FederationLeavePreview || failed.State.Model.Federation.Leaving || len(failed.Cmds) != 0 {
			t.Errorf("%s: screen %v, leaving %v, cmds %#v", name, failed.State.Model.Federation.Screen, failed.State.Model.Federation.Leaving, failed.Cmds)
		}
		text := wireTextOf(t, failed.State)
		wantText(t, name+" failure", text, failure, "local spoke project: acme", "local-only: true")
		if strings.Contains(text, "leaving...") {
			t.Errorf("%s: still marked as leaving", name)
		}

		retry := step(t, failed.State, key("enter"))
		again, ok := askedLeave(retry.Cmds)
		if !ok || again.Attempt == leave.Attempt || again.Draft != leave.Draft {
			t.Errorf("%s: Enter after the failure asked %#v, want the same leave as a new attempt", name, retry.Cmds)
		}
		if strings.Contains(wireTextOf(t, retry.State), failure) {
			t.Errorf("%s: the failure stayed on screen during the next run", name)
		}
	}
}

func TestALeaveThatSucceededShowsTheResultAndReadsTheStatusAgain(t *testing.T) {
	for name, c := range map[string]struct {
		outcome func(model.FederationLeaveDraft) model.FederationLeaveOutcome
		want    []string
		absent  []string
	}{
		"detached after revoking": {
			func(d model.FederationLeaveDraft) model.FederationLeaveOutcome {
				return model.FederationLeaveOutcome{Draft: d, Revoked: 2}
			},
			[]string{"Leave Result", "status: detached", "local spoke project: acme", "hub: hub.example:7474", "revoked 2 enrollment(s) on the hub"},
			[]string{"warning:", "skipped"},
		},
		"archived": {
			func(d model.FederationLeaveDraft) model.FederationLeaveOutcome {
				return model.FederationLeaveOutcome{Draft: d, Archived: true, Revoked: 1}
			},
			[]string{"status: archived", "revoked 1 enrollment(s) on the hub"}, nil,
		},
		"local-only": {
			func(d model.FederationLeaveDraft) model.FederationLeaveOutcome {
				return model.FederationLeaveOutcome{Draft: d, SkippedRevoke: true}
			},
			[]string{"hub revoke skipped (local-only): the enrollment token remains valid until manually revoked"}, []string{"revoked 0"},
		},
		"provider managed": {
			func(d model.FederationLeaveDraft) model.FederationLeaveOutcome {
				return model.FederationLeaveOutcome{Draft: d, ProviderReleased: true}
			},
			[]string{"credential provider confirmed release"}, []string{"revoked 0"},
		},
		"global enrollments": {
			func(d model.FederationLeaveDraft) model.FederationLeaveOutcome {
				return model.FederationLeaveOutcome{Draft: d, Revoked: 1, GlobalEnrollmentIDs: []int64{8, 11}}
			},
			[]string{"warning: global enrollment(s) #8, #11 for this spoke remain active and still authorize this project; revoke manually if intended"}, nil,
		},
	} {
		out := step(t, leavePreviewed(t, leavingPane(t)), key("enter"))
		leave, _ := askedLeave(out.Cmds)
		done := step(t, out.State, model.FederationLeft{Attempt: leave.Attempt, Outcome: c.outcome(leave.Draft)})
		if done.State.Model.Federation.Screen != model.FederationLeaveDone || done.State.Model.CurrentMode() != model.ModeFederationLeft {
			t.Fatalf("%s: screen %v, mode %q, want the result", name, done.State.Model.Federation.Screen, done.State.Model.CurrentMode())
		}
		read, asked := askedFederation(done.Cmds)
		if !asked || read.Workspace != "/repo" {
			t.Errorf("%s: the result asked %#v, want a status read", name, done.Cmds)
		}
		text := wireTextOf(t, done.State)
		wantText(t, name, text, c.want...)
		for _, absent := range c.absent {
			if strings.Contains(text, absent) {
				t.Errorf("%s: the result shows %q: %s", name, absent, text)
			}
		}
		wantText(t, name+" footer", footerText(t, done.State), "list")
	}
}

func TestEnterAndEscOnTheResultReturnToTheListWithoutTheLeftProject(t *testing.T) {
	for _, back := range []string{"enter", "escape", "backspace"} {
		out := step(t, leavePreviewed(t, leavingPane(t)), key("enter"))
		leave, _ := askedLeave(out.Cmds)
		done := step(t, out.State, model.FederationLeft{Attempt: leave.Attempt, Outcome: model.FederationLeaveOutcome{Draft: leave.Draft, Revoked: 1}})
		read, _ := askedFederation(done.Cmds)

		// The read answers while the result is still shown: it does not replace the result.
		answered := step(t, done.State, federationAnswer(read, troubledSpoke("zeta"))).State
		if answered.Model.Federation.Screen != model.FederationLeaveDone || !strings.Contains(wireTextOf(t, answered), "Leave Result") {
			t.Fatalf("%s: the status answer left the result: screen %v", back, answered.Model.Federation.Screen)
		}
		list := step(t, answered, key(back))
		if list.State.Model.Federation.Screen != model.FederationList || list.State.Model.Page != model.PageFederation || len(list.Cmds) != 0 {
			t.Errorf("%s: screen %v on page %v with %#v, want the list", back, list.State.Model.Federation.Screen, list.State.Model.Page, list.Cmds)
		}
		text := wireTextOf(t, list.State)
		wantText(t, back+" list", text, "1 spoke federations", "zeta  Hub")
		if strings.Contains(text, "acme") {
			t.Errorf("%s: the left Project is still listed: %s", back, text)
		}
	}
}

func TestLeavingTheFederationPageDuringALeaveDropsItsAnswer(t *testing.T) {
	out := step(t, leavePreviewed(t, leavingPane(t)), key("enter"))
	leave, _ := askedLeave(out.Cmds)
	// Page keys do nothing in the preview, so the way out is esc twice.
	list := step(t, out.State, key("escape")).State
	away := step(t, list, key("escape")).State
	if away.Model.Page != model.PageIssues {
		t.Fatalf("page %v, want Issues", away.Model.Page)
	}
	late := step(t, away, model.FederationLeft{Attempt: leave.Attempt, Outcome: model.FederationLeaveOutcome{Draft: leave.Draft}})
	if _, asked := askedFederation(late.Cmds); asked || late.State.Model.Page != model.PageIssues {
		t.Errorf("an answer after leaving the Page asked %#v on page %v", late.Cmds, late.State.Model.Page)
	}
}
