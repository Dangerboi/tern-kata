package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// loadedAt is the Lane list load answering with the Project read at revision 4.
func loadedAt(lanes []string, shared bool) model.LanesLoaded {
	return model.LanesLoaded{ProjectUID: "p", Lanes: lanes, Shared: shared, ProjectID: 7, Revision: 4}
}

// sharedCard is the Lanes card open on a Board whose Project shares triage, review.
func sharedCard(t *testing.T) driver.State {
	t.Helper()
	s := step(t, boardPane(t, boardCard("a", "triage")), loadedAt([]string{"triage", "review"}, true)).State
	return step(t, s, key("shift+b")).State
}

func sentShare(t *testing.T, out driver.Output) model.ShareLanes {
	t.Helper()
	if len(out.Cmds) != 1 {
		t.Fatalf("sent %#v, want one shared write", out.Cmds)
	}
	share, ok := out.Cmds[0].(model.ShareLanes)
	if !ok {
		t.Fatalf("sent %#v, want a shared write", out.Cmds[0])
	}
	return share
}

func TestTheBoardAndTheLanesCardNameTheListsSource(t *testing.T) {
	for _, tt := range []struct {
		loaded model.LanesLoaded
		want   string
	}{
		{model.LanesLoaded{ProjectUID: "p"}, "default"},
		{loadedAt([]string{"mine"}, false), "this machine"},
		{loadedAt([]string{"ours"}, true), "shared with the Project"},
	} {
		s := step(t, boardPane(t, boardCard("a")), tt.loaded).State
		if got, _ := nodeText(t, s, "main.columns.list.lane-source"); !strings.Contains(got, tt.want) {
			t.Errorf("Board says %q, want %q", got, tt.want)
		}
		card := step(t, s, key("shift+b")).State
		if got, ok := textOnCard(t, card.Model, "In use"); !ok || !strings.Contains(got, tt.want) {
			t.Errorf("Lanes card says %q (drawn %v), want %q", got, ok, tt.want)
		}
	}
}

func TestTheLanesCardSharesWithOneWriteGuardedOnTheRevisionItOpenedWith(t *testing.T) {
	s := sharedCard(t)
	if !s.Model.LanesForm.Share {
		t.Fatal("the card opened on this machine although the Project shares its list")
	}
	s = typed(t, step(t, s, key("ctrl+u")).State, "triage, In review")
	out := step(t, s, key("enter"))
	want := model.ShareLanes{Workspace: "/repo", Actor: s.Model.Actor, ProjectUID: "p", ProjectID: 7, Revision: 4, Lanes: []string{"triage", "in-review"}}
	if got := sentShare(t, out); !reflect.DeepEqual(got, want) {
		t.Fatalf("sent %#v, want %#v", got, want)
	}
	saved := step(t, out.State, model.LanesShared{Share: want, Revision: 5}).State
	if saved.Model.TopCard() != model.NoCard {
		t.Fatalf("card %v after the write", saved.Model.TopCard())
	}
	wantLane(t, saved, 1, "Triage", "a")
	wantLane(t, saved, 2, "In review")
	// The next card guards on the revision the write left.
	if got := sentShare(t, step(t, step(t, saved, key("shift+b")).State, key("enter"))); got.Revision != 5 {
		t.Errorf("the next write is guarded on %d, want 5", got.Revision)
	}
}

func TestAConflictingShareKeepsTheTextShowsTheSharedListAndGuardsTheRetryOnItsRevision(t *testing.T) {
	s := typed(t, step(t, sharedCard(t), key("ctrl+u")).State, "mine")
	out := step(t, s, key("enter"))
	current := model.LanesLoaded{ProjectUID: "p", Shared: true, Lanes: []string{"theirs", "qa"}, DoneDays: 7, ProjectID: 7, Revision: 9}
	s = step(t, out.State, model.LanesShared{Share: sentShare(t, out), Conflict: true, Current: current}).State
	msg, ok := errorOnCard(t, s.Model)
	if s.Model.TopCard() != model.CardLanes || lanesField(t, s) != "mine" || !ok || !strings.Contains(msg, "theirs, qa") {
		t.Fatalf("card %v field %q error %q (drawn %v), want the text kept and the shared list named", s.Model.TopCard(), lanesField(t, s), msg, ok)
	}
	if got := sentShare(t, step(t, s, key("enter"))); got.Revision != 9 || !reflect.DeepEqual(got.Lanes, []string{"mine"}) {
		t.Errorf("retry sent %#v, want mine guarded on revision 9", got)
	}
}

func TestARefusedShareShowsTheReasonAndThisMachineStillSaves(t *testing.T) {
	s := step(t, boardPane(t, boardCard("a")), loadedAt(nil, false)).State
	s = typed(t, step(t, step(t, s, key("shift+b")).State, key("ctrl+u")).State, "wip")
	for range 3 {
		s = step(t, s, key("tab")).State
	}
	s = step(t, s, key("right")).State
	if !s.Model.LanesForm.Share {
		t.Fatalf("→ on Save to left %+v", s.Model.LanesForm)
	}
	out := step(t, s, key("enter"))
	s = step(t, out.State, model.LanesShared{Share: sentShare(t, out), Err: errors.New("this token is read-only")}).State
	if msg, ok := errorOnCard(t, s.Model); s.Model.TopCard() != model.CardLanes || !ok || !strings.Contains(msg, "read-only") {
		t.Fatalf("card %v error %q (drawn %v), want kata's reason on the card", s.Model.TopCard(), msg, ok)
	}
	local := step(t, step(t, s, key("left")).State, key("enter"))
	if want := []model.Cmd{model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"wip"}}}; !reflect.DeepEqual(local.Cmds, want) {
		t.Errorf("saving to this machine sent %#v, want %#v", local.Cmds, want)
	}
}

func TestStopSharingClearsTheSharedListAndReadsTheListAgain(t *testing.T) {
	out := step(t, sharedCard(t), key("ctrl+x"))
	stop := sentShare(t, out)
	if want := (model.ShareLanes{Workspace: "/repo", Actor: out.State.Model.Actor, ProjectUID: "p", ProjectID: 7, Revision: 4, Stop: true}); !reflect.DeepEqual(stop, want) {
		t.Fatalf("ctrl+x sent %#v, want %#v", stop, want)
	}
	done := step(t, out.State, model.LanesShared{Share: stop, Revision: 5})
	if done.State.Model.TopCard() != model.NoCard || len(loadCmds(done.Cmds)) != 1 {
		t.Fatalf("card %v loads %v, want the card closed and the list read again", done.State.Model.TopCard(), loadCmds(done.Cmds))
	}
	local := step(t, step(t, boardPane(t), loadedAt(nil, false)).State, key("shift+b")).State
	refused := step(t, local, key("ctrl+x"))
	if msg, _ := errorOnCard(t, refused.State.Model); len(refused.Cmds) != 0 || !strings.Contains(msg, "shares no Lane list") {
		t.Errorf("ctrl+x with nothing shared sent %v, error %q", refused.Cmds, msg)
	}
}

func TestASharedValueThatIsNotALaneListIsReportedOnceAndCanBeCleared(t *testing.T) {
	bad := loadedAt([]string{"mine"}, false)
	bad.SharedErr = errors.New("tern.lanes is not a Lane list: labels is not a list")
	s := step(t, boardPane(t, boardCard("a", "mine")), bad).State
	if !strings.Contains(s.Model.Notice, "labels is not a list") {
		t.Fatalf("Notice %q, want the shared value's error", s.Model.Notice)
	}
	wantLane(t, s, 1, "Mine", "a")
	s.Model.Notice = ""
	if again := step(t, s, bad).State; again.Model.Notice != "" {
		t.Errorf("the same error was reported again: %q", again.Model.Notice)
	}
	if stop := sentShare(t, step(t, step(t, s, key("shift+b")).State, key("ctrl+x"))); !stop.Stop {
		t.Errorf("ctrl+x sent %#v, want the bad value cleared", stop)
	}
}

func TestASaveToThisMachineWhileTheProjectSharesKeepsTheSharedList(t *testing.T) {
	s := step(t, sharedCard(t), key("tab")).State
	s = step(t, step(t, step(t, s, key("tab")).State, key("tab")).State, key("left")).State
	out := step(t, s, key("enter"))
	s = step(t, out.State, model.LanesSaved{ProjectUID: "p", Lanes: []string{"triage", "review"}}).State
	if s.Model.TopCard() != model.NoCard || !s.Model.Board.Shared || !strings.Contains(s.Model.Notice, "shared list stays in use") {
		t.Fatalf("card %v shared %v Notice %q", s.Model.TopCard(), s.Model.Board.Shared, s.Model.Notice)
	}
}

func TestAProjectMetadataEventReadsTheLaneListAgain(t *testing.T) {
	s := boardPane(t)
	event := func(kind string) model.StreamEvent {
		return model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: kind}
	}
	if got := loadCmds(step(t, s, event("project.metadata_updated")).Cmds); len(got) != 1 || got[0] != (model.LoadLanes{Workspace: "/repo", ProjectUID: "p"}) {
		t.Errorf("a Project metadata event asked %v, want one load", got)
	}
	if got := loadCmds(step(t, s, event("issue.updated")).Cmds); len(got) != 0 {
		t.Errorf("an Issue event asked %v", got)
	}
}

func TestAProjectMetadataEventTheSnapshotAlreadyCoversStillReadsTheLaneListAgain(t *testing.T) {
	s := boardPane(t)
	s.Model.Cursor = 5
	covered := model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 3, Type: "project.metadata_updated"}
	if got := loadCmds(step(t, s, covered).Cmds); len(got) != 1 || got[0] != (model.LoadLanes{Workspace: "/repo", ProjectUID: "p"}) {
		t.Errorf("a covered Project metadata event asked %v, want one load", got)
	}
	covered.Type = "issue.updated"
	if got := loadCmds(step(t, s, covered).Cmds); len(got) != 0 {
		t.Errorf("a covered Issue event asked %v", got)
	}
}

func TestAnArchivedProjectsSharedListIsRefusedUpFrontAndTheFileStillSaves(t *testing.T) {
	archived := loadedAt([]string{"triage", "review"}, true)
	archived.Archived = true
	s := step(t, boardPane(t, boardCard("a", "triage")), archived).State
	s = step(t, s, key("shift+b")).State
	for _, chord := range []string{"enter", "ctrl+x"} {
		out := step(t, s, key(chord))
		msg, _ := errorOnCard(t, out.State.Model)
		if len(out.Cmds) != 0 || !strings.Contains(msg, "archived Project") || !strings.Contains(msg, "Restore the Project") {
			t.Errorf("%s sent %v with error %q, want the plain refusal and no write", chord, out.Cmds, msg)
		}
	}
	for range 3 {
		s = step(t, s, key("tab")).State
	}
	s = step(t, s, key("left")).State
	if s.Model.LanesForm.Share {
		t.Fatalf("← on Save to left %+v", s.Model.LanesForm)
	}
	out := step(t, s, key("enter"))
	if want := []model.Cmd{model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"triage", "review"}}}; !reflect.DeepEqual(out.Cmds, want) {
		t.Errorf("saving to this machine sent %#v, want %#v", out.Cmds, want)
	}
}
