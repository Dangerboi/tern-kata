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
	s := statusAnswered(t, out.State, read, offWithDownload)
	if got := searchLineDrawn(t, s.State); got != lead+"search by meaning is off on this daemon" {
		t.Errorf("search line %q after the status", got)
	}
	if _, _, ok := turnOnButton(t, s.State); !ok {
		t.Error("Turn on… is not drawn although kata would take a turn-on")
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
