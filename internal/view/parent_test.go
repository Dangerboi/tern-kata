package view_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// A parent Issue with this many children has far more detail than a Pane shows.
const manyChildren = 60

// parentBrowsing is a wide Pane on a Project whose only root is a parent with
// manyChildren children, its detail loaded with one Link and one change-history
// event per child.
func parentBrowsing() model.Model {
	parent := model.Issue{UID: "parent", ShortID: "prnt", Title: "Parent", Status: "open", ProjectName: "work"}
	issues := []model.Issue{parent}
	var links []model.Link
	var events []model.Event
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	for i := range manyChildren {
		uid := fmt.Sprintf("child%02d", i)
		issues = append(issues, model.Issue{UID: uid, ShortID: uid, Title: "Child " + uid, Status: "open", ProjectName: "work"})
		links = append(links, model.Link{ID: int64(i + 1), Type: "parent", FromIssueUID: uid, FromQualifiedID: "work#" + uid, ToIssueUID: "parent", ToQualifiedID: "work#parent"})
		events = append(events, model.Event{UID: "event-" + uid, Actor: "dana", Type: "issue.linked", Payload: `{"type":"parent"}`, CreatedAt: at})
	}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Links: links}}},
		model.DetailLoaded{UID: parent.UID, Selected: &model.IssueDetailData{Issue: &parent, Links: links, History: events}},
	)
}

func TestParentIssueWithManyChildrenRendersItsWholeTree(t *testing.T) {
	collapsed := parentBrowsing()
	expanded := apply(collapsed, model.KeyMsg{Chord: "space"})
	for name, m := range map[string]model.Model{"collapsed": collapsed, "expanded": expanded} {
		t.Run(name, func(t *testing.T) {
			nodes := render(t, m)
			if m.SelectedUID != "parent" {
				t.Fatalf("fixture: selected %q", m.SelectedUID)
			}
			if _, ok := nodes[view.IssueID("parent")]; !ok {
				t.Errorf("the parent's row is missing from the list: %v", nodes.under(view.IssueListID))
			}
			if title := text(nodes[view.DetailContentID+".heading.title"]); !strings.Contains(title, "Parent") {
				t.Errorf("detail does not show the parent: %q", title)
			}
			links := render(t, apply(m, model.ActivityTabMsg{Tab: model.TabLinks}))
			events := render(t, apply(m, model.ActivityTabMsg{Tab: model.TabEvents}))
			for i := range manyChildren {
				uid := fmt.Sprintf("child%02d", i)
				// a child is in the Children table, not a row of the Links tab
				if _, ok := links[fmt.Sprintf("%s.%d.row.jump", view.DetailActivityID, i+1)]; ok {
					t.Errorf("the Links tab lists %s", uid)
				}
				if _, ok := events[view.DetailActivityID+".event-"+uid]; !ok {
					t.Errorf("no history row for %s", uid)
				}
				_, shown := nodes[view.IssueID(uid)]
				if shown != (name == "expanded") {
					t.Errorf("%s in the list = %v while %s", uid, shown, name)
				}
			}
		})
	}
}

// tableCells are the table node's cells of row, by column, as the text each draws.
func tableCells(t *testing.T, n tern.WireNode, row string) map[string]string {
	t.Helper()
	rows, _ := n.P["rows"].([]any)
	for _, r := range rows {
		fields, _ := r.(map[string]any)
		if fields["id"] != row {
			continue
		}
		out := map[string]string{}
		cells, _ := fields["cells"].(map[string]any)
		for column, cell := range cells {
			raw, _ := json.Marshal(cell)
			var texts []string
			for _, m := range regexp.MustCompile(`"t":"([^"]*)"`).FindAllStringSubmatch(string(raw), -1) {
				texts = append(texts, m[1])
			}
			out[column] = strings.Join(texts, "")
		}
		return out
	}
	t.Fatalf("no row %s in the table: %v", row, n.P["rows"])
	return nil
}

// The Children table fills Status, Title and Updated for every child, those the Pane's list holds and the closed ones it does
// not (the status filter leaves them out) whose title the data layer read with the Link.
func TestChildrenTableShowsStatusTitleAndUpdatedForEveryChild(t *testing.T) {
	parent := model.Issue{UID: "parent", ShortID: "prnt", Title: "Parent", Status: "open", ProjectName: "work"}
	listed := model.Issue{UID: "listed", ShortID: "lstd", Title: "Listed child", Status: "open", ProjectName: "work"}
	links := []model.Link{
		{ID: 1, Type: "parent", FromIssueUID: "listed", FromQualifiedID: "work#lstd", FromStatus: "open", ToIssueUID: "parent", ToQualifiedID: "work#prnt"},
		{ID: 2, Type: "parent", FromIssueUID: "gone", FromQualifiedID: "work#gone", FromStatus: "closed", FromTitle: "Closed child", ToIssueUID: "parent", ToQualifiedID: "work#prnt"},
	}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{parent, listed}, Links: links}}},
		model.DetailLoaded{UID: parent.UID, Selected: &model.IssueDetailData{Issue: &parent, Links: links}},
	)
	table := render(t, m)[view.ChildrenTableID]
	for uid, want := range map[string][2]string{"listed": {"open", "Listed child"}, "gone": {"closed", "Closed child"}} {
		cells := tableCells(t, table, uid)
		if cells["status"] != want[0] || cells["title"] != want[1] || cells["updated"] == "" {
			t.Errorf("child %s cells = %v, want status %q, title %q and an updated cell", uid, cells, want[0], want[1])
		}
	}
}
