package model_test

import (
	"errors"
	"github.com/Dangerboi/tern-kata/internal/model"
	"testing"
)

func TestQuickFieldCommands(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		action     model.Action
	}{
		{"+", "bug", model.AddLabelAction}, {"-", "bug", model.RemoveLabelAction}, {"a", "alice", model.AssignOwnerAction}, {"shift+a", "", model.ClearOwnerAction}, {"!", "4", model.PriorityAction}, {"!", "-", model.PriorityAction},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			m := model.Model{Actor: "reviewer", SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Labels: []string{"bug"}}}}
			m, cmds := model.Update(m, model.KeyMsg{Chord: tc.key})
			if tc.key != "shift+a" {
				m, _ = model.Update(m, model.FieldValueMsg{Value: tc.value})
				m, cmds = model.Update(m, model.KeyMsg{Chord: "enter"})
			}
			found := false
			for _, c := range cmds {
				if c, ok := c.(model.WriteField); ok {
					found = true
					if c.Action != tc.action || c.Actor != "reviewer" || c.Value != tc.value {
						t.Fatalf("command: %#v", c)
					}
				}
			}
			if !found {
				t.Fatal("missing write command")
			}
			m, _ = model.Update(m, model.FieldWritten{UID: "issue", Err: errors.New("permission denied\nask owner")})
			if m.FieldForm.Err == nil || m.FieldForm.Err.Error() != "permission denied\nask owner" {
				t.Fatal("rejection not readable")
			}
		})
	}
}

func TestSetParentFieldStripsTheHashAndRequiresAValue(t *testing.T) {
	for _, focus := range []model.Column{model.ColumnList, model.ColumnDetail} {
		m := model.Model{Actor: "reviewer", Focus: focus, SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2}}}
		m, cmds := model.Update(m, model.KeyMsg{Chord: "p"})
		if !m.FieldForm.Open || m.FieldForm.Action != model.SetParentAction {
			t.Fatalf("focus %v: p opened %+v cmds %#v", focus, m.FieldForm, cmds)
		}
		empty, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
		if len(cmds) != 0 || empty.FieldForm.Err == nil || empty.FieldForm.Err.Error() != "Enter a value." {
			t.Fatalf("focus %v: empty submit err=%v cmds=%#v", focus, empty.FieldForm.Err, cmds)
		}
		m, _ = model.Update(m, model.FieldValueMsg{Value: "#abcd"})
		_, cmds = model.Update(m, model.KeyMsg{Chord: "enter"})
		if want := (model.WriteField{UID: "issue", ProjectID: 2, Action: model.SetParentAction, Value: "abcd", Actor: "reviewer"}); len(cmds) != 1 || cmds[0] != want {
			t.Fatalf("focus %v: submit %#v, want %#v", focus, cmds, want)
		}
	}
}

func TestLabelCompletionAndPriorityBounds(t *testing.T) {
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i", Labels: []string{"bug", "build"}}, {UID: "j", Labels: []string{"other"}}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "+"})
	m, _ = model.Update(m, model.FieldLabelsLoaded{UID: "i", Labels: []string{"bug", "build", "other"}})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "bu"})
	if got := m.FieldSuggestions(); len(got) != 2 || got[0] != "bug" {
		t.Fatalf("suggestions: %v", got)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "tab"})
	if m.FieldForm.Value != "bug" {
		t.Fatal("completion not applied")
	}
	for _, value := range []string{"-1", "5", "bad"} {
		p := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}}
		p, _ = model.Update(p, model.KeyMsg{Chord: "!"})
		p, _ = model.Update(p, model.FieldValueMsg{Value: value})
		p, cmds := model.Update(p, model.KeyMsg{Chord: "enter"})
		for _, c := range cmds {
			if _, ok := c.(model.WriteField); ok {
				t.Fatalf("invalid priority %q wrote", value)
			}
		}
		if p.FieldForm.Err == nil {
			t.Fatalf("invalid priority %q unreported", value)
		}
	}
}

func TestSetPriorityIsAPickerThatOpensOnTheIssuesPriority(t *testing.T) {
	two := int64(2)
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i", Priority: &two}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "!"})
	set, ok := m.CardChoices(model.CardField, model.FieldPriority)
	if !ok || set.Chosen != 3 || set.Labels[0] != "None" {
		t.Fatalf("picker %+v ok %v, want P2 chosen among None and P0 to P4", set, ok)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	m, _ = model.Update(m, model.ChooseMsg{Field: model.FieldPriority, Index: 0})
	if m.FieldForm.Value != "-" {
		t.Fatalf("choosing None holds %q, want the clearing dash", m.FieldForm.Value)
	}
	_, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	for _, c := range cmds {
		if w, ok := c.(model.WriteField); ok && w.Value == "-" && w.Action == model.PriorityAction {
			return
		}
	}
	t.Fatalf("enter wrote %#v, want the priority cleared", cmds)
}

func TestThePriorityPickersShowEveryChoiceInARowNotInAClippedMenu(t *testing.T) {
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}}
	set, _ := model.Update(m, model.KeyMsg{Chord: "!"})
	field, _ := set.CardChoices(model.CardField, model.FieldPriority)
	create, _ := model.Update(m, model.KeyMsg{Chord: "n"})
	created, _ := create.CardChoices(model.CardCreate, model.CreatePriority)
	for name, got := range map[string]model.ChoiceSet{"Set priority": field, "Create": created} {
		if got.Control != model.ControlTabs || len(got.Labels) != 6 {
			t.Errorf("%s picker: control %v with %d choices, want a tabs row of None and P0 to P4", name, got.Control, len(got.Labels))
		}
	}
}
