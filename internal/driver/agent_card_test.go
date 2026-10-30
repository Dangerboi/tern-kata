package driver_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func agentCardEntries(uid string) []model.AgentEntry {
	at := time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)
	return []model.AgentEntry{
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "31", StartedAt: at, Placement: model.AgentSplit},
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "32", StartedAt: at.Add(time.Hour), Placement: model.AgentTab},
		{IssueUID: uid, IssueRef: "work#" + uid, Block: "33", StartedAt: at.Add(2 * time.Hour)},
	}
}

func withAgentRows(s driver.State) driver.State {
	s.Model.AgentMapping = agentCardEntries("t1")
	s.Model.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "31", State: "working"}, {Block: "32", State: "waiting_input"}, {Block: "33", State: "idle"}}}
	return s
}

func TestWOpensTheAgentCardInEveryIssueModeOnlyWithAgents(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		for _, chord := range []string{"w", "shift+w"} {
			if got := step(t, s, key(chord)); got.State.Model.TopCard() == model.CardAgent {
				t.Errorf("%s %s: the card opened with no agents", name, chord)
			}
			got := step(t, withAgentRows(s), key(chord))
			if got.State.Model.TopCard() != model.CardAgent || len(got.Cmds) != 0 {
				t.Errorf("%s %s: card %v, cmds %#v", name, chord, got.State.Model.TopCard(), got.Cmds)
			}
		}
	}
}

func TestTheAgentCardRowsShowStateStartAndWhere(t *testing.T) {
	s := step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key("w")).State
	for block, want := range map[string]string{
		"31": "agent working · started 2026-10-30 09:05 · split",
		"32": "agent waiting for you · started 2026-10-30 10:05 · tab",
		"33": "agent idle · started 2026-10-30 11:05 · split",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(block) {
			t.Errorf("rows reading %q: %v, want only %s", want, rows, view.AgentItemID(block))
		}
	}
}

func TestTheAgentCardKeysAndButtonsReachTheGateway(t *testing.T) {
	open := func(chord string) driver.State {
		return step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key(chord)).State
	}
	for chord, placement := range map[string]model.AgentPlacement{"w": model.AgentSplit, "shift+w": model.AgentTab} {
		got := step(t, open(chord), key("n"))
		if start, ok := startedAgent(got.Cmds); !ok || start.Placement != placement || got.State.Model.TopCard() == model.CardAgent {
			t.Errorf("%s then n: cmds %#v", chord, got.Cmds)
		}
	}
	got := feed(t, open("w"), key("down"), key("enter"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.FocusAgent{Block: "32"}) {
		t.Errorf("down, enter: cmds %#v, want focus of 32", got.Cmds)
	}
	if esc := step(t, open("w"), key("escape")); esc.State.Model.TopCard() == model.CardAgent || len(esc.Cmds) != 0 {
		t.Errorf("esc: card %v, cmds %#v", esc.State.Model.TopCard(), esc.Cmds)
	}
}

func TestSOpensTheAgentCardAndBGoesToTheGateway(t *testing.T) {
	open := func(chord string) driver.State {
		return step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key(chord)).State
	}
	s := open("shift+s")
	if s.Model.TopCard() != model.CardAgent {
		t.Fatalf("S: card %v", s.Model.TopCard())
	}
	if start, ok := startedAgent(step(t, s, key("n")).Cmds); !ok || start.Placement != model.AgentBackground {
		t.Errorf("S then n: %#v", start)
	}
	got := feed(t, open("w"), key("down"), key("b"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.SendAgentBack{Block: "32", IssueRef: "work#t1"}) || got.State.Model.TopCard() == model.CardAgent {
		t.Errorf("down, b: cmds %#v", got.Cmds)
	}
}

func TestTheAgentCardWhereColumnNamesBackgroundAndOpenAgents(t *testing.T) {
	s := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	s.Model.AgentMapping[0].Background = true
	s.Model.AgentMapping[1].Background, s.Model.AgentMapping[1].Open = true, true
	s = step(t, s, key("shift+s")).State
	for block, want := range map[string]string{
		"31": "agent working · started 2026-10-30 09:05 · background",
		"32": "agent waiting for you · started 2026-10-30 10:05 · open",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(block) {
			t.Errorf("rows reading %q: %v", want, rows)
		}
	}
}
