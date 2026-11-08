package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestAnAnswerListingCredentialsDaemonsOrConfigFilesRevealsNoRow(t *testing.T) {
	for name, tc := range map[string]struct {
		s   driver.State
		msg model.Msg
	}{
		"Credentials": {tokenList(t), model.CredentialsLoaded{}},
		"Daemons":     {daemonsListed(t), model.DaemonsLoaded{}},
		"Config":      {configListed(t), model.ConfigFilesLoaded{}},
	} {
		for _, op := range step(t, tc.s, tc.msg).Ops {
			if op[0] == "reveal" {
				t.Errorf("%s: the list answer sent %v, a reveal of a row Tern does not know yet", name, op)
			}
		}
	}
}

// switchedFrom is the Pane after a switch to home, and the Connection the Pane had before it.
func switchedFrom(t *testing.T, s driver.State) (driver.Output, int) {
	t.Helper()
	before := s.Model.Connection
	return switchedTo(t, s, model.Daemon{Name: "home"}), before
}

func TestTheProjectOfADaemonSwitchedAwayFromIsDropped(t *testing.T) {
	out, before := switchedFrom(t, daemonsListed(t))
	late := step(t, out.State, model.ProjectLoaded{Connection: before, Project: model.Project{ID: 9, UID: "old", Name: "old"}, Daemon: model.Daemon{Name: "work"}})
	m := late.State.Model
	if m.Daemon.Name != "home" || m.Project.UID != "" || len(late.Cmds) != 0 {
		t.Errorf("the old daemon's Project landed: daemon %+v, project %+v, cmds %#v", m.Daemon, m.Project, late.Cmds)
	}
	load, _ := askedProject(out.Cmds)
	now := step(t, out.State, model.ProjectLoaded{Connection: load.Connection, Project: model.Project{ID: 9, UID: "new", Name: "new"}, Daemon: model.Daemon{Name: "home"}})
	if now.State.Model.Project.UID != "new" {
		t.Error("the new daemon's Project was dropped")
	}
}

func TestAClearStartedOnADaemonSwitchedAwayFromTouchesNothing(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	clear := askedClear(step(t, s, key("c")).Cmds)[0]
	out := switchedAnswer(t, step(t, s, key("c")).State)
	reloaded := step(t, out.State, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}, Connection: out.State.Model.Connection})
	late := step(t, reloaded.State, model.AttentionCleared{Connection: clear.Connection, UID: clear.UID})
	if len(late.Cmds) != 0 || late.State.Model.Notice != reloaded.State.Model.Notice {
		t.Errorf("the old daemon's clear answer changed the Pane: %#v, notice %q", late.Cmds, late.State.Model.Notice)
	}
}

func TestATokenCreatedOnADaemonSwitchedAwayFromDoesNotMarkTheNewCardCreated(t *testing.T) {
	out := step(t, validToken(t), key("enter"))
	create, _ := askedCreateToken(out.Cmds)
	switched := switchedAnswer(t, out.State).State
	s := step(t, switched, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}, Connection: switched.Model.Connection}).State
	s.Model.TokenForm.Open, s.Model.TokenForm.Saving = true, true
	late := step(t, s, model.TokenCreated{Connection: create.Connection, ID: 11, File: "/secrets/ci.token"})
	if f := late.State.Model.TokenForm; f.Done || !f.Saving {
		t.Errorf("the old daemon's token answer marked the new card: %+v", f)
	}
}
