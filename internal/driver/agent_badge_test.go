package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func badgeAgent(block string, minute int) model.AgentEntry {
	return model.AgentEntry{IssueUID: "t1", Block: block, StartedAt: time.Date(2026, 10, 30, 9, minute, 0, 0, time.UTC)}
}

// withAgents is s with the Agent mapping and the relayed list.
func withAgents(t *testing.T, s driver.State, mapping []model.AgentEntry, relay ...model.AgentState) driver.State {
	t.Helper()
	s.Model.AgentMapping = mapping
	if relay != nil {
		s = step(t, s, model.AgentStatesMsg{States: relay}).State
	}
	return s
}

// badgeSpan is the text and style of the span of a row's label that starts with "agent".
func badgeSpan(t *testing.T, s driver.State, uid string) (text, style string) {
	t.Helper()
	row := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.IssueListID, uid))
	spans, _ := row.P["label"].([]any)
	for _, sp := range spans {
		span, _ := sp.(map[string]any)
		if got, _ := span["t"].(string); strings.HasPrefix(strings.TrimSpace(got), "agent") {
			style, _ := span["s"].(string)
			return strings.TrimSpace(got), style
		}
	}
	return "", ""
}

func actionOf(n tern.WireNode) map[string]any {
	actions, _ := n.P["actions"].(map[string]any)
	return actions
}

func TestTheAgentBadgeIsInTheListAndLaneRowsAsAStyledSpan(t *testing.T) {
	panes := agentPanes(t, agentIssue("t1"), agentIssue("t2"))
	for _, name := range []string{"list", "lane"} {
		s := panes[name]
		for _, c := range []struct {
			state, text, style string
		}{
			{"working", "agent working", "accent"},
			{"waiting_input", "agent waiting for you", "warning"},
			{"idle", "agent idle", "muted"},
			{"exited", "agent exited", "muted"},
		} {
			got := withAgents(t, s, []model.AgentEntry{badgeAgent("5", 1)}, model.AgentState{Block: "5", State: c.state})
			if text, style := badgeSpan(t, got, "t1"); text != c.text || style != c.style {
				t.Errorf("%s %s: badge %q in %q, want %q in %q", name, c.state, text, style, c.text, c.style)
			}
		}
		if text, style := badgeSpan(t, withAgents(t, s, []model.AgentEntry{badgeAgent("5", 1)}), "t1"); text != "agent" || style != "muted" {
			t.Errorf("%s with no relay: badge %q in %q", name, text, style)
		}
		if text, _ := badgeSpan(t, s, "t1"); text != "" {
			t.Errorf("%s: a badge without an agent: %q", name, text)
		}
		got := withAgents(t, s, []model.AgentEntry{badgeAgent("5", 1), badgeAgent("6", 2)}, model.AgentState{Block: "5", State: "idle"}, model.AgentState{Block: "6", State: "waiting_input"})
		if text, _ := badgeSpan(t, got, "t1"); text != "agent waiting for you · 2" {
			t.Errorf("%s summary: %q", name, text)
		}
		if text, _ := badgeSpan(t, got, "t2"); text != "" {
			t.Errorf("%s: the other Issue shows %q", name, text)
		}
	}
}

func TestAClosedIssueKeepsItsAgentBadge(t *testing.T) {
	closed := agentIssue("t1")
	closed.Status = "closed"
	s := withAgents(t, projectPane(t, 160, closed), []model.AgentEntry{badgeAgent("5", 1)}, model.AgentState{Block: "5", State: "idle"})
	s = step(t, s, key("shift+e")).State
	if text, _ := badgeSpan(t, s, "t1"); text != "agent idle" {
		t.Errorf("badge %q", text)
	}
}

func TestTheBoardCardHasItsOwnClickableAgentBadge(t *testing.T) {
	s := withAgents(t, boardPane(t, agentIssue("t1")), []model.AgentEntry{badgeAgent("18494129176578", 1)}, model.AgentState{Block: "18494129176578", State: "working"})
	nodes := wireNodes(t, view.Render(s.Model, now))
	card := rowOf(t, s.Model, view.BoardID, "t1")
	badge := node(t, nodes, card+".agent")
	if badge.P["text"] != "agent working" || badge.P["tone"] != "accent" {
		t.Errorf("badge %v", badge.P)
	}
	if got := actionOf(badge)["click"]; got != view.AgentClick("18494129176578") {
		t.Errorf("badge click %v", got)
	}
	if got := actionOf(node(t, nodes, card))["click"]; got != view.CardClick("t1") {
		t.Errorf("card click %v", got)
	}
	// The badge's click is its own: it focuses the agent and does not select the card.
	out := step(t, s, click(card+".agent", view.AgentClick("18494129176578")))
	if len(out.Cmds) != 1 || out.Cmds[0] != (model.FocusAgent{Block: "18494129176578"}) {
		t.Errorf("cmds %#v", out.Cmds)
	}
	s.Model.AgentMapping = nil
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == card+".agent" {
			t.Error("a card without an agent has a badge")
		}
	}
}

func TestTheIssueDetailHasAnAgentPropertyWithItsOwnClick(t *testing.T) {
	issue := agentIssue("t1")
	s := withAgents(t, projectPane(t, 160, issue), []model.AgentEntry{badgeAgent("5", 1), badgeAgent("6", 2)}, model.AgentState{Block: "5", State: "waiting_input"}, model.AgentState{Block: "6", State: "waiting_input"})
	s = step(t, s, model.DetailLoaded{UID: "t1", Selected: &model.IssueDetailData{Issue: &issue}}).State
	var badge tern.WireNode
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasSuffix(n.ID, ".properties.agent.v") {
			badge = n
		}
	}
	if badge.ID == "" {
		t.Fatal("the detail has no Agent property")
	}
	if badge.P["text"] != "agent waiting for you · 2" || badge.P["tone"] != "warning" {
		t.Errorf("badge %v", badge.P)
	}
	// Both are waiting: the newest start is named.
	if got := actionOf(badge)["click"]; got != view.AgentClick("6") {
		t.Errorf("badge click %v", got)
	}
}

func TestAClickOnTheAgentBadgeFocusesTheAgentItNames(t *testing.T) {
	out := step(t, projectPane(t, 160, agentIssue("t1")), click("", view.AgentClick("144115188075855871")))
	if len(out.Cmds) != 1 || out.Cmds[0] != (model.FocusAgent{Block: "144115188075855871"}) {
		t.Fatalf("cmds %#v", out.Cmds)
	}
	failed := step(t, out.State, model.AgentFocused{Err: errString("exit status 1: tern: no such block")})
	if failed.State.Model.Notice != "Cannot focus the agent: exit status 1: tern: no such block" {
		t.Errorf("notice %q", failed.State.Model.Notice)
	}
}
