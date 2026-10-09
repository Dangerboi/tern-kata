package view_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

// A parent Issue with this many children has far more detail than a Pane shows.
const manyChildren = 60

// parentBrowsing is a wide Pane on a Project whose only root is a parent with
// manyChildren children, its detail loaded with one Link and one change-history
// event per child.
func parentBrowsing() model.Model {
	parent := model.Issue{UID: "parent", ShortID: "prnt", Title: "Parent", Status: "open", ProjectName: "work"}
	issues := []model.Issue{parent}
	var links []g.UILink
	var events []g.Event
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	for i := range manyChildren {
		uid := fmt.Sprintf("child%02d", i)
		issues = append(issues, model.Issue{UID: uid, ShortID: uid, Title: "Child " + uid, Status: "open", ProjectName: "work"})
		links = append(links, g.UILink{ID: int64(i + 1), Type: "parent", FromIssueUID: uid, FromQualifiedID: "work#" + uid, ToIssueUID: "parent", ToQualifiedID: "work#parent"})
		events = append(events, g.Event{UID: "event-" + uid, Actor: "dana", Type: "issue.linked", Payload: `{"type":"parent"}`, CreatedAt: at})
	}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, Snapshot: g.UISnapshotResponseBody{Collection: issues, CollectionLinks: links}},
		model.DetailLoaded{UID: parent.UID, Selected: &g.UISelectedAuthority{Issue: &parent, Links: links, History: events}},
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
			if title := text(nodes[view.DetailID+".heading.title"]); !strings.Contains(title, "Parent") {
				t.Errorf("detail does not show the parent: %q", title)
			}
			links, history := view.DetailSectionID(model.SectionLinks), view.DetailSectionID(model.SectionHistory)
			for i := range manyChildren {
				uid := fmt.Sprintf("child%02d", i)
				jump, ok := nodes[fmt.Sprintf("%s.%d.jump", links, i+1)]
				if !ok {
					t.Fatalf("no Link row for %s: %v", uid, nodes.under(links))
				}
				if click := jump.P["actions"].(map[string]any)["click"]; click != "jump="+uid {
					t.Errorf("%s's jump button does %v", uid, click)
				}
				if _, ok := nodes[history+".event-"+uid]; !ok {
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
