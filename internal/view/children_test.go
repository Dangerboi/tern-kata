package view_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

const childrenID = view.DetailContentID + ".children"

// parenting is a wide Pane on a parent "alpha" with three children: "kid" (open,
// owned by dana, updated 38 minutes before testNow), "done" (closed, unowned, 3 hours
// before) and "ghost", which is not in the Collection. The links count 1 open of 3.
func parenting(tab model.ActivityTab) model.Model {
	parent := alpha
	parent.Body = "The parent's body."
	dana := "dana"
	kid := model.Issue{UID: "kid", ShortID: "kidd", Title: "Open kid", Status: "open", ProjectName: "work", Owner: &dana, AssignmentExpiresOn: ptr(testNow.Add(time.Hour)), UpdatedAt: testNow.Add(-38 * time.Minute)}
	done := model.Issue{UID: "done", ShortID: "done", Title: "Closed kid", Status: "closed", ProjectName: "work", UpdatedAt: testNow.Add(-3 * time.Hour)}
	child := func(id int64, uid, status string) model.Link {
		return model.Link{ID: id, Type: "parent", FromIssueUID: uid, FromStatus: status, FromQualifiedID: "work#" + uid, ToIssueUID: "alpha", ToQualifiedID: "work#aaaa"}
	}
	links := []model.Link{child(1, "ghost", "closed"), child(2, "done", "closed"), child(3, "kid", "open")}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{parent, kid, done}, Links: links}}},
		model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &parent, Links: links, Comments: []model.Comment{{UID: "c1", Body: "hi"}}}},
		model.ActivityTabMsg{Tab: tab},
	)
}

func TestPropertiesShowTheChildCountsAndNoChildLines(t *testing.T) {
	nodes := render(t, parenting(model.TabLinks))
	if got := text(nodes[view.DetailContentID+".properties.children.v"]); got != "1 open / 3 total" {
		t.Errorf("counts line = %q", got)
	}
	if ids := nodes.under(view.DetailContentID + ".properties.parent"); len(ids) != 0 {
		t.Errorf("properties still list relatives with no parent: %v", ids)
	}
}

func TestChildrenTableSitsBetweenTheBodyAndActivity(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	var order []string
	for _, c := range nodes[view.DetailContentID].C {
		order = append(order, c.ID)
	}
	body, table, activity := slices.Index(order, view.DetailContentID+".body"), slices.Index(order, childrenID), slices.Index(order, view.DetailActivityID)
	if body < 0 || table <= body || activity <= table {
		t.Fatalf("content order = %v, want body, children, activity", order)
	}
	if got := text(nodes[childrenID+".heading"]); got != "Children" {
		t.Errorf("heading = %q", got)
	}
}

// tableCell is the cell of column col in the row with id, as the wire has it: a string, a span list or a text cell.
func tableCell(t *testing.T, nodes shown, row, col string) any {
	t.Helper()
	for _, r := range nodes[view.ChildrenTableID].P["rows"].([]any) {
		r := r.(map[string]any)
		if r["id"] == row {
			cells, _ := r["cells"].(map[string]any)
			return cells[col]
		}
	}
	t.Fatalf("no row %q in %v", row, nodes[view.ChildrenTableID].P["rows"])
	return nil
}

// cellSpans is a cell's spans: a bare string is one unstyled span, a text cell's text is read the same way.
func cellSpans(cell any) []map[string]any {
	switch c := cell.(type) {
	case string:
		return []map[string]any{{"t": c}}
	case map[string]any:
		return cellSpans(c["text"])
	case []any:
		var spans []map[string]any
		for _, span := range c {
			spans = append(spans, span.(map[string]any))
		}
		return spans
	}
	return nil
}

func cellText(t *testing.T, nodes shown, row, col string) string {
	t.Helper()
	var b strings.Builder
	for _, span := range cellSpans(tableCell(t, nodes, row, col)) {
		b.WriteString(span["t"].(string))
	}
	return b.String()
}

func cellStyle(t *testing.T, nodes shown, row, col string) string {
	t.Helper()
	style := ""
	for _, span := range cellSpans(tableCell(t, nodes, row, col)) {
		style, _ = span["s"].(string)
	}
	return style
}

func TestChildrenAreANativeTableWithFiveColumnsInTheListsOrder(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	if got := nodes[childrenID].C; len(got) != 2 || got[0].ID != childrenID+".heading" || got[1].ID != view.ChildrenTableID || got[1].K != "table" {
		t.Fatalf("children hold %v, want the heading then the table", got)
	}
	table := nodes[view.ChildrenTableID].P
	if table["selectable"] != true {
		t.Errorf("selectable = %v", table["selectable"])
	}
	type col struct {
		id, head string
		priority float64
	}
	var cols []col
	var titlePriority, otherPriority float64
	for _, c := range table["cols"].([]any) {
		c := c.(map[string]any)
		priority := num(c["priority"])
		head, _ := c["head"].(string)
		cols = append(cols, col{c["id"].(string), head, priority})
		switch c["id"] {
		case "title":
			titlePriority = priority
			if num(c["grow"]) != 1 || c["truncate"] != "end" {
				t.Errorf("title column = %v, want grow 1 and truncate end", c)
			}
		case "ticket", "updated":
			if c["mono"] != true {
				t.Errorf("%v column is not mono", c["id"])
			}
		}
	}
	for _, c := range cols {
		if c.id != "title" {
			otherPriority = max(otherPriority, c.priority)
		}
	}
	if titlePriority <= otherPriority {
		t.Errorf("title priority %v is not the highest (others up to %v)", titlePriority, otherPriority)
	}
	var ids []string
	for _, c := range cols {
		ids = append(ids, c.id+":"+c.head)
	}
	if want := []string{"ticket:ticket", "status:status", "title:title", "owner:owner", "updated:updated"}; !slices.Equal(ids, want) {
		t.Errorf("columns = %v, want %v", ids, want)
	}
	var rows []string
	for _, r := range table["rows"].([]any) {
		rows = append(rows, r.(map[string]any)["id"].(string))
	}
	if want := []string{"kid", "done", "ghost"}; !slices.Equal(rows, want) {
		t.Errorf("row ids = %v, want the children's UIDs %v", rows, want)
	}
}

func TestChildCellsShowReferenceStatusTitleOwnerAndUpdated(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for row, want := range map[string][5]string{
		"kid":  {"#kid", "open", "Open kid", "dana until 13:00", "38m ago"},
		"done": {"#done", "closed", "Closed kid", "", "3h ago"},
	} {
		for i, col := range []string{"ticket", "status", "title", "owner", "updated"} {
			if got := cellText(t, nodes, row, col); got != want[i] {
				t.Errorf("%s %s = %q, want %q", row, col, got, want[i])
			}
		}
	}
}

func TestStatusCellHasADotInTheStatusTone(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for row, want := range map[string]string{"kid": "accent", "done": "muted"} {
		cell, ok := tableCell(t, nodes, row, "status").(map[string]any)
		if !ok || cell["dot"] != want {
			t.Errorf("%s status cell = %v, want a text cell with a %s dot", row, tableCell(t, nodes, row, "status"), want)
		}
	}
}

func TestSelectingAChildRowJumpsToTheChild(t *testing.T) {
	for _, uid := range []string{"kid", "done", "ghost"} {
		if got, want := view.DecodeSelect(view.ChildrenTableID, uid), (model.JumpIssueMsg{UID: uid}); got != want {
			t.Errorf("select %s = %v, want %v", uid, got, want)
		}
	}
}

func TestClosedChildIsMutedAndOpenChildIsNot(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, tt := range []struct {
		row, col, want string
	}{
		{"done", "status", "muted"}, {"done", "title", "muted"}, {"done", "updated", "muted"},
		{"kid", "status", ""}, {"kid", "title", ""}, {"kid", "owner", "muted"}, {"kid", "updated", "muted"},
	} {
		if got := cellStyle(t, nodes, tt.row, tt.col); got != tt.want {
			t.Errorf("%s %s style = %q, want %q", tt.row, tt.col, got, tt.want)
		}
	}
}

func TestUnloadedChildShowsItsReferenceOnly(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	if got := cellText(t, nodes, "ghost", "ticket"); got != "#ghost" {
		t.Errorf("ticket = %q", got)
	}
	for _, r := range nodes[view.ChildrenTableID].P["rows"].([]any) {
		if r := r.(map[string]any); r["id"] == "ghost" && len(r["cells"].(map[string]any)) != 1 {
			t.Errorf("unloaded child cells = %v, want the ticket only", r["cells"])
		}
	}
}

func TestNoChildrenTableNorCountsLineWithoutChildren(t *testing.T) {
	nodes := render(t, browsing())
	for _, id := range []string{childrenID, view.DetailContentID + ".properties.children"} {
		if ids := nodes.under(id); len(ids) != 0 {
			t.Errorf("%s shown without children: %v", id, ids)
		}
	}
}

func TestChildRowUnderTheCursorIsMarkedAndTheActivityRowIsNotWhileItIsOnAChild(t *testing.T) {
	selected := func(nodes shown) any { return nodes[view.ChildrenTableID].P["selected"] }
	m := apply(parenting(model.TabComments), model.KeyMsg{Chord: "k"}, model.KeyMsg{Chord: "k"}) // from the comment up to the 2nd child; focus is the detail
	nodes := render(t, m)
	if selected(nodes) != "done" || nodes[view.DetailActivityID+".c1"].P["mark"] != nil {
		t.Errorf("selected = %v, comment mark = %v", selected(nodes), nodes[view.DetailActivityID+".c1"].P["mark"])
	}
	m = apply(m, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "j"}) // down past the 3 children: the comment
	nodes = render(t, m)
	if selected(nodes) != nil || nodes[view.DetailActivityID+".c1"].P["mark"] != "pick" {
		t.Errorf("selected = %v, comment mark = %v", selected(nodes), nodes[view.DetailActivityID+".c1"].P["mark"])
	}
}

// num reads a numeric prop whatever Go type the wire holds it in.
func num(v any) float64 {
	f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
	return f
}
