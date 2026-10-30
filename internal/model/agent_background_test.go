package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func backgroundEntry(block, pane string, open bool) model.AgentEntry {
	e := entry("a", block, 1)
	e.IssueRef, e.PaneBlock, e.Background, e.Open = "demo#ab12", pane, true, open
	return e
}

func relayedTwice(m model.Model, before, after model.AgentState) (model.Model, []model.Cmd) {
	m, _ = relayed(m, before)
	return relayed(m, after)
}

func sentBack(cmds []model.Cmd) []model.SendAgentBack {
	var back []model.SendAgentBack
	for _, c := range cmds {
		if b, ok := c.(model.SendAgentBack); ok {
			back = append(back, b)
		}
	}
	return back
}

func TestAnOpenBackgroundAgentGoesBackWhenItWorksAfterWaiting(t *testing.T) {
	waiting, working := model.AgentState{Block: "5", State: "waiting_input"}, model.AgentState{Block: "5", State: "working"}
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", true)}}
	_, cmds := relayedTwice(m, waiting, working)
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Fatalf("cmds %#v, want %v", cmds, want)
	}
}

func TestAnAgentStaysWhereItIsUnlessTheRuleForGoingBackHolds(t *testing.T) {
	waiting := model.AgentState{Block: "5", State: "waiting_input"}
	for _, tt := range []struct {
		name   string
		entry  model.AgentEntry
		pane   string
		before model.AgentState
		after  model.AgentState
	}{
		{"idle after waiting", backgroundEntry("5", "9", true), "9", waiting, model.AgentState{Block: "5", State: "idle"}},
		{"exited after waiting", backgroundEntry("5", "9", true), "9", waiting, model.AgentState{Block: "5", State: "exited"}},
		{"still waiting", backgroundEntry("5", "9", true), "9", waiting, waiting},
		{"working after idle", backgroundEntry("5", "9", true), "9", model.AgentState{Block: "5", State: "idle"}, model.AgentState{Block: "5", State: "working"}},
		{"working after working", backgroundEntry("5", "9", true), "9", model.AgentState{Block: "5", State: "working"}, model.AgentState{Block: "5", State: "working"}},
		{"not open", backgroundEntry("5", "9", false), "9", waiting, model.AgentState{Block: "5", State: "working"}},
		{"not a background agent", func() model.AgentEntry { e := backgroundEntry("5", "9", true); e.Background = false; return e }(), "9", waiting, model.AgentState{Block: "5", State: "working"}},
		{"another Pane's agent", backgroundEntry("5", "8", true), "9", waiting, model.AgentState{Block: "5", State: "working"}},
		{"a Pane that cannot name itself", backgroundEntry("5", "", true), "", waiting, model.AgentState{Block: "5", State: "working"}},
	} {
		m := model.Model{PaneBlock: tt.pane, AgentMapping: []model.AgentEntry{tt.entry}}
		if _, cmds := relayedTwice(m, tt.before, tt.after); len(sentBack(cmds)) != 0 {
			t.Errorf("%s: cmds %#v", tt.name, cmds)
		}
	}
	// The first relay has no earlier list to have waited in.
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", true)}}
	if _, cmds := relayed(m, model.AgentState{Block: "5", State: "working"}); len(sentBack(cmds)) != 0 {
		t.Errorf("first relay: cmds %#v", cmds)
	}
}

func TestAJumpOpensABackgroundAgentThatIsNotOpenAndFocusesAnyOther(t *testing.T) {
	foreground := entry("a", "3", 1)
	closed, open := backgroundEntry("5", "9", false), backgroundEntry("6", "9", true)
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{foreground, closed, open}}
	for block, want := range map[string]model.Cmd{
		"5":  model.OpenAgent{Block: "5", PaneBlock: "9"},
		"6":  model.FocusAgent{Block: "6"},
		"3":  model.FocusAgent{Block: "3"},
		"77": model.FocusAgent{Block: "77"},
	} {
		if _, cmds := model.Update(m, model.AgentJumpMsg{Block: block}); !reflect.DeepEqual(cmds, []model.Cmd{want}) {
			t.Errorf("block %s: cmds %#v, want %#v", block, cmds, want)
		}
	}
}

func TestAPlacementAnswerMarksTheEntryAndShowsTheFailure(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	opened, _ := model.Update(m, model.AgentPlaced{Block: "5", Moved: true, Open: true})
	if !opened.AgentMapping[0].Open || m.AgentMapping[0].Open {
		t.Fatalf("after opening %+v (the old mapping %+v)", opened.AgentMapping, m.AgentMapping)
	}
	back, _ := model.Update(opened, model.AgentPlaced{Block: "5", Back: true, Moved: true, Err: errors.New("cannot name its tab")})
	if back.AgentMapping[0].Open || back.Notice != "Cannot move the agent back: cannot name its tab" {
		t.Fatalf("after going back %+v, notice %q", back.AgentMapping, back.Notice)
	}
	refused, _ := model.Update(m, model.AgentPlaced{Block: "5", Err: errors.New("exit status 1: no block")})
	if refused.AgentMapping[0].Open || refused.Notice != "Cannot open the agent: exit status 1: no block" {
		t.Fatalf("after a refusal %+v, notice %q", refused.AgentMapping, refused.Notice)
	}
}
