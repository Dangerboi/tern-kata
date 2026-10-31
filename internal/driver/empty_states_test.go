package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// emptyNode is the empty-kind node with id in what s draws, or nil.
func emptyNode(t *testing.T, s driver.State, id string) *tern.WireNode {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id {
			if n.K != "empty" {
				t.Fatalf("%s is a %q node, want empty", id, n.K)
			}
			return &n
		}
	}
	return nil
}

func wantEmpty(t *testing.T, what string, s driver.State, id, variant string, words ...string) {
	t.Helper()
	n := emptyNode(t, s, id)
	if n == nil {
		t.Fatalf("%s: no empty state at %s", what, id)
	}
	if got, _ := n.P["variant"].(string); got != variant {
		t.Errorf("%s: variant %q, want %q", what, got, variant)
	}
	if list := strings.TrimSuffix(id, "-empty"); list != id {
		for _, other := range wireNodes(t, view.Render(s.Model, now)) {
			if other.ID == list {
				if other.P["hidden"] != true {
					t.Errorf("%s: the list stays mounted but is not hidden", what)
				}
				list = ""
			}
		}
		if list != "" {
			t.Errorf("%s: the list %s is not mounted beside its empty state", what, list)
		}
	}
	raw := wireText(*n)
	for _, w := range words {
		if !strings.Contains(raw, w) {
			t.Errorf("%s: empty state lacks %q: %s", what, w, raw)
		}
	}
}

func TestAttentionPageEmptyIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "attention", attentionPageWith(t, nil, 0), "main.columns.list.attention"+"-empty", "", "No requests")
}

func TestRunsPageEmptyIsAnEmptyStateAndOffersEveryRunForAJob(t *testing.T) {
	wantEmpty(t, "runs", runsPane(t, 160), "main.columns.list.runs"+"-empty", "", "No runs")
	s := runsPane(t, 160)
	s.Model.Runs.JobUID = "J1"
	wantEmpty(t, "job runs", s, "main.columns.list.runs"+"-empty", "", "No runs for this job", "press=f", `"keys"`, `"k":"button"`)
}

func TestJobsPageEmptyIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "jobs", jobsPane(t, 160), "main.columns.list.jobs"+"-empty", "", "No Cron jobs")
}

func TestWorkflowsPageEmptyIsAnEmptyStateWithAKeyButton(t *testing.T) {
	wantEmpty(t, "workflows", workflowsPane(t, 160), view.WorkflowListID+"-empty", "", "No workflows", "press=n", `"keys"`, `"k":"button"`)
}

func TestIssueListEmptyIsAnEmptyState(t *testing.T) {
	s := projectPane(t, 160)
	wantEmpty(t, "no issues", s, view.IssueListID+"-empty", "", "No Issues", "press=n", `"keys"`, `"k":"button"`)
	s.Model.Filters = model.Filters{Status: "closed"}
	wantEmpty(t, "filtered", s, view.IssueListID+"-empty", "", "No Issues match", "press=c", `"keys"`, `"k":"button"`)
}

// A button presses its key in the mode the Pane is in, and with Search open that mode types the key, so the empty list
// offers no button while the user searches.
func TestIssueListEmptyOffersNoButtonWhileSearching(t *testing.T) {
	for name, s := range map[string]driver.State{
		"no filters": feed(t, projectPane(t, 160), key("/")).State,
		"typed":      feed(t, projectPane(t, 160), key("/"), key("z")).State,
	} {
		n := emptyNode(t, s, view.IssueListID+"-empty")
		if n == nil {
			t.Fatalf("%s: no empty state", name)
		}
		if raw := wireText(*n); strings.Contains(raw, `"k":"button"`) || strings.Contains(raw, "press=") {
			t.Errorf("%s: a button that types into the search: %s", name, raw)
		}
	}
}

func TestDetailWithNoIssueSelectedIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "detail", projectPane(t, 160), "main.columns.detail.empty", "", "No Issue selected")
}

func TestBoardLaneWithNoCardsIsAnInlineEmptyState(t *testing.T) {
	wantEmpty(t, "lane", boardPane(t, boardCard("a")), view.BoardID+".lane1.cards"+"-empty", "inline", "No cards")
}

// emptyListButton is the id and click of the first button the empty Issue list draws; ok is false when it draws none.
func emptyListButton(t *testing.T, s driver.State) (id, action string, ok bool) {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasPrefix(n.ID, view.IssueListID+"-empty.") && n.K == "button" {
			actions, _ := n.P["actions"].(map[string]any)
			action, _ = actions["click"].(string)
			return n.ID, action, true
		}
	}
	return "", "", false
}

func TestEmptyIssueListClearFiltersIsNeverDead(t *testing.T) {
	alice := "alice"
	issue := model.Issue{UID: "a", ShortID: "aaaa", Title: "A", Status: "open", Owner: &alice}
	filtered := func() model.Model {
		m := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{}})
		m.Filters = model.Filters{Owner: "nobody"}
		return m
	}
	detailFocused := func(t *testing.T) driver.State {
		m := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{issue}}})
		m.Filters = model.Filters{Owner: "alice"}
		s := feed(t, driver.State{Model: m}, model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue}}, key("enter")).State
		ev := step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "issue.updated"})
		snap, ok := snapshotFor(ev.Cmds)
		if !ok {
			t.Fatalf("no snapshot read: %#v", ev.Cmds)
		}
		snap.Snapshot.Issues = nil
		return step(t, ev.State, snap).State
	}
	cases := map[string]driver.State{
		"list focused":    driver.State{Model: filtered()},
		"sidebar focused": feed(t, driver.State{Model: filtered()}, key("escape")).State,
		"detail focused":  detailFocused(t),
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			id, act, ok := emptyListButton(t, s)
			if !ok {
				return
			}
			out := step(t, s, click(id, act))
			if out.State.Model.Filters != (model.Filters{}) {
				t.Errorf("Clear filters is drawn but its click left %+v", out.State.Model.Filters)
			}
		})
	}
	if _, _, ok := emptyListButton(t, cases["list focused"]); !ok {
		t.Errorf("no Clear filters button with the list focused")
	}
	if _, _, ok := emptyListButton(t, cases["sidebar focused"]); ok {
		t.Errorf("Clear filters is drawn with the sidebar focused")
	}
}
