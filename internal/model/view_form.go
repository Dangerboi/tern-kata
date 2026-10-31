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

// ViewFields is the View card's field list, in tab order: the name, the base Collection, then the Filter card's fields and
// the search text.
var ViewFields = []FieldSpec{
	{ViewName, true}, {ViewBase, false},
	{FilterProject, false}, {FilterStatus, false}, {FilterOwner, true}, {FilterLabel, true}, {FilterRelationship, false},
	{FilterSearch, true},
}

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

// viewTextField is the text of the focused field, nil when it is a choice field.
func viewTextField(f *ViewFormState) *string {
	if !takesText(ViewFields, f.Field) {
		return nil
	}
	if f.Field == ViewName {
		return &f.Name
	}
	return f.Values.value(f.Field)
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
	return f.Values.Value(name)
}

func (m Model) setViewChoice(name Field, value string) Model {
	if name == ViewBase {
		if i := slices.IndexFunc(m.ViewBases(), func(c Collection) bool { return c.Key() == value }); i >= 0 {
			m.ViewForm.Base = m.ViewBases()[i]
		}
		return m
	}
	if p := m.ViewForm.Values.value(name); p != nil {
		*p = value
	}
	return m
}

// chooseView gives the focused choice field its value at index i in ViewChoices; an index out of range changes nothing.
func chooseView(m Model, i int) Model {
	if choices := m.ViewChoices(m.ViewForm.Field); i >= 0 && i < len(choices) {
		return m.setViewChoice(m.ViewForm.Field, choices[i])
	}
	return m
}

// viewStep moves the focused choice field to its next choice, or the previous one when back, wrapping around.
func viewStep(m Model, back bool) Model {
	choices := m.ViewChoices(m.ViewForm.Field)
	if choices == nil {
		return m
	}
	index := max(slices.Index(choices, m.ViewForm.choiceValue(m.ViewForm.Field)), 0)
	step := 1
	if back {
		step = len(choices) - 1
	}
	return m.setViewChoice(m.ViewForm.Field, choices[(index+step)%len(choices)])
}

// viewFormKey is one key for the View card on top: a bound chord acts, any other types.
func viewFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeViewForm)
	bound = bound && !(isCaretChord(msg.Chord) && viewTextField(&m.ViewForm) != nil)
	if m.ViewForm.Saving && !(bound && binding.Action == CancelViewForm) {
		return m, nil // the save has not answered: the input stays as sent
	}
	m.ViewForm.Err = nil
	if bound {
		return updateViewForm(m, binding.Action)
	}
	if field := viewTextField(&m.ViewForm); field != nil {
		*field, _ = editText(*field, &m.ViewForm.Caret, m.ViewForm.Field, false, msg)
	}
	return m, nil
}

func updateViewForm(m Model, action Action) (Model, []Cmd) {
	switch action {
	case InputSpace:
		if field := viewTextField(&m.ViewForm); field != nil {
			*field, _ = editText(*field, &m.ViewForm.Caret, m.ViewForm.Field, false, KeyMsg{Text: " "})
			return m, nil
		}
		return viewStep(m, false), nil
	case SaveViewForm:
		return saveViewForm(m)
	case CancelViewForm:
		m.ViewForm = ViewFormState{}
	case DeleteInput, ClearInput: // the shared step took them in a text field
		if viewTextField(&m.ViewForm) == nil && m.ViewForm.Field != ViewBase {
			m = m.setViewChoice(m.ViewForm.Field, "")
		}
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
