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
	// hidden says a field does not apply to the card as it stands: tab skips it, it holds no caret and it takes no step. Nil
	// when every field always applies.
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
	// empty says the choice has no choices right now, so stepping it changes nothing; nil for a choice that always has some.
	empty func(*F) bool
	// held reaches the string a choice holds as its value, nil for a choice that holds none as a string.
	held func(*F) *string
	// reset sets a choice that can be none to none; nil for a choice that cannot.
	reset func(f *F)
}

// textField declares a field holding the text at get.
func textField[F any](name Field, get func(*F) *string) formField[F] {
	return formField[F]{name: name, text: get}
}

// choiceField declares a field of choices that live in the card's own state.
func choiceField[F any](name Field, step func(*F, bool), choose func(*F, int) bool) formField[F] {
	return formField[F]{name: name,
		step:   func(_ *Model, f *F, back bool) { step(f, back) },
		choose: func(_ *Model, f *F, index int) bool { return choose(f, index) }}
}

// cycleIndex is the index after i, or before it when back, in a ring of n.
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

// modelChoiceField declares a field of choices that come from the model, such as the Projects of the catalog.
func modelChoiceField[F any](name Field, step func(*Model, bool), choose func(*Model, int) bool) formField[F] {
	return formField[F]{name: name,
		step:   func(m *Model, _ *F, back bool) { step(m, back) },
		choose: func(m *Model, _ *F, index int) bool { return choose(m, index) }}
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

// value is the text the field called name holds, or the string a choice holds as its value; "" for any other name.
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

// stepFocused moves the focused choice field to its next choice, or the previous one when back, and dismisses the error.
// It reports whether the focused field is a choice.
func (t *fieldTable[F]) stepFocused(m *Model, back bool) bool {
	f := t.form(m)
	field := t.find(*t.focus(f))
	if field == nil || field.step == nil || (t.hidden != nil && t.hidden(*m, field.name)) {
		return false
	}
	if field.empty != nil && field.empty(f) {
		return true
	}
	field.step(m, f, back)
	*t.err(f) = nil
	return true
}

// resetFocused sets the focused choice field to none, when it can be none.
func (t *fieldTable[F]) resetFocused(m *Model) {
	f := t.form(m)
	if field := t.find(*t.focus(f)); field != nil && field.reset != nil {
		field.reset(f)
	}
}

// filterFieldList declares the Filter card's fields, in tab order, for a card that holds them as a Filters: the Project,
// Status, Owner, Label and Relationship. choices gives a choice field its values, which depend on the card.
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

// fill sets on e everything that derives from the table: the field list, busy condition, focus, text and caret for the
// shared delete and clear actions, the error a changed text dismisses, and the choice steps of a table with choices.
func (t *fieldTable[F]) fill(e *cardEntry) {
	e.fields = t.specs()
	e.busy = func(m Model) bool { return t.busy(*t.form(&m)) }
	if t.hidden != nil {
		e.hidden = t.hidden
	}
	if t.dismissErr {
		e.clearErr = func(m Model) Model {
			*t.err(t.form(&m)) = nil
			return m
		}
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
		t.stepFocused(&m, back)
		return m
	}
	e.choose = func(m Model, index int) Model {
		f := t.form(&m)
		if field := t.find(*t.focus(f)); field != nil && field.choose != nil && field.choose(&m, f, index) {
			*t.err(f) = nil
		}
		return m
	}
}
