package model_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// wordsAnswered is a ranked search answered by words with no reason: the model after it, and the commands the answer sent.
func wordsAnswered(t *testing.T) (model.Model, []model.Cmd) {
	t.Helper()
	m, search := searched(t, model.Filters{}, "slow login")
	done := model.SearchByMeaningDone{Request: search.Request, RankedAnswer: model.RankedAnswer{Query: search.Query, UIDs: []string{"a"}, Lexical: true}}
	return model.Update(m, done)
}

// statusRead delivers status as the answer to the read in cmds.
func statusRead(t *testing.T, m model.Model, cmds []model.Cmd, status model.EmbeddingsStatus) model.Model {
	t.Helper()
	read := cmdOf[model.LoadEmbeddingsStatus](t, cmds)
	m, _ = model.Update(m, model.EmbeddingsStatusLoaded{Request: read.Request, Status: status})
	return m
}

func TestAnswerByWordsWithNoReasonReadsTheStatusOnce(t *testing.T) {
	m, cmds := wordsAnswered(t)
	cmdOf[model.LoadEmbeddingsStatus](t, cmds)
	m = statusRead(t, m, cmds, model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true})
	for _, text := range []string{"x", "y"} {
		var typed []model.Cmd
		m, typed = model.Update(m, model.KeyMsg{Text: text})
		if slices.ContainsFunc(typed, func(c model.Cmd) bool { _, ok := c.(model.LoadEmbeddingsStatus); return ok }) {
			t.Fatalf("typing %q read the status again: %#v", text, typed)
		}
	}
}

func TestAnswersThatNeedNoStatusDoNotReadIt(t *testing.T) {
	for name, answer := range map[string]model.RankedAnswer{
		"words with kata's reason": {UIDs: []string{"a"}, Lexical: true, Reason: "embedding model is loading"},
		"by meaning":               {UIDs: []string{"a"}},
		"by meaning but degraded":  {UIDs: []string{"a"}, Reason: "results may be incomplete"},
	} {
		m, search := searched(t, model.Filters{}, "slow login")
		answer.Query = search.Query
		_, cmds := model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: answer})
		if slices.ContainsFunc(cmds, func(c model.Cmd) bool { _, ok := c.(model.LoadEmbeddingsStatus); return ok }) {
			t.Errorf("%s: read the status: %#v", name, cmds)
		}
	}
}

func TestSearchLineWordsAnAnswerByWordsFromTheStatus(t *testing.T) {
	const lead = "kata searched by words: "
	for _, tt := range []struct {
		name   string
		status *model.EmbeddingsStatus
		want   string
	}{
		{"not read yet", nil, lead + "search by meaning is not in use on this daemon"},
		{"can be turned on", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true}, lead + "search by meaning is off on this daemon"},
		{"a failed job is offered again", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsFailed}}, lead + "search by meaning is off on this daemon"},
		{"restart owed, local", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}, lead + "search by meaning is set up and waits for a kata daemon restart (`r` on the Config Page, `K`)"},
		{"restart owed, remote", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff, Remote: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}, lead + "search by meaning is set up and waits for a kata daemon restart (run `kata daemon restart` on that machine)"},
		{"administrators only", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff}, lead + "search by meaning is off; a daemon administrator can turn it on (kata's Settings or `kata daemon local-embeddings`)"},
		{"downloading", &model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading}}, lead + "search by meaning is being set up on this daemon (see the Config Page, `K`)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := wordsAnswered(t)
			if tt.status != nil {
				m = statusRead(t, m, cmds, *tt.status)
			}
			if got := m.SearchLine(); got != tt.want {
				t.Fatalf("search line %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCtrlOAndTheButtonOfferTheCardOnlyWhenKataWouldTakeATurnOn(t *testing.T) {
	for name, tt := range map[string]struct {
		status *model.EmbeddingsStatus
		offer  bool
	}{
		"status not read":   {nil, false},
		"can change":        {&model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true}, true},
		"administrator":     {&model.EmbeddingsStatus{Provider: model.EmbeddingsOff}, false},
		"restart owed":      {&model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}}, false},
		"already on":        {&model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, CanChange: true}, false},
		"download underway": {&model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading}}, false},
	} {
		for _, chord := range []string{"ctrl+o", "meta+o"} {
			t.Run(name+" "+chord, func(t *testing.T) {
				m, cmds := wordsAnswered(t)
				if tt.status != nil {
					m = statusRead(t, m, cmds, *tt.status)
				}
				if m.SearchTurnOnOffered() != tt.offer {
					t.Fatalf("button offered %v, want %v", m.SearchTurnOnOffered(), tt.offer)
				}
				mode := m.CurrentMode()
				opened, _ := press(m, chord)
				if opened.EmbeddingsCard.Open != tt.offer {
					t.Fatalf("card open %v, want %v", opened.EmbeddingsCard.Open, tt.offer)
				}
				if !tt.offer {
					if opened.CurrentMode() != mode {
						t.Fatalf("mode %s, want %s", opened.CurrentMode(), mode)
					}
					return
				}
				if opened.CurrentMode() != model.ModeEmbeddingsAsk {
					t.Fatalf("mode %s, want the question", opened.CurrentMode())
				}
				closed, _ := press(opened, "n")
				if closed.CurrentMode() != mode || !closed.Search.Open || closed.Search.Results != m.Search.Results {
					t.Fatalf("closing the card left mode %s with search %+v, want %s", closed.CurrentMode(), closed.Search, mode)
				}
			})
		}
	}
}

func TestCtrlOIsNotOfferedWhenKataGaveAReasonOrAnsweredByMeaning(t *testing.T) {
	for name, answer := range map[string]model.RankedAnswer{
		"a reason":   {UIDs: []string{"a"}, Lexical: true, Reason: "embedding model is loading"},
		"by meaning": {UIDs: []string{"a"}},
	} {
		m, search := searched(t, model.Filters{}, "slow login")
		m.Embeddings = model.EmbeddingsSetup{Read: true, Status: model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true}}
		answer.Query = search.Query
		m, _ = model.Update(m, model.SearchByMeaningDone{Request: search.Request, RankedAnswer: answer})
		if m.SearchTurnOnOffered() {
			t.Errorf("%s: the button shows", name)
		}
		if opened, _ := press(m, "ctrl+o"); opened.EmbeddingsCard.Open {
			t.Errorf("%s: ctrl+o opened the card", name)
		}
	}
}

func TestCtrlOIsTheTurnOnBindingInTheSearchModes(t *testing.T) {
	m, cmds := wordsAnswered(t)
	m = statusRead(t, m, cmds, model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true})
	if b, ok := model.LookupBindingForMode("ctrl+o", m.CurrentMode()); !ok || b.Action != model.TurnOnSearchMeaning || !strings.Contains(b.Help, "turn on") {
		t.Fatalf("ctrl+o in %s: %+v (bound %v)", m.CurrentMode(), b, ok)
	}
}
