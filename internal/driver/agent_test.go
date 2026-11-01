package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func agentIssue(uid string) model.Issue {
	issue := boardCard(uid)
	issue.ProjectID, issue.QualifiedID = 7, "work#"+uid
	return issue
}

func startedAgent(cmds []model.Cmd) (model.StartAgent, bool) {
	for _, cmd := range cmds {
		if start, ok := cmd.(model.StartAgent); ok {
			return start, true
		}
	}
	return model.StartAgent{}, false
}

// agentPanes are the four modes w works in, each with the Issue t1 selected.
func agentPanes(t *testing.T, issues ...model.Issue) map[string]driver.State {
	t.Helper()
	board := boardPane(t, issues...)
	list := projectPane(t, 160, issues...)
	list.Model.PaneBlock = "24489903521795"
	board.Model.PaneBlock = "24489903521795"
	return map[string]driver.State{
		"list":   list,
		"board":  board,
		"lane":   feed(t, board, key("enter")).State,
		"detail": feed(t, list, key("tab")).State,
	}
}

func TestTheAgentCardStartsAnAgentOnTheSelectedIssueInEveryIssueMode(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		for chord, placement := range map[string]model.AgentPlacement{"s": model.AgentSplit, "t": model.AgentTab, "g": model.AgentBackground} {
			t.Run(name+" "+chord, func(t *testing.T) {
				got := feed(t, s, key("w"), key(chord))
				start, ok := startedAgent(got.Cmds)
				want := model.StartAgent{Placement: placement, IssueUID: "t1", IssueRef: "work#t1", IssueTitle: "Title of t1", ProjectID: 7, ProjectName: "work",
					Workspace: "/repo", PaneBlock: "24489903521795"}
				if !ok || len(got.Cmds) != 1 || start != want || got.State.Model.TopCard() == model.CardAgent {
					t.Fatalf("cmds %#v, want %#v", got.Cmds, want)
				}
			})
		}
	}
}

func TestAnIssueOfAnotherProjectStartsAnAgentWithoutTheBindingsWorkspace(t *testing.T) {
	other := agentIssue("t1")
	other.ProjectID, other.ProjectName = 9, "elsewhere"
	got := feed(t, agentPanes(t, other)["list"], key("w"), key("s"))
	if start, ok := startedAgent(got.Cmds); !ok || start.Workspace != "" || start.ProjectName != "elsewhere" {
		t.Fatalf("cmds %#v, want an agent with no workspace for elsewhere", got.Cmds)
	}
}

func TestAClosedIssueOpensNoCardAndSaysToReopenIt(t *testing.T) {
	closed := closedAt("t1", 10, 1)
	closed.ProjectID, closed.QualifiedID = 7, "work#t1"
	for name, s := range agentPanes(t, closed) {
		got := step(t, s, key("w"))
		if len(got.Cmds) != 0 || got.State.Model.TopCard() == model.CardAgent || got.State.Model.Notice != "This Issue is closed; reopen it first (r)." {
			t.Errorf("%s: card %v, cmds %#v, notice %q", name, got.State.Model.TopCard(), got.Cmds, got.State.Model.Notice)
		}
	}
}

func TestAClosedIssueWithLiveAgentsRefusesWToo(t *testing.T) {
	closed := closedAt("t1", 10, 1)
	closed.ProjectID, closed.QualifiedID = 7, "work#t1"
	for name, s := range agentPanes(t, closed) {
		s = withAgentRows(s)
		got := step(t, s, key("w"))
		if len(got.Cmds) != 0 || got.State.Model.TopCard() == model.CardAgent || got.State.Model.Notice != "This Issue is closed; reopen it first (r)." {
			t.Errorf("%s: card %v, cmds %#v, notice %q", name, got.State.Model.TopCard(), got.Cmds, got.State.Model.Notice)
		}
	}
}

func TestTheStartAndItsResultsReachTheModel(t *testing.T) {
	s := agentPanes(t, agentIssue("t1"))["list"]
	entry := model.AgentEntry{IssueUID: "t1", IssueRef: "work#t1", Block: "31"}
	got := step(t, s, model.AgentStarted{Placement: model.AgentTab, IssueRef: "work#t1", Entry: entry})
	if len(got.State.Model.AgentMapping) != 1 || got.State.Model.AgentMapping[0] != entry || got.State.Model.Notice != "Started an agent on work#t1 in a new tab." {
		t.Fatalf("agents %+v, notice %q", got.State.Model.AgentMapping, got.State.Model.Notice)
	}
	background := step(t, s, model.AgentStarted{Placement: model.AgentBackground, IssueRef: "work#t1", Entry: entry})
	if background.State.Model.Notice != "Started an agent on work#t1 in the background (open it from its badge)." {
		t.Fatalf("notice %q", background.State.Model.Notice)
	}
	failed := step(t, s, model.AgentStarted{IssueRef: "work#t1", Err: errString("no checkout")})
	if failed.State.Model.Notice != "Cannot start an agent on work#t1: no checkout" || len(failed.State.Model.AgentMapping) != 0 {
		t.Fatalf("notice %q", failed.State.Model.Notice)
	}
	// An answer to a request asked before the start may have read the file before the entry was written, so it keeps the entry;
	// the answer to a request asked after the start drops it.
	pruned := step(t, step(t, got.State, model.AgentsPruned{}).State, model.AgentsPruned{Seq: 99})
	if len(pruned.State.Model.AgentMapping) != 0 {
		t.Fatalf("pruned agents %+v", pruned.State.Model.AgentMapping)
	}
	kept := step(t, s, model.AgentsPruned{Agents: []model.AgentEntry{entry}, Err: errString("tern ls failed")})
	if len(kept.State.Model.AgentMapping) != 1 || kept.State.Model.AgentMapping[0] != entry {
		t.Fatalf("agents after a failed tern ls %+v, want the file's entry", kept.State.Model.AgentMapping)
	}
	if started := step(t, driver.State{}, model.Start{Workspace: "/repo"}); !hasPrune(started.Cmds) {
		t.Fatalf("start cmds %#v, want the agents pruned", started.Cmds)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func hasPrune(cmds []model.Cmd) bool {
	for _, cmd := range cmds {
		if _, ok := cmd.(model.PruneAgents); ok {
			return true
		}
	}
	return false
}
