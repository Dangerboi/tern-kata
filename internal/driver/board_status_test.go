package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// statusDefaultPane is the Issues Page of a Project whose saved default filter is status=open.
func statusDefaultPane(t *testing.T, issues ...model.Issue) driver.State {
	t.Helper()
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: project, Default: model.Filters{Status: "open"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}, LocalDate: boardToday, TimeZone: "UTC"})
	return feed(t, s).State
}

func TestTheBoardReadsWithoutTheStatusFilterAndTheIssuesPageWithIt(t *testing.T) {
	closed := closedAt("done1", 10, 5)
	s := statusDefaultPane(t, boardCard("a"))
	if s.Model.Filters.Status != "open" {
		t.Fatalf("the Pane starts with filters %#v", s.Model.Filters)
	}
	out := step(t, s, key("b"))
	read := firstRead(t, out.Cmds)
	if read.Query.Status != nil {
		t.Fatalf("the Board's read carries status %v", read.Query.Status)
	}
	if got := out.State.Model.Filters.Status; got != "open" {
		t.Errorf("the status filter is %q on the Board, want it kept", got)
	}
	board := feed(t, out.State, model.LanesLoaded{ProjectUID: "p"}).State
	board = step(t, board, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Issues: []model.Issue{boardCard("a"), closed}}, LocalDate: boardToday, TimeZone: "UTC"}).State
	wantLane(t, board, 5, "Done", "done1")

	lane := feed(t, board, key("right"), key("enter")).State
	if !lane.Model.OnBoard() || lane.Model.Board.Mode != model.BoardLane {
		t.Fatalf("not in the Lane view")
	}
	if q := lane.Model.SnapshotQuery(); q.Status != nil {
		t.Errorf("the Lane view's query carries status %v", q.Status)
	}

	back := step(t, board, key("b"))
	if read := firstRead(t, back.Cmds); !slices.Equal(read.Query.Status, []string{"open"}) {
		t.Errorf("the Issues Page's read carries status %v, want open", read.Query.Status)
	}
}

func boardFilterCard(t *testing.T) driver.State {
	t.Helper()
	out := step(t, statusDefaultPane(t, boardCard("a")), key("b"))
	loaded, _ := snapshotFor(out.Cmds, boardCard("a"))
	return feed(t, out.State, loaded, model.LanesLoaded{ProjectUID: "p"}, key("f")).State
}

func drawsStatusRow(t *testing.T, s driver.State) bool {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasPrefix(n.ID, "layer.filters.status") {
			return true
		}
	}
	return false
}

func TestTheBoardsFilterCardHasNoStatusRowButKeepsTheValue(t *testing.T) {
	card := boardFilterCard(t)
	if !card.Model.FilterForm.Open {
		t.Fatal("the Filter card is not open")
	}
	if drawsStatusRow(t, card) {
		t.Error("the Board's Filter card draws a Status row")
	}
	if !drawsStatusRow(t, feed(t, statusDefaultPane(t, boardCard("a")), key("f")).State) {
		t.Error("the Issues Page's Filter card lost its Status row")
	}
	tabbed := feed(t, card, key("tab")).State.Model.FilterForm.Field
	if tabbed == model.FilterStatus {
		t.Error("tab focuses the hidden Status field")
	}

	save := step(t, card, key("ctrl+d"))
	want := []model.Cmd{model.SaveFilters{ProjectUID: "p", ProjectName: "work", Default: model.Filters{Status: "open"}}}
	if !slices.Equal(save.Cmds, want) {
		t.Errorf("ctrl+d sent %#v, want %#v", save.Cmds, want)
	}

	apply := step(t, feed(t, card, key("tab"), key("x")).State, key("ctrl+o"))
	if got := apply.State.Model.Filters; got.Status != "open" || got.Owner != "x" {
		t.Errorf("Apply left filters %#v, want status open kept and owner x", got)
	}
	if read := firstRead(t, apply.Cmds); read.Query.Status != nil {
		t.Errorf("the read after Apply carries status %v", read.Query.Status)
	}
}

func TestTheBoardsFilterLineOmitsStatusAndKeepsDefault(t *testing.T) {
	board := feed(t, statusDefaultPane(t, boardCard("a")), key("b"), model.LanesLoaded{ProjectUID: "p"}).State
	if got := filterLineText(t, board); strings.Contains(got, "status") {
		t.Errorf("the Board's filter line = %q, want no status", got)
	}
	withOwner := model.Filters{Status: "open", Owner: "me"}
	pane := board
	pane.Model.Filters, pane.Model.DefaultFilters = withOwner, withOwner
	if got := filterLineText(t, pane); got != "owner=me (default)" {
		t.Errorf("filter line = %q, want %q", got, "owner=me (default)")
	}
	issues := feed(t, pane, key("b")).State
	if got := filterLineText(t, issues); got != "status=open · owner=me (default)" {
		t.Errorf("the Issues Page's filter line = %q", got)
	}
}

func TestSDoesNothingOnTheBoardAndIsNotInItsFooter(t *testing.T) {
	board := feed(t, statusDefaultPane(t, boardCard("a", "needs-triage")), key("b"), model.LanesLoaded{ProjectUID: "p"}).State
	lane := feed(t, board, key("right"), key("enter")).State
	for name, s := range map[string]driver.State{"Board": board, "Lane view": lane} {
		out := step(t, s, key("s"))
		if out.State.Model.Filters != s.Model.Filters || readsSnapshot(out) {
			t.Errorf("%s: s changed filters to %#v or read", name, out.State.Model.Filters)
		}
		if footer := footerText(t, s); strings.Contains(footer, "status") {
			t.Errorf("%s footer lists status: %s", name, footer)
		}
	}
	issues := statusDefaultPane(t, boardCard("a"))
	if got := step(t, issues, key("s")).State.Model.Filters.Status; got != "closed" {
		t.Errorf("s on the Issues Page left status %q, want closed", got)
	}
	if footer := footerText(t, issues); !strings.Contains(footer, "status") {
		t.Errorf("the Issues Page footer lost status: %s", footer)
	}
}
