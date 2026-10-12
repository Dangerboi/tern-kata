package model_test

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"testing"
)

func TestFieldsInDetailAndRefresh(t *testing.T) {
	for _, key := range []string{"+", "-", "a", "shift+a", "!"} {
		m := model.Model{Focus: model.ColumnDetail, SelectedUID: "i", Issues: []model.Issue{{UID: "i", ProjectID: 3}}}
		m, cmds := model.Update(m, model.KeyMsg{Chord: key})
		if !m.FieldForm.Open {
			t.Fatalf("%s did not open in detail", key)
		}
		if key == "shift+a" {
			found := false
			for _, c := range cmds {
				if _, ok := c.(model.WriteField); ok {
					found = true
				}
			}
			if !found {
				t.Fatal("clear did not write")
			}
		}
		m, cmds = model.Update(m, model.FieldWritten{UID: "i"})
		if m.FieldForm.Open {
			t.Fatal("successful write retained form")
		}
		found := false
		for _, c := range cmds {
			if _, ok := c.(model.ReadSnapshot); ok {
				found = true
			}
		}
		if !found {
			t.Fatal("write did not refresh live snapshot")
		}
	}
}

func TestPriorityValuesAndFormIsolation(t *testing.T) {
	for _, value := range []string{"0", "1", "2", "3", "4", "-"} {
		m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}}
		m, _ = model.Update(m, model.KeyMsg{Chord: "!"})
		m, _ = model.Update(m, model.FieldValueMsg{Value: value})
		m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
		if !m.FieldForm.Saving {
			t.Fatalf("priority %s not saving", value)
		}
		found := false
		for _, c := range cmds {
			if c, ok := c.(model.WriteField); ok && c.Value == value {
				found = true
			}
		}
		if !found {
			t.Fatalf("priority %s missing write", value)
		}
		_, cmds = model.Update(m, model.KeyMsg{Chord: "enter"})
		for _, c := range cmds {
			if _, ok := c.(model.WriteField); ok {
				t.Fatal("duplicate write")
			}
		}
	}
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i", Labels: []string{"attached"}}, {UID: "other", Labels: []string{"elsewhere"}}}}
	m, _ = model.Update(m, model.KeyMsg{Chord: "-"})
	if got := m.FieldSuggestions(); len(got) != 1 || got[0] != "attached" {
		t.Fatalf("remove suggestions %v", got)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "q"})
	if m.Quitting || m.FieldForm.Value != "q" {
		t.Fatal("form leaked quit key")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if m.FieldForm.Open {
		t.Fatal("form not cancelled")
	}
	m.Stale = true
	m, cmds := model.Update(m, model.KeyMsg{Chord: "shift+a"})
	if m.FieldForm.Open {
		t.Fatal("stale write")
	}
	for _, c := range cmds {
		if _, ok := c.(model.WriteField); ok {
			t.Fatal("stale command")
		}
	}
}
