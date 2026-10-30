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

func TestWOpensTheAgentCardOnlyWhenTheIssueHasAgents(t *testing.T) {
	for _, chord := range []string{"w", "shift+w"} {
		m, cmds := press(cardModel(), chord)
		if _, started := agentCmd[model.StartAgent](cmds); m.TopCard() == model.CardAgent || !started {
			t.Fatalf("%s with no agents: card %v, cmds %#v; want a start", chord, m.TopCard(), cmds)
		}
		m, cmds = press(cardModel(entry("other", "1", 1)), chord)
		if _, started := agentCmd[model.StartAgent](cmds); m.TopCard() == model.CardAgent || !started {
			t.Fatalf("%s with an agent on another Issue: card %v, cmds %#v; want a start", chord, m.TopCard(), cmds)
		}
		m, cmds = press(cardModel(entry("a", "1", 1)), chord)
		if m.TopCard() != model.CardAgent || m.CurrentMode() != model.ModeAgentCard {
			t.Fatalf("%s with an agent: card %v, mode %s; want the Agent card", chord, m.TopCard(), m.CurrentMode())
		}
		if _, started := agentCmd[model.StartAgent](cmds); started {
			t.Fatalf("%s with an agent started one: %#v", chord, cmds)
		}
	}
}

func TestTheAgentCardKeepsThePlacementTheOpeningKeyAskedAndNStartsThere(t *testing.T) {
	for chord, want := range map[string]model.AgentPlacement{"w": model.AgentSplit, "shift+w": model.AgentTab} {
		opened, _ := press(cardModel(entry("a", "1", 1)), chord)
		if opened.AgentCard.Placement != want {
			t.Fatalf("%s: placement %q, want %q", chord, opened.AgentCard.Placement, want)
		}
		m, cmds := press(opened, "n")
		start, ok := agentCmd[model.StartAgent](cmds)
		if !ok || start.Placement != want || start.IssueUID != "a" || m.TopCard() == model.CardAgent {
			t.Errorf("%s then n: cmds %#v, card %v; want a start %q with the card closed", chord, cmds, m.TopCard(), want)
		}
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
