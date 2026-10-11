package model

import (
	"strings"
	"unicode/utf8"
)

// FocusFieldMsg is the pointer putting focus on field Field of the open popup card, so the keys
// edit that field as they do after tab moves there.
type FocusFieldMsg struct{ Field int }

func (FocusFieldMsg) msg() {}

// cardFields is the field list of the open popup card that has several: Field is the focused
// index, Count the length of the list, and Locked says the card takes no focus change (it is
// saving or asking to discard).
type cardFields struct {
	Field  *int
	Count  int
	Locked bool
}

// openCardFields is the field list of the topmost open popup card with more than one field.
func openCardFields(m *Model) (cardFields, bool) {
	switch {
	case m.Create.Open:
		return cardFields{&m.Create.Field, createFieldCount, m.Create.Saving || m.Create.DiscardConfirm}, true
	case m.CloseForm.Open:
		return cardFields{&m.CloseForm.Field, len(m.CloseForm.Values), m.CloseForm.Saving}, true
	case m.FilterForm.Open:
		return cardFields{Field: &m.FilterForm.Field, Count: filterFieldCount}, true
	}
	return cardFields{}, false
}

// focusField puts focus on field of the open card; an index outside its field list is ignored.
func focusField(m Model, field int) Model {
	if fields, ok := openCardFields(&m); ok && !fields.Locked && field >= 0 && field < fields.Count {
		*fields.Field = field
	}
	return m
}

// cycleField moves focus step fields on, wrapping around the open card's field list.
func cycleField(m *Model, step int) {
	if fields, ok := openCardFields(m); ok {
		*fields.Field = ((*fields.Field+step)%fields.Count + fields.Count) % fields.Count
	}
}

// editText applies one key to a single-line text field. Backspace deletes a rune and ctrl+u clears
// the field, both through the card's DeleteInput and ClearInput bindings; a key bound to anything
// else is not text; a key with no binding types its character, a space, or pasted text with its
// line breaks flattened. It reports whether the field was edited.
func editText(value string, binding Binding, bound bool, msg KeyMsg) (string, bool) {
	switch {
	case bound && binding.Action == DeleteInput:
		return deleteLastRune(value), true
	case bound && binding.Action == ClearInput:
		return "", true
	case bound:
		return value, false
	}
	text := typedText(msg)
	return value + text, text != ""
}

func deleteLastRune(text string) string {
	_, size := utf8.DecodeLastRuneInString(text)
	return text[:len(text)-size]
}

// typedText is what a key adds to a text field: a bare character, a space, or
// pasted text with its line breaks flattened.
func typedText(msg KeyMsg) string {
	text := msg.Text
	switch {
	case msg.Chord == "space":
		text = " "
	case text == "" && utf8.RuneCountInString(msg.Chord) == 1:
		text = msg.Chord
	}
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
}
