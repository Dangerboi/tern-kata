package driver_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// The Pane calendar of these tests is 2026-10-08, so Done reaches back to 2026-09-24.
const boardToday = "2026-10-08"

func boardCard(uid string, labels ...string) model.Issue {
	return model.Issue{UID: uid, ShortID: uid, Title: "Title of " + uid, Status: "open", ProjectName: "work", Labels: labels,
		UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
}

// updatedAt is the card last updated at the given hour of 2026-10-01: a later hour lists first.
func updatedAt(issue model.Issue, hour int) model.Issue {
	issue.UpdatedAt = time.Date(2026, 10, 1, hour, 0, 0, 0, time.UTC)
	return issue
}

// closedAt is the card closed at noon of the given day.
func closedAt(uid string, month time.Month, day int, labels ...string) model.Issue {
	issue := boardCard(uid, labels...)
	at := time.Date(2026, month, day, 12, 0, 0, 0, time.UTC)
	issue.Status, issue.ClosedAt = "closed", &at
	return issue
}

func ownedBy(issue model.Issue, owner string) model.Issue {
	issue.Owner = &owner
	return issue
}

// projectPane is a Pane of the given width on a Project with the Issues loaded.
func projectPane(t *testing.T, cols int, issues ...model.Issue) driver.State {
	t.Helper()
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(cols)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues}, LocalDate: boardToday, TimeZone: "UTC"})
	return feed(t, s).State
}

// boardPane is projectPane at 160 columns with the Board on and the Lane list load answered with no saved list.
func boardPane(t *testing.T, issues ...model.Issue) driver.State {
	t.Helper()
	return feed(t, projectPane(t, 160, issues...), key("b"), model.LanesLoaded{ProjectUID: "p"}).State
}

// snapshotFor is the daemon answering the read cmds hold with issues.
func snapshotFor(cmds []model.Cmd, issues ...model.Issue) (model.SnapshotLoaded, bool) {
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			return model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Issues: issues}, LocalDate: boardToday, TimeZone: "UTC"}, true
		}
	}
	return model.SnapshotLoaded{}, false
}

// answered runs in, then answers the snapshot read it asks for with issues.
func answered(t *testing.T, s driver.State, in any, issues ...model.Issue) driver.Output {
	t.Helper()
	out := step(t, s, in)
	snapshot, ok := snapshotFor(out.Cmds, issues...)
	if !ok {
		t.Fatalf("%T asked for no snapshot: %#v", in, out.Cmds)
	}
	return step(t, out.State, snapshot)
}

func wireText(node tern.WireNode) string {
	raw, _ := json.Marshal(node)
	return string(raw)
}

// lanes reads the Board as drawn: each column's head text and the uids of its cards, in drawn order.
func lanes(t *testing.T, s driver.State) (heads []string, cards [][]string) {
	t.Helper()
	nodes := wireNodes(t, view.Render(s.Model, now))
	for i := 0; ; i++ {
		id := fmt.Sprintf("%s.lane%d", view.BoardID, i)
		at := slices.IndexFunc(nodes, func(n tern.WireNode) bool { return n.ID == id })
		if at < 0 {
			return heads, cards
		}
		head := slices.IndexFunc(nodes, func(n tern.WireNode) bool { return n.ID == id+".head" })
		if head < 0 {
			t.Fatalf("Lane %d draws no head", i)
		}
		heads = append(heads, wireText(nodes[head]))
		uids := []string{}
		for _, n := range nodes {
			if rest, ok := strings.CutPrefix(n.ID, id+".cards."); ok && !strings.Contains(rest, ".") {
				uids = append(uids, rest)
			}
		}
		cards = append(cards, uids)
	}
}

func cardNode(t *testing.T, s driver.State, uid string) tern.WireNode {
	t.Helper()
	id := view.CardID(s.Model, uid)
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("the Board draws no card for %s", uid)
	return tern.WireNode{}
}

// wantLane checks column i is titled title and holds exactly want, in order.
func wantLane(t *testing.T, s driver.State, i int, title string, want ...string) {
	t.Helper()
	heads, cards := lanes(t, s)
	if i >= len(heads) {
		t.Fatalf("the Board draws %d columns, want a column %d", len(heads), i)
	}
	if !strings.Contains(heads[i], title) {
		t.Errorf("column %d head = %s, want %q", i, heads[i], title)
	}
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(cards[i], want) {
		t.Errorf("%s holds %v, want %v", title, cards[i], want)
	}
}

func TestBoardShowsNoLaneTheDefaultLanesAndDone(t *testing.T) {
	s := boardPane(t,
		boardCard("plain"), boardCard("bugonly", "bug"),
		boardCard("triage", "needs-triage"), boardCard("info", "needs-info"), boardCard("agent", "ready-for-agent", "bug"), boardCard("human", "ready-for-human"),
		closedAt("fresh", time.October, 7, "ready-for-agent"), closedAt("stale", time.September, 20))
	heads, _ := lanes(t, s)
	if len(heads) != 6 {
		t.Fatalf("the Board draws %d columns, want 6", len(heads))
	}
	wantLane(t, s, 0, "No lane", "bugonly", "plain") // equal update times list by uid
	wantLane(t, s, 1, "Needs triage", "triage")
	wantLane(t, s, 2, "Needs info", "info")
	wantLane(t, s, 3, "Ready for agent", "agent")
	wantLane(t, s, 4, "Ready for human", "human")
	wantLane(t, s, 5, "Done", "fresh")
}

func TestLaneHeadersShowTheirCardCount(t *testing.T) {
	s := boardPane(t, boardCard("a"), boardCard("b"), boardCard("c", "needs-info"))
	heads, _ := lanes(t, s)
	if !strings.Contains(heads[0], `"t":"2"`) || !strings.Contains(heads[2], `"t":"1"`) || !strings.Contains(heads[1], `"t":"0"`) {
		t.Errorf("heads = %v, want counts 2, 0 and 1", heads)
	}
}

func TestCardsFollowTheListOrder(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("old"), 1), updatedAt(boardCard("new"), 9), updatedAt(boardCard("mid"), 5))
	wantLane(t, s, 0, "No lane", "new", "mid", "old")
}

func TestDoneHoldsTheLastFourteenDaysNewestFirst(t *testing.T) {
	s := boardPane(t, closedAt("before", time.September, 23), closedAt("first", time.September, 24), closedAt("mid", time.October, 3), closedAt("last", time.October, 8))
	wantLane(t, s, 5, "Done", "last", "mid", "first")
}

func TestAnIssueWithTwoLaneLabelsShowsOnceWithAnAlsoNote(t *testing.T) {
	s := boardPane(t, boardCard("both", "ready-for-human", "needs-info"))
	wantLane(t, s, 2, "Needs info", "both")
	wantLane(t, s, 4, "Ready for human")
	if text := wireText(cardNode(t, s, "both")); !strings.Contains(text, "also: ready-for-human") {
		t.Errorf("card = %s, want an also note", text)
	}
}

func TestDoneCardsNameTheLaneTheyReopenInto(t *testing.T) {
	s := boardPane(t, closedAt("labelled", time.October, 7, "needs-info"), closedAt("bare", time.October, 6), boardCard("open"))
	if text := wireText(cardNode(t, s, "labelled")); !strings.Contains(text, "r → Needs info") {
		t.Errorf("labelled card = %s", text)
	}
	if text := wireText(cardNode(t, s, "bare")); !strings.Contains(text, "r → No lane") {
		t.Errorf("bare card = %s", text)
	}
	if text := wireText(cardNode(t, s, "open")); strings.Contains(text, "r →") {
		t.Errorf("open card = %s, want no reopen note", text)
	}
}

func TestOnlyTheSelectedCardShowsItsFullTitleOwnerAndLabels(t *testing.T) {
	selected := ownedBy(boardCard("sel", "ready-for-agent", "bug"), "dana")
	other := ownedBy(boardCard("oth", "ready-for-agent", "bug"), "erin")
	s := boardPane(t, updatedAt(selected, 9), updatedAt(other, 1))
	if s.Model.SelectedUID != "sel" {
		t.Fatalf("fixture: selected %q", s.Model.SelectedUID)
	}
	sel, oth := wireText(cardNode(t, s, "sel")), wireText(cardNode(t, s, "oth"))
	for _, want := range []string{"Title of sel", "dana", "ready-for-agent", "bug"} {
		if !strings.Contains(sel, want) {
			t.Errorf("selected card lacks %q: %s", want, sel)
		}
	}
	if strings.Contains(sel, `"lines":2`) {
		t.Errorf("selected card clamps its title: %s", sel)
	}
	if !strings.Contains(oth, `"lines":2`) || !strings.Contains(oth, "bug") {
		t.Errorf("other card = %s, want a two line title and its bug label", oth)
	}
	if strings.Contains(oth, "erin") || strings.Contains(oth, "ready-for-agent") {
		t.Errorf("other card shows owner or Lane label: %s", oth)
	}
}

func TestArrowsAndJKMoveTheSelectionSkippingEmptyLanes(t *testing.T) {
	s := boardPane(t,
		updatedAt(boardCard("a1"), 9), updatedAt(boardCard("a2"), 8),
		boardCard("c1", "needs-info"),
		boardCard("e1", "ready-for-human"), updatedAt(boardCard("e2", "ready-for-human"), 0))
	selected := func(s driver.State) string { return s.Model.SelectedUID }
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"j"}, "a2"},
		{[]string{"down", "down"}, "a2"},
		{[]string{"j", "k"}, "a1"},
		{[]string{"right"}, "c1"},
		{[]string{"right", "right"}, "e1"},
		{[]string{"right", "right", "right"}, "e1"},
		{[]string{"right", "left"}, "a1"},
		{[]string{"left"}, "a1"},
	} {
		got := selected(feed(t, s, keyList(tc.keys)...).State)
		if got != tc.want {
			t.Errorf("%v selects %s, want %s", tc.keys, got, tc.want)
		}
	}
	// Row 1 of the first Lane crosses to the last row of a shorter Lane.
	if got := selected(feed(t, s, key("j"), key("right")).State); got != "c1" {
		t.Errorf("j, right selects %s, want c1", got)
	}
}

func keyList(chords []string) []any {
	list := make([]any, len(chords))
	for i, c := range chords {
		list[i] = key(c)
	}
	return list
}

func TestClickingACardSelectsIt(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b", "needs-info"))
	out := step(t, s, click(view.CardID(s.Model, "b"), view.CardClick("b")))
	if out.State.Model.SelectedUID != "b" {
		t.Fatalf("selected %q", out.State.Model.SelectedUID)
	}
	wantOps(t, out)
}

func TestKeyMovesRevealTheSelectedCard(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b"))
	out := step(t, s, key("j"))
	wantOps(t, out, tern.OpReveal(view.CardID(out.State.Model, "b"), "nearest"))
}

func TestBoardSwitchesOnAndBackKeepingTheSelection(t *testing.T) {
	s := projectPane(t, 160, updatedAt(boardCard("a"), 9), boardCard("b", "needs-info"))
	s = feed(t, s, key("j")).State
	if s.Model.SelectedUID != "b" {
		t.Fatalf("fixture: selected %q", s.Model.SelectedUID)
	}
	on := step(t, s, key("b"))
	if heads, _ := lanes(t, on.State); len(heads) == 0 || on.State.Model.SelectedUID != "b" {
		t.Fatalf("the Board is not shown with b selected: %d columns, selected %q", len(heads), on.State.Model.SelectedUID)
	}
	off := step(t, on.State, key("b"))
	if heads, _ := lanes(t, off.State); len(heads) != 0 || off.State.Model.SelectedUID != "b" {
		t.Fatalf("the list did not come back with b selected: %d columns, selected %q", len(heads), off.State.Model.SelectedUID)
	}
	if !drawnIDs(t, *off.View)[view.IssueID("b")] {
		t.Error("the list does not draw the selected Issue")
	}
}

func TestBoardSelectionOnAnIssueFoldedInTheListSurvivesTheRoundTrip(t *testing.T) {
	kid := boardCard("kid")
	s := loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"}, model.Snapshot{
		Issues: []model.Issue{boardCard("dad"), kid}, Links: []model.Link{{Type: "parent", FromIssueUID: "kid", ToIssueUID: "dad"}}})
	s = feed(t, s, key("b"), key("j")).State
	if s.Model.SelectedUID != "kid" {
		t.Fatalf("fixture: selected %q", s.Model.SelectedUID)
	}
	if got := feed(t, s, key("b")).State.Model.SelectedUID; got != "kid" {
		t.Errorf("selected %q after leaving the Board, want kid", got)
	}
}

func TestBoardModeNeedsAProject(t *testing.T) {
	inbox := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}})
	out := step(t, inbox, key("b"))
	if !strings.Contains(out.State.Model.Notice, "Project") {
		t.Errorf("notice outside a Project = %q", out.State.Model.Notice)
	}
	if heads, _ := lanes(t, out.State); len(heads) != 0 {
		t.Error("the Board opened outside a Project")
	}
}

func TestSwitchingToAnotherCollectionEndsBoardMode(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	out := step(t, s, model.CollectionSelected{Collection: model.FixedCollections()[0]})
	if heads, _ := lanes(t, out.State); len(heads) != 0 {
		t.Error("the Board stays on in a Collection that is not a Project")
	}
	if out.View == nil || !drawnIDs(t, *out.View)["main.columns.list.issues"] {
		t.Error("the list is not drawn")
	}
}

func TestBoardReadsNoDetailAndSelectionLoadsNone(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b"))
	out := step(t, s, key("j"))
	for _, cmd := range out.Cmds {
		if _, ok := cmd.(model.LoadDetail); ok {
			t.Errorf("moving a card loaded a detail: %#v", out.Cmds)
		}
	}
	read, ok := snapshotFor(step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "issue.updated"}).Cmds)
	if !ok || read.Query.SelectedIssueUID != nil {
		t.Errorf("the Board's snapshot read asks for a detail: %+v", read.Query)
	}
}

func TestLeavingTheBoardLoadsTheSelectedDetailAgain(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b"))
	s = feed(t, s, key("j")).State
	out := step(t, s, key("b"))
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}) {
		t.Errorf("commands = %#v", out.Cmds)
	}
}

func TestXOpensTheCloseCardForTheSelectedCard(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b"))
	out := feed(t, s, key("j"), key("x"))
	if out.State.Model.TopCard() != model.CardClose || out.State.Model.CloseForm.UID != "b" {
		t.Errorf("card %v for %q", out.State.Model.TopCard(), out.State.Model.CloseForm.UID)
	}
}

func TestRReopensADoneCardAndItsLaneShowsItNext(t *testing.T) {
	closed := closedAt("done1", time.October, 7, "ready-for-agent")
	s := boardPane(t, closed)
	wantLane(t, s, 5, "Done", "done1")
	out := step(t, s, key("r"))
	want := []model.Cmd{model.ReopenIssueCmd{Workspace: "/repo", UID: "done1", ProjectID: 0, Actor: ""}}
	if !reflect.DeepEqual(out.Cmds, want) {
		t.Fatalf("commands = %#v, want %#v", out.Cmds, want)
	}
	reopened := closed
	reopened.Status, reopened.ClosedAt = "open", nil
	after := answered(t, out.State, model.CloseResultMsg{UID: "done1"}, reopened)
	wantLane(t, after.State, 3, "Ready for agent", "done1")
	wantLane(t, after.State, 5, "Done")
	if after.State.Model.SelectedUID != "done1" {
		t.Errorf("selected %q", after.State.Model.SelectedUID)
	}
}

func TestIssueKeysActOnTheSelectedCardAndEscOpensTheMenu(t *testing.T) {
	s := boardPane(t, updatedAt(boardCard("a"), 9), boardCard("b"))
	s = feed(t, s, key("j")).State
	for chord, want := range map[string]model.Card{"+": model.CardField, "a": model.CardField, "!": model.CardField, "n": model.CardCreate, "x": model.CardClose, "escape": model.CardPicker} {
		got := step(t, s, key(chord)).State.Model.TopCard()
		if got != want {
			t.Errorf("%s opens card %v, want %v", chord, got, want)
		}
	}
	if got := step(t, s, key("+")).State.Model.FieldForm.UID; got != "b" {
		t.Errorf("+ acts on %q, want b", got)
	}
}

func TestFiltersNarrowTheBoard(t *testing.T) {
	open, closed := boardCard("open"), closedAt("shut", time.October, 7)
	s := boardPane(t, open, closed)
	wantLane(t, s, 5, "Done", "shut")
	out := answered(t, s, key("s"), open)
	wantLane(t, out.State, 0, "No lane", "open")
	wantLane(t, out.State, 5, "Done")
}

func TestALiveRefreshMovesCards(t *testing.T) {
	s := boardPane(t, boardCard("a"), boardCard("b", "needs-info"))
	moved := boardCard("a", "needs-triage")
	out := answered(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "issue.labeled"}, moved, boardCard("b", "needs-info"), boardCard("c"))
	wantLane(t, out.State, 0, "No lane", "c")
	wantLane(t, out.State, 1, "Needs triage", "a")
	wantLane(t, out.State, 2, "Needs info", "b")
}

func TestFooterAndHelpSheetListTheBoardKeys(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	footer := ""
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "dock.hints" || strings.HasPrefix(n.ID, "dock.hints.") {
			footer += wireText(n)
		}
	}
	for _, want := range []string{"←→", "Lane", "list"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer lacks %q: %s", want, footer)
		}
	}
	helpText := ""
	for _, n := range wireNodes(t, view.Render(feed(t, s, key("?")).State.Model, now)) {
		helpText += wireText(n)
	}
	for _, want := range []string{"Board", "toggle Issues and Board Pages", "next Lane", "previous Lane"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("help sheet lacks %q", want)
		}
	}
}

func footerText(t *testing.T, s driver.State) string {
	t.Helper()
	footer := ""
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "dock.hints" || strings.HasPrefix(n.ID, "dock.hints.") {
			footer += wireText(n)
		}
	}
	return footer
}

func TestBoardAndLaneFootersListTheLabelAndOwnerKeys(t *testing.T) {
	board := boardPane(t, boardCard("a", "needs-triage"))
	lane := feed(t, board, key("right"), key("enter")).State
	for name, s := range map[string]driver.State{"Board": board, "Lane view": lane} {
		footer := footerText(t, s)
		for _, want := range []string{"label", "unlabel", "owner", "unassign"} {
			if !strings.Contains(footer, want) {
				t.Errorf("%s footer lacks %q: %s", name, want, footer)
			}
		}
	}
}

func TestBoardLaneListsScrollInsideTheirLanesAndLanesShrink(t *testing.T) {
	s := boardPane(t, ownedBy(closedAt("d", 10, 5, "needs-triage"), "someone-with-a-long-name"), boardCard("a"))
	s = feed(t, s, key("right"), key("right")).State // the selected card draws its owner
	nodes := wireNodes(t, view.Render(s.Model, now))
	for col := 0; col < 6; col++ {
		id := view.BoardID + ".lane" + fmt.Sprint(col)
		for _, n := range nodes {
			switch n.ID {
			case id + ".cards":
				if max, err := strconv.ParseFloat(fmt.Sprint(n.P["max"]), 64); err != nil || max >= 1 {
					t.Errorf("Lane %d list max = %v, want a fraction of the window below 1", col, n.P["max"])
				}
			case id:
				if min, _ := json.Marshal(n.P["min"]); !strings.Contains(string(min), `"w":"0ch"`) {
					t.Errorf("Lane %d min = %s, want a 0ch width so it can shrink", col, min)
				}
			}
		}
	}
	for _, n := range nodes {
		if strings.Contains(n.ID, ".owner") || strings.Contains(n.ID, ".reopen") {
			if _, nowrap := n.P["wrap"]; nowrap {
				t.Errorf("%s does not wrap: %v", n.ID, n.P)
			}
		}
	}
}
