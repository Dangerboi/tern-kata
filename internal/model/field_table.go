package model

import "slices"

// fieldTable is a form card's fields declared once: which of them take text, which are choices, where the focus and
// caret live and the one busy condition. fill derives from it everything a card entry needs about its fields, so a card
// states no lookup, text, erase or busy closure of its own.
type fieldTable[F any] struct {
	form func(*Model) *F
	// busy is the card's one busy condition: it is saving or otherwise answering something. focus, caret and err reach the
	// focused field, the caret of the focused text field and the card's error. A state that embeds formCard leaves all four
	// nil: they are its shell's Field, Caret and Err, and busy is its Saving.
	busy   func(F) bool
	focus  func(*F) *Field
	caret  func(*F) *Caret
	err    func(*F) *error
	hidden func(Model, Field) bool
	// dismissErr says every shared field action, a move between fields included, dismisses the card's error, not just an
	// edit of its text.
	dismissErr bool
	fields     []formField[F]
}

// formField is one field of a fieldTable, in tab order. A field with text takes typed text; a field without it is a
// choice, which must say how step moves to its next or previous choice and how choose picks the choice at an index.
type formField[F any] struct {
	name Field
	text func(*F) *string
	// step moves the choice on, or back; choose picks the choice at index and says whether the index named one. They
	// reach the whole model for a choice whose choices come from it; f is the card's state within m.
	step   func(m *Model, f *F, back bool)
	choose func(m *Model, f *F, index int) bool
	empty  func(*F) bool
	// held reaches the string a choice holds as its value, nil for a choice that holds none as a string.
	held  func(*F) *string
	reset func(f *F)
}

func textField[F any](name Field, get func(*F) *string) formField[F] {
	return formField[F]{name: name, text: get}
}

func choiceField[F any](name Field, step func(*F, bool), choose func(*F, int) bool) formField[F] {
	return formField[F]{name: name,
		step:   func(_ *Model, f *F, back bool) { step(f, back) },
		choose: func(_ *Model, f *F, index int) bool { return choose(f, index) }}
}

func cycleIndex(i, n int, back bool) int {
	d := 1
	if back {
		d = -1
	}
	return ((i+d)%n + n) % n
}

// yesNoField declares a field of two choices, no (index 0) and yes (index 1), that live in the bool at get; stepping flips it.
func yesNoField[F any](name Field, get func(*F) *bool) formField[F] {
	return choiceField(name,
		func(f *F, _ bool) { *get(f) = !*get(f) },
		func(f *F, i int) bool {
			if i != 0 && i != 1 {
				return false
			}
			*get(f) = i == 1
			return true
		})
}

func modelChoiceField[F any](name Field, step func(*Model, bool), choose func(*Model, int) bool) formField[F] {
	return formField[F]{name: name,
		step:   func(m *Model, _ *F, back bool) { step(m, back) },
		choose: func(m *Model, _ *F, index int) bool { return choose(m, index) }}
}

func (t *fieldTable[F]) shell(f *F) *formCard { return any(f).(cardShell).card() }

func (t *fieldTable[F]) busyOf(f F) bool {
	if t.busy != nil {
		return t.busy(f)
	}
	return t.shell(&f).Saving
}

func (t *fieldTable[F]) focusOf(f *F) *Field {
	if t.focus != nil {
		return t.focus(f)
	}
	return &t.shell(f).Field
}

func (t *fieldTable[F]) caretOf(f *F) *Caret {
	if t.caret != nil {
		return t.caret(f)
	}
	return &t.shell(f).Caret
}

func (t *fieldTable[F]) errOf(f *F) *error {
	if t.err != nil {
		return t.err(f)
	}
	return &t.shell(f).Err
}

func (t *fieldTable[F]) clear(m *Model) {
	var zero F
	*t.form(m) = zero
}

// typeKey gives a key no card action took to the focused field: a field with text takes it, a bound space typed like any other
// character, and an edit dismisses the card's error. It reports whether the focused field takes text.
func (t *fieldTable[F]) typeKey(m *Model, binding Binding, bound bool, msg KeyMsg) bool {
	f := t.form(m)
	field := *t.focusOf(f)
	text := t.textAt(f, field)
	if text == nil {
		return false
	}
	if bound && binding.Action == InputSpace {
		bound = false
	}
	var edited bool
	if *text, edited = editText(*text, t.caretOf(f), field, bound, msg); edited {
		*t.errOf(f) = nil
	}
	return true
}

func (t *fieldTable[F]) specs() []FieldSpec {
	list := make([]FieldSpec, len(t.fields))
	for i, f := range t.fields {
		list[i] = FieldSpec{Name: f.name, Text: f.text != nil}
	}
	return list
}

func (t *fieldTable[F]) find(name Field) *formField[F] {
	if i := slices.IndexFunc(t.fields, func(f formField[F]) bool { return f.name == name }); i >= 0 {
		return &t.fields[i]
	}
	return nil
}

func (t *fieldTable[F]) textAt(f *F, name Field) *string {
	if field := t.find(name); field != nil && field.text != nil {
		return field.text(f)
	}
	return nil
}

func (t *fieldTable[F]) value(f F, name Field) string {
	field := t.find(name)
	switch {
	case field == nil:
	case field.text != nil:
		if text := field.text(&f); text != nil {
			return *text
		}
	case field.held != nil:
		return *field.held(&f)
	}
	return ""
}

func (t *fieldTable[F]) stepFocused(m *Model, back bool) bool {
	f := t.form(m)
	field := t.find(*t.focusOf(f))
	if field == nil || field.step == nil || (t.hidden != nil && t.hidden(*m, field.name)) {
		return false
	}
	if field.empty != nil && field.empty(f) {
		return true
	}
	field.step(m, f, back)
	*t.errOf(f) = nil
	return true
}

func (t *fieldTable[F]) resetFocused(m *Model) {
	f := t.form(m)
	if field := t.find(*t.focusOf(f)); field != nil && field.reset != nil {
		field.reset(f)
	}
}

func filterFieldList[F any](values func(*F) *Filters, choices func(Model, Field) []string) []formField[F] {
	text := func(name Field, get func(*Filters) *string) formField[F] {
		return textField(name, func(f *F) *string { return get(values(f)) })
	}
	choice := func(name Field, get func(*Filters) *string) formField[F] {
		return formField[F]{name: name,
			step: func(m *Model, f *F, back bool) {
				value := get(values(f))
				list := choices(*m, name)
				if len(list) > 0 {
					*value = list[cycleIndex(max(slices.Index(list, *value), 0), len(list), back)]
				}
			},
			choose: func(m *Model, f *F, i int) bool {
				list := choices(*m, name)
				if i < 0 || i >= len(list) {
					return false
				}
				*get(values(f)) = list[i]
				return true
			},
			held:  func(f *F) *string { return get(values(f)) },
			reset: func(f *F) { *get(values(f)) = "" },
		}
	}
	return []formField[F]{
		choice(FilterProject, func(v *Filters) *string { return &v.ProjectUID }),
		choice(FilterStatus, func(v *Filters) *string { return &v.Status }),
		text(FilterOwner, func(v *Filters) *string { return &v.Owner }),
		text(FilterLabel, func(v *Filters) *string { return &v.Label }),
		choice(FilterRelationship, func(v *Filters) *string { return &v.Relationship }),
	}
}

func (t *fieldTable[F]) fill(e *cardEntry) {
	e.fields = t.specs()
	e.busy = func(m Model) bool { return t.busyOf(*t.form(&m)) }
	if _, shelled := any(new(F)).(cardShell); shelled {
		e.open = func(m Model) bool { return t.shell(t.form(&m)).Open }
		e.saving = func(m Model) bool { return t.shell(t.form(&m)).Saving }
	}
	if t.hidden != nil {
		e.hidden = t.hidden
	}
	if t.dismissErr {
		e.clearErr = func(m Model) Model {
			*t.errOf(t.form(&m)) = nil
			return m
		}
	}
	e.focus = func(m *Model) *Field { return t.focusOf(t.form(m)) }
	e.text = func(m *Model) (*string, *Caret) {
		f := t.form(m)
		return t.textAt(f, *t.focusOf(f)), t.caretOf(f)
	}
	e.erased = func(m Model, _ Action, changed bool) Model {
		if changed {
			*t.errOf(t.form(&m)) = nil
		}
		return m
	}
	if !slices.ContainsFunc(t.fields, func(f formField[F]) bool { return f.text == nil }) {
		return
	}
	e.step = func(m Model, back bool) Model {
		t.stepFocused(&m, back)
		return m
	}
	e.choose = func(m Model, index int) Model {
		f := t.form(&m)
		if field := t.find(*t.focusOf(f)); field != nil && field.choose != nil && field.choose(&m, f, index) {
			*t.errOf(f) = nil
		}
		return m
	}
}

// The card is not saving, which the one guard before every key handler sees to.
func (t *fieldTable[F]) plainKey(mode Mode, submit, cancel Action, send func(Model) (Model, []Cmd)) func(Model, KeyMsg) (Model, []Cmd) {
	return func(m Model, msg KeyMsg) (Model, []Cmd) {
		binding, bound := LookupBindingForMode(msg.Chord, mode)
		switch {
		case bound && binding.Action == submit:
			return send(m)
		case bound && binding.Action == cancel:
			t.clear(&m)
		default:
			t.typeKey(&m, binding, bound, msg)
		}
		return m, nil
	}
}
