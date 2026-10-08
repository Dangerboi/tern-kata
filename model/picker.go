package model

import (
	"strings"
	"unicode/utf8"
)

const (
	ModePicker     Mode   = "picker"
	OpenPicker     Action = "open-picker"
	ClosePicker    Action = "close-picker"
	PickCollection Action = "pick-collection"
)

// PickerState is the Collections picker, a popup layer over the main columns.
// Cursor indexes PickerEntries.
type PickerState struct {
	Open   bool
	Filter string
	Cursor int
}

// PickerSelected is a click on a picker row; Key is Collection.Key.
type PickerSelected struct{ Key string }

func (PickerSelected) msg() {}

// Key names a Collection in rendered rows, matching Tern's node ids.
func (c Collection) Key() string {
	if c.View == "project" {
		return "project." + c.ProjectUID
	}
	return c.View
}

// PickerEntries are the sidebar's Collections with the bound Project pinned
// first, narrowed to titles containing the filter (any case).
func (m Model) PickerEntries() []Collection {
	filter := strings.ToLower(m.Picker.Filter)
	entries := make([]Collection, 0, len(m.Collections))
	pinned := func(c Collection) bool { return m.Bound && c.View == "project" && c.ProjectUID == m.Project.UID }
	for _, wantPinned := range []bool{true, false} {
		for _, c := range m.Collections {
			if pinned(c) == wantPinned && strings.Contains(strings.ToLower(c.Title), filter) {
				entries = append(entries, c)
			}
		}
	}
	return entries
}

// openPicker opens the picker over the list or detail; other layers keep the keys.
func openPicker(m Model) Model {
	if mode := m.CurrentMode(); mode == ModeList || mode == ModeDetail {
		m.Picker = PickerState{Open: true}
	}
	return m
}

// pickCollection switches to the entry under the cursor, as enter does in the sidebar.
func pickCollection(m Model) (Model, []Cmd) {
	entries := m.PickerEntries()
	if len(entries) == 0 {
		return m, nil
	}
	collection := entries[min(m.Picker.Cursor, len(entries)-1)]
	m.Picker = PickerState{}
	m.Focus = ColumnList
	return updateCollection(m, CollectionSelected{Collection: collection})
}

func pickCollectionKey(m Model, key string) (Model, []Cmd) {
	if m.Picker.Open {
		for i, collection := range m.PickerEntries() {
			if collection.Key() == key {
				m.Picker.Cursor = i
				return pickCollection(m)
			}
		}
	}
	return m, nil
}

func updatePicker(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, ok := LookupBindingForMode(msg.Chord, ModePicker)
	// Once the filter has text, single characters (j, k) belong to it.
	if ok && m.Picker.Filter != "" && utf8.RuneCountInString(binding.Chord) == 1 {
		ok = false
	}
	if !ok {
		return typePickerFilter(m, msg), nil
	}
	last := len(m.PickerEntries()) - 1
	switch binding.Action {
	case ClosePicker:
		m.Picker = PickerState{}
	case PickCollection:
		return pickCollection(m)
	case MoveDown:
		m.Picker.Cursor = max(0, min(m.Picker.Cursor+1, last))
	case MoveUp:
		m.Picker.Cursor = max(0, m.Picker.Cursor-1)
	case MoveFirst:
		m.Picker.Cursor = 0
	case MoveLast:
		m.Picker.Cursor = max(0, last)
	case DeleteInput:
		m.Picker.Filter, m.Picker.Cursor = deleteLastRune(m.Picker.Filter), 0
	case ClearInput:
		m.Picker.Filter, m.Picker.Cursor = "", 0
	case QuitAction:
		return updateNavigation(m, QuitAction)
	}
	return m, nil
}

func typePickerFilter(m Model, msg KeyMsg) Model {
	text := msg.Text
	if text == "" && utf8.RuneCountInString(msg.Chord) == 1 {
		text = msg.Chord
	}
	if msg.Chord == "space" {
		text = " "
	}
	if text = strings.NewReplacer("\r", " ", "\n", " ").Replace(text); text != "" {
		m.Picker.Filter += text
		m.Picker.Cursor = 0
	}
	return m
}
