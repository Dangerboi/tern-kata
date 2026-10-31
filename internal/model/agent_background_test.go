package model_test

import (
	"errors"
	"reflect"
	"slices"
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

func at(block string, state model.AgentStateWord, where model.AgentSeen) model.AgentState {
	return model.AgentState{Block: block, State: state, Hidden: where}
}

func flagWrites(cmds []model.Cmd) []model.SetAgentOpen {
	var writes []model.SetAgentOpen
	for _, c := range cmds {
		if w, ok := c.(model.SetAgentOpen); ok {
			writes = append(writes, w)
		}
	}
	return writes
}

func TestTheOpenFlagFollowsWhereTheRelayShowsAnOwnBackgroundAgent(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	m, cmds := relayed(m, at("5", "waiting_input", model.SeenShown))
	if want := []model.SetAgentOpen{{Block: "5", Open: true}}; !reflect.DeepEqual(flagWrites(cmds), want) {
		t.Fatalf("out of the hidden session: %#v", cmds)
	}
	// One write per change: a second relay while the first is in flight asks for none.
	m, cmds = relayed(m, at("5", "waiting_input", model.SeenShown))
	if len(flagWrites(cmds)) != 0 {
		t.Fatalf("write repeated while in flight: %#v", cmds)
	}
	m, _ = model.Update(m, model.AgentOpenSet{Block: "5", Open: true})
	if !m.AgentMapping[0].Open {
		t.Fatalf("mapping %+v", m.AgentMapping)
	}
	if _, cmds = relayed(m, at("5", "waiting_input", model.SeenShown)); len(flagWrites(cmds)) != 0 {
		t.Fatalf("nothing changed, yet %#v", cmds)
	}
	// Back in the hidden session clears it.
	m, cmds = relayed(m, at("5", "waiting_input", model.SeenHidden))
	if want := []model.SetAgentOpen{{Block: "5", Open: false}}; !reflect.DeepEqual(flagWrites(cmds), want) {
		t.Fatalf("back in the hidden session: %#v", cmds)
	}
	m, _ = model.Update(m, model.AgentOpenSet{Block: "5", Open: false})
	if m.AgentMapping[0].Open {
		t.Fatalf("mapping %+v", m.AgentMapping)
	}
}

func TestAFailedFlagWriteAsksAgainOnTheNextRelay(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	m, _ = relayed(m, at("5", "waiting_input", model.SeenShown))
	m, _ = model.Update(m, model.AgentOpenSet{Block: "5", Open: true, Err: errors.New("disk full")})
	if m.AgentMapping[0].Open || m.Notice == "" {
		t.Fatalf("open %v, notice %q", m.AgentMapping[0].Open, m.Notice)
	}
	if _, cmds := relayed(m, at("5", "waiting_input", model.SeenShown)); len(flagWrites(cmds)) != 1 {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestTheOpenFlagIsLeftAloneUnlessItIsAnOwnBackgroundAgentWithAKnownPlace(t *testing.T) {
	split := backgroundEntry("5", "9", false)
	split.Background = false
	for name, tt := range []struct {
		entry model.AgentEntry
		state model.AgentState
	}{
		{split, at("5", "idle", model.SeenShown)},
		{backgroundEntry("5", "8", false), at("5", "idle", model.SeenShown)},
		{backgroundEntry("5", "9", false), at("5", "idle", model.SeenUnknown)},
		{backgroundEntry("5", "9", true), at("5", "idle", model.SeenUnknown)},
		{backgroundEntry("5", "9", false), at("5", "idle", model.SeenHidden)},
		{backgroundEntry("5", "9", true), at("5", "idle", model.SeenShown)},
	} {
		m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{tt.entry}}
		if _, cmds := relayed(m, tt.state); len(flagWrites(cmds)) != 0 {
			t.Errorf("case %d: %#v", name, cmds)
		}
	}
}

func TestAnAgentSeenOutOfTheHiddenSessionGoesBackWhenItWorksAgain(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	m, _ = relayed(m, at("5", "waiting_input", model.SeenShown))
	m, _ = model.Update(m, model.AgentOpenSet{Block: "5", Open: true})
	_, cmds := relayed(m, at("5", "working", model.SeenShown))
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestNoFlagIsWrittenWhileThisPaneMovesTheAgent(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	m, cmds := model.Update(m, model.AgentJumpMsg{Block: "5"})
	if len(cmds) != 1 {
		t.Fatalf("cmds %#v", cmds)
	}
	if _, cmds = relayed(m, at("5", "waiting_input", model.SeenShown)); len(flagWrites(cmds)) != 0 {
		t.Fatalf("flag written during the move: %#v", cmds)
	}
	m, _ = model.Update(m, model.AgentPlaced{Block: "5", Moved: true, Open: true})
	if _, cmds = relayed(m, at("5", "waiting_input", model.SeenShown)); len(flagWrites(cmds)) != 0 {
		t.Fatalf("flag already open, yet %#v", cmds)
	}
}

func TestAnAgentOpenedFromAnotherPaneGoesBackOnTheFirstAnswer(t *testing.T) {
	// This Pane's copy says not open and no relay showed it open before the working one.
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{backgroundEntry("5", "9", false)}}
	m, _ = relayed(m, at("5", "waiting_input", model.SeenHidden))
	_, cmds := relayed(m, at("5", "working", model.SeenShown))
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Fatalf("cmds %#v", cmds)
	}
}

// sentToBackgroundByAnotherPane is the file after Pane B's b on Pane 9's split agent: no relay follows it.
func sentToBackgroundByAnotherPane() (split, moved model.AgentEntry) {
	split = backgroundEntry("5", "9", false)
	split.Background = false
	moved = split
	moved.Background = true
	return split, moved
}

func refreshes(cmds []model.Cmd) bool {
	return slices.ContainsFunc(cmds, func(c model.Cmd) bool { _, ok := c.(model.RefreshAgents); return ok })
}

func TestAnAnsweredSplitAgentThatAnotherPaneSentToTheBackgroundGoesBackOnceSomeoneOpenedIt(t *testing.T) {
	split, moved := sentToBackgroundByAnotherPane()
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{split}}
	m, _ = relayed(m, at("5", "waiting_input", model.SeenShown))
	// The user answers it in the hidden session, where it was opened: the relay shows it outside, and this Pane's entry still says split.
	m, cmds := relayed(m, at("5", "working", model.SeenShown))
	if len(sentBack(cmds)) != 0 || !refreshes(cmds) {
		t.Fatalf("answered split agent: cmds %#v, want a re-read of agents.json", cmds)
	}
	_, cmds = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{moved}})
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Fatalf("after the re-read: cmds %#v", cmds)
	}
}

func TestAnAnsweredSplitAgentTheFileStillCallsSplitStaysWhereItIs(t *testing.T) {
	split, _ := sentToBackgroundByAnotherPane()
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{split}}
	m, _ = relayed(m, at("5", "waiting_input", model.SeenShown))
	m, _ = relayed(m, at("5", "working", model.SeenShown))
	if _, cmds := model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{split}}); len(sentBack(cmds)) != 0 {
		t.Fatalf("a split agent went back: %#v", cmds)
	}
}

func TestAFinishedSplitAgentThatAnotherPaneSentToTheBackgroundIsEndedOnceItsIssueIsClosed(t *testing.T) {
	split, moved := sentToBackgroundByAnotherPane()
	m := closePane([]model.Issue{closed("a")}, split)
	m.AgentMapping[0].IssueUID, m.AgentMapping[0].ProjectID = "a", 3
	moved.IssueUID, moved.ProjectID = "a", 3
	m, _ = relayed(m, at("5", "working", model.SeenHidden))
	m, cmds := relayed(m, at("5", "idle", model.SeenHidden))
	if !refreshes(cmds) {
		t.Fatalf("finished split agent: cmds %#v, want a re-read of agents.json", cmds)
	}
	m, _ = model.Update(m, model.AgentClosed{Block: "5", Agents: []model.AgentEntry{split}, Kept: true})
	if _, cmds = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{moved}}); len(closes(cmds)) != 1 {
		t.Fatalf("after the re-read: cmds %#v", cmds)
	}
}
