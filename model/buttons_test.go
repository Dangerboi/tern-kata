package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// A card's button clicks its key: the click arrives as ActionMsg{key, chord}.
func TestCardButtonsClickLikeTheirKeys(t *testing.T) {
	issue := model.Model{Actor: "reviewer", SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Labels: []string{"bug"}}}}
	titled, _ := clickPress(model.Model{Project: model.Project{ID: 7}}, "n")
	titled, _ = model.Update(titled, model.KeyMsg{Text: "title"})
	asking, _ := clickPress(titled, "escape")
	draft, _ := clickPress(editorModel(), "e")
	typed, _ := model.Update(draft, model.KeyMsg{Chord: "paste", Text: "more"})
	asked, _ := clickPress(typed, "escape")
	for _, tt := range []struct {
		name   string
		start  model.Model
		chords []string
		check  func(model.Model) bool
	}{
		{"Create Issue with no title", mustPress(model.Model{Project: model.Project{ID: 7}}, "n"), []string{"ctrl+o"}, func(m model.Model) bool { return m.Create.Err != nil && m.Create.Open }},
		{"Create Issue", titled, []string{"ctrl+o"}, func(m model.Model) bool { return m.Create.Saving }},
		{"Create Cancel", titled, []string{"escape"}, func(m model.Model) bool { return m.Create.DiscardConfirm }},
		{"Create discard prompt", asking, []string{"y", "n"}, func(m model.Model) bool { return !m.Create.Open || !m.Create.DiscardConfirm }},
		{"field form Save without a value", mustPress(issue, "+"), []string{"enter"}, func(m model.Model) bool { return m.FieldForm.Err != nil }},
		{"field form Cancel", mustPress(issue, "+"), []string{"escape"}, func(m model.Model) bool { return !m.FieldForm.Open }},
		{"New Project with no name", mustPress(newProjectOpened(false), "ctrl+u"), []string{"enter"}, func(m model.Model) bool { return m.ProjectForm.Err != nil }},
		{"New Project Cancel", newProjectOpened(false), []string{"escape"}, func(m model.Model) bool { return !m.ProjectForm.Open }},
		{"Draft Save", draft, []string{"ctrl+o"}, func(m model.Model) bool { return m.Drafts["a"].Saving }},
		{"Draft $EDITOR", draft, []string{"ctrl+e"}, func(model.Model) bool { return true }},
		{"Draft Discard", typed, []string{"escape"}, func(m model.Model) bool { return m.Drafts["a"].DiscardConfirm }},
		{"Draft discard prompt", asked, []string{"y", "n"}, func(model.Model) bool { return true }},
		{"Close Issue with no message", mustPress(issue, "x"), []string{"ctrl+o"}, func(m model.Model) bool { return m.CloseForm.Err != nil }},
		{"Close Quick done", mustPress(issue, "x"), []string{"ctrl+d"}, func(m model.Model) bool { return m.CloseForm.Pending }},
		{"Close Cancel", mustPress(issue, "x"), []string{"escape"}, func(m model.Model) bool { return !m.CloseForm.Open }},
		{"Filters Apply", mustPress(issue, "f"), []string{"ctrl+o"}, func(m model.Model) bool { return !m.FilterForm.Open }},
		{"Filters Cancel", mustPress(issue, "f"), []string{"escape"}, func(m model.Model) bool { return !m.FilterForm.Open }},
	} {
		for _, chord := range tt.chords {
			t.Run(tt.name+"/"+chord, func(t *testing.T) {
				keyboard, keyCmds := model.Update(tt.start, model.KeyMsg{Chord: chord})
				pointer, clickCmds := model.Update(tt.start, model.ActionMsg{Action: "key", Value: chord})
				if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(clickCmds, keyCmds) {
					t.Fatalf("click and key diverge (click != key):%s\ncommands: %#v != %#v", clickDiff(pointer, keyboard), clickCmds, keyCmds)
				}
				if len(tt.chords) == 1 && !tt.check(pointer) {
					t.Fatalf("the click did not do what the button says: %+v", pointer)
				}
			})
		}
	}
}

func mustPress(m model.Model, chords ...string) model.Model {
	m, _ = clickPress(m, chords...)
	return m
}
