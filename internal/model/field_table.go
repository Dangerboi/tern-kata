package model

import "slices"

// fieldTable is a form card's fields declared once: which of them take text, which are choices, where the focus and
// caret live and the one busy condition. fill derives from it everything a card entry needs about its fields, so a card
// states no lookup, text, erase or busy closure of its own.
type fieldTable[F any] struct {
	// form is where the card's state lives on the model.
	form func(*Model) *F
	// busy is the card's one busy condition: it is saving or otherwise answering something.
	busy func(F) bool
	// focus, caret and err reach the focused field, the caret of the focused text field and the card's error.
	focus func(*F) *Field
	caret func(*F) *Caret
	err   func(*F) *error
	// hidden says a field does not apply to the card as it stands: tab skips it and it holds no caret. Nil when every field always applies.
	hidden func(F, Field) bool
	fields []formField[F]
}

// formField is one field of a fieldTable, in tab order. A field with text takes typed text; a field without it is a
// choice, which must say how step moves to its next or previous choice and how choose picks the choice at an index.
type formField[F any] struct {
	name Field
	text func(*F) *string
	// step moves the choice on, or back; choose picks the choice at index and says whether the index named one.
	step   func(f *F, back bool)
	choose func(f *F, index int) bool
}

// textField declares a field holding the text at get.
func textField[F any](name Field, get func(*F) *string) formField[F] {
	return formField[F]{name: name, text: get}
}

// choiceField declares a field of choices.
func choiceField[F any](name Field, step func(*F, bool), choose func(*F, int) bool) formField[F] {
	return formField[F]{name: name, step: step, choose: choose}
}

// specs is the field list the model and the view read: each field's name and whether it takes text, in tab order.
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

// textAt is the text of the field called name, nil for a name that is not a text field of the table.
func (t *fieldTable[F]) textAt(f *F, name Field) *string {
	if field := t.find(name); field != nil && field.text != nil {
		return field.text(f)
	}
	return nil
}

// value is the text the field called name holds, "" for a name that is not a text field of the table.
func (t *fieldTable[F]) value(f F, name Field) string {
	if text := t.textAt(&f, name); text != nil {
		return *text
	}
	return ""
}

// stepFocused moves the focused choice field to its next choice, or the previous one when back, and dismisses the error.
// It reports whether the focused field is a choice.
func (t *fieldTable[F]) stepFocused(f *F, back bool) bool {
	field := t.find(*t.focus(f))
	if field == nil || field.step == nil {
		return false
	}
	field.step(f, back)
	*t.err(f) = nil
	return true
}

// fill sets on e everything that derives from the table: the field list, busy condition, focus, text and caret for the
// shared delete and clear actions, the error a changed text dismisses, and the choice steps of a table with choices.
func (t *fieldTable[F]) fill(e *cardEntry) {
	e.fields = t.specs()
	e.busy = func(m Model) bool { return t.busy(*t.form(&m)) }
	if t.hidden != nil {
		e.hidden = func(m Model, name Field) bool { return t.hidden(*t.form(&m), name) }
	}
	e.focus = func(m *Model) *Field { return t.focus(t.form(m)) }
	e.text = func(m *Model) (*string, *Caret) {
		f := t.form(m)
		return t.textAt(f, *t.focus(f)), t.caret(f)
	}
	e.erased = func(m Model, _ Action, changed bool) Model {
		if changed {
			*t.err(t.form(&m)) = nil
		}
		return m
	}
	if !slices.ContainsFunc(t.fields, func(f formField[F]) bool { return f.text == nil }) {
		return
	}
	e.step = func(m Model, back bool) Model {
		t.stepFocused(t.form(&m), back)
		return m
	}
	e.choose = func(m Model, index int) Model {
		f := t.form(&m)
		if field := t.find(*t.focus(f)); field != nil && field.choose != nil && field.choose(f, index) {
			*t.err(f) = nil
		}
		return m
	}
}
