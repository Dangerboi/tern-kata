package model_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func finishedEntry(block, uid string) model.AgentEntry {
	e := backgroundEntry(block, "9", false)
	e.IssueUID, e.ProjectID = uid, 3
	return e
}

func closePane(issues []model.Issue, entries ...model.AgentEntry) model.Model {
	return model.Model{Workspace: "/w", PaneBlock: "9", AgentMapping: entries, Issues: issues}
}

func closed(uid string) model.Issue { return model.Issue{UID: uid, Status: "closed"} }

func closes(cmds []model.Cmd) []model.CloseAgent {
	var out []model.CloseAgent
	for _, c := range cmds {
		if c, ok := c.(model.CloseAgent); ok {
			out = append(out, c)
		}
	}
	return out
}

func reads(cmds []model.Cmd) []model.ReadAgentIssue {
	var out []model.ReadAgentIssue
	for _, c := range cmds {
		if c, ok := c.(model.ReadAgentIssue); ok {
			out = append(out, c)
		}
	}
	return out
}

func TestAFinishedBackgroundAgentOfAClosedIssueIsClosed(t *testing.T) {
	for _, state := range []model.AgentStateWord{model.AgentIdle, model.AgentExited} {
		m := closePane([]model.Issue{closed("a")}, finishedEntry("5", "a"))
		_, cmds := relayed(m, model.AgentState{Block: "5", State: state})
		if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
			t.Errorf("%s: cmds %#v", state, cmds)
		}
	}
}

func TestAnAgentStaysUnlessAllThreeConditionsHold(t *testing.T) {
	idle := model.AgentState{Block: "5", State: "idle"}
	with := func(f func(*model.AgentEntry)) model.AgentEntry { e := finishedEntry("5", "a"); f(&e); return e }
	for _, tt := range []struct {
		name   string
		entry  model.AgentEntry
		issue  model.Issue
		agent  model.AgentState
		paneID string
	}{
		{"an open Issue", finishedEntry("5", "a"), model.Issue{UID: "a", Status: "open"}, idle, "9"},
		{"an open background agent", with(func(e *model.AgentEntry) { e.Open = true }), closed("a"), idle, "9"},
		{"another Pane's agent", with(func(e *model.AgentEntry) { e.PaneBlock = "8" }), closed("a"), idle, "9"},
		{"a Pane without a block", with(func(e *model.AgentEntry) { e.PaneBlock = "" }), closed("a"), idle, ""},
		{"a working agent", finishedEntry("5", "a"), closed("a"), model.AgentState{Block: "5", State: "working"}, "9"},
		{"a waiting agent", finishedEntry("5", "a"), closed("a"), model.AgentState{Block: "5", State: "waiting_input"}, "9"},
		{"an agent the relay does not list", finishedEntry("5", "a"), closed("a"), model.AgentState{Block: "6", State: "idle"}, "9"},
	} {
		m := closePane([]model.Issue{tt.issue}, tt.entry)
		m.PaneBlock = tt.paneID
		_, cmds := relayed(m, tt.agent)
		if len(closes(cmds)) != 0 || len(reads(cmds)) != 0 {
			t.Errorf("%s: cmds %#v", tt.name, cmds)
		}
	}
}

func TestASplitAgentOfAClosedIssueIsAskedAboutOnceAndClosedOnlyOnceTheFileCallsItBackground(t *testing.T) {
	split := finishedEntry("5", "a")
	split.Background = false
	m := closePane([]model.Issue{closed("a")}, split)
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"})
	if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Fatalf("cmds %#v", cmds)
	}
	// The gateway reads agents.json: still a split agent there, so nothing closed and no retry on later relays.
	m, _ = model.Update(m, model.AgentClosed{Block: "5", Agents: []model.AgentEntry{split}, Kept: true})
	if _, cmds = relayed(m, model.AgentState{Block: "5", State: "working"}); len(closes(cmds)) != 0 {
		t.Fatalf("asked again: %#v", cmds)
	}
	if _, cmds = relayed(m, model.AgentState{Block: "5", State: "idle"}); len(closes(cmds)) != 0 {
		t.Fatalf("asked again: %#v", cmds)
	}
	// Another Pane's b then shows in a re-read: it is a background agent now and is ended.
	moved := split
	moved.Background = true
	_, cmds = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{moved}})
	if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Fatalf("after the re-read: %#v", cmds)
	}
}

func TestAnIssueClosedWhileTheAgentIsIdleClosesIt(t *testing.T) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/w", PaneBlock: "9"}, model.ProjectLoaded{Project: model.Project{ID: 3, Name: "Work"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{{UID: "a", Status: "open"}}}})
	m.AgentMapping = []model.AgentEntry{finishedEntry("5", "a")}
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"})
	if len(closes(cmds)) != 0 {
		t.Fatalf("closed with the Issue open: %#v", cmds)
	}
	m, cmds = step(m, model.StreamEvent{ID: 1, Type: "issue.closed", Generation: m.StreamGeneration})
	if len(closes(cmds)) != 0 || len(reads(cmds)) != 0 {
		t.Fatalf("stream event: %#v", cmds)
	}
	_, cmds = step(m, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Cursor: 1, Issues: []model.Issue{closed("a")}}}})
	if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestAnIssueOutsideTheCollectionIsReadAndClosesOnAClosedAnswer(t *testing.T) {
	m := closePane(nil, finishedEntry("5", "a"))
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"})
	if want := []model.ReadAgentIssue{{Workspace: "/w", ProjectID: 3, UID: "a"}}; !reflect.DeepEqual(reads(cmds), want) || len(closes(cmds)) != 0 {
		t.Fatalf("cmds %#v", cmds)
	}
	// A read in flight is not asked again.
	m, cmds = relayed(m, model.AgentState{Block: "5", State: "idle"})
	if len(reads(cmds)) != 0 {
		t.Fatalf("second read: %#v", cmds)
	}
	m, cmds = model.Update(m, model.AgentIssueRead{UID: "a", Status: "closed"})
	if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Fatalf("cmds %#v", cmds)
	}
	// Nothing closes twice.
	_, cmds = relayed(m, model.AgentState{Block: "5", State: "idle"})
	if len(closes(cmds)) != 0 || len(reads(cmds)) != 0 {
		t.Fatalf("repeat: %#v", cmds)
	}
}

func TestAnOpenIssueOutsideTheCollectionIsNotReadOnEveryRelay(t *testing.T) {
	m := closePane(nil, finishedEntry("5", "a"))
	idle := model.AgentState{Block: "5", State: "idle"}
	m, _ = relayed(m, idle)
	m, cmds := model.Update(m, model.AgentIssueRead{UID: "a", Status: "open"})
	if len(closes(cmds)) != 0 {
		t.Fatalf("closed: %#v", cmds)
	}
	for range 2 {
		m, cmds = relayed(m, idle)
		if len(reads(cmds)) != 0 {
			t.Fatalf("read on a repeated relay: %#v", cmds)
		}
	}
	// It is asked again once the agent finishes anew, and when the Issues change.
	m, _ = relayed(m, model.AgentState{Block: "5", State: "working"})
	if _, cmds = relayed(m, idle); len(reads(cmds)) != 1 {
		t.Fatalf("after working: %#v", cmds)
	}
}

func TestAnEntryWithoutAProjectOutsideTheCollectionIsLeftAlone(t *testing.T) {
	e := finishedEntry("5", "a")
	e.ProjectID = 0
	_, cmds := relayed(closePane(nil, e), model.AgentState{Block: "5", State: "idle"})
	if len(closes(cmds)) != 0 || len(reads(cmds)) != 0 {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestAFailedCloseLeavesTheEntryAndShowsTernsWords(t *testing.T) {
	m := closePane([]model.Issue{closed("a")}, finishedEntry("5", "a"))
	m, _ = relayed(m, model.AgentState{Block: "5", State: "idle"})
	m, cmds := model.Update(m, model.AgentClosed{Block: "5", Err: errors.New("exit status 1: no block")})
	if m.Notice != "Cannot close the agent: exit status 1: no block" || len(m.AgentMapping) != 1 || len(cmds) != 0 {
		t.Fatalf("notice %q, mapping %+v", m.Notice, m.AgentMapping)
	}
	if _, cmds = relayed(m, model.AgentState{Block: "5", State: "idle"}); len(closes(cmds)) != 0 {
		t.Fatalf("retried: %#v", cmds)
	}
}

func TestAClosedAgentLeavesTheMapping(t *testing.T) {
	m := closePane([]model.Issue{closed("a")}, finishedEntry("5", "a"), finishedEntry("6", "b"))
	kept := slices.Clone(m.AgentMapping[1:])
	m, _ = model.Update(m, model.AgentClosed{Block: "5", Agents: kept})
	if len(m.AgentMapping) != 1 || m.AgentMapping[0].Block != "6" || m.Notice != "" {
		t.Fatalf("mapping %+v, notice %q", m.AgentMapping, m.Notice)
	}
}

func TestAnAgentOpenFromAnotherPaneIsNeverEnded(t *testing.T) {
	m := closePane([]model.Issue{closed("a")}, finishedEntry("5", "a"))
	// The relay shows it outside the hidden session though this Pane's entry says not open.
	_, cmds := relayed(m, model.AgentState{Block: "5", State: "idle", Hidden: model.SeenShown})
	if len(closes(cmds)) != 0 {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestAnAgentTheGatewayKeptCanBeEndedAgainOnceItIsBack(t *testing.T) {
	m := closePane([]model.Issue{closed("a")}, finishedEntry("5", "a"))
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"})
	if len(closes(cmds)) != 1 {
		t.Fatalf("cmds %#v", cmds)
	}
	open := finishedEntry("5", "a")
	open.Open = true
	m, _ = model.Update(m, model.AgentClosed{Block: "5", Agents: []model.AgentEntry{open}, Kept: true})
	if !m.AgentMapping[0].Open {
		t.Fatalf("mapping %+v", m.AgentMapping)
	}
	back := finishedEntry("5", "a")
	m, _ = model.Update(m, model.AgentOpenSet{Block: "5", Open: false})
	m.AgentMapping = []model.AgentEntry{back}
	if _, cmds = relayed(m, model.AgentState{Block: "5", State: "exited"}); len(closes(cmds)) != 1 {
		t.Fatalf("cmds %#v", cmds)
	}
}
