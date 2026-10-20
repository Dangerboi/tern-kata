package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func loadCmds(cmds []model.Cmd) (loads []model.LoadLanes) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadLanes); ok {
			loads = append(loads, load)
		}
	}
	return loads
}

func TestBoardOnAsksForTheProjectsLanesOnce(t *testing.T) {
	out := step(t, projectPane(t, 160, boardCard("a")), key("b"))
	if got := loadCmds(out.Cmds); len(got) != 1 || got[0].ProjectUID != "p" {
		t.Fatalf("turning the Board on asked %v, want one load for Project p", got)
	}
	if got := loadCmds(step(t, out.State, key("j")).Cmds); len(got) != 0 {
		t.Errorf("a key on the Board asked %v, want no load", got)
	}
	if got := loadCmds(step(t, out.State, key("b")).Cmds); len(got) != 0 {
		t.Errorf("turning the Board off asked %v, want no load", got)
	}
}

func TestBoardAsksAgainWhenTheProjectChanges(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	out := step(t, s, model.ProjectLoaded{Project: model.Project{ID: 8, UID: "q", Name: "home"}})
	if got := loadCmds(out.Cmds); len(got) != 1 || got[0].ProjectUID != "q" {
		t.Fatalf("a Project change on the Board asked %v, want one load for Project q", got)
	}
}

func TestBoardShowsLoadedLanes(t *testing.T) {
	s := boardPane(t, boardCard("a", "in-review"), boardCard("b", "needs-triage"))
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review", "shipping"}}).State
	heads, _ := lanes(t, s)
	if len(heads) != 4 {
		t.Fatalf("the Board draws %d columns, want No lane, 2 Lanes and Done", len(heads))
	}
	wantLane(t, s, 1, "In review", "a")
	wantLane(t, s, 2, "Shipping")
	wantLane(t, s, 0, "No lane", "b")
}

func TestBoardKeepsTheDefaultLanesWhenTheProjectHasNone(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage"))
	s = step(t, s, model.LanesLoaded{ProjectUID: "p"}).State
	wantLane(t, s, 1, "Needs triage", "a")
	if s.Model.Notice != "" {
		t.Errorf("a missing list left the Notice %q", s.Model.Notice)
	}
}

func TestBoardShowsTheDefaultLanesAndANoticeForAnUnreadableFile(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage"))
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Err: errors.New("lanes.toml: bad syntax")}).State
	wantLane(t, s, 1, "Needs triage", "a")
	if s.Model.Notice == "" {
		t.Error("an unreadable file left no Notice")
	}
	var cmds []model.Cmd
	s.Model.Notice = ""
	s.Model, cmds = s.Model.SaveLaneList([]string{"x"})
	if len(cmds) != 0 || s.Model.Notice == "" {
		t.Errorf("a save to an unreadable file was not refused with its error: %v, Notice %q", cmds, s.Model.Notice)
	}
}

func TestBoardSaveAsksForTheProjectsList(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	_, cmds := s.Model.SaveLaneList([]string{"in-review"})
	want := model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"in-review"}}
	if len(cmds) != 1 || !equalSave(cmds[0], want) {
		t.Fatalf("save asked %#v, want %#v", cmds, want)
	}
}

func equalSave(cmd model.Cmd, want model.SaveLanes) bool {
	got, ok := cmd.(model.SaveLanes)
	return ok && got.ProjectUID == want.ProjectUID && got.ProjectName == want.ProjectName && len(got.Lanes) == 1 && got.Lanes[0] == want.Lanes[0]
}

func TestRemovingALaneLabelMovesItsIssuesWithoutACommand(t *testing.T) {
	s := boardPane(t, boardCard("only", "in-review"), boardCard("both", "in-review", "needs-triage"))
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"in-review", "needs-triage"}}).State
	wantLane(t, s, 1, "In review", "both", "only")
	out := step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"needs-triage"}})
	if len(out.Cmds) != 0 {
		t.Errorf("removing a Lane asked %#v, want no command", out.Cmds)
	}
	wantLane(t, out.State, 0, "No lane", "only")
	wantLane(t, out.State, 1, "Needs triage", "both")
}
