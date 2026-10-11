package model

import (
	"strings"
	"unicode/utf8"
)

// FocusFieldMsg is the pointer putting focus on field Field of the open popup card, so the keys
// edit that field as they do after tab moves there.
type FocusFieldMsg struct{ Field int }

func (FocusFieldMsg) msg() {}

// Card names a popup card.
type Card int

const (
	NoCard Card = iota
	CardCreate
	CardDraft
	CardField
	CardClose
	CardProject
	CardFilter
	CardPicker
)

// TopCard is the one popup card that takes the keys, is drawn on top and holds the text caret: the
// first open card in this order. The help sheet covers every card, so none is on top while it is
// open. The mode, key dispatch, the view and the Pane driver all read this one order.
func (m Model) TopCard() Card {
	_, draft := m.ActiveDraft()
	switch {
	case m.HelpOpen:
		return NoCard
	case m.Create.Open:
		return CardCreate
	case draft:
		return CardDraft
	case m.FieldForm.Open:
		return CardField
	case m.CloseForm.Open:
		return CardClose
	case m.ProjectForm.Open:
		return CardProject
	case m.FilterForm.Open:
		return CardFilter
	case m.Picker.Open:
		return CardPicker
	}
	return NoCard
}

// cardMode is the keymap mode of the card on top, "" when there is none.
func (m Model) cardMode() Mode {
	switch m.TopCard() {
	case CardCreate:
		if m.Create.DiscardConfirm {
			return ModeDiscard
		}
		return ModeCreate
	case CardDraft:
		return m.EditorMode()
	case CardField:
		return ModeFieldForm
	case CardClose:
		return ModeCloseForm
	case CardProject:
		return ModeProjectForm
	case CardFilter:
		return ModeFilterForm
	case CardPicker:
		return ModePicker
	}
	return ""
}

// cardGlobalChords lists, for each mode a popup card owns, the global chords it lets through to
// their global binding; every other global chord is the card's to swallow or type. ctrl+c quits
// from the Menu, Close and New Project cards and is swallowed by the others.
var cardGlobalChords = map[Mode][]string{
	ModeCreate:      nil,
	ModeDiscard:     nil,
	ModeEditor:      nil,
	ModeConflict:    nil,
	ModeFieldForm:   nil,
	ModeFilterForm:  nil,
	ModeCloseForm:   {"ctrl+c"},
	ModeProjectForm: {"ctrl+c"},
	ModePicker:      {"ctrl+c"},
}

// updateTopCardKey gives a key to the card on top; it reports false when no card is open.
func updateTopCardKey(m Model, msg KeyMsg) (Model, []Cmd, bool) {
	var cmds []Cmd
	switch m.TopCard() {
	case CardCreate:
		m, cmds = createKey(m, msg)
	case CardDraft:
		m, cmds = draftKey(m, msg)
	case CardField:
		m, cmds = updateFieldKey(m, msg)
	case CardClose:
		m, cmds = closeKey(m, msg)
	case CardProject:
		m, cmds = updateProjectFormKey(m, msg)
	case CardFilter:
		m, cmds = filterFormKey(m, msg)
	case CardPicker:
		m, cmds = updatePicker(m, msg)
	default:
		return m, nil, false
	}
	return m, cmds, true
}

// EditTarget names the multi-line body a native edit or undo is for. The Pane driver maps a Tern
// editor node to it, so the model never names a node id.
type EditTarget int

const (
	NoEditTarget EditTarget = iota
	EditCreateBody
	EditDraftBody
)

// editBody applies a native edit or undo to target, if the card on top owns it. It reports whether
// the card took the message, which it does even when the Draft is read-only.
func editBody(m Model, target EditTarget, apply func(TextEditor) TextEditor) (Model, bool) {
	switch {
	case target == EditCreateBody && m.TopCard() == CardCreate:
		if m.Create.Draft.editable() {
			m.Create.Editor = apply(m.Create.Editor)
		}
		return m, true
	case target == EditDraftBody && m.TopCard() == CardDraft:
		if draft, _ := m.ActiveDraft(); draft.editable() {
			draft.Editor = apply(draft.Editor)
			m = putDraft(m, m.EditorUID, draft)
		}
		return m, true
	}
	return m, false
}

// editable says the Draft takes edits: it is not saving, asking to discard or in conflict.
func (d Draft) editable() bool { return !d.Saving && !d.DiscardConfirm && d.Conflict == nil }

// leaveDraft is esc in a card holding a Draft: it leaves at once when the Draft is empty and
// otherwise asks whether to discard it.
func leaveDraft(d Draft, empty bool) (Draft, bool) {
	if empty {
		return d, true
	}
	d.DiscardConfirm = true
	return d, false
}

// answerDiscard is one key while the card asks whether to discard its Draft: the discard binding
// leaves, the keep binding goes back to editing, and any other key is swallowed.
func answerDiscard(d Draft, binding Binding, bound bool) (Draft, bool) {
	switch {
	case bound && binding.Action == DiscardDraft:
		return d, true
	case bound && binding.Action == KeepDraft:
		d.DiscardConfirm = false
	}
	return d, false
}

// cardFields is the field list of the open popup card that has several: Field is the focused
// index, Count the length of the list, and Locked says the card takes no focus change (it is
// saving or asking to discard).
type cardFields struct {
	Field  *int
	Count  int
	Locked bool
}

// openCardFields is the field list of the card on top, when it has more than one field.
func openCardFields(m *Model) (cardFields, bool) {
	switch m.TopCard() {
	case CardCreate:
		return cardFields{&m.Create.Field, createFieldCount, m.Create.Saving || m.Create.DiscardConfirm}, true
	case CardClose:
		return cardFields{&m.CloseForm.Field, len(m.CloseForm.Values), m.CloseForm.Saving}, true
	case CardFilter:
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
