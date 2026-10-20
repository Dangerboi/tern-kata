package model

import (
	"slices"
	"unicode/utf8"
)

// Field names one field of a multi-field popup card; a card lists its fields in tab order.
type Field string

// FieldSpec is one field of a popup card: Name addresses it and names its node, and Text says it
// takes typed text.
type FieldSpec struct {
	Name Field
	Text bool
}

// FocusFieldMsg is the pointer putting focus on the field called Field of the open popup card, so
// the keys edit that field as they do after tab moves there.
type FocusFieldMsg struct{ Field Field }

func (FocusFieldMsg) msg() {}

// ChooseMsg is the pointer choosing the choice at Index, in the order the card shows them, of the choice field called Field of
// the open popup card. The field takes focus, as after FocusFieldMsg, and holds that choice.
type ChooseMsg struct {
	Field Field
	Index int
}

func (ChooseMsg) msg() {}

// Card names a popup card.
type Card int

const (
	NoCard Card = iota
	CardCreate
	CardDraft
	CardField
	CardClose
	CardRecurrence
	CardProject
	CardLanes
	CardFilter
	CardPicker
)

// cardEntry is everything the model knows about one popup card. Each card fills in one.
type cardEntry struct {
	card Card
	open func(Model) bool
	// modes is every keymap mode the card can be in; mode picks the current one from its state.
	modes []Mode
	mode  func(Model) Mode
	// chords lists the global chords the card lets through to their global binding; every other
	// global chord is the card's to swallow or type. ctrl+c quits from the Menu, Close and New
	// Project cards and is swallowed by the others.
	chords []string
	key    func(Model, KeyMsg) (Model, []Cmd)
	// fields is the card's field list in tab order; a card without fields declares an empty,
	// non-nil list. focus finds the focused field and locked says the card takes no focus change
	// (it is saving or asking to discard); both are nil for a card without fields.
	fields []FieldSpec
	focus  func(*Model) *Field
	locked func(Model) bool
	// hidden says a field of the list does not apply to the card as it stands: tab skips it. Nil when every field always applies.
	hidden func(Model, Field) bool
	// caret finds the field holding the text caret, false when the card holds none; a card without
	// fields has one input and names it "".
	caret func(Model) (Field, bool)
	// target names the card's multi-line body for native edits, and edit applies one to it; both
	// are zero for a card without a body.
	target EditTarget
	edit   func(Model, func(TextEditor) TextEditor) Model
	// choose gives the focused choice field its choice at an index; nil for a card without choice fields.
	choose func(Model, int) Model
}

// cards lists every popup card in the one order that decides which is on top. It is filled in by
// init because the cards' key handlers read it back through cycleField.
var cards []cardEntry

func init() {
	cards = []cardEntry{
		{
			card: CardCreate, open: func(m Model) bool { return m.Create.Open },
			modes: []Mode{ModeCreate, ModeDiscard},
			mode: func(m Model) Mode {
				if m.Create.DiscardConfirm {
					return ModeDiscard
				}
				return ModeCreate
			},
			key:    createCard.key,
			fields: CreateFields, focus: func(m *Model) *Field { return &m.Create.Field },
			locked: func(m Model) bool { return m.Create.Saving || m.Create.DiscardConfirm },
			caret:  func(m Model) (Field, bool) { return m.Create.Field, !(m.Create.Saving || m.Create.DiscardConfirm) },
			target: EditCreateBody,
			edit: func(m Model, apply func(TextEditor) TextEditor) Model {
				if m.Create.Draft.editable() {
					m.Create.Editor = apply(m.Create.Editor)
				}
				return m
			},
		},
		{
			card: CardDraft, open: func(m Model) bool { _, ok := m.ActiveDraft(); return ok },
			modes:  []Mode{ModeEditor, ModeDiscard, ModeConflict},
			mode:   Model.EditorMode,
			key:    draftCard.key,
			fields: []FieldSpec{},
			caret: func(m Model) (Field, bool) {
				draft, _ := m.ActiveDraft()
				return "", m.EditorMode() == ModeEditor && !draft.Saving
			},
			target: EditDraftBody,
			edit: func(m Model, apply func(TextEditor) TextEditor) Model {
				if draft, _ := m.ActiveDraft(); draft.editable() {
					draft.Editor = apply(draft.Editor)
					m = putDraft(m, m.EditorUID, draft)
				}
				return m
			},
		},
		{
			card: CardField, open: func(m Model) bool { return m.FieldForm.Open },
			modes: []Mode{ModeFieldForm}, mode: func(Model) Mode { return ModeFieldForm },
			key: updateFieldKey, fields: []FieldSpec{},
			caret: func(m Model) (Field, bool) { return "", !m.FieldForm.Saving },
		},
		{
			card: CardClose, open: func(m Model) bool { return m.CloseForm.Open },
			modes: []Mode{ModeCloseForm}, mode: func(Model) Mode { return ModeCloseForm },
			chords: []string{"ctrl+c"},
			key:    closeKey,
			fields: CloseFields, focus: func(m *Model) *Field { return &m.CloseForm.Field },
			locked: func(m Model) bool { return m.CloseForm.Saving },
			caret:  func(m Model) (Field, bool) { return m.CloseForm.Field, !m.CloseForm.Saving },
		},
		{
			card: CardRecurrence, open: func(m Model) bool { return m.RecurrenceForm.Open },
			modes: []Mode{ModeRecurrenceForm, ModeRecurrenceStop},
			mode: func(m Model) Mode {
				if m.RecurrenceForm.StopConfirm {
					return ModeRecurrenceStop
				}
				return ModeRecurrenceForm
			},
			chords: []string{"ctrl+c"},
			key:    recurrenceKey,
			fields: RecurrenceFields, focus: func(m *Model) *Field { return &m.RecurrenceForm.Field },
			locked: func(m Model) bool { return m.RecurrenceForm.Saving || m.RecurrenceForm.StopConfirm },
			hidden: func(m Model, name Field) bool { return !m.RecurrenceForm.Shows(name) },
			choose: func(m Model, i int) Model { m.RecurrenceForm.choose(i); return m },
			caret: func(m Model) (Field, bool) {
				f := m.RecurrenceForm
				return f.Field, !f.Saving && !f.StopConfirm && takesText(RecurrenceFields, f.Field)
			},
		},
		{
			card: CardProject, open: func(m Model) bool { return m.ProjectForm.Open },
			modes: []Mode{ModeProjectForm}, mode: func(Model) Mode { return ModeProjectForm },
			chords: []string{"ctrl+c"},
			key:    updateProjectFormKey, fields: []FieldSpec{},
			caret: func(m Model) (Field, bool) { return "", !m.ProjectForm.Saving },
		},
		{
			card: CardLanes, open: func(m Model) bool { return m.LanesForm.Open },
			modes: []Mode{ModeLanesForm}, mode: func(Model) Mode { return ModeLanesForm },
			key: updateLanesFormKey, fields: []FieldSpec{},
			caret: func(Model) (Field, bool) { return "", true },
		},
		{
			card: CardFilter, open: func(m Model) bool { return m.FilterForm.Open },
			modes: []Mode{ModeFilterForm}, mode: func(Model) Mode { return ModeFilterForm },
			key:    filterFormKey,
			fields: FilterFields, focus: func(m *Model) *Field { return &m.FilterForm.Field },
			locked: func(Model) bool { return false },
			caret:  func(m Model) (Field, bool) { return m.FilterForm.Field, takesText(FilterFields, m.FilterForm.Field) },
			choose: chooseFilter,
		},
		{
			card: CardPicker, open: func(m Model) bool { return m.Picker.Open },
			modes: []Mode{ModePicker}, mode: func(Model) Mode { return ModePicker },
			chords: []string{"ctrl+c"},
			key:    updatePicker, fields: []FieldSpec{},
			caret: func(Model) (Field, bool) { return "", true },
		},
	}
}

// Cards lists every popup card in the order TopCard tries them.
func Cards() []Card {
	list := make([]Card, len(cards))
	for i, entry := range cards {
		list[i] = entry.card
	}
	return list
}

// CardFields is the field list a popup card declares, in tab order. Declared is false for a card
// that does not declare one; a card without fields declares an empty list.
func CardFields(card Card) (fields []FieldSpec, declared bool) {
	for _, entry := range cards {
		if entry.card == card {
			return entry.fields, entry.fields != nil
		}
	}
	return nil, false
}

// topEntry is the entry of the card on top, false when none is.
func (m Model) topEntry() (cardEntry, bool) {
	if m.HelpOpen {
		return cardEntry{}, false
	}
	for _, entry := range cards {
		if entry.open(m) {
			return entry, true
		}
	}
	return cardEntry{}, false
}

// TopCard is the one popup card that takes the keys, is drawn on top and holds the text caret: the
// first open card in the order of cards. The help sheet covers every card, so none is on top while
// it is open. The mode, key dispatch, the view and the Pane driver all read this one order.
func (m Model) TopCard() Card {
	entry, _ := m.topEntry()
	return entry.card
}

// TextCaret is the popup card on top and the field of it that holds the text caret; ok is false when
// no card is on top or the card holds no caret, as while it saves.
func (m Model) TextCaret() (card Card, field Field, ok bool) {
	entry, open := m.topEntry()
	if !open {
		return NoCard, "", false
	}
	field, ok = entry.caret(m)
	return entry.card, field, ok
}

// cardMode is the keymap mode of the card on top, "" when there is none.
func (m Model) cardMode() Mode {
	if entry, ok := m.topEntry(); ok {
		return entry.mode(m)
	}
	return ""
}

// cardLetsThrough reports whether mode belongs to a popup card and, if so, whether the card lets
// chord through to its global binding.
func cardLetsThrough(mode Mode, chord string) (isCard, through bool) {
	for _, entry := range cards {
		if slices.Contains(entry.modes, mode) {
			return true, slices.Contains(entry.chords, chord)
		}
	}
	return false, false
}

// updateTopCardKey gives a key to the card on top; it reports false when no card is open.
func updateTopCardKey(m Model, msg KeyMsg) (Model, []Cmd, bool) {
	entry, ok := m.topEntry()
	if !ok || slices.Contains(entry.chords, msg.Chord) {
		return m, nil, false
	}
	m, cmds := entry.key(m, msg)
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
	if entry, ok := m.topEntry(); ok && target != NoEditTarget && entry.target == target {
		return entry.edit(m, apply), true
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

// draftCardKeys is what the Create card and the Draft editor each supply to the one key handler they
// share: where the Draft lives, what saving sends and which keys and edits are the card's own.
type draftCardKeys struct {
	draft func(Model) Draft
	set   func(Model, Draft) Model
	// close removes the card and its Draft.
	close func(Model) Model
	save  func(Model) (Model, []Cmd)
	// empty says the card holds nothing worth asking to discard.
	empty func(Model) bool
	// own handles a bound key only this card has; it reports false for any other.
	own func(Model, Binding, KeyMsg) (Model, []Cmd, bool)
	// edit applies a key that no card action took to the card's text.
	edit func(Model, Binding, bool, KeyMsg) Model
}

// key is one key for a card holding a Draft. While saving it does nothing, before any binding is
// looked up; an edit clears the card's error.
func (c draftCardKeys) key(m Model, msg KeyMsg) (Model, []Cmd) {
	d := c.draft(m)
	if d.Saving {
		return m, nil
	}
	binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode())
	switch {
	case d.Conflict != nil:
		if ok && binding.Action == OverwriteDraft {
			d.BaseRevision, d.BaseText = d.Conflict.Revision, d.Conflict.Body
			d.Conflict = nil
			return c.save(c.set(m, d))
		}
		if ok && binding.Action == DiscardDraft {
			return c.close(m), nil
		}
		return m, nil
	case d.DiscardConfirm:
		d, gone := answerDiscard(d, binding, ok)
		if gone {
			return c.close(m), nil
		}
		return c.set(m, d), nil
	}
	if ok {
		switch binding.Action {
		case SaveDraftAction:
			return c.save(m)
		case CancelDraft:
			d, gone := leaveDraft(d, c.empty(m))
			if gone {
				return c.close(m), nil
			}
			return c.set(m, d), nil
		}
		if m, cmds, handled := c.own(m, binding, msg); handled {
			return m, cmds
		}
	}
	m = c.edit(m, binding, ok, msg)
	d = c.draft(m)
	d.Err = nil
	return c.set(m, d), nil
}

// editorKey is one key for a multi-line body: a bound editor action, or typed text.
func editorKey(e TextEditor, binding Binding, bound bool, msg KeyMsg) TextEditor {
	if bound {
		return e.Key(binding.Action, editorSelectionChord(msg.Chord))
	}
	if msg.Text != "" {
		return e.Insert(msg.Text)
	}
	return e
}

// focusField puts focus on the field called name of the open card; a name outside its field list is ignored.
func focusField(m Model, name Field) Model {
	if entry, ok := m.topEntry(); ok && entry.focus != nil && !entry.locked(m) && takesFocus(entry.fields, name) && (entry.hidden == nil || !entry.hidden(m, name)) {
		*entry.focus(&m) = name
	}
	return m
}

// chooseField focuses the choice field called name of the open card and gives it its choice at index; a text field, a
// field the card hides or a locked card changes nothing.
func chooseField(m Model, name Field, index int) Model {
	m = focusField(m, name)
	if entry, ok := m.topEntry(); ok && entry.choose != nil && !entry.locked(m) && *entry.focus(&m) == name && !takesText(entry.fields, name) {
		m = entry.choose(m, index)
	}
	return m
}

func takesFocus(list []FieldSpec, name Field) bool {
	return slices.ContainsFunc(list, func(f FieldSpec) bool { return f.Name == name })
}

// takesText reports whether the field called name in list takes typed text.
func takesText(list []FieldSpec, name Field) bool {
	return slices.ContainsFunc(list, func(f FieldSpec) bool { return f.Name == name && f.Text })
}

// cycleField moves focus step fields on, wrapping around the open card's field list and skipping the fields it hides.
func cycleField(m *Model, step int) {
	if entry, ok := m.topEntry(); ok && entry.focus != nil && !entry.locked(*m) {
		field := entry.focus(m)
		n := len(entry.fields)
		at := max(slices.IndexFunc(entry.fields, func(f FieldSpec) bool { return f.Name == *field }), 0)
		for i := 1; i <= n; i++ {
			next := entry.fields[((at+step*i)%n+n)%n].Name
			if entry.hidden == nil || !entry.hidden(*m, next) {
				*field = next
				return
			}
		}
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
	return singleLineText(text)
}
