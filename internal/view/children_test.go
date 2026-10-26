package view_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

const childrenID = view.DetailContentID + ".children"

// A child row's id ends in its Link's id, never in the child's UID (that is the list row's suffix).
const (
	kidRow   = childrenID + ".3"
	doneRow  = childrenID + ".2"
	ghostRow = childrenID + ".1"
)

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

func TestChildRowsShowReferenceStatusTitleOwnerAndUpdatedInTheListsOrder(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	var rows []string
	for _, c := range nodes[childrenID].C {
		if c.ID != childrenID+".heading" && c.ID != childrenID+".header" {
			rows = append(rows, c.ID)
		}
	}
	if want := []string{kidRow, doneRow, ghostRow}; !slices.Equal(rows, want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for row, want := range map[string][5]string{
		kidRow:  {"#kid\u00a0\u00a0", "open\u00a0\u00a0", "Open kid", "dana until 13:00", "38m ago"},
		doneRow: {"#done\u00a0", "closed", "Closed kid", "\u00a0", "3h ago\u00a0"},
	} {
		for i, cell := range []string{"jump", "status", "title", "owner", "updated"} {
			if got := text(nodes[row+"."+cell]); got != want[i] {
				t.Errorf("%s %s = %q, want %q", row, cell, got, want[i])
			}
		}
	}
}

func TestChildCellsRunTicketStatusTitleOwnerUpdatedUnderALabelledHeader(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	order := func(row string) (ids, labels []string) {
		for _, c := range nodes[row].C {
			ids = append(ids, strings.TrimPrefix(c.ID, row+"."))
			labels = append(labels, strings.TrimRight(text(nodes[c.ID]), "\u00a0"))
		}
		return ids, labels
	}
	ids, labels := order(childrenID + ".header")
	if want := []string{"jump", "status", "title", "owner", "updated"}; !slices.Equal(ids, want) {
		t.Errorf("header cells = %v, want %v", ids, want)
	}
	if want := []string{"ticket", "status", "title", "owner", "updated"}; !slices.Equal(labels, want) {
		t.Errorf("header labels = %v, want %v", labels, want)
	}
	if ids, _ := order(kidRow); !slices.Equal(ids, []string{"jump", "status", "title", "owner", "updated"}) {
		t.Errorf("row cells = %v", ids)
	}
	for _, cell := range []string{"jump", "status", "updated"} {
		h, r := text(nodes[childrenID+".header."+cell]), text(nodes[kidRow+"."+cell])
		if utf8.RuneCountInString(h) != utf8.RuneCountInString(r) {
			t.Errorf("%s header %q and row %q differ in width", cell, h, r)
		}
	}
}

// The shading is the Issue list's: the same role on every other drawn row, the first unshaded; the header is not shaded.
func TestChildRowsAlternateTheListsShadingBelowAnUnshadedHeader(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	if role := nodes[childrenID+".header"].P["role"]; role != nil {
		t.Errorf("header role = %v, want none", role)
	}
	for row, want := range map[string]any{kidRow: nil, doneRow: view.StripeRole, ghostRow: nil} {
		if got := nodes[row].P["role"]; got != want {
			t.Errorf("%s role = %v, want %v", row, got, want)
		}
	}
}

func TestChildCellsArePaddedToTheWidestRowAndNeverWrap(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, cell := range []string{"jump", "status", "owner", "updated"} {
		if cell != "owner" { // the owner is not padded: its box gives it the width
			a, b := text(nodes[kidRow+"."+cell]), text(nodes[doneRow+"."+cell])
			if utf8.RuneCountInString(a) != utf8.RuneCountInString(b) {
				t.Errorf("%s widths differ: %q vs %q", cell, a, b)
			}
		}
		if nodes[kidRow+"."+cell].P["wrap"] != "none" {
			t.Errorf("%s wraps: %v", cell, nodes[kidRow+"."+cell].P["wrap"])
		}
	}
}

// Tern draws a glyph the font lacks from a fallback font whose advance is not the character cell's, so the owner of a row
// with CJK in it would start off the "owner" header if its width came from padding; every cell of a column has the
// column's width as its least width instead.
func TestChildCellsOfAColumnShareTheColumnsBox(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for cell, width := range map[string]string{"jump": "6ch", "status": "6ch", "owner": "16ch", "updated": "7ch"} {
		want := map[string]any{"w": width}
		for _, row := range []string{childrenID + ".header", kidRow, doneRow} {
			if got := nodes[row+"."+cell].P["min"]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s min = %v, want %v", row, cell, got, want)
			}
		}
	}
}

func TestEveryChildCellJumpsToTheChild(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, cell := range []string{"jump", "status", "title", "owner", "updated"} {
		n := nodes[kidRow+"."+cell]
		if click := n.P["actions"].(map[string]any)["click"]; click != view.JumpClick("kid") {
			t.Errorf("%s click = %v", cell, click)
		}
	}
	assertReference(t, nodes, kidRow+".jump", "#kid", "kid")
}

func TestClosedChildIsMutedAndOpenChildIsNot(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, tt := range []struct {
		row, cell, want string
	}{
		{doneRow, "status", "muted"}, {doneRow, "title", "muted"}, {doneRow, "owner", "muted"}, {doneRow, "updated", "muted"},
		{kidRow, "status", "accent"}, {kidRow, "title", ""}, {kidRow, "owner", "muted"}, {kidRow, "updated", "muted"},
	} {
		got := ""
		for _, style := range spanStyles(t, nodes[tt.row+"."+tt.cell].P) {
			got = style
		}
		if got != tt.want {
			t.Errorf("%s %s style = %q, want %q", tt.row, tt.cell, got, tt.want)
		}
	}
}

func TestUnloadedChildShowsItsReferenceOnly(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	assertReference(t, nodes, ghostRow+".jump", "#ghost", "ghost")
	if ids := nodes.under(ghostRow); len(ids) != 2 { // the row and its reference
		t.Errorf("unloaded child shows %v", ids)
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
	mark := func(n tern.WireNode) any { return n.P["mark"] }
	m := apply(parenting(model.TabComments), model.KeyMsg{Chord: "k"}, model.KeyMsg{Chord: "k"}) // from the comment up to the 2nd child; focus is the detail
	nodes := render(t, m)
	if mark(nodes[doneRow]) != "pick" || mark(nodes[kidRow]) != nil || mark(nodes[view.DetailActivityID+".c1"]) != nil {
		t.Errorf("marks: kid=%v done=%v comment=%v", mark(nodes[kidRow]), mark(nodes[doneRow]), mark(nodes[view.DetailActivityID+".c1"]))
	}
	m = apply(m, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "j"}) // down past the 3 children: the comment
	nodes = render(t, m)
	if mark(nodes[view.DetailActivityID+".c1"]) != "pick" || strings.Contains(strings.Join(markedIDs(nodes), ","), "children") {
		t.Errorf("marked rows = %v", markedIDs(nodes))
	}
}

func markedIDs(nodes shown) []string {
	var ids []string
	for id, n := range nodes {
		if n.P["mark"] == "pick" {
			ids = append(ids, id)
		}
	}
	return ids
}
