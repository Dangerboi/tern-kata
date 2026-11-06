package driver_test

import (
	"strings"
	"reflect"
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

func TestWOpensTheAgentCardInEveryIssueModeWithOrWithoutAgents(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		for with, state := range map[string]driver.State{"no agents": s, "agents": withAgentRows(s)} {
			got := step(t, state, key("w"))
			var seq int64
			if len(got.Cmds) == 1 {
				seq = got.Cmds[0].(model.RefreshAgents).Seq
			}
			if want := []model.Cmd{model.RefreshAgents{PaneBlock: got.State.Model.PaneBlock, Seq: seq}}; seq == 0 || got.State.Model.TopCard() != model.CardAgent || !reflect.DeepEqual(got.Cmds, want) {
				t.Errorf("%s, %s: card %v, cmds %#v", name, with, got.State.Model.TopCard(), got.Cmds)
			}
			// Shift+w and shift+s start nothing on the Issue.
			for _, chord := range []string{"shift+w", "shift+s"} {
				if off := step(t, state, key(chord)); off.State.Model.TopCard() == model.CardAgent || len(off.Cmds) != 0 {
					t.Errorf("%s, %s, %s: card %v, cmds %#v", name, with, chord, off.State.Model.TopCard(), off.Cmds)
				}
			}
		}
	}
}

func TestTheAgentCardWithNoAgentsStillStartsOneAndIgnoresEnterAndB(t *testing.T) {
	for name, s := range agentPanes(t, agentIssue("t1")) {
		open := step(t, s, key("w")).State
		for _, chord := range []string{"enter", "b"} {
			if got := step(t, open, key(chord)); got.State.Model.TopCard() != model.CardAgent || len(got.Cmds) != 0 {
				t.Errorf("%s, %s: card %v, cmds %#v", name, chord, got.State.Model.TopCard(), got.Cmds)
			}
		}
		if start, ok := startedAgent(step(t, open, key("g")).Cmds); !ok || start.Placement != model.AgentBackground {
			t.Errorf("%s, g: %#v", name, start)
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

func TestTheAgentCardKeysReachTheGateway(t *testing.T) {
	open := func() driver.State {
		return step(t, withAgentRows(agentPanes(t, agentIssue("t1"))["list"]), key("w")).State
	}
	for chord, placement := range map[string]model.AgentPlacement{"s": model.AgentSplit, "t": model.AgentTab, "g": model.AgentBackground} {
		got := step(t, open(), key(chord))
		if start, ok := startedAgent(got.Cmds); !ok || start.Placement != placement || got.State.Model.TopCard() == model.CardAgent {
			t.Errorf("w then %s: cmds %#v", chord, got.Cmds)
		}
	}
	got := feed(t, open(), key("down"), key("enter"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.FocusAgent{Block: "32"}) {
		t.Errorf("down, enter: cmds %#v, want focus of 32", got.Cmds)
	}
	got = feed(t, open(), key("down"), key("b"))
	if len(got.Cmds) != 1 || got.Cmds[0] != (model.SendAgentBack{Block: "32", IssueRef: "work#t1"}) || got.State.Model.TopCard() == model.CardAgent {
		t.Errorf("down, b: cmds %#v", got.Cmds)
	}
	if esc := step(t, open(), key("escape")); esc.State.Model.TopCard() == model.CardAgent || len(esc.Cmds) != 0 {
		t.Errorf("esc: card %v, cmds %#v", esc.State.Model.TopCard(), esc.Cmds)
	}
}

func TestTheAgentCardWhereColumnNamesBackgroundAndOpenAgents(t *testing.T) {
	s := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	s.Model.AgentMapping[0].Background = true
	s.Model.AgentMapping[1].Background, s.Model.AgentMapping[1].Open = true, true
	s = step(t, s, key("w")).State
	for block, want := range map[string]string{
		"31": "agent working · started 2026-10-30 09:05 · background",
		"32": "agent waiting for you · started 2026-10-30 10:05 · open",
	} {
		if rows := rowsNamed(t, s.Model, view.AgentListID, want); len(rows) != 1 || rows[0] != view.AgentItemID(block) {
			t.Errorf("rows reading %q: %v", want, rows)
		}
	}
}

func TestTheAgentRowShowsTheFirstLineOfAnUnclearedMessageAndLosesItWhenTheSlotClears(t *testing.T) {
	entries := agentCardEntries("t1")
	entries[0].Address = "me/omp-31"
	m := model.Model{AgentMapping: entries, Issues: []model.Issue{{UID: "t1", Metadata: map[string]any{
		model.NotificationKey("me/omp-31"): map[string]any{"from": "me", "message": "rebase first\nthen test"},
	}}}}
	m.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "31", State: "working"}}}
	row := view.AgentRowText(m, m.IssueAgents("t1")[0])
	if !strings.HasSuffix(row, " · message unread: rebase first") || !strings.HasPrefix(row, "agent working · started ") {
		t.Errorf("row %q", row)
	}
	m.Issues[0].Metadata = map[string]any{}
	if row := view.AgentRowText(m, m.IssueAgents("t1")[0]); strings.Contains(row, "unread") {
		t.Errorf("cleared row %q", row)
	}
}
