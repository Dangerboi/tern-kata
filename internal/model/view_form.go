package model

import (
	"cmp"
	"errors"
	"slices"
)

// The View card's fields beyond the Filter card's own.
const (
	ViewName Field = "name"
	ViewBase Field = "base"
)

// viewTable declares the View card's fields, in tab order: the name, the base Collection, then the Filter card's fields and
// the search text. Every key of the card dismisses its error.
var viewTable = fieldTable[ViewFormState]{
	form:       func(m *Model) *ViewFormState { return &m.ViewForm },
	busy:       func(f ViewFormState) bool { return f.Saving },
	focus:      func(f *ViewFormState) *Field { return &f.Field },
	caret:      func(f *ViewFormState) *Caret { return &f.Caret },
	err:        func(f *ViewFormState) *error { return &f.Err },
	dismissErr: true,
	fields: slices.Concat(
		[]formField[ViewFormState]{
			textField(ViewName, func(f *ViewFormState) *string { return &f.Name }),
			modelChoiceField[ViewFormState](ViewBase, stepViewBase, chooseViewBase),
		},
		filterFieldList(func(f *ViewFormState) *Filters { return &f.Values }, Model.ViewChoices),
		[]formField[ViewFormState]{textField(FilterSearch, func(f *ViewFormState) *string { return &f.Values.Search })},
	),
}

// ViewFields is the View card's field list, in tab order.
var ViewFields = viewTable.specs()

const (
	ModeViewForm            Mode   = "view-form"
	ModeDeleteView          Mode   = "delete-view"
	SaveViewForm            Action = "save-view-form"
	CancelViewForm          Action = "cancel-view-form"
	EditViewAction          Action = "edit-view"
	AskDeleteView           Action = "ask-delete-view"
	ConfirmDeleteViewAction Action = "confirm-delete-view"
	KeepViewAction          Action = "keep-view"
)

// ViewFormState is the View card: a saved view being made (Self is "") or edited (Self is the name it had when the card opened). Its Filter fields are the
// Filter card's, held in Values with the search text.
type ViewFormState struct {
	Open, Saving bool
	Field        Field
	Name         string
	Base         Collection
	Values       Filters
	Caret        Caret
	Self         string
	// Err is why the last save did not happen, naming the field; the card and its input stay.
	Err error
	// Sent is the save the card is waiting on, so only its own answer closes or fails the card.
	Sent SaveView
}

func openViewForm(m Model, self string, v SavedView) Model {
	m.ViewForm = ViewFormState{Open: true, Field: ViewName, Name: v.Name, Base: v.Base, Values: v.Filters, Self: self}
	return m
}

// ViewBases are the Collections the Base choice cycles through: the fixed Collections, every Project the catalog lists, and the
// card's own base when it is neither (a Project that is gone).
func (m Model) ViewBases() []Collection {
	bases := FixedCollections()
	for _, entry := range m.Snapshot.Catalog {
		bases = append(bases, ProjectCollection(entry.Project.UID, entry.Project.Name))
	}
	if b := m.ViewForm.Base; !slices.ContainsFunc(bases, func(c Collection) bool { return c.Key() == b.Key() }) {
		bases = append(bases, b)
	}
	return bases
}

// ViewChoices is the values the choice field called name cycles through in the View card, in the order the card shows them;
// nil for a text field. The Project values are every Project, not the Filter card's one-Project list.
func (m Model) ViewChoices(name Field) []string {
	switch name {
	case ViewBase:
		bases := m.ViewBases()
		keys := make([]string, len(bases))
		for i, b := range bases {
			keys[i] = b.Key()
		}
		return keys
	case FilterProject:
		values := m.projectChoices("")
		if uid := m.ViewForm.Values.ProjectUID; uid != "" && !slices.Contains(values, uid) {
			values = append(values, uid)
		}
		return values
	}
	return fixedFilterChoices(name)
}

// choiceValue is the value the choice field holds: the base's key for Base.
func (f ViewFormState) choiceValue(name Field) string {
	if name == ViewBase {
		return f.Base.Key()
	}
	return viewTable.value(f, name)
}

// chooseViewBase gives the Base field the Collection at index i in ViewBases; an index out of range changes nothing.
func chooseViewBase(m *Model, i int) bool {
	bases := m.ViewBases()
	if i < 0 || i >= len(bases) {
		return false
	}
	m.ViewForm.Base = bases[i]
	return true
}

// stepViewBase moves the Base field to its next Collection, or the previous one when back, wrapping around.
func stepViewBase(m *Model, back bool) {
	bases := m.ViewBases()
	at := slices.IndexFunc(bases, func(c Collection) bool { return c.Key() == m.ViewForm.Base.Key() })
	m.ViewForm.Base = bases[cycleIndex(max(at, 0), len(bases), back)]
}

// viewFormKey is one key for the View card on top: a bound chord acts, any other types.
func viewFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeViewForm)
	bound = bound && !(isCaretChord(msg.Chord) && viewTable.textAt(&m.ViewForm, m.ViewForm.Field) != nil)
	if m.ViewForm.Saving && !(bound && binding.Action == CancelViewForm) {
		return m, nil // the save has not answered: the input stays as sent
	}
	m.ViewForm.Err = nil
	if bound {
		return updateViewForm(m, binding.Action)
	}
	if field := viewTable.textAt(&m.ViewForm, m.ViewForm.Field); field != nil {
		*field, _ = editText(*field, &m.ViewForm.Caret, m.ViewForm.Field, false, msg)
	}
	return m, nil
}

func updateViewForm(m Model, action Action) (Model, []Cmd) {
	switch action {
	case InputSpace:
		if field := viewTable.textAt(&m.ViewForm, m.ViewForm.Field); field != nil {
			*field, _ = editText(*field, &m.ViewForm.Caret, m.ViewForm.Field, false, KeyMsg{Text: " "})
			return m, nil
		}
		viewTable.stepFocused(&m, false)
		return m, nil
	case SaveViewForm:
		return saveViewForm(m)
	case CancelViewForm:
		m.ViewForm = ViewFormState{}
	case DeleteInput, ClearInput: // the shared step took them in a text field
		viewTable.resetFocused(&m)
	}
	return m, nil
}

// saveViewForm is ctrl+o in the View card: it checks the input, as `tern-kata views add` does, and sends it with the read it rests
// on. A refusal names its field, focuses it and keeps everything typed.
func saveViewForm(m Model) (Model, []Cmd) {
	f := m.ViewForm
	checked, err := CheckSavedViewIn(SavedView{Name: f.Name, Base: f.Base, Filters: f.Values}, m.Views.List, f.Self, m.ProjectFilterName)
	if err != nil {
		m.ViewForm.Err = err
		if refusal, ok := errors.AsType[ViewFieldError](err); ok {
			m.ViewForm.Field = refusal.Field
		}
		return m, nil
	}
	if m.Views.Token == nil {
		m.ViewForm.Err = errors.New("Saved views cannot be written: " + cmp.Or(errText(m.Views.Err), "views.toml has not been read"))
		return m, nil
	}
	send := SaveView{Self: f.Self, View: checked, Token: m.Views.Token}
	m.ViewForm.Saving, m.ViewForm.Err, m.ViewForm.Sent = true, nil, send
	return m, []Cmd{send}
}
