package model_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// popupCard describes one card (or Search) for the shared behaviour test: how to open it, which
// field a click focuses, and how to read the text and the focused field back.
type popupCard struct {
	name      string
	open      func() model.Model
	fields    int // number of fields; focus cycling is checked when above 1
	textField int // a text field of the card
	field     func(model.Model) int
	text      func(model.Model, int) string
	// save submits the card; result answers it with an error. Nil when the card does not save.
	save   []string
	result func(model.Model, error) model.Msg
	// cleared is true when opening leaves the field empty already.
	cleared bool
}

func opened(keys ...string) func() model.Model {
	return func() model.Model {
		m, _ := clickPress(clickAt(model.ColumnList), keys...)
		return m
	}
}

// openedCreate opens Create on a bound Project, so a card with a title can be saved.
func openedCreate() model.Model {
	m, _ := model.Update(model.Model{Project: model.Project{ID: 7}}, model.KeyMsg{Chord: "n"})
	return m
}

func popupCards() []popupCard {
	return []popupCard{
		{
			name: "Close", open: opened("x"), fields: 7, textField: 2, cleared: true,
			field:  func(m model.Model) int { return m.CloseForm.Field },
			text:   func(m model.Model, f int) string { return m.CloseForm.Values[f] },
			save:   []string{"ctrl+d"},
			result: func(m model.Model, err error) model.Msg { return model.CloseResultMsg{UID: m.CloseForm.UID, Err: err} },
		},
		{
			name: "single-field form", open: opened("+"), fields: 1, cleared: true,
			field:  func(model.Model) int { return 0 },
			text:   func(m model.Model, _ int) string { return m.FieldForm.Value },
			save:   []string{"enter"},
			result: func(m model.Model, err error) model.Msg { return model.FieldWritten{UID: m.FieldForm.UID, Err: err} },
		},
		{
			name: "New Project", fields: 1,
			open: func() model.Model {
				m, _ := model.Update(clickAt(model.ColumnList), model.CollectionPointerSelected{Key: model.NewProjectKey})
				m, _ = clickPress(m, "ctrl+u")
				return m
			},
			field:  func(model.Model) int { return 0 },
			text:   func(m model.Model, _ int) string { return m.ProjectForm.Name },
			save:   []string{"enter"},
			result: func(_ model.Model, err error) model.Msg { return model.ProjectSaved{Err: err} },
		},
		{
			name: "Filter form", open: opened("f"), fields: 5, textField: 2, cleared: true,
			field: func(m model.Model) int { return m.FilterForm.Field },
			text: func(m model.Model, f int) string {
				if f == 3 {
					return m.FilterForm.Values.Label
				}
				return m.FilterForm.Values.Owner
			},
		},
		{
			name: "Menu", open: opened("p"), fields: 1, cleared: true,
			field: func(model.Model) int { return 0 },
			text:  func(m model.Model, _ int) string { return m.Picker.Filter },
		},
		{
			name: "Search", open: opened("/"), fields: 1, cleared: true,
			field: func(model.Model) int { return 0 },
			text:  func(m model.Model, _ int) string { return m.Filters.Search },
		},
		{
			name: "Create", open: openedCreate, fields: 5, textField: 0, cleared: true,
			field:  func(m model.Model) int { return m.Create.Field },
			text:   func(m model.Model, _ int) string { return m.Create.Title },
			save:   []string{"ctrl+o"},
			result: func(_ model.Model, err error) model.Msg { return model.IssueCreated{Err: err} },
		},
	}
}

func typeKeys(m model.Model, keys ...model.KeyMsg) model.Model {
	for _, key := range keys {
		m, _ = model.Update(m, key)
	}
	return m
}

func TestPopupCardSharedBehaviour(t *testing.T) {
	for _, c := range popupCards() {
		focused := func() model.Model {
			m := c.open()
			if m.CurrentMode() == model.ModeList {
				t.Fatalf("%s did not open", c.name)
			}
			m, _ = model.Update(m, model.FocusFieldMsg{Field: c.textField})
			return m
		}
		t.Run(c.name+"/typing", func(t *testing.T) {
			// q is a global binding (quit) and space a binding in some modes: both are text here.
			m := typeKeys(focused(), model.KeyMsg{Chord: "q"}, model.KeyMsg{Chord: "space"}, model.KeyMsg{Chord: "x", Text: "x"})
			if got := c.text(m, c.textField); got != "q x" || m.Quitting {
				t.Fatalf("typed %q quitting=%v", got, m.Quitting)
			}
		})
		t.Run(c.name+"/backspace by rune", func(t *testing.T) {
			for _, tt := range []struct{ typed, want string }{{"aé", "a"}, {"a😀", "a"}} {
				m := typeKeys(focused(), model.KeyMsg{Text: tt.typed}, model.KeyMsg{Chord: "backspace"})
				if got := c.text(m, c.textField); got != tt.want {
					t.Fatalf("%q then backspace = %q, want %q", tt.typed, got, tt.want)
				}
			}
		})
		t.Run(c.name+"/clear", func(t *testing.T) {
			m := typeKeys(focused(), model.KeyMsg{Text: "abc"}, model.KeyMsg{Chord: "ctrl+u"})
			if got := c.text(m, c.textField); got != "" {
				t.Fatalf("after ctrl+u: %q", got)
			}
		})
		t.Run(c.name+"/paste is flattened", func(t *testing.T) {
			m := typeKeys(focused(), model.KeyMsg{Text: "one\ntwo\r\nthree"})
			if got := c.text(m, c.textField); got != "one two  three" {
				t.Fatalf("pasted: %q", got)
			}
		})
		t.Run(c.name+"/esc after a click", func(t *testing.T) {
			m := focused()
			card := m.CurrentMode()
			if m, _ = clickPress(m, "escape"); m.CurrentMode() == card {
				t.Fatalf("esc left the card open in mode %s", card)
			}
		})
		t.Run(c.name+"/click focuses the field", func(t *testing.T) {
			if got := c.field(focused()); got != c.textField {
				t.Fatalf("focused field %d, want %d", got, c.textField)
			}
			m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: c.fields})
			if got := c.field(m); got >= c.fields {
				t.Fatalf("click past the last field focused %d", got)
			}
		})
		if c.fields > 1 {
			t.Run(c.name+"/field cycling wraps", func(t *testing.T) {
				m := c.open()
				m, _ = model.Update(m, model.FocusFieldMsg{Field: 0})
				for range c.fields {
					m, _ = clickPress(m, "tab")
				}
				if got := c.field(m); got != 0 {
					t.Fatalf("%d tabs from field 0 reached %d", c.fields, got)
				}
				if m, _ = clickPress(m, "shift+tab"); c.field(m) != c.fields-1 {
					t.Fatalf("shift+tab from field 0 reached %d", c.field(m))
				}
			})
		}
		if c.save != nil {
			t.Run(c.name+"/save error keeps the input", func(t *testing.T) {
				m := typeKeys(focused(), model.KeyMsg{Text: "keep"})
				mode := m.CurrentMode()
				m, cmds := clickPress(m, c.save...)
				if len(cmds) == 0 {
					t.Fatalf("saving asked the daemon for nothing: %+v", m)
				}
				m, _ = model.Update(m, c.result(m, errors.New("daemon said no")))
				if m.CurrentMode() != mode || c.text(m, c.textField) != "keep" {
					t.Fatalf("after a save error: mode %s text %q", m.CurrentMode(), c.text(m, c.textField))
				}
			})
		}
	}
}
