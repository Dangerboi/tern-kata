package driver_test

import (
	"encoding/json"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// drawnCursor is the caret Tern is sent for the focused text field, -1 when the node carries none.
func drawnCursor(t *testing.T, out driver.Output) int {
	t.Helper()
	for _, node := range wireNodes(t, *out.View) {
		if node.ID == out.State.Focus {
			if c, ok := node.P["cursor"]; ok {
				n, err := c.(json.Number).Int64()
				if err != nil {
					t.Fatal(err)
				}
				return int(n)
			}
			return -1
		}
	}
	t.Fatalf("focused node %q is not drawn", out.State.Focus)
	return -1
}

// New Project's name field takes every editing rule once; the other cards share the same function.
func TestTextFieldEditingRules(t *testing.T) {
	// the field opens prefilled, so each case starts from a cleared one
	open := feed(t, browsing(t, 160), model.CollectionPointerSelected{Key: model.NewProjectKey}, key("ctrl+u"))
	tests := []struct {
		name   string
		keys   []tern.Key
		text   string
		cursor int
	}{
		{"typing appends and the caret follows", []tern.Key{key("a"), key("b"), key("c")}, "abc", 3},
		{"left then typing inserts", []tern.Key{key("a"), key("c"), key("left"), key("b")}, "abc", 2},
		{"backspace removes before the caret", []tern.Key{key("a"), key("b"), key("c"), key("left"), key("backspace")}, "ac", 1},
		{"delete removes after the caret", []tern.Key{key("a"), key("b"), key("c"), key("left"), key("left"), key("delete")}, "ac", 1},
		{"delete at the end does nothing", []tern.Key{key("a"), key("delete")}, "a", 1},
		{"backspace at the start does nothing", []tern.Key{key("a"), key("home"), key("backspace")}, "a", 0},
		{"home and end", []tern.Key{key("a"), key("b"), key("home"), key("x"), key("end"), key("y")}, "xaby", 4},
		{"ctrl+a and ctrl+e", []tern.Key{key("a"), key("b"), key("ctrl+a"), key("x"), key("ctrl+e"), key("y")}, "xaby", 4},
		{"right stops at the end", []tern.Key{key("a"), key("left"), key("right"), key("right"), key("b")}, "ab", 2},
		{"left stops at the start", []tern.Key{key("a"), key("left"), key("left"), key("b")}, "ba", 1},
		{"ctrl+u clears", []tern.Key{key("a"), key("b"), key("left"), key("ctrl+u"), key("c")}, "c", 1},
		{"pasted text goes in at the caret", []tern.Key{key("a"), key("b"), key("left"), {Name: "paste", Text: "x\ny"}}, "ax yb", 4},
		{"a character past the BMP counts two UTF-16 units", []tern.Key{{Name: "paste", Text: "a😀b"}, key("left")}, "a😀b", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := open
			for _, k := range tt.keys {
				out = step(t, out.State, k)
			}
			if got := out.State.Model.ProjectForm.Name; got != tt.text {
				t.Errorf("name %q, want %q", got, tt.text)
			}
			if got := drawnCursor(t, out); got != tt.cursor {
				t.Errorf("drawn caret %d, want %d", got, tt.cursor)
			}
		})
	}
}

// Every card with a text field edits at the caret; a card's own binding of a caret key keeps its job where the field
// does not use the key.
func TestEveryTextFieldEditsAtTheCaret(t *testing.T) {
	for _, card := range popups() {
		if card.name == "Draft editor" {
			continue // a multi-line body with its own editor keys
		}
		t.Run(card.name, func(t *testing.T) {
			s := driver.State{Model: card.open(t, browsing(t, 160))}
			s = step(t, s, model.ResizeMsg{Cols: 160}).State
			before := card.typed(s.Model)
			out := feed(t, s, key("a"), key("b"), key("c"), key("left"), key("left"), key("x"))
			if got, want := card.typed(out.State.Model), before+"axbc"; got != want {
				t.Fatalf("text %q after a, b, c, left, left, x; want %q", got, want)
			}
			if got := drawnCursor(t, out); got != len([]rune(before))+2 {
				t.Errorf("drawn caret %d, want %d", got, len([]rune(before))+2)
			}
			out = feed(t, out.State, key("delete"), key("backspace"))
			if got, want := card.typed(out.State.Model), before+"ac"; got != want {
				t.Fatalf("text %q after delete and backspace; want %q", got, want)
			}
		})
	}
}

func TestSearchEditsAtTheCaret(t *testing.T) {
	out := feed(t, browsing(t, 160), key("/"), key("a"), key("c"), key("left"), key("b"))
	if got := out.State.Model.Filters.Search; got != "abc" {
		t.Fatalf("query %q, want abc", got)
	}
	if got := drawnCursor(t, out); got != 2 {
		t.Errorf("drawn caret %d, want 2", got)
	}
	out = feed(t, out.State, key("backspace"), key("delete"))
	if got := out.State.Model.Filters.Search; got != "a" {
		t.Fatalf("query %q after backspace and delete, want a", got)
	}
	out = feed(t, out.State, key("ctrl+u"))
	if got := out.State.Model.Filters.Search; got != "" {
		t.Fatalf("ctrl+u left %q", got)
	}
}

// The Menu keeps Home and End for its entries; the Filter form keeps ← and → for its choice fields.
func TestCaretKeysKeepTheirOtherJobs(t *testing.T) {
	menu := feed(t, browsing(t, 160), openMenu, key("down"), key("down"))
	if menu.State.Model.Picker.Cursor != 2 {
		t.Fatalf("Menu cursor %d after two downs", menu.State.Model.Picker.Cursor)
	}
	if got := step(t, menu.State, key("home")).State.Model.Picker.Cursor; got != 0 {
		t.Errorf("Home moved the Menu cursor to %d, want 0", got)
	}
	form := feed(t, browsing(t, 160), key("f"))
	form.State.Model.FilterForm.Field = model.FilterStatus
	if got := step(t, form.State, key("right")).State.Model.FilterForm.Values.Status; got != "open" {
		t.Errorf("→ on the status choice gave %q, want open", got)
	}
}

// Backspace with nothing before the caret is no edit: the selection stays and no snapshot is read.
func TestSearchBackspaceAtTheStartKeepsTheSelection(t *testing.T) {
	s := feed(t, browsing(t, 160), key("/"), key("a"), key("c"), key("home")).State
	s.Model.SelectedUID, s.Model.Refreshing = "alpha", false
	out := step(t, s, key("backspace"))
	if readsSnapshot(out) || out.State.Model.SelectedUID != "alpha" || out.State.Model.Filters.Search != "ac" {
		t.Errorf("cmds %v, selected %q, query %q; want no read, alpha, ac", out.Cmds, out.State.Model.SelectedUID, out.State.Model.Filters.Search)
	}
	s.Model.Search.Caret = model.Caret{Back: 1}
	out = step(t, s, key("backspace"))
	if !readsSnapshot(out) || out.State.Model.Filters.Search != "c" {
		t.Errorf("mid-text backspace: cmds %v, query %q; want a read for c", out.Cmds, out.State.Model.Filters.Search)
	}
}

func readsSnapshot(out driver.Output) bool {
	for _, c := range out.Cmds {
		if _, ok := c.(model.ReadSnapshot); ok {
			return true
		}
	}
	return false
}
