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
	raw := wireText(*n)
	for _, w := range words {
		if !strings.Contains(raw, w) {
			t.Errorf("%s: empty state lacks %q: %s", what, w, raw)
		}
	}
}

func TestAttentionPageEmptyIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "attention", attentionPageWith(t, nil, 0), "main.columns.list.attention", "", "No requests")
}

func TestRunsPageEmptyIsAnEmptyStateAndOffersEveryRunForAJob(t *testing.T) {
	wantEmpty(t, "runs", runsPane(t, 160), "main.columns.list.runs", "", "No runs")
	s := runsPane(t, 160)
	s.Model.Runs.JobUID = "J1"
	wantEmpty(t, "job runs", s, "main.columns.list.runs", "", "No runs for this job", "key=f")
}

func TestJobsPageEmptyIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "jobs", jobsPane(t, 160), "main.columns.list.jobs", "", "No Cron jobs")
}

func TestWorkflowsPageEmptyIsAnEmptyStateWithAKeyButton(t *testing.T) {
	wantEmpty(t, "workflows", workflowsPane(t, 160), view.WorkflowListID, "", "No workflows", "key=n")
}

func TestIssueListEmptyIsAnEmptyState(t *testing.T) {
	s := projectPane(t, 160)
	wantEmpty(t, "no issues", s, view.IssueListID, "", "No Issues")
	s.Model.Filters = model.Filters{Status: "closed"}
	wantEmpty(t, "filtered", s, view.IssueListID, "", "No Issues match", "key=c")
}

func TestDetailWithNoIssueSelectedIsAnEmptyState(t *testing.T) {
	wantEmpty(t, "detail", projectPane(t, 160), "main.columns.detail.empty", "", "No Issue selected")
}

func TestBoardLaneWithNoCardsIsAnInlineEmptyState(t *testing.T) {
	wantEmpty(t, "lane", boardPane(t, boardCard("a")), view.BoardID+".lane1.cards", "inline", "No cards")
}
