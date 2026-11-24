package model_test

import (
	"errors"
	"reflect"
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
	if len(saves) != 1 || saves[0].Lane != model.LaneNone || !reflect.DeepEqual(saves[0].Order, []string{"c", "a", "b"}) {
		t.Fatalf("saves = %#v, want No lane's order c a b", saves)
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
	if len(saves) != 1 || saves[0].Lane != model.LaneNone || !reflect.DeepEqual(saves[0].Order, []string{"hidden", "b"}) {
		t.Fatalf("saves = %#v, want No lane's order without the closed a", saves)
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
	want := model.SaveLaneOrder{Workspace: "/repo", Actor: saves[0].Actor, ProjectUID: "p", ProjectName: saves[0].ProjectName, ProjectID: saves[0].ProjectID,
		Lane: "needs-triage", Order: []string{"a", "b", "d", "x", "c"}}
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
	if len(saves) != 1 || !reflect.DeepEqual(saves[0].Order, []string{"x", "b", "a", "c"}) {
		t.Fatalf("saves = %#v, want the order x b a c", saves)
	}
	if got, want := laneUIDs(m, 1), []string{"b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shows %v, want %v", got, want)
	}
}

func TestReorderInALaneWithoutAnOrderStartsOneFromTheShownCards(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	saves := orderSaves(cmds)
	if len(saves) != 1 || !reflect.DeepEqual(saves[0].Order, []string{"c", "a", "b"}) {
		t.Fatalf("saves = %#v, want the order c a b", saves)
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
		drop model.CardDropped
		want []string
	}{
		"before the card": {model.CardDropped{UID: "m", Column: 1, Target: "c"}, []string{"b", "d", "m", "c", "a"}},
		"after the card":  {model.CardDropped{UID: "m", Column: 1, Target: "c", After: true}, []string{"b", "d", "c", "m", "a"}},
		"empty space":     {model.CardDropped{UID: "m", Column: 1}, []string{"b", "d", "c", "a", "m"}},
	} {
		t.Run(name, func(t *testing.T) {
			m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"c", "a"}}},
				triage("a"), triage("b"), triage("c"), triage("d"), dragCard("m", "needs-info"))
			m, cmds := model.Update(m, dropOn(m, tc.drop))
			saves := orderSaves(cmds)
			if len(moveCmds(cmds)) != 1 || len(saves) != 1 || len(cmds) != 2 {
				t.Fatalf("cmds = %#v, want the Move and one SaveLaneOrder", cmds)
			}
			if saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Order, tc.want) {
				t.Errorf("save = %#v, want Lane needs-triage order %v", saves[0], tc.want)
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
	other, _ := model.Update(m, model.LaneOrderSaved{Save: model.SaveLaneOrder{ProjectUID: "elsewhere"}, Current: model.LaneOrders{"needs-triage": {"b"}}, Err: errors.New("boom")})
	if other.LaneOrderCustom("needs-triage") || other.Notice != "" {
		t.Errorf("another Project's answer changed the Board: notice %q", other.Notice)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: model.SaveLaneOrder{ProjectUID: "p"}, Current: model.LaneOrders{"needs-triage": {"b", "a"}}})
	if got, want := laneUIDs(m, 1), []string{"b", "a"}; !reflect.DeepEqual(got, want) || m.Notice != "" {
		t.Errorf("Lane shows %v, notice %q", got, m.Notice)
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
	if len(cmds) != 1 || len(saves) != 1 || saves[0].Lane != "needs-triage" || len(saves[0].Order) != 0 {
		t.Fatalf("cmds = %#v, want one SaveLaneOrder with no order", cmds)
	}
	if m.LaneOrderCustom("needs-triage") || !reflect.DeepEqual(laneUIDs(m, 1), []string{"a", "b"}) {
		t.Errorf("custom %v, Lane shows %v; want the list's order", m.LaneOrderCustom("needs-triage"), laneUIDs(m, 1))
	}
}

func TestSharedOrderOnAnUnreadOrArchivedProjectIsRefused(t *testing.T) {
	for name, loaded := range map[string]model.LanesLoaded{
		"unread":   {Shared: true},
		"archived": {Shared: true, ProjectID: 7, Revision: 3, Archived: true},
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
	m, cmds = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: old.Request})
	if got, want := laneUIDs(m, 1), []string{"c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the old read changed the Lane to %v, want %v", got, want)
	}
	laneRead(t, cmds)
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
	if len(saves) != 1 || saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Order, []string{"b"}) {
		t.Fatalf("saves = %#v, want Needs triage's order without a", saves)
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
	if len(saves) != 1 || saves[0].Lane != "needs-triage" || !reflect.DeepEqual(saves[0].Order, []string{"hidden", "b"}) {
		t.Fatalf("saves = %#v, want the order without the closed a and with the hidden uid", saves)
	}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: saves[0], Current: model.LaneOrders{"needs-triage": {"hidden", "b"}}})
	_, cmds = snapshotAfter(m, dragClosed("a", "needs-triage"), triage("b"))
	if len(orderSaves(cmds)) != 0 {
		t.Errorf("the pruned order was saved again: %#v", cmds)
	}
}

func TestQuickReordersKeepOneSaveOutAndSendTheLatestAfterItsAnswer(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"), triage("d"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0}))
	first := orderSaves(cmds)
	if len(first) != 1 || !reflect.DeepEqual(first[0].Order, []string{"c", "a", "b", "d"}) {
		t.Fatalf("first reorder saves = %#v, want the order c a b d", first)
	}
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "d", Column: 1, Reorder: true, Index: 0}))
	if len(cmds) != 0 {
		t.Fatalf("second reorder sent %#v while the first save is out", cmds)
	}
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0}))
	if len(cmds) != 0 {
		t.Fatalf("third reorder sent %#v while the first save is out", cmds)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: first[0], Current: model.LaneOrders{"needs-triage": {"c", "a", "b", "d"}}})
	next := orderSaves(cmds)
	if len(next) != 1 || len(cmds) != 1 || !reflect.DeepEqual(next[0].Order, []string{"b", "d", "c", "a"}) {
		t.Fatalf("after the first answer cmds = %#v, want one save of the latest order b d c a", cmds)
	}
	if got, want := laneUIDs(m, 1), []string{"b", "d", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the first answer's orders were shown: Lane shows %v, want %v", got, want)
	}
	m, cmds = model.Update(m, model.LaneOrderSaved{Save: next[0], Current: model.LaneOrders{"needs-triage": {"b", "d", "c", "a"}}})
	if len(cmds) != 0 || !reflect.DeepEqual(laneUIDs(m, 1), []string{"b", "d", "c", "a"}) {
		t.Errorf("cmds %#v, Lane shows %v", cmds, laneUIDs(m, 1))
	}
}
