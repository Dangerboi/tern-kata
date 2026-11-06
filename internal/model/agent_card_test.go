package model_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
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

// The card's handlers refuse with the reason its buttons show.
func TestAgentCardRefusalNoticeIsTheButtonsReason(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	m, _ = model.Update(m, model.KeyMsg{Chord: "w"})
	m.AgentCard.IssueUID = "missing"
	reason := m.AgentCardUnavailable(model.AgentCardTab)
	if reason == "" {
		t.Fatal("a start on an Issue the Pane does not hold is available")
	}
	after, cmds := model.Update(m, model.KeyMsg{Chord: "t"})
	if _, started := agentCmd[model.StartAgent](cmds); started || after.Notice != reason || after.TopCard() == model.CardAgent {
		t.Errorf("notice %q, started %v, card %v; want the button's reason %q and the card closed", after.Notice, started, after.TopCard(), reason)
	}
}

// openCard is the Agent card open on Issue u1 over the mapping of exitedPane, whose agent 11 is this Pane's.
func openCard(status string, states ...model.AgentState) model.Model {
	m := exitedPane(status)
	m.Agents.States = states
	m.AgentCard = model.AgentCardState{Open: true, IssueUID: "u1", IssueRef: "kata-u1"}
	return m
}

func TestIInterruptsTheSelectedAgentAndKeepsTheCardOpen(t *testing.T) {
	for _, state := range []model.AgentStateWord{model.AgentWorking, model.AgentIdle, model.AgentWaitingInput, ""} {
		m := openCard("open", st("11", state, ""), st("12", state, ""))
		m.AgentCard.Cursor = 1
		after, cmds := press(m, "i")
		if want := []model.Cmd{model.InterruptAgent{Block: "12"}}; !reflect.DeepEqual(cmds, want) || !after.AgentCard.Open || after.AgentCardCursor() != 1 {
			t.Errorf("%q: cmds %#v, card open %v, cursor %d; want %v with the card open on the same agent", state, cmds, after.AgentCard.Open, after.AgentCardCursor(), want)
		}
	}
}

func TestInterruptIsRefusedWithAReasonWhereItCannotWork(t *testing.T) {
	notOmp := openCard("open", st("11", model.AgentWorking, ""))
	notOmp.AgentMapping[0].NoAddress = model.AgentNotOmp
	for name, tt := range map[string]struct {
		m      model.Model
		reason string
	}{
		"exited":       {openCard("open", st("11", model.AgentExited, "")), model.AgentCardExitedReason},
		"not omp":      {notOmp, model.AgentCardNotOmpReason},
		"no agent yet": {func() model.Model { m := openCard("open"); m.AgentCard.IssueUID = "none"; return m }(), "No agent on this Issue yet."},
	} {
		if got := tt.m.AgentCardUnavailable(model.AgentCardInterrupt); got != tt.reason {
			t.Errorf("%s: unavailable says %q, want %q", name, got, tt.reason)
		}
		after, cmds := press(tt.m, "i")
		if len(cmds) != 0 {
			t.Errorf("%s: cmds %#v", name, cmds)
		}
		if name != "no agent yet" && (after.Notice != tt.reason || after.AgentCard.Open) {
			t.Errorf("%s: notice %q, card open %v", name, after.Notice, after.AgentCard.Open)
		}
	}
}

func TestXAsksBeforeStoppingAndOnlyYStops(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""))
	asked, cmds := press(m, "x")
	if len(cmds) != 0 || asked.AgentCard.Stopping != "11" || asked.CurrentMode() != model.ModeAgentCardStop {
		t.Fatalf("x: cmds %#v, stopping %q, mode %s; want the question and no command", cmds, asked.AgentCard.Stopping, asked.CurrentMode())
	}
	for _, chord := range []string{"n", "escape"} {
		kept, cmds := press(asked, chord)
		if len(cmds) != 0 || !kept.AgentCard.Open || kept.AgentCard.Stopping != "" || kept.CurrentMode() != model.ModeAgentCard {
			t.Errorf("%s: cmds %#v, card %+v", chord, cmds, kept.AgentCard)
		}
	}
	if other, cmds := press(asked, "z"); len(cmds) != 0 || other.AgentCard.Stopping != "11" {
		t.Errorf("another key left the question: cmds %#v, stopping %q", cmds, other.AgentCard.Stopping)
	}
	stopped, cmds := press(asked, "y")
	if want := []model.Cmd{model.StopAgent{Block: "11"}}; !reflect.DeepEqual(cmds, want) || stopped.AgentCard.Open {
		t.Errorf("y: cmds %#v, card open %v; want %v and the card closed", cmds, stopped.AgentCard.Open, want)
	}
}

func TestStopIsOfferedForAnyPlacementAndRefusedForAnExitedAgent(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""), st("12", model.AgentIdle, ""), st("21", model.AgentIdle, ""))
	m.AgentMapping[1].Placement = model.AgentTab
	m.AgentMapping[2] = model.AgentEntry{IssueUID: "u1", Block: "21", PaneBlock: "20", Background: true}
	for cursor, want := range []string{"11", "12", "21"} {
		m.AgentCard.Cursor = cursor
		asked, _ := press(m, "x")
		if asked.AgentCard.Stopping != want {
			t.Errorf("row %d: asked about %q, want %q (another Pane's Background agent included)", cursor, asked.AgentCard.Stopping, want)
		}
	}
	exited := openCard("open", st("11", model.AgentExited, ""))
	after, cmds := press(exited, "x")
	if len(cmds) != 0 || after.AgentCard.Open || after.Notice != model.AgentCardExitedReason {
		t.Errorf("exited: cmds %#v, card open %v, notice %q", cmds, after.AgentCard.Open, after.Notice)
	}
}

func handOff(label bool) model.HandOffAgent {
	return model.HandOffAgent{Workspace: "/w", ProjectID: 3, UID: "u1", Actor: "dana", Message: model.AgentStopHandOffMessage, SwapLabels: label}
}

func TestStoppingAnAgentOfThisPaneGivesItsIssueTheExitHandOff(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""))
	m.Issues[0].Labels = []string{"ready-for-agent"}
	rest := slices.Delete(slices.Clone(m.AgentMapping), 0, 1)
	m, cmds := model.Update(m, model.AgentStopped{Block: "11", Agents: rest})
	if want := []model.Cmd{raise("u1", 3, "agent exited without closing kata-u1"), handOff(true)}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v, want %v", cmds, want)
	}
	if slices.ContainsFunc(m.AgentMapping, func(e model.AgentEntry) bool { return e.Block == "11" }) || !strings.HasPrefix(m.Notice, "Stopped ") {
		t.Errorf("mapping %+v, notice %q", m.AgentMapping, m.Notice)
	}
}

func TestStoppingAnAgentOfAnIssueWithoutReadyForAgentSwapsNoLabels(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""))
	rest := slices.Delete(slices.Clone(m.AgentMapping), 0, 1)
	_, cmds := model.Update(m, model.AgentStopped{Block: "11", Agents: rest})
	if !slices.Contains(cmds, model.Cmd(handOff(false))) {
		t.Errorf("cmds %#v, want the hand-off without the label swap", cmds)
	}
}

func TestStoppingAgentOfAClosedIssueRaisesNothing(t *testing.T) {
	m := openCard("closed", st("11", model.AgentWorking, ""))
	rest := slices.Delete(slices.Clone(m.AgentMapping), 0, 1)
	if _, cmds := model.Update(m, model.AgentStopped{Block: "11", Agents: rest}); len(cmds) != 0 {
		t.Errorf("cmds %#v", cmds)
	}
}

func TestStopAnsweredForAnAgentAlreadyGoneWritesNoHandOff(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""))
	rest := slices.Delete(slices.Clone(m.AgentMapping), 0, 1)
	_, cmds := model.Update(m, model.AgentStopped{Block: "11", Agents: rest, Gone: true})
	for _, c := range cmds {
		if _, is := c.(model.HandOffAgent); is {
			t.Errorf("cmds %#v hold a hand-off for an agent already gone", cmds)
		}
	}
}

func TestAFailedHandOffIsANoticeNotACrash(t *testing.T) {
	after, cmds := model.Update(model.Model{}, model.AgentHandedOff{UID: "u1", Err: errors.New("kata is down")})
	if len(cmds) != 0 || after.Notice != "Cannot hand the Issue to a human after the stop: kata is down" {
		t.Errorf("cmds %#v, notice %q", cmds, after.Notice)
	}
}

func TestAFailedStopKeepsTheMappingAndSaysWhy(t *testing.T) {
	m := openCard("open", st("11", model.AgentWorking, ""))
	after, cmds := model.Update(m, model.AgentStopped{Block: "11", Err: errors.New("tern: no such block")})
	if len(cmds) != 0 || !reflect.DeepEqual(after.AgentMapping, m.AgentMapping) || after.Notice != "Cannot stop the agent: tern: no such block" {
		t.Errorf("cmds %#v, mapping %+v, notice %q", cmds, after.AgentMapping, after.Notice)
	}
}

func TestAFailedInterruptSaysWhy(t *testing.T) {
	after, _ := model.Update(model.Model{}, model.AgentInterrupted{Block: "11", Err: errors.New("no title")})
	if after.Notice != "Cannot interrupt the agent: no title" {
		t.Errorf("notice %q", after.Notice)
	}
}
