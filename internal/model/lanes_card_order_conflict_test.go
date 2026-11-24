package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// sharedLanesWrite opens the Lanes card on a Project sharing triage, review at revision 4 and sends its share.
func sharedLanesWrite(t *testing.T) (model.Model, model.ShareLanes) {
	t.Helper()
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, _ = clickPress(m, "b")
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest(), Lanes: []string{"triage", "review"}, DoneDays: 7, Shared: true, ProjectID: 7, Revision: 4})
	m, _ = clickPress(m, "shift+b", "ctrl+u")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "triage, qa"})
	m, cmds := clickPress(m, "enter")
	if len(cmds) != 1 {
		t.Fatalf("saving sent %#v, want one shared write", cmds)
	}
	share, ok := cmds[0].(model.ShareLanes)
	if !ok {
		t.Fatalf("saving sent %#v, want a shared write", cmds[0])
	}
	return m, share
}

func TestAConflictFromAnUnchangedSharedListSendsTheShareAgainOnTheNewRevision(t *testing.T) {
	m, share := sharedLanesWrite(t)
	current := model.LanesLoaded{ProjectUID: "p", Shared: true, Lanes: []string{"triage", "review"}, DoneDays: 7, ProjectID: 7, Revision: 6}
	m, cmds := model.Update(m, model.LanesShared{Share: share, Conflict: true, Current: current})
	want := share
	want.Revision = 6
	if len(cmds) != 1 || !reflect.DeepEqual(cmds[0], want) {
		t.Fatalf("a conflict over an unchanged list sent %#v, want %#v", cmds, want)
	}
	if m.TopCard() != model.CardLanes || !m.LanesForm.Saving || m.LanesForm.Err != nil || m.LanesForm.Revision != 6 {
		t.Fatalf("card %v saving %v error %v revision %d, want the card open and saving with no conflict on revision 6",
			m.TopCard(), m.LanesForm.Saving, m.LanesForm.Err, m.LanesForm.Revision)
	}
	m, _ = model.Update(m, model.LanesShared{Share: want, Revision: 7})
	if m.TopCard() != model.NoCard || !reflect.DeepEqual(m.Board.Lanes, []string{"triage", "qa"}) {
		t.Fatalf("after the second write card %v lanes %v, want it closed on the new list", m.TopCard(), m.Board.Lanes)
	}
}

func TestASecondConflictOverAnUnchangedListShowsTheConflict(t *testing.T) {
	m, share := sharedLanesWrite(t)
	current := model.LanesLoaded{ProjectUID: "p", Shared: true, Lanes: []string{"triage", "review"}, DoneDays: 7, ProjectID: 7, Revision: 6}
	m, _ = model.Update(m, model.LanesShared{Share: share, Conflict: true, Current: current})
	again := share
	again.Revision = 6
	current.Revision = 8
	m, cmds := model.Update(m, model.LanesShared{Share: again, Conflict: true, Current: current})
	if len(cmds) != 0 || m.LanesForm.Saving || m.LanesForm.Err == nil || m.LanesForm.Revision != 8 {
		t.Fatalf("second conflict sent %#v saving %v error %v revision %d, want the conflict shown on revision 8",
			cmds, m.LanesForm.Saving, m.LanesForm.Err, m.LanesForm.Revision)
	}
}

func TestAConflictFromAChangedSharedListShowsTheConflict(t *testing.T) {
	for name, current := range map[string]model.LanesLoaded{
		"lanes":      {ProjectUID: "p", Shared: true, Lanes: []string{"triage", "ship"}, DoneDays: 7, ProjectID: 7, Revision: 6},
		"done limit": {ProjectUID: "p", Shared: true, Lanes: []string{"triage", "review"}, DoneDays: 3, ProjectID: 7, Revision: 6},
		"unshared":   {ProjectUID: "p", ProjectID: 7, Revision: 6},
	} {
		t.Run(name, func(t *testing.T) {
			m, share := sharedLanesWrite(t)
			m, cmds := model.Update(m, model.LanesShared{Share: share, Conflict: true, Current: current})
			if len(cmds) != 0 || m.LanesForm.Saving || m.LanesForm.Err == nil || m.LanesForm.Revision != 6 {
				t.Fatalf("sent %#v saving %v error %v revision %d, want the conflict shown on revision 6",
					cmds, m.LanesForm.Saving, m.LanesForm.Err, m.LanesForm.Revision)
			}
		})
	}
}

func TestAConflictShownOnAChangedListThenAnOrderOnlyBumpSendsTheShareAgain(t *testing.T) {
	m, share := sharedLanesWrite(t)
	changed := model.LanesLoaded{ProjectUID: "p", Shared: true, Lanes: []string{"triage", "ship"}, DoneDays: 7, ProjectID: 7, Revision: 6}
	m, _ = model.Update(m, model.LanesShared{Share: share, Conflict: true, Current: changed})
	if m.LanesForm.Err == nil {
		t.Fatal("a conflict over a changed list showed no conflict")
	}
	m, cmds := clickPress(m, "enter")
	if len(cmds) != 1 {
		t.Fatalf("saving again sent %#v, want one shared write", cmds)
	}
	second, ok := cmds[0].(model.ShareLanes)
	if !ok {
		t.Fatalf("saving again sent %#v, want a shared write", cmds[0])
	}
	changed.Revision = 8
	_, cmds = model.Update(m, model.LanesShared{Share: second, Conflict: true, Current: changed})
	want := second
	want.Revision = 8
	if len(cmds) != 1 || !reflect.DeepEqual(cmds[0], want) {
		t.Fatalf("a conflict from a write to other metadata sent %#v, want %#v", cmds, want)
	}
}

func TestAConflictOverAnUnreadableListAlwaysShowsTheConflictEvenOnceItIsCleared(t *testing.T) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, _ = clickPress(m, "b")
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest(), Shared: true, SharedErr: errors.New("not a list"), ProjectID: 7, Revision: 4})
	m, _ = clickPress(m, "shift+b", "ctrl+u")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "triage, qa"})
	m, cmds := clickPress(m, "enter")
	if len(cmds) != 1 {
		t.Fatalf("saving sent %#v, want one shared write", cmds)
	}
	share, ok := cmds[0].(model.ShareLanes)
	if !ok {
		t.Fatalf("saving sent %#v, want a shared write", cmds[0])
	}
	cleared := model.LanesLoaded{ProjectUID: "p", ProjectID: 7, Revision: 6}
	m, cmds = model.Update(m, model.LanesShared{Share: share, Conflict: true, Current: cleared})
	if len(cmds) != 0 || m.LanesForm.Saving || m.LanesForm.Err == nil {
		t.Fatalf("sent %#v saving %v error %v, want the conflict shown over a list that was unreadable", cmds, m.LanesForm.Saving, m.LanesForm.Err)
	}
}
