package model_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// cardModel is a model with Issue "a" selected and the given agents mapped to it.
func cardModel(entries ...model.AgentEntry) model.Model {
	return model.Model{
		Issues:       []model.Issue{{UID: "a", QualifiedID: "work#a", Title: "A", Status: "open", ProjectID: 7}},
		SelectedUID:  "a",
		AgentMapping: entries,
		PaneBlock:    "9",
	}
}

func press(m model.Model, chords ...string) (model.Model, []model.Cmd) {
	var cmds []model.Cmd
	for _, c := range chords {
		m, cmds = model.Update(m, model.KeyMsg{Chord: c})
	}
	return m, cmds
}

func TestWOpensTheAgentCardWithOrWithoutAgents(t *testing.T) {
	for name, m := range map[string]model.Model{
		"no agents":                      cardModel(),
		"an agent on another Issue":      cardModel(entry("other", "1", 1)),
		"an agent on the selected Issue": cardModel(entry("a", "1", 1)),
	} {
		opened, cmds := press(m, "w")
		if opened.TopCard() != model.CardAgent || opened.CurrentMode() != model.ModeAgentCard || opened.AgentCard.IssueUID != "a" {
			t.Errorf("%s: card %v, mode %s; want the Agent card on a", name, opened.TopCard(), opened.CurrentMode())
		}
		if _, started := agentCmd[model.StartAgent](cmds); started {
			t.Errorf("%s: w started an agent: %#v", name, cmds)
		}
	}
}

func TestWOnAClosedIssueOpensNoCard(t *testing.T) {
	m := cardModel()
	m.Issues[0].Status = "closed"
	after, cmds := press(m, "w")
	if _, started := agentCmd[model.StartAgent](cmds); after.TopCard() == model.CardAgent || started || after.Notice != model.ClosedIssueRefusal {
		t.Errorf("card %v, cmds %#v, notice %q", after.TopCard(), cmds, after.Notice)
	}
}

func agentCmd[T model.Cmd](cmds []model.Cmd) (T, bool) {
	for _, c := range cmds {
		if v, ok := c.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func TestTheAgentCardCursorSelectsTheAgentEnterGoesTo(t *testing.T) {
	m, _ := press(cardModel(entry("a", "1", 1), entry("a", "2", 2), entry("a", "3", 3)), "w")
	for _, tt := range []struct {
		keys  []string
		block string
	}{
		{nil, "1"},
		{[]string{"j"}, "2"},
		{[]string{"down", "down"}, "3"},
		{[]string{"down", "down", "down", "down"}, "3"},
		{[]string{"j", "k"}, "1"},
		{[]string{"up"}, "1"},
		{[]string{"end"}, "3"},
		{[]string{"end", "home"}, "1"},
		{[]string{"down", "up", "down"}, "2"},
	} {
		moved, _ := press(m, tt.keys...)
		after, cmds := press(moved, "enter")
		focus, ok := agentCmd[model.FocusAgent](cmds)
		if !ok || focus.Block != tt.block || after.TopCard() == model.CardAgent {
			t.Errorf("%v then enter: cmds %#v, card %v; want focus of %s with the card closed", tt.keys, cmds, after.TopCard(), tt.block)
		}
	}
}

func TestEscClosesTheAgentCardAndStartsNothing(t *testing.T) {
	m, _ := press(cardModel(entry("a", "1", 1)), "w")
	m, cmds := press(m, "escape")
	if m.TopCard() == model.CardAgent || len(cmds) != 0 || m.AgentCard != (model.AgentCardState{}) {
		t.Fatalf("card %v, cmds %#v, state %+v", m.TopCard(), cmds, m.AgentCard)
	}
}

func TestOtherKeysAndAGoneAgentLeaveTheAgentCardOpen(t *testing.T) {
	m, _ := press(cardModel(entry("a", "1", 1)), "w")
	for _, chord := range []string{"z", "q", "?", "1"} {
		after, cmds := press(m, chord)
		if after.TopCard() != model.CardAgent || len(cmds) != 0 || after.Quitting {
			t.Errorf("%s: card %v, cmds %#v, quitting %v", chord, after.TopCard(), cmds, after.Quitting)
		}
	}
	m.AgentMapping = nil
	after, cmds := press(m, "enter")
	if after.TopCard() != model.CardAgent || len(cmds) != 0 {
		t.Errorf("enter with no agent: card %v, cmds %#v", after.TopCard(), cmds)
	}
	if quit, _ := press(m, "ctrl+c"); !quit.Quitting {
		t.Error("ctrl+c did not quit")
	}
}

func TestAClickOnAnAgentRowMovesTheCursor(t *testing.T) {
	m, _ := press(cardModel(entry("a", "1", 1), entry("a", "2", 2)), "w")
	m, _ = model.Update(m, model.AgentCardSelected{Block: "2"})
	if m.AgentCardCursor() != 1 {
		t.Fatalf("cursor %d, want 1", m.AgentCardCursor())
	}
}

func TestAgentWhereReadsTheStartedPlacementAndDefaultsToASplit(t *testing.T) {
	for placement, want := range map[model.AgentPlacement]string{"": "split", model.AgentSplit: "split", model.AgentTab: "tab"} {
		if got := model.AgentWhere(model.AgentEntry{Placement: placement, StartedAt: time.Time{}}); got != want {
			t.Errorf("%q: %q, want %q", placement, got, want)
		}
	}
}

func TestTheCardsStartKeysStartWhereTheyNameAndCloseTheCard(t *testing.T) {
	for name, withAgent := range map[string]bool{"no agents": false, "an agent": true} {
		for chord, want := range map[string]model.AgentPlacement{"s": model.AgentSplit, "t": model.AgentTab, "g": model.AgentBackground} {
			m := cardModel()
			if withAgent {
				m = cardModel(entry("a", "1", 1))
			}
			opened, _ := press(m, "w")
			m, cmds := press(opened, chord)
			start, ok := agentCmd[model.StartAgent](cmds)
			if !ok || len(cmds) != 1 || start.Placement != want || start.IssueUID != "a" || start.PaneBlock != "9" || m.TopCard() == model.CardAgent {
				t.Errorf("%s, w then %s: cmds %#v, card %v; want a start %q with the card closed", name, chord, cmds, m.TopCard(), want)
			}
		}
	}
}

func TestEnterAndBDoNothingOnACardWithNoAgents(t *testing.T) {
	opened, _ := press(cardModel(), "w")
	for _, chord := range []string{"enter", "b"} {
		m, cmds := press(opened, chord)
		if len(cmds) != 0 || m.TopCard() != model.CardAgent {
			t.Errorf("%s with no agents: cmds %#v, card %v", chord, cmds, m.TopCard())
		}
	}
}

func TestShiftWAndShiftSStartNothingAndOpenNoCard(t *testing.T) {
	for _, chord := range []string{"shift+w", "shift+s"} {
		for _, m := range []model.Model{cardModel(), cardModel(entry("a", "1", 1))} {
			after, cmds := press(m, chord)
			if _, started := agentCmd[model.StartAgent](cmds); started || after.TopCard() == model.CardAgent {
				t.Errorf("%s: cmds %#v, card %v", chord, cmds, after.TopCard())
			}
		}
	}
}

func TestBSendsTheSelectedAgentInViewToTheBackgroundAndMarksIt(t *testing.T) {
	first, second := entry("a", "1", 1), entry("a", "2", 2)
	second.IssueRef = "work#a"
	opened, _ := press(cardModel(first, second), "w", "down")
	m, cmds := press(opened, "b")
	back, ok := agentCmd[model.SendAgentBack](cmds)
	if !ok || back != (model.SendAgentBack{Block: "2", IssueRef: "work#a"}) || m.TopCard() == model.CardAgent {
		t.Fatalf("b: cmds %#v, card %v", cmds, m.TopCard())
	}
	m, _ = model.Update(m, model.AgentPlaced{Block: "2", Back: true, Moved: true})
	if got := m.AgentMapping[1]; !got.Background || got.Open || model.AgentWhere(got) != "background" {
		t.Errorf("entry after b: %+v", got)
	}
	if m.AgentMapping[0].Background {
		t.Errorf("the other agent changed: %+v", m.AgentMapping[0])
	}
}

func TestBOnAnOpenBackgroundAgentSendsItBackAndOnAClosedOneKeepsItThere(t *testing.T) {
	open := entry("a", "1", 1)
	open.Background, open.Open = true, true
	_, cmds := press(cardModel(open), "w", "b")
	if _, ok := agentCmd[model.SendAgentBack](cmds); !ok {
		t.Errorf("open background agent: cmds %#v", cmds)
	}
	closed := entry("a", "1", 1)
	closed.Background = true
	m, cmds := press(cardModel(closed), "w", "b")
	if len(cmds) != 0 || m.TopCard() == model.CardAgent || m.Notice == "" {
		t.Errorf("background agent: cmds %#v, card %v, notice %q", cmds, m.TopCard(), m.Notice)
	}
}

func TestEnterOnAClosedBackgroundAgentOpensItBesideThePane(t *testing.T) {
	e := entry("a", "1", 1)
	e.Background = true
	_, cmds := press(cardModel(e), "w", "enter")
	if open, ok := agentCmd[model.OpenAgent](cmds); !ok || open != (model.OpenAgent{Block: "1", PaneBlock: "9"}) {
		t.Errorf("cmds %#v", cmds)
	}
}

func TestAgentWhereNamesBackgroundAndOpenAgents(t *testing.T) {
	for want, e := range map[string]model.AgentEntry{
		"background": {Background: true, Placement: model.AgentBackground},
		"open":       {Background: true, Open: true, Placement: model.AgentBackground},
		"tab":        {Placement: model.AgentTab},
	} {
		if got := model.AgentWhere(e); got != want {
			t.Errorf("%+v: %q, want %q", e, got, want)
		}
	}
}
