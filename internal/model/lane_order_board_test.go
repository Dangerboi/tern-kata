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
	for i := range issues {
		if issues[i].ClosedAt == nil {
			issues[i].UpdatedAt = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour)
		}
	}
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
	m, cmds := model.Update(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: 0})
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
	m, cmds := model.Update(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: 1})
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
	m, cmds := model.Update(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0})
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
		next, cmds := model.Update(m, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: index})
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
			m, cmds := model.Update(m, tc.drop)
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
	m, cmds := model.Update(m, model.CardDropped{UID: "m", Column: 1, Target: "a"})
	if len(cmds) != 1 || len(moveCmds(cmds)) != 1 || m.LaneOrderCustom("needs-triage") {
		t.Errorf("cmds = %#v, custom %v; want only the Move", cmds, m.LaneOrderCustom("needs-triage"))
	}
}

func TestRefusedMoveLeavesTheOrderAlone(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Order: model.LaneOrders{"needs-triage": {"a"}}}, triage("a"), triage("b"), dragCard("m", "needs-info"))
	m.Stale = true
	m, cmds := model.Update(m, model.CardDropped{UID: "m", Column: 1, Target: "a"})
	if len(orderSaves(cmds)) != 0 || !reflect.DeepEqual(laneUIDs(m, 1), []string{"b", "a"}) {
		t.Errorf("cmds %#v, Lane shows %v", cmds, laneUIDs(m, 1))
	}
}

func TestFailedOrderSaveShowsTheStoredOrderAndSays(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{}, triage("a"), triage("b"), triage("c"))
	m, cmds := model.Update(m, model.CardDropped{UID: "c", Column: 1, Reorder: true, Index: 0})
	save := orderSaves(cmds)[0]
	stored := model.LaneOrders{"needs-triage": {"b", "a"}}
	m, _ = model.Update(m, model.LaneOrderSaved{Save: save, Current: stored, Err: errors.New("the Project changed")})
	if !strings.HasPrefix(m.Notice, "Cannot save the Lane order: ") || !strings.Contains(m.Notice, "the Project changed") || m.NoticeKind != model.NoticeFail {
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
			m, cmds := model.Update(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0})
			if len(orderSaves(cmds)) != 0 || m.LaneOrderCustom("needs-triage") {
				t.Errorf("cmds %#v, custom %v; want a refusal", cmds, m.LaneOrderCustom("needs-triage"))
			}
			if !strings.HasPrefix(m.Notice, "Cannot save the Lane order: ") || m.NoticeKind != model.NoticeFail {
				t.Errorf("notice = %q (%v)", m.Notice, m.NoticeKind)
			}
		})
	}
}

func TestSharedOrderOfAReadProjectIsSavedShared(t *testing.T) {
	m := orderedBoard(t, 160, model.LanesLoaded{Shared: true, ProjectID: 7, Revision: 3}, triage("a"), triage("b"))
	_, cmds := model.Update(m, model.CardDropped{UID: "b", Column: 1, Reorder: true, Index: 0})
	saves := orderSaves(cmds)
	if len(saves) != 1 || !saves[0].Shared || saves[0].ProjectID != 7 {
		t.Errorf("saves = %#v, want one shared save for Project 7", saves)
	}
}
