package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// searchedByWords is a Project Pane whose by-meaning search kata answered by words with no reason: the state after the answer and
// the commands the answer sent.
func searchedByWords(t *testing.T) driver.Output {
	t.Helper()
	s := feed(t, projectPane(t, 160, boardCard("a")), key("/"), key("ctrl+t"), key("a")).State
	out := step(t, s, key("enter"))
	var search model.SearchByMeaning
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.SearchByMeaning); ok {
			search = c
		}
	}
	if search.Query == "" {
		t.Fatalf("enter sent %#v, want the ranked search", out.Cmds)
	}
	return step(t, out.State, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query, UIDs: []string{"a"}, Lexical: true}})
}

// turnOnButton is the search line's Turn on… button as drawn, with its click action.
func turnOnButton(t *testing.T, s driver.State) (id, act string, ok bool) {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if strings.HasSuffix(n.ID, ".search-turn-on") {
			actions, _ := n.P["actions"].(map[string]any)
			act, _ = actions["click"].(string)
			if text, _ := n.P["text"].(string); text != "Turn on…" {
				t.Fatalf("button reads %q", text)
			}
			if keys, _ := n.P["keys"].([]any); len(keys) != 1 || keys[0] != "ctrl+o" {
				t.Fatalf("button keycaps %v, want ctrl+o", keys)
			}
			return n.ID, act, true
		}
	}
	return "", "", false
}

func TestSearchLineDrawsWhatKataAnsweredByWordsAndTheStatusSays(t *testing.T) {
	const lead = "kata searched by words: "
	out := searchedByWords(t)
	read := lastRead(t, out.Cmds)
	if got := searchLineDrawn(t, out.State); got != lead+"search by meaning is not in use on this daemon" {
		t.Fatalf("before the status: %q", got)
	}
	if _, _, ok := turnOnButton(t, out.State); ok {
		t.Fatal("Turn on… shows before the status is known")
	}
	for name, tc := range map[string]struct {
		status model.EmbeddingsStatus
		line   string
		button bool
	}{
		"can change":    {offWithDownload, lead + "search by meaning is off on this daemon", true},
		"restart owed":  {restartOwed, lead + "search by meaning is set up and waits for a kata daemon restart (`r` on the Config Page, `K`)", false},
		"administrator": {model.EmbeddingsStatus{Provider: model.EmbeddingsOff}, lead + "search by meaning is off; a daemon administrator can turn it on (kata's Settings or `kata daemon local-embeddings`)", false},
	} {
		s := statusAnswered(t, out.State, read, tc.status)
		if got := searchLineDrawn(t, s.State); got != tc.line {
			t.Errorf("%s: search line %q, want %q", name, got, tc.line)
		}
		if _, _, ok := turnOnButton(t, s.State); ok != tc.button {
			t.Errorf("%s: Turn on… drawn %v, want %v", name, ok, tc.button)
		}
		if opened := step(t, s.State, key("ctrl+o")).State; opened.Model.EmbeddingsCard.Open != tc.button {
			t.Errorf("%s: ctrl+o opened the card: %v, want %v", name, opened.Model.EmbeddingsCard.Open, tc.button)
		}
	}
}

func TestTurnOnButtonAndCtrlOOpenTheQuestionAndClosingReturnsToTheSearch(t *testing.T) {
	out := searchedByWords(t)
	s := statusAnswered(t, out.State, lastRead(t, out.Cmds), offWithDownload).State
	mode := s.Model.CurrentMode()
	id, act, ok := turnOnButton(t, s)
	if !ok {
		t.Fatal("no Turn on… button")
	}
	keyboard, keyCmds := keys("ctrl+o")(s)
	pointer, clickCmds := clicked(s, click(id, act))
	sameAs(t, "the button differs from ctrl+o", pointer, clickCmds, keyboard, keyCmds)
	headless, headlessCmds := keys("meta+o")(s)
	sameAs(t, "meta+o differs from ctrl+o", headless, headlessCmds, keyboard, keyCmds)
	if keyboard.Model.CurrentMode() != model.ModeEmbeddingsAsk {
		t.Fatalf("mode %s, want the question", keyboard.Model.CurrentMode())
	}
	closed := step(t, keyboard, key("n")).State
	if closed.Model.CurrentMode() != mode || !closed.Model.Search.Open {
		t.Fatalf("after n: mode %s, want %s with the search still open", closed.Model.CurrentMode(), mode)
	}
}

func TestTypingAfterAnAnswerByWordsDoesNotReadTheStatus(t *testing.T) {
	out := searchedByWords(t)
	s := statusAnswered(t, out.State, lastRead(t, out.Cmds), offWithDownload).State
	typed, cmds := inputs(chord("b"))(s)
	if _, ok := askedEmbeddings(cmds); ok {
		t.Fatalf("typing read the status: %#v", cmds)
	}
	_ = typed
}
