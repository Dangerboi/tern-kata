package model_test

import (
	"github.com/Dangerboi/tern-kata/model"
	"testing"
)

func TestCloseFormOpensAndQuickDone(t *testing.T) {
	m := model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Revision: 7}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "x"})
	if !m.CloseForm.Open {
		t.Fatal("close form did not open")
	}
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	var close model.CloseIssueCmd
	for _, cmd := range cmds {
		if c, ok := cmd.(model.CloseIssueCmd); ok {
			close = c
		}
	}
	if close.UID != "issue" || close.Revision != 7 || close.Body.Source == nil || *close.Body.Source != "tui" || close.Body.Reason == nil || *close.Body.Reason != "done" || close.Body.RetryProtocol == nil || *close.Body.RetryProtocol != "close-v1" || close.Body.Message != nil || len(close.Body.Evidence) != 0 || !m.CloseForm.Saving {
		t.Fatalf("quick done: %+v %+v", close, m.CloseForm)
	}
}

func TestCloseCardKeepsItsFieldWhileSaving(t *testing.T) {
	m := model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Revision: 7}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "x"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+d"})
	if !m.CloseForm.Saving {
		t.Fatal("close did not start saving")
	}
	field := m.CloseForm.Field
	for _, chord := range []string{"tab", "shift+tab"} {
		if m, _ = model.Update(m, model.KeyMsg{Chord: chord}); m.CloseForm.Field != field {
			t.Fatalf("%s moved the Close card's field from %d to %d while saving", chord, field, m.CloseForm.Field)
		}
	}
}
