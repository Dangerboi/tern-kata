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
	wantEmpty(t, "job runs", s, "main.columns.list.runs"+"-empty", "", "No runs for this job", "key=f", `"keys"`, `"k":"button"`)
}

func TestJobsPageEmptyIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "jobs", jobsPane(t, 160), "main.columns.list.jobs"+"-empty", "", "No Cron jobs")
}

func TestWorkflowsPageEmptyIsAnEmptyStateWithAKeyButton(t *testing.T) {
	wantEmpty(t, "workflows", workflowsPane(t, 160), view.WorkflowListID+"-empty", "", "No workflows", "key=n", `"keys"`, `"k":"button"`)
}

func TestIssueListEmptyIsAnEmptyState(t *testing.T) {
	s := projectPane(t, 160)
	wantEmpty(t, "no issues", s, view.IssueListID+"-empty", "", "No Issues", "key=n", `"keys"`, `"k":"button"`)
	s.Model.Filters = model.Filters{Status: "closed"}
	wantEmpty(t, "filtered", s, view.IssueListID+"-empty", "", "No Issues match", "key=c", `"keys"`, `"k":"button"`)
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
		if raw := wireText(*n); strings.Contains(raw, `"k":"button"`) || strings.Contains(raw, "key=") {
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
