package model_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// orderedBoard is dragBoard with the Lane list answering with order and shared. Issues are given newest first, so the list's
// order within a Lane is the order they are given in.
func orderedBoard(t *testing.T, cols int, loaded model.LanesLoaded, issues ...model.Issue) model.Model {
	t.Helper()
	newestFirst(issues)
	m, _ := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	m, _ = model.Update(m, model.ResizeMsg{Cols: cols})
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	m, _ = loadFirst(m, model.ProjectLoaded{Project: project},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}, LocalDate: "2026-10-08", TimeZone: "UTC"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "b"})
	loaded.ProjectUID, loaded.Request = "p", m.LanesRequest()
	m, _ = model.Update(m, loaded)
	return m
}

// newestFirst dates the open issues so the list orders them as given.
func newestFirst(issues []model.Issue) {
	for i := range issues {
		if issues[i].ClosedAt == nil {
			issues[i].UpdatedAt = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour)
		}
	}
}

// snapshotAfter answers the snapshot read asked for last with issues, as the daemon lists them.
func snapshotAfter(m model.Model, issues ...model.Issue) (model.Model, []model.Cmd) {
	newestFirst(issues)
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	return model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest(), Query: m.SnapshotQuery(), LocalDate: "2026-10-08", TimeZone: "UTC",
		Snapshot: model.Snapshot{Cursor: m.Cursor + 1, Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}})
}

// laneRead is the Lane list read among cmds.
func laneRead(t *testing.T, cmds []model.Cmd) model.LoadLanes {
	t.Helper()
	for _, cmd := range cmds {
		if read, ok := cmd.(model.LoadLanes); ok {
			return read
		}
	}
	t.Fatalf("cmds = %#v, want a Lane list read", cmds)
	return model.LoadLanes{}
}

func laneUIDs(m model.Model, column int) []string {
	var uids []string
	for _, card := range m.BoardColumns()[column].Cards {
		uids = append(uids, card.Issue.UID)
	}
	return uids
}

func orderSaves(cmds []model.Cmd) (saves []model.SaveLaneOrder) {
	for _, cmd := range cmds {
		if save, ok := cmd.(model.SaveLaneOrder); ok {
			saves = append(saves, save)
		}
	}
	return saves
}

func triage(uid string) model.Issue { return dragCard(uid, "needs-triage") }

func TestLaneShowsUnnamedCardsFirstThenTheOrder(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"gone", "c", "a"}}},
		triage("a"), triage("b"), triage("c"), triage("d"), dragCard("n", "needs-info"))
	if got, want := laneUIDs(m, 1), []string{"b", "d", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Needs triage shows %v, want %v", got, want)
	}
	if got, want := laneUIDs(m, 2), []string{"n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Needs info shows %v, want %v", got, want)
	}
	if !m.LaneOrderCustom("needs-triage") || m.LaneOrderCustom("needs-info") {
		t.Errorf("custom order: triage %v, info %v", m.LaneOrderCustom("needs-triage"), m.LaneOrderCustom("needs-info"))
	}
}

func TestDoneIgnoresAnOrder(t *testing.T) {
	old, newer := dragClosed("old"), dragClosed("new")
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	newer.ClosedAt = &at
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{" done": {"old", "new"}}}, old, newer)
	done := len(m.BoardColumns()) - 1
	if got, want := laneUIDs(m, done), []string{"new", "old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Done shows %v, want newest closed first %v", got, want)
	}
}

func TestNoLaneReorderSavesUnderTheEmptyKeyAndShows(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, dragCard("a"), dragCard("b"), dragCard("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 0, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if want := (model.OrderChange{Shown: []string{"a", "b", "c"}, Move: "c", Anchor: "a"}); len(saves) != 1 || saves[0].Lane != model.LaneNone || !reflect.DeepEqual(saves[0].Change, want) {
		t.Fatalf("saves = %#v, want c moved before a under No lane's key", saves)
	}
	if got, want := laneUIDs(m, 0), []string{"c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("No lane shows %v, want %v", got, want)
	}
	if !m.LaneOrderCustom(model.LaneNone) {
		t.Error("No lane has no custom order after a reorder")
	}
}

func TestNoLaneShowsItsStoredOrder(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"": {"b", "a"}}}, dragCard("a"), dragCard("b"), dragCard("c"))
	if got, want := laneUIDs(m, 0), []string{"c", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("No lane shows %v, want %v", got, want)
	}
}

func TestDoneReorderSendsNothing(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, dragClosed("a"), dragClosed("b"))
	done := len(m.BoardColumns()) - 1
	_, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: done, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("cmds = %#v, want no save", cmds)
	}
}

func TestClosedCardIsPrunedFromNoLaneOrder(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"": {"hidden", "b", "a"}}}, dragCard("a"), dragCard("b"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	_, cmds := snapshotAfter(m, dragClosed("a"), dragCard("b"))
	saves := orderSaves(cmds)
	if len(saves) != 1 || saves[0].Lane != model.LaneNone || !reflect.DeepEqual(saves[0].Change, model.OrderChange{Remove: []string{"a"}}) {
		t.Fatalf("saves = %#v, want the removal of the closed a from No lane's order", saves)
	}
}

func TestLaneViewFollowsTheOrder(t *testing.T) {
	m := orderedBoard(t, 60, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}},
		triage("a"), triage("b"), triage("c"))
	if !m.LaneViewShown() {
		t.Fatal("the Lane view is not shown on a narrow Pane")
	}
	var got []string
	for _, row := range m.VisibleRows() {
		got = append(got, row.UID)
	}
	if want := []string{"c", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lane view rows %v, want %v", got, want)
	}
}

func TestReorderSavesTheOrderKeepingHiddenCardsInPlace(t *testing.T) {
	// "x" is named by the order but not shown (filtered out or not loaded); "g" is named but now in another Lane.
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"x", "g", "c", "a"}}},
		triage("a"), triage("b"), triage("c"), triage("d"), dragCard("g", "needs-info"))
	if got, want := laneUIDs(m, 1), []string{"b", "d", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Needs triage shows %v, want %v", got, want)
	}
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(cmds) != 1 || len(saves) != 1 {
		t.Fatalf("cmds = %#v, want one SaveLaneOrder", cmds)
	}
	want := model.SaveLaneOrder{Workspace: "/repo", Actor: saves[0].Actor, ProjectUID: "p", ProjectName: saves[0].ProjectName, ProjectID: saves[0].ProjectID, Seq: saves[0].Seq,
		Lane: "needs-triage", Change: model.OrderChange{Shown: []string{"b", "d", "c", "a"}, Move: "a", Anchor: "b", Remove: []string{"g"}}}
	if saves[0].Seq == 0 {
		t.Error("the save carries no sequence number")
	}
	if !reflect.DeepEqual(saves[0], want) {
		t.Errorf("save = %#v, want %#v", saves[0], want)
	}
	if got, want := laneUIDs(m, 1), []string{"a", "b", "d", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("next frame shows %v, want %v", got, want)
	}
}

func TestReorderDownPlacesAfterTheNewUpperNeighbour(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"a", "x", "b", "c"}}},
		triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: 1}))
	saves := orderSaves(cmds)
	if want := (model.OrderChange{Shown: []string{"a", "b", "c"}, Move: "a", Anchor: "b", After: true}); len(saves) != 1 || !reflect.DeepEqual(saves[0].Change, want) {
		t.Fatalf("saves = %#v, want a moved after b", saves)
	}
	if got, want := laneUIDs(m, 1), []string{"b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shows %v, want %v", got, want)
	}
}

func TestReorderInALaneWithoutAnOrderStartsOneFromTheShownCards(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if want := (model.OrderChange{Shown: []string{"a", "b", "c"}, Move: "c", Anchor: "a"}); len(saves) != 1 || !reflect.DeepEqual(saves[0].Change, want) {
		t.Fatalf("saves = %#v, want c moved before a, naming the shown cards", saves)
	}
	if !m.LaneOrderCustom("needs-triage") {
		t.Error("the Lane has no custom order after a reorder")
	}
}

func TestReorderThatChangesNothingSendsNothing(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"))
	for _, index := range []int{0, -3} {
		next, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: index}))
		if len(orderSaves(cmds)) != 0 || next.LaneOrderCustom("needs-triage") {
			t.Errorf("index %d: cmds %#v, custom %v", index, cmds, next.LaneOrderCustom("needs-triage"))
		}
	}
}

func TestDropFromAnotherLaneLandsBesideTheTargetCard(t *testing.T) {
	for name, tc := range map[string]struct {
		drop   model.CardDropped
		change model.OrderChange
		want   []string
	}{
		"before the card": {model.CardDropped{UID: "m", Column: 1, Target: "c"}, model.OrderChange{Move: "m", Anchor: "c"}, []string{"b", "d", "m", "c", "a"}},
		"after the card":  {model.CardDropped{UID: "m", Column: 1, Target: "c", After: true}, model.OrderChange{Move: "m", Anchor: "c", After: true}, []string{"b", "d", "c", "m", "a"}},
		"empty space":     {model.CardDropped{UID: "m", Column: 1}, model.OrderChange{Move: "m"}, []string{"b", "d", "c", "a", "m"}},
	} {
		t.Run(name, func(t *testing.T) {
			m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"c", "a"}}},
				triage("a"), triage("b"), triage("c"), triage("d"), dragCard("m", "needs-info"))
			m, cmds := model.Update(m, dropOn(m, tc.drop))
			saves := orderSaves(cmds)
			if len(moveCmds(cmds)) != 1 || len(saves) != 1 || len(cmds) != 2 {
				t.Fatalf("cmds = %#v, want the Move and one SaveLaneOrder", cmds)
			}
			tc.change.Shown, tc.change.OnlyOrdered = []string{"b", "d", "c", "a"}, true
			if saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Change, tc.change) {
				t.Errorf("save = %#v, want Lane needs-triage change %#v", saves[0], tc.change)
			}
			if got := laneUIDs(m, 1); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Lane shows %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDropIntoALaneWithoutAnOrderOnlyMoves(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), dragCard("m", "needs-info"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "m", Column: 1, Target: "a"}))
	if len(cmds) != 1 || len(moveCmds(cmds)) != 1 || m.LaneOrderCustom("needs-triage") {
		t.Errorf("cmds = %#v, custom %v; want only the Move", cmds, m.LaneOrderCustom("needs-triage"))
	}
}

func TestRefusedMoveLeavesTheOrderAlone(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"a"}}}, triage("a"), triage("b"), dragCard("m", "needs-info"))
	m.Stale = true
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "m", Column: 1, Target: "a"}))
	if len(orderSaves(cmds)) != 0 || !reflect.DeepEqual(laneUIDs(m, 1), []string{"b", "a"}) {
		t.Errorf("cmds %#v, Lane shows %v", cmds, laneUIDs(m, 1))
	}
}

func TestFailedOrderSaveShowsTheStoredOrderAndSays(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	save := orderSaves(cmds)[0]
	stored := model.LaneOrders{"needs-triage": {"b", "a"}}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: save, Current: stored, Err: errors.New("the Project changed")})
	if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || !strings.Contains(m.Notice, "the Project changed") || m.NoticeKind != model.NoticeFail {
		t.Errorf("notice = %q (%v)", m.Notice, m.NoticeKind)
	}
	if got, want := laneUIDs(m, 1), []string{"c", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lane shows %v, want the unnamed card first, then the stored order: %v", got, want)
	}
}

func TestOrderSavedAnswerAdoptsTheStoredOrdersAndIgnoresAnotherProject(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	save := orderSaves(cmds)[0]
	elsewhere := save
	elsewhere.ProjectUID, elsewhere.Seq = "elsewhere", save.Seq+1000
	other, _ := model.Update(m, model.LaneOrderSaved{Save: elsewhere, Current: model.LaneOrders{"needs-triage": {"a"}}, Err: errors.New("boom")})
	if !reflect.DeepEqual(laneUIDs(other, 1), []string{"b", "a"}) || other.Notice != "" {
		t.Errorf("another Project's answer changed the Board: shows %v, notice %q", laneUIDs(other, 1), other.Notice)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: save, Current: model.LaneOrders{"needs-triage": {"b", "a"}}})
	if got, want := laneUIDs(m, 1), []string{"b", "a"}; !reflect.DeepEqual(got, want) || m.Notice != "" {
		t.Errorf("Lane shows %v, notice %q", got, m.Notice)
	}
	again, _ := model.Update(m, model.LaneOrderSaved{Save: save, Current: model.LaneOrders{"needs-triage": {"a"}}})
	if !reflect.DeepEqual(laneUIDs(again, 1), []string{"b", "a"}) {
		t.Errorf("an answer that is no longer pending changed the Lane to %v", laneUIDs(again, 1))
	}
}

func TestLaneListReloadReplacesTheOrders(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("a"), triage("b"))
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest()})
	if m.LaneOrderCustom("needs-triage") {
		t.Error("a reload with no orders kept the old one")
	}
}

func TestResetLaneOrderDropsItAndSavesNone(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("a"), triage("b"))
	m, cmds := model.Update(m, model.ResetLaneOrder{Lane: "needs-triage"})
	saves := orderSaves(cmds)
	if len(cmds) != 1 || len(saves) != 1 || saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Change, model.OrderChange{Reset: true}) {
		t.Fatalf("cmds = %#v, want one SaveLaneOrder that resets the Lane", cmds)
	}
	if m.LaneOrderCustom("needs-triage") || !reflect.DeepEqual(laneUIDs(m, 1), []string{"a", "b"}) {
		t.Errorf("custom %v, Lane shows %v; want the list's order", m.LaneOrderCustom("needs-triage"), laneUIDs(m, 1))
	}
}

func TestSharedOrderOnAnUnreadOrArchivedProjectIsRefused(t *testing.T) {
	for name, loaded := range map[string]model.LanesLoaded{
		"unread":                                {Shared: true},
		"read failed, falling back to the file": {SharedErr: errors.New("kata is down")},
		"archived":                              {Shared: true, ProjectID: 7, Revision: 3, Archived: true},
	} {
		t.Run(name, func(t *testing.T) {
			m := orderedBoard(t, 160, loaded, triage("a"), triage("b"))
			m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
			if len(orderSaves(cmds)) != 0 || m.LaneOrderCustom("needs-triage") {
				t.Errorf("cmds %#v, custom %v; want a refusal", cmds, m.LaneOrderCustom("needs-triage"))
			}
			if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || m.NoticeKind != model.NoticeFail {
				t.Errorf("notice = %q (%v)", m.Notice, m.NoticeKind)
			}
		})
	}
}

func TestOrderOfAProjectKataDoesNotListIsSavedToTheFile(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"))
	_, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(saves) != 1 || saves[0].Shared {
		t.Errorf("saves = %#v, want one save to the Lane list file", saves)
	}
}

func TestAReadFailureRefusesAReorderWithItsReason(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{SharedErr: errors.New("the Project could not be read: kata is down")}, triage("a"), triage("b"))
	m, _ = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || strings.Count(m.Notice, "the Project could not be read") != 1 || !strings.Contains(m.Notice, "kata is down") {
		t.Errorf("notice = %q, want the unread Project, once, and its reason", m.Notice)
	}
}

func TestSharedOrderOfAReadProjectIsSavedShared(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Shared: true, ProjectID: 7, Revision: 3}, triage("a"), triage("b"))
	_, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(saves) != 1 || !saves[0].Shared || saves[0].ProjectID != 7 {
		t.Errorf("saves = %#v, want one shared save for Project 7", saves)
	}
}

func TestFailedOrderSaveWithoutStoredOrdersReadsTheLaneListAgain(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	save := orderSaves(cmds)[0]
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: save, Err: errors.New("lanes.toml is read-only")})
	read := laneRead(t, cmds)
	if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || !strings.Contains(m.Notice, "lanes.toml is read-only") {
		t.Errorf("notice = %q", m.Notice)
	}
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: read.Request})
	if m.LaneOrderCustom("needs-triage") || !reflect.DeepEqual(laneUIDs(m, 1), []string{"a", "b", "c"}) {
		t.Errorf("after the read the Lane shows %v (custom %v), want the list's order", laneUIDs(m, 1), m.LaneOrderCustom("needs-triage"))
	}
}

func TestOrderSaveWhileDisconnectedIsRefused(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m.Stale = true
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 0 || m.LaneOrderCustom("needs-triage") || !reflect.DeepEqual(laneUIDs(m, 1), []string{"a", "b", "c"}) {
		t.Errorf("cmds %#v, Lane shows %v", cmds, laneUIDs(m, 1))
	}
	if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || !strings.Contains(m.Notice, model.ErrDisconnected.Error()) || m.NoticeKind != model.NoticeFail {
		t.Errorf("notice = %q (%v)", m.Notice, m.NoticeKind)
	}
}

func TestOrderSaveIsRefusedWhileTheLaneListFileCannotBeRead(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Err: errors.New("lanes.toml: line 3: bad value")}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 0 || m.LaneOrderCustom("needs-triage") {
		t.Errorf("cmds %#v, custom %v; want the file left alone", cmds, m.LaneOrderCustom("needs-triage"))
	}
	if !strings.HasPrefix(m.Notice, "Cannot save the custom order: ") || !strings.Contains(m.Notice, "bad value") {
		t.Errorf("notice = %q, want the refusal and why", m.Notice)
	}
}

func TestLaneListReadAskedBeforeASaveCannotPutTheOldOrderBack(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, Type: "project.metadata_updated"})
	old := laneRead(t, cmds)
	m, _ = model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: old.Request})
	if got, want := laneUIDs(m, 1), []string{"c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the old read changed the Lane to %v, want %v", got, want)
	}
}

func TestTwoQuickReordersSurviveALaneListReadAnsweredBetweenThem(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Shared: true, ProjectID: 7, Revision: 3}, triage("a"), triage("b"), triage("c"), triage("d"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	first := orderSaves(cmds)
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "d", Column: 1, Reorder: true, Index: 0}))
	if len(first) != 1 || len(orderSaves(cmds)) != 0 {
		t.Fatalf("saves %#v then %#v, want the first sent and the second waiting", first, cmds)
	}
	m, cmds = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, Type: "project.metadata_updated"})
	read := laneRead(t, cmds)
	// The read answers after the first write committed.
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: read.Request, Shared: true, ProjectID: 7, Revision: 4,
		Order: model.LaneOrders{"needs-triage": {"c", "a", "b", "d"}}})
	if got, want := laneUIDs(m, 1), []string{"d", "c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the read the Lane shows %v, want both moves %v", got, want)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: first[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b", "d"}}})
	second := orderSaves(cmds)
	if got, want := laneUIDs(m, 1), []string{"d", "c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the first answer the Lane shows %v, want %v", got, want)
	}
	if len(second) != 1 {
		t.Fatalf("the first answer sent %#v, want the second change", cmds)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: second[0], Current: model.LaneOrders{"needs-triage": {"d", "c", "a", "b"}}})
	if got, want := laneUIDs(m, 1), []string{"d", "c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the second answer the Lane shows %v, want %v", got, want)
	}
}

func TestOrderAnswerOfAnotherLaneListSourceShowsItsErrorOnly(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	save := orderSaves(cmds)[0]
	m, cmds = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, Type: "project.metadata_updated"})
	read := laneRead(t, cmds)
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: read.Request, Shared: true, ProjectID: 7, Revision: 3,
		Order: model.LaneOrders{"needs-triage": {"b"}}})
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: save, Current: model.LaneOrders{"needs-triage": {"c", "a", "b"}}, Err: errors.New("boom")})
	if got, want := laneUIDs(m, 1), []string{"a", "c", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lane shows %v, want the shared order's %v", got, want)
	}
	if len(cmds) != 0 || !strings.Contains(m.Notice, "boom") {
		t.Errorf("cmds %#v, notice %q; want no read and the error", cmds, m.Notice)
	}
}

func TestCardMovedOutByKeysAndBackShowsFirst(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("x"), triage("a"), triage("b"))
	if got, want := laneUIDs(m, 1), []string{"x", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Needs triage shows %v, want %v", got, want)
	}
	m, _ = model.MoveCard(m, "a", 1)
	m, _ = model.Update(m, model.MoveResultMsg{UID: "a"})
	m, cmds := snapshotAfter(m, triage("x"), dragCard("a", "needs-info"), triage("b"))
	saves := orderSaves(cmds)
	if len(saves) != 1 || saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Change, model.OrderChange{Remove: []string{"a"}}) {
		t.Fatalf("saves = %#v, want the removal of a from Needs triage's order", saves)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: model.LaneOrders{"needs-triage": {"b"}}})
	m, _ = model.MoveCard(m, "a", -1)
	m, _ = model.Update(m, model.MoveResultMsg{UID: "a"})
	m, cmds = snapshotAfter(m, triage("x"), triage("a"), triage("b"))
	if got, want := laneUIDs(m, 1), []string{"x", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Needs triage shows %v, want the returned card with the unnamed ones, ahead of the named b: %v", got, want)
	}
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("an order with nothing stale was saved: %#v", cmds)
	}
}

func TestClosedCardIsPrunedFromTheOrderButAHiddenOneIsNot(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"hidden", "b", "a"}}}, triage("a"), triage("b"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	saves := orderSaves(cmds)
	if len(saves) != 1 || saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Change, model.OrderChange{Remove: []string{"a"}}) {
		t.Fatalf("saves = %#v, want only the closed a removed, the hidden uid left", saves)
	}
	if _, again := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b")); len(orderSaves(again)) != 0 {
		t.Errorf("the prune was sent again while its answer is out: %#v", again)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: model.LaneOrders{"needs-triage": {"hidden", "b"}}})
	_, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("the pruned order was saved again: %#v", cmds)
	}
}

func TestQuickReordersGoOutOneAtATimeAndAreShownOverTheStoredOrders(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"), triage("d"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	first := orderSaves(cmds)
	if want := (model.OrderChange{Shown: []string{"a", "b", "c", "d"}, Move: "c", Anchor: "a"}); len(first) != 1 || !reflect.DeepEqual(first[0].Change, want) {
		t.Fatalf("first reorder saves = %#v, want %#v", first, want)
	}
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "d", Column: 1, Reorder: true, Index: 0}))
	m, cmds2 := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 0 || len(orderSaves(cmds2)) != 0 {
		t.Fatalf("a second and third reorder sent %#v and %#v while the first is out, want none", cmds, cmds2)
	}
	want := []string{"b", "d", "c", "a"}
	if got := laneUIDs(m, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("Lane shows %v, want %v", got, want)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: first[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b", "d"}}})
	second := orderSaves(cmds)
	if wantChange := (model.OrderChange{Shown: []string{"c", "a", "b", "d"}, Move: "d", Anchor: "c"}); len(second) != 1 || !reflect.DeepEqual(second[0].Change, wantChange) {
		t.Fatalf("the first answer sent %#v, want only the second reorder %#v", cmds, wantChange)
	}
	if got := laneUIDs(m, 1); !reflect.DeepEqual(got, want) {
		t.Errorf("the first answer showed %v, want %v", got, want)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: second[0], Current: model.LaneOrders{"needs-triage": {"d", "c", "a", "b"}}})
	third := orderSaves(cmds)
	if len(third) != 1 || third[0].Change.Move != "b" {
		t.Fatalf("the second answer sent %#v, want only the third reorder", cmds)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: third[0], Current: model.LaneOrders{"needs-triage": {"b", "d", "c", "a"}}})
	if got := laneUIDs(m, 1); !reflect.DeepEqual(got, want) || len(cmds) != 0 {
		t.Errorf("after every answer the Lane shows %v with cmds %#v, want %v and none", got, cmds, want)
	}
}

func TestPrunesOfSeveralLanesGoOutOneByOne(t *testing.T) {
	stored := model.LaneOrders{"needs-triage": {"a", "t"}, "needs-info": {"b", "i"}, "": {"c", "n"}}
	m := orderedBoard(t, 160, model.LanesLoaded{Order: stored},
		triage("t"), triage("a"), dragCard("i", "needs-info"), dragCard("b", "needs-info"), dragCard("n"), dragCard("c"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("t"), dragClosed("b", "needs-info"), dragCard("i", "needs-info"), dragClosed("c"), dragCard("n"))
	var seen []string
	for round := range 3 {
		saves := orderSaves(cmds)
		if len(saves) != 1 || !saves[0].Prune || len(saves[0].Change.Remove) != 1 {
			t.Fatalf("round %d: cmds %#v, want exactly one prune", round, cmds)
		}
		seen = append(seen, saves[0].Change.Remove[0])
		m, cmds = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: stored})
	}
	slices.Sort(seen)
	if !slices.Equal(seen, []string{"a", "b", "c"}) || len(cmds) != 0 {
		t.Errorf("pruned %v, then cmds %#v; want a, b and c one by one and nothing after", seen, cmds)
	}
}

func TestAChangeOfAProjectSwitchedAwayFromIsStillSent(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	first := orderSaves(cmds)
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if len(first) != 1 || len(orderSaves(cmds)) != 0 {
		t.Fatalf("first %#v then %#v, want one sent and one waiting", first, cmds)
	}
	m, _ = model.Update(m, model.CollectionSelected{Collection: model.Collection{View: "all-open", Title: "All Open"}})
	if m.Collection.ProjectUID == "p" {
		t.Fatal("the Project did not switch")
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: first[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b"}}})
	sent := orderSaves(cmds)
	if len(sent) != 1 || sent[0].ProjectUID != "p" || sent[0].Change.Move != "b" {
		t.Errorf("after the switch the first answer sent %#v, want the waiting change of Project p", cmds)
	}
}

func TestAPruneLeavesAnotherPanesReorderAlone(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a", "c"}}}, triage("a"), triage("b"), triage("c"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"), triage("c"))
	saves := orderSaves(cmds)
	if len(saves) != 1 || !reflect.DeepEqual(saves[0].Change, model.OrderChange{Remove: []string{"a"}}) {
		t.Fatalf("saves = %#v, want only the removal of a", saves)
	}
	// Another Pane put c first meanwhile; a read answers with that.
	m, cmds = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, Type: "project.metadata_updated"})
	read := laneRead(t, cmds)
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: read.Request, Order: model.LaneOrders{"needs-triage": {"c", "b", "a"}}})
	if got, want := laneUIDs(m, 1), []string{"c", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Lane shows %v, want the other Pane's order without the closed a: %v", got, want)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: model.LaneOrders{"needs-triage": {"c", "b"}}})
	if got, want := laneUIDs(m, 1), []string{"c", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the answer the Lane shows %v, want %v", got, want)
	}
}

func TestAFailedPruneIsSilentAndNotSentAgainUntilTheNextLaneListRead(t *testing.T) {
	stored := model.LaneOrders{"needs-triage": {"hidden", "b", "a"}}
	m := orderedBoard(t, 160, model.LanesLoaded{Order: stored}, triage("a"), triage("b"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	saves := orderSaves(cmds)
	if len(saves) != 1 {
		t.Fatalf("saves = %#v, want one prune", saves)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: stored, Err: errors.New("lanes.toml is read-only")})
	if m.Notice != "" || len(cmds) != 0 {
		t.Errorf("a failed prune showed %q and sent %#v, want nothing", m.Notice, cmds)
	}
	m, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("the failed prune was sent again: %#v", cmds)
	}
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest(), Order: stored})
	_, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	if len(orderSaves(cmds)) != 1 {
		t.Errorf("after a Lane list read the prune was not tried again: %#v", cmds)
	}
}

func TestAFailedPruneWithNoStoredOrdersDoesNotReadTheLaneListAgain(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("a"), triage("b"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	saves := orderSaves(cmds)
	if len(saves) != 1 {
		t.Fatalf("saves = %#v, want one prune", saves)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: saves[0], Err: errors.New("the Project could not be read")})
	if m.Notice != "" || len(cmds) != 0 {
		t.Errorf("a failed prune showed %q and sent %#v, want nothing", m.Notice, cmds)
	}
}

func TestAReorderWhileAMoveOfANamedCardIsOutDoesNotRemoveIt(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("x"), triage("a"), triage("b"))
	m, _ = model.MoveCard(m, "a", 1)
	if got, want := laneUIDs(m, 1), []string{"x", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Needs triage shows %v while a moves out, want %v", got, want)
	}
	_, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(saves) != 1 || len(saves[0].Change.Remove) != 0 {
		t.Fatalf("saves = %#v, want one reorder that removes nothing while the Move is out", saves)
	}
}

func TestAnAnswerFromBeforeADaemonSwitchDoesNotMatchANewIdenticalChange(t *testing.T) {
	issues := func() []model.Issue { return []model.Issue{triage("a"), triage("b"), triage("c")} }
	m := orderedBoard(t, 160, model.LanesLoaded{}, issues()...)
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	old := orderSaves(cmds)
	if len(old) != 1 {
		t.Fatalf("cmds = %#v, want one SaveLaneOrder", cmds)
	}
	m, ask := switchDaemon(t, m)
	m, _ = model.Update(m, model.DaemonSwitched{Request: ask.Request, Name: "other"})
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	fresh := issues()
	newestFirst(fresh)
	m, _ = loadFirst(m, model.ProjectLoaded{Connection: m.Connection, Project: project},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: fresh, Catalog: []model.CatalogProject{{Project: project}}}, LocalDate: "2026-10-08", TimeZone: "UTC"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "b"})
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest()})
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	again := orderSaves(cmds)
	if len(again) != 1 || !reflect.DeepEqual(again[0].Change, old[0].Change) || again[0].Seq == old[0].Seq {
		t.Fatalf("new saves = %#v, want one identical change under another number than %d", again, old[0].Seq)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: old[0], Err: errors.New("the old daemon timed out")})
	if m.Notice != "" || len(cmds) != 0 {
		t.Errorf("the old daemon's answer showed %q and sent %#v, want it dropped", m.Notice, cmds)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: again[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b"}}})
	if got, want := laneUIDs(m, 1), []string{"c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the real answer the Lane shows %v, want %v", got, want)
	}
	if _, cmds = model.Update(m, model.LaneOrderSaved{Save: again[0], Err: errors.New("again")}); len(cmds) != 0 {
		t.Errorf("an answer already taken sent %#v", cmds)
	}
}

func TestADragBeforeTheLaneListLoadsIsRefused(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	m, _ = model.Update(m, model.ResizeMsg{Cols: 160})
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	issues := []model.Issue{triage("a"), triage("b")}
	newestFirst(issues)
	m, _ = loadFirst(m, model.ProjectLoaded{Project: project},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}, LocalDate: "2026-10-08", TimeZone: "UTC"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "b"})
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 0 || m.LaneOrderCustom("needs-triage") {
		t.Errorf("cmds = %#v, custom %v; want nothing saved before the Lane list loads", cmds, m.LaneOrderCustom("needs-triage"))
	}
	if want := "Cannot save the custom order: the Board is still loading its Lanes"; !strings.HasPrefix(m.Notice, want) || m.NoticeKind != model.NoticeFail {
		t.Errorf("notice = %q (%v), want %q", m.Notice, m.NoticeKind, want)
	}
}

func TestAUserChangeToALaneLiftsTheBarOnPruningIt(t *testing.T) {
	stored := model.LaneOrders{"needs-triage": {"hidden", "c", "b", "a"}}
	m := orderedBoard(t, 160, model.LanesLoaded{Order: stored}, triage("a"), triage("b"), triage("c"))
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: m.Cursor + 1, Type: "issue.closed"})
	m, cmds := snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"), triage("c"))
	saves := orderSaves(cmds)
	if len(saves) != 1 || !saves[0].Prune {
		t.Fatalf("saves = %#v, want one prune", saves)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: stored, Err: errors.New("lanes.toml is read-only")})
	m, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"), triage("c"))
	if len(orderSaves(cmds)) != 0 {
		t.Fatalf("the failed prune was sent again: %#v", cmds)
	}
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	user := orderSaves(cmds)
	if len(user) != 1 || user[0].Prune {
		t.Fatalf("saves = %#v, want the user's reorder", user)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: user[0], Current: stored})
	_, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"), triage("c"))
	if got := orderSaves(cmds); len(got) != 1 || !got[0].Prune {
		t.Errorf("after a change of the Lane the prune was not sent again: %#v", cmds)
	}
}

func TestADropWhereAnotherPaneResetTheLaneLeavesItInTheListsOrder(t *testing.T) {
	issues := func() []model.Issue {
		return []model.Issue{triage("a"), triage("b"), triage("c"), dragCard("m", "needs-info")}
	}
	drop := model.CardDropped{UID: "m", Column: 1, Target: "c"}
	control := orderedBoard(t, 160, model.LanesLoaded{}, issues()...)
	control, _ = model.Update(control, dropOn(control, drop))

	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"c", "a"}}}, issues()...)
	m, cmds := model.Update(m, dropOn(m, drop))
	saves := orderSaves(cmds)
	if len(saves) != 1 || !saves[0].Change.OnlyOrdered {
		t.Fatalf("cmds = %#v, want the Move and one drop change", cmds)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: model.LaneOrders{}})
	if m.LaneOrderCustom("needs-triage") {
		t.Error("the Lane still has a custom order after another Pane reset it")
	}
	if got, want := laneUIDs(m, 1), laneUIDs(control, 1); !reflect.DeepEqual(got, want) {
		t.Errorf("Lane shows %v, want the list's order %v", got, want)
	}
}

func TestADragDuringAReloadOfTheSameProjectIsAccepted(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, _ = model.Update(m, model.KeyMsg{Chord: "b"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "b"})
	laneRead(t, cmds)
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	if len(orderSaves(cmds)) != 1 || m.NoticeKind == model.NoticeFail {
		t.Errorf("cmds = %#v, notice %q; want the reorder saved while the Lane list is read again", cmds, m.Notice)
	}
}

func TestAReorderWhileOnlyASnapshotReadIsOutRemovesNothing(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"b", "a"}}}, triage("x"), triage("a"), triage("b"))
	m, _ = model.MoveCard(m, "a", 1)
	m, _ = model.Update(m, model.MoveResultMsg{UID: "a"})
	if m.Moving != "" || !m.Refreshing {
		t.Fatalf("Moving %q, Refreshing %v; want the Move done and a snapshot read out", m.Moving, m.Refreshing)
	}
	_, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(saves) != 1 || len(saves[0].Change.Remove) != 0 {
		t.Fatalf("saves = %#v, want one reorder that removes nothing while a read is out", saves)
	}
}

func TestAChangeQueuedBehindOneOutIsNeverSentAfterADaemonSwitch(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	first := orderSaves(cmds)
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if len(first) != 1 || len(orderSaves(cmds)) != 0 {
		t.Fatalf("first %#v then %#v, want one sent and one queued", first, cmds)
	}
	m, ask := switchDaemon(t, m)
	m, _ = model.Update(m, model.DaemonSwitched{Request: ask.Request, Name: "other"})
	_, cmds = model.Update(m, model.LaneOrderSaved{Save: first[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b"}}})
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("the old daemon's answer sent %#v, want the queued change gone", cmds)
	}
}
