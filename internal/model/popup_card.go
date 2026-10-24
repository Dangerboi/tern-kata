package model

import (
	"cmp"
	"slices"
	"unicode/utf16"
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
	CardWorkflow
	CardField
	CardClose
	CardDates
	CardToken
	CardRevoke
	CardRecurrence
	CardJob
	CardBackfill
	CardProject
	CardLanes
	CardWorker
	CardFilter
	CardPicker
	CardMove
	CardDelete
	CardFederationAdopt
)

// cardEntry is everything the model knows about one popup card. Each card fills in one.
type cardEntry struct {
	card Card
	// name is the stem of the card's node ids: its layer is "layer." + name and each of its fields' nodes is the layer's id
	// and the field's name; the one input of a card without fields is the layer's id and "input". The view adds nothing
	// else, so a card whose ids cannot follow that says so here.
	name string
	open func(Model) bool
	// modes is every keymap mode the card can be in; mode picks the current one from its state.
	modes []Mode
	mode  func(Model) Mode
	// key handles a key for the card; every global chord but ctrl+c, which quits at once from any card, is the card's to swallow or type.
	key func(Model, KeyMsg) (Model, []Cmd)
	// busy is the card's one busy condition: it is saving, asking to discard, showing a conflict or
	// otherwise answering something, so no focus moves and no text field holds the caret. Every
	// other rule about focus and the caret derives from it.
	busy func(Model) bool
	// fields is the card's field list in tab order; a card without fields declares an empty,
	// non-nil list. focus finds the focused field; it is nil for a card without fields, which has
	// one text input named "" when input says so.
	fields []FieldSpec
	focus  func(*Model) *Field
	input  bool
	// hidden says a field of the list does not apply to the card as it stands: tab skips it and it holds no caret. Nil when every field always applies.
	hidden func(Model, Field) bool
	// editing declares, for each keymap mode the card takes field-editing keys in, which shared actions it offers and the
	// card's wording for them. The keymap rows come from it and the shared step applies the actions before key sees them.
	editing []fieldEditing
	// step moves the focused choice field to its next choice, or the previous one when back; nil for a card without choice fields.
	step func(m Model, back bool) Model
	// clearErr dismisses the card's error; set only where every key, field moves included, dismisses it (the Filter form).
	clearErr func(Model) Model
	// target names the card's multi-line body for native edits, and edit applies one to it; both
	// are zero for a card without a body. edit is called only while the card is not busy.
	target EditTarget
	// bodyField is the field the target body is, "" when it is the card's one input.
	bodyField Field
	edit      func(Model, func(TextEditor) TextEditor) Model
	// choose gives the focused choice field its choice at an index; nil for a card without choice fields.
	choose func(Model, int) Model
	// text reaches the text and caret of the focused field for the shared delete and clear actions; the text is nil when the
	// focused field takes none. Nil for a card whose keys never delete or clear text.
	text func(*Model) (*string, *Caret)
	// erased runs after the shared delete or clear acted on the text, with whether it changed: the card's side effect of an edit.
	erased func(m Model, changed bool) Model
}

// fieldEditing is one keymap mode's declaration of the shared field-editing actions: NextField, PreviousField, NextChoice,
// PreviousChoice, DeleteInput and ClearInput. A card lists the ones it offers; a card with one field lists only DeleteInput and ClearInput.
type fieldEditing struct {
	mode    Mode
	actions []Action
	// delete and clear are the card's wording for its backspace and ctrl+u rows; blank is "delete" and "clear field".
	delete, clear string
	// nextAlso lists chords that take NextField besides tab (enter in the Filter form).
	nextAlso []string
	// before names the card's own action whose row the field rows precede in the keymap, which sets the order of the footer;
	// blank puts them first. The choice, delete and clear rows follow the card's own rows.
	before Action
}

func (d fieldEditing) offers(a Action) bool { return slices.Contains(d.actions, a) }

// moveRows are the rows for NextField and PreviousField; tab stays in the footer.
func (d fieldEditing) moveRows() []Binding {
	var rows []Binding
	if d.offers(NextField) {
		rows = append(rows, Binding{Chord: "tab", Key: "tab", Action: NextField, Help: "next field", Mode: d.mode, ShowInFooter: true})
		for _, chord := range d.nextAlso {
			rows = append(rows, Binding{Chord: chord, Key: chord, Action: NextField, Help: "next field", Mode: d.mode})
		}
	}
	if d.offers(PreviousField) {
		rows = append(rows, Binding{Chord: "shift+tab", Key: "shift+tab", Action: PreviousField, Help: "previous field", Mode: d.mode})
	}
	return rows
}

// editRows are the rows for the choice, delete and clear actions; right stays in the footer.
func (d fieldEditing) editRows() []Binding {
	var rows []Binding
	if d.offers(NextChoice) {
		rows = append(rows, Binding{Chord: "right", Key: "right", Action: NextChoice, Help: "next choice", Mode: d.mode, ShowInFooter: true})
	}
	if d.offers(PreviousChoice) {
		rows = append(rows, Binding{Chord: "left", Key: "left", Action: PreviousChoice, Help: "previous choice", Mode: d.mode})
	}
	if d.offers(DeleteInput) {
		rows = append(rows, Binding{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: cmp.Or(d.delete, "delete"), Mode: d.mode})
	}
	if d.offers(ClearInput) {
		rows = append(rows, Binding{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: cmp.Or(d.clear, "clear field"), Mode: d.mode})
	}
	return rows
}

// withFieldEditing is the card keymap rows own with the rows of every card's field-editing declaration put in.
func withFieldEditing(own []Binding) []Binding {
	var declared []fieldEditing
	for _, entry := range cards {
		declared = append(declared, entry.editing...)
	}
	placed := make([]bool, len(declared))
	out := make([]Binding, 0, len(own)+8*len(declared))
	for i, d := range declared {
		if d.before == "" {
			out, placed[i] = append(out, d.moveRows()...), true
		}
	}
	for _, row := range own {
		for i, d := range declared {
			if !placed[i] && d.before == row.Action && d.mode == row.Mode {
				out, placed[i] = append(out, d.moveRows()...), true
			}
		}
		out = append(out, row)
	}
	for i, d := range declared {
		if !placed[i] {
			panic("field-editing rows of mode " + string(d.mode) + " precede no row of " + string(d.before))
		}
		out = append(out, d.editRows()...)
	}
	return out
}

// cards lists every popup card in the one order that decides which is on top. It is filled in by
// init, through buildCards, because the cards' key handlers read it back through the keymap lookup.
var cards []cardEntry

// Busy conditions: each card states its own once, and focus, the caret and native edits all derive from it.
func createBusy(m Model) bool { return m.Create.Saving || m.Create.DiscardConfirm }

// draftBusy is the Draft editor's busy condition: saving, asking to discard or showing a conflict.
func draftBusy(m Model) bool {
	draft, _ := m.ActiveDraft()
	return m.EditorMode() != ModeEditor || draft.Saving
}

// workflowBusy is the Workflow card's busy condition, the same as the Draft editor's.
func workflowBusy(m Model) bool { return workflowMode(m) != ModeEditor || m.Workflows.Card.Saving }

func recurrenceBusy(m Model) bool {
	f := m.RecurrenceForm
	return f.Saving || f.StopConfirm || f.Refused
}

func jobBusy(m Model) bool { return m.JobForm.Saving || m.JobForm.Conflict != nil }

func workerBusy(m Model) bool {
	f := m.WorkerCard
	return f.Saving || f.Stopping || f.Switching || f.Restart || f.AskOff
}

// buildCards fills in cards.
func buildCards() []cardEntry {
	return []cardEntry{
		{
			card: CardCreate, name: "create", open: func(m Model) bool { return m.Create.Open },
			modes: []Mode{ModeCreate, ModeDiscard},
			mode: func(m Model) Mode {
				if m.Create.DiscardConfirm {
					return ModeDiscard
				}
				return ModeCreate
			},
			key:  createCard.key,
			busy: createBusy, fields: CreateFields, focus: func(m *Model) *Field { return &m.Create.Field },
			editing: []fieldEditing{{mode: ModeCreate, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}}},
			text: func(m *Model) (*string, *Caret) {
				if value := createValue(&m.Create); value != nil {
					return value, &m.Create.Caret
				}
				return nil, nil
			},
			erased: func(m Model, _ bool) Model { m.Create.Err = nil; return m },
			target: EditCreateBody, bodyField: CreateBody,
			edit: func(m Model, apply func(TextEditor) TextEditor) Model {
				if !createBusy(m) {
					m.Create.Editor = apply(m.Create.Editor)
				}
				return m
			},
		},
		{
			card: CardDraft, name: "editor", open: func(m Model) bool { _, ok := m.ActiveDraft(); return ok },
			modes:  []Mode{ModeEditor, ModeDiscard, ModeConflict},
			mode:   Model.EditorMode,
			key:    draftCard.key,
			busy:   draftBusy,
			fields: []FieldSpec{}, input: true,
			target: EditDraftBody,
			edit: func(m Model, apply func(TextEditor) TextEditor) Model {
				if !draftBusy(m) {
					draft, _ := m.ActiveDraft()
					draft.Editor = apply(draft.Editor)
					m = putDraft(m, m.EditorUID, draft)
				}
				return m
			},
		},
		{
			card: CardWorkflow, name: "workflow", open: func(m Model) bool { return m.Workflows.Card.Open },
			modes:  []Mode{ModeEditor, ModeDiscard, ModeConflict},
			mode:   workflowMode,
			key:    workflowCardKey,
			busy:   workflowBusy,
			fields: []FieldSpec{}, input: true,
			target: EditWorkflowBody,
			edit: func(m Model, apply func(TextEditor) TextEditor) Model {
				if !workflowBusy(m) {
					m.Workflows.Card.Editor = apply(m.Workflows.Card.Editor)
				}
				return m
			},
		},
		{
			card: CardField, name: "fields", open: func(m Model) bool { return m.FieldForm.Open },
			modes: []Mode{ModeFieldForm}, mode: func(Model) Mode { return ModeFieldForm },
			key:    updateFieldKey,
			busy:   func(m Model) bool { return m.FieldForm.Saving },
			fields: []FieldSpec{}, input: true,
			editing: []fieldEditing{{mode: ModeFieldForm, actions: []Action{DeleteInput, ClearInput}, delete: "erase", clear: "clear input"}},
			text:    func(m *Model) (*string, *Caret) { return &m.FieldForm.Value, &m.FieldForm.Caret },
			erased: func(m Model, changed bool) Model {
				m.FieldForm.Err = nil
				if changed {
					m.FieldForm.Found = nil
				}
				return m
			},
		},
		{
			card: CardClose, name: "close", open: func(m Model) bool { return m.CloseForm.Open },
			modes: []Mode{ModeCloseForm}, mode: func(Model) Mode { return ModeCloseForm },
			key:    closeKey,
			busy:   func(m Model) bool { return m.CloseForm.Saving },
			fields: CloseFields, focus: func(m *Model) *Field { return &m.CloseForm.Field },
			editing: []fieldEditing{{mode: ModeCloseForm, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}, delete: "erase"}},
			text: func(m *Model) (*string, *Caret) {
				return m.CloseForm.Values.at(m.CloseForm.Field), &m.CloseForm.Caret
			},
			erased: func(m Model, changed bool) Model {
				if changed {
					m.CloseForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardDates, name: "dates", open: func(m Model) bool { return m.DatesForm.Open },
			modes: []Mode{ModeDatesForm}, mode: func(Model) Mode { return ModeDatesForm },
			key:    updateDatesFormKey,
			busy:   func(m Model) bool { return m.DatesForm.Saving },
			fields: DatesFields, focus: func(m *Model) *Field { return &m.DatesForm.Field },
			editing: []fieldEditing{{mode: ModeDatesForm, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}}},
			text: func(m *Model) (*string, *Caret) {
				return m.DatesForm.Text.at(m.DatesForm.Field), &m.DatesForm.Caret
			},
			erased: func(m Model, changed bool) Model {
				if changed {
					m.DatesForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardToken, name: "token", open: func(m Model) bool { return m.TokenForm.Open },
			modes: []Mode{ModeTokenForm}, mode: func(Model) Mode { return ModeTokenForm },
			key:    updateTokenFormKey,
			busy:   func(m Model) bool { return m.TokenForm.Saving || m.TokenForm.Done },
			fields: TokenFields, focus: func(m *Model) *Field { return &m.TokenForm.Field },
			editing: []fieldEditing{{mode: ModeTokenForm, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}}},
			text: func(m *Model) (*string, *Caret) {
				return m.TokenForm.Values.at(m.TokenForm.Field), &m.TokenForm.Caret
			},
			erased: func(m Model, changed bool) Model {
				if changed {
					m.TokenForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardRevoke, name: "revoke", open: func(m Model) bool { return m.RevokeCard.Open },
			modes: []Mode{ModeRevokeToken}, mode: func(Model) Mode { return ModeRevokeToken },
			key:  updateRevokeKey,
			busy: func(m Model) bool { return m.RevokeCard.Saving }, fields: []FieldSpec{},
		},
		{
			card: CardMove, name: "move", open: func(m Model) bool { return m.MoveProject.Open },
			modes: []Mode{ModeMoveProject, ModeMoveConfirm}, mode: Model.MoveCardMode,
			key: moveProjectKey,
			// The card holds no caret once a Project is chosen or while a request is out.
			busy:   func(m Model) bool { return m.MoveProject.Sending || m.MoveProject.Target.UID != "" },
			fields: []FieldSpec{}, input: true,
			editing: []fieldEditing{{mode: ModeMoveProject, actions: []Action{DeleteInput, ClearInput}, clear: "clear filter"}},
			text:    func(m *Model) (*string, *Caret) { return &m.MoveProject.Filter, &m.MoveProject.Caret },
			erased:  func(m Model, _ bool) Model { m.MoveProject.Cursor, m.MoveProject.Err = 0, nil; return m },
		},
		{
			card: CardDelete, name: "delete", open: func(m Model) bool { return m.DeleteForm.Open },
			modes: []Mode{ModeDeleteForm}, mode: func(Model) Mode { return ModeDeleteForm },
			key:    updateDeleteFormKey,
			busy:   func(m Model) bool { return m.DeleteForm.Saving },
			fields: DeleteFields, focus: func(m *Model) *Field { return &m.DeleteForm.Field },
			editing: []fieldEditing{{mode: ModeDeleteForm, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}}},
			text: func(m *Model) (*string, *Caret) {
				return m.DeleteForm.at(m.DeleteForm.Field), &m.DeleteForm.Caret
			},
			erased: func(m Model, changed bool) Model {
				if changed {
					m.DeleteForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardFederationAdopt, name: "federation-adopt", open: func(m Model) bool { return m.Federation.Adopting },
			modes: []Mode{ModeFederationAdopt}, mode: func(Model) Mode { return ModeFederationAdopt },
			key:    federationAdoptKey,
			busy:   func(Model) bool { return false },
			fields: []FieldSpec{}, input: true,
			editing: []fieldEditing{{mode: ModeFederationAdopt, actions: []Action{DeleteInput, ClearInput}, clear: "clear name"}},
			text:    func(m *Model) (*string, *Caret) { return &m.Federation.AdoptInput, &m.Federation.AdoptCaret },
			erased:  func(m Model, _ bool) Model { m.Federation.AdoptErr = nil; return m },
		},
		{
			card: CardRecurrence, name: "recurrence", open: func(m Model) bool { return m.RecurrenceForm.Open },
			modes: []Mode{ModeRecurrenceForm, ModeRecurrenceStop},
			mode: func(m Model) Mode {
				if m.RecurrenceForm.StopConfirm {
					return ModeRecurrenceStop
				}
				return ModeRecurrenceForm
			},
			key:    recurrenceKey,
			busy:   recurrenceBusy,
			fields: RecurrenceFields, focus: func(m *Model) *Field { return &m.RecurrenceForm.Field },
			hidden:  func(m Model, name Field) bool { return !m.RecurrenceForm.Shows(name) },
			editing: []fieldEditing{{mode: ModeRecurrenceForm, actions: []Action{NextField, PreviousField, NextChoice, PreviousChoice, DeleteInput, ClearInput}, delete: "erase"}},
			text: func(m *Model) (*string, *Caret) {
				return m.RecurrenceForm.text(m.RecurrenceForm.Field), &m.RecurrenceForm.Caret
			},
			erased: func(m Model, changed bool) Model {
				if changed {
					m.RecurrenceForm.Err = nil
				}
				return m
			},
			step: func(m Model, back bool) Model {
				m.RecurrenceForm.step(back)
				m.RecurrenceForm.Err = nil
				return m
			},
			choose: func(m Model, i int) Model { m.RecurrenceForm.choose(i); return m },
		},
		{
			card: CardBackfill, name: "backfill", open: func(m Model) bool { return m.Backfill.Open },
			modes: []Mode{ModeBackfill}, mode: func(Model) Mode { return ModeBackfill },
			key:  backfillKey,
			busy: func(Model) bool { return false }, fields: []FieldSpec{},
		},
		{
			card: CardJob, name: "job", open: func(m Model) bool { return m.JobForm.Open },
			modes: []Mode{ModeJobForm, ModeJobConflict},
			mode: func(m Model) Mode {
				if m.JobForm.Conflict != nil {
					return ModeJobConflict
				}
				return ModeJobForm
			},
			key:    jobFormKey,
			busy:   jobBusy,
			fields: JobFields, focus: func(m *Model) *Field { return &m.JobForm.Field },
			hidden:  func(m Model, name Field) bool { return !m.JobForm.Shows(name) },
			editing: []fieldEditing{{mode: ModeJobForm, actions: []Action{NextField, PreviousField, NextChoice, PreviousChoice, DeleteInput, ClearInput}, delete: "erase"}},
			step:    func(m Model, back bool) Model { m.JobForm.step(back); return m },
			text:    func(m *Model) (*string, *Caret) { return m.JobForm.text(m.JobForm.Field), &m.JobForm.Caret },
			erased: func(m Model, changed bool) Model {
				if changed {
					m.JobForm.Err = nil
				}
				return m
			},
			choose: func(m Model, i int) Model { m.JobForm.choose(i); return m },
		},
		{
			card: CardProject, name: "project", open: func(m Model) bool { return m.ProjectForm.Open },
			modes: []Mode{ModeProjectForm}, mode: func(Model) Mode { return ModeProjectForm },
			key:    updateProjectFormKey,
			busy:   func(m Model) bool { return m.ProjectForm.Saving },
			fields: []FieldSpec{}, input: true,
			editing: []fieldEditing{{mode: ModeProjectForm, actions: []Action{DeleteInput, ClearInput}, clear: "clear name"}},
			text:    func(m *Model) (*string, *Caret) { return &m.ProjectForm.Name, &m.ProjectForm.Caret },
			erased: func(m Model, changed bool) Model {
				if changed {
					m.ProjectForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardLanes, name: "lanes", open: func(m Model) bool { return m.LanesForm.Open },
			modes: []Mode{ModeLanesForm}, mode: func(Model) Mode { return ModeLanesForm },
			key:    updateLanesFormKey,
			busy:   func(m Model) bool { return m.LanesForm.Saving },
			fields: LanesFields, focus: func(m *Model) *Field { return &m.LanesForm.Field },
			editing: []fieldEditing{{mode: ModeLanesForm, actions: []Action{NextField, PreviousField, DeleteInput, ClearInput}}},
			text:    func(m *Model) (*string, *Caret) { return m.LanesForm.at(m.LanesForm.Field), &m.LanesForm.Caret },
			erased: func(m Model, changed bool) Model {
				if changed {
					m.LanesForm.Err = nil
				}
				return m
			},
		},
		{
			card: CardWorker, name: "worker", open: func(m Model) bool { return m.WorkerCard.Open },
			modes: []Mode{ModeWorkerCard, ModeWorkerRead, ModeWorkerRestart, ModeWorkerOff},
			mode: func(m Model) Mode {
				if m.WorkerCard.Restart {
					return ModeWorkerRestart
				}
				if m.WorkerCard.AskOff {
					return ModeWorkerOff
				}
				return m.WorkerCardMode()
			},
			key:    workerCardKey,
			busy:   workerBusy,
			fields: WorkerFields, focus: func(m *Model) *Field { return &m.WorkerCard.Field },
			hidden:  func(m Model, name Field) bool { return !m.WorkerCardShows(name) },
			editing: []fieldEditing{{mode: ModeWorkerCard, actions: []Action{NextField, PreviousField, NextChoice, PreviousChoice, DeleteInput, ClearInput}, delete: "erase", before: WorkerEdit}},
			text:    func(m *Model) (*string, *Caret) { return m.WorkerCard.at(m.WorkerCard.Field), &m.WorkerCard.Caret },
			erased: func(m Model, changed bool) Model {
				if changed {
					m.WorkerCard.Err = nil
				}
				return m
			},
			step: func(m Model, _ bool) Model {
				m.WorkerCard.toggleTrust(m)
				return m
			},
			choose: func(m Model, i int) Model { m.WorkerCard.choose(i); return m },
		},
		{
			card: CardFilter, name: "filters", open: func(m Model) bool { return m.FilterForm.Open },
			modes: []Mode{ModeFilterForm}, mode: func(Model) Mode { return ModeFilterForm },
			key:    filterFormKey,
			busy:   func(m Model) bool { return m.FilterForm.Saving },
			fields: FilterFields, focus: func(m *Model) *Field { return &m.FilterForm.Field },
			hidden:   func(m Model, name Field) bool { return !m.FilterFieldShown(name) },
			editing:  []fieldEditing{{mode: ModeFilterForm, actions: []Action{NextField, PreviousField, NextChoice, PreviousChoice, DeleteInput, ClearInput}, nextAlso: []string{"enter"}, before: ResetFilterForm}},
			step:     filterStep,
			clearErr: func(m Model) Model { m.FilterForm.Err = nil; return m },
			text:     func(m *Model) (*string, *Caret) { return filterTextField(&m.FilterForm), &m.FilterForm.Caret },
			choose:   chooseFilter,
		},
		{
			card: CardPicker, name: "collections", open: func(m Model) bool { return m.Picker.Open },
			modes: []Mode{ModePicker}, mode: func(Model) Mode { return ModePicker },
			key:    updatePicker,
			busy:   func(Model) bool { return false },
			fields: []FieldSpec{}, input: true,
			editing: []fieldEditing{{mode: ModePicker, actions: []Action{DeleteInput, ClearInput}, clear: "clear filter"}},
			text:    func(m *Model) (*string, *Caret) { return &m.Picker.Filter, &m.Picker.Caret },
			erased:  func(m Model, _ bool) Model { m.Picker.Cursor = 0; return m },
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

// Name is the stem of the card's node ids, "" for NoCard.
func (c Card) Name() string {
	for _, entry := range cards {
		if entry.card == c {
			return entry.name
		}
	}
	return ""
}

// CardInput reports whether the card has one text input without a field: its Draft, Workflow, value, name or filter.
func CardInput(card Card) bool {
	for _, entry := range cards {
		if entry.card == card {
			return entry.input
		}
	}
	return false
}

// CardBody is the multi-line body a native edit of the card reaches, and the field of the card that holds it ("" for the card's
// one input). The target is NoEditTarget for a card without a body.
func CardBody(card Card) (EditTarget, Field) {
	for _, entry := range cards {
		if entry.card == card {
			return entry.target, entry.bodyField
		}
	}
	return NoEditTarget, ""
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
	field, ok = entry.caretAt(m)
	return entry.card, field, ok
}

// caretAt finds the field holding the text caret. It derives from the card's one busy condition: the caret sits in the
// focused field when the card is not busy, the card does not hide the field and the field takes text. A card without fields
// has one input named "" when it declares one. The field is named even when ok is false.
func (e cardEntry) caretAt(m Model) (field Field, ok bool) {
	if e.focus == nil {
		return "", e.input && !e.busy(m)
	}
	field = *e.focus(&m)
	return field, !e.busy(m) && (e.hidden == nil || !e.hidden(m, field)) && takesText(e.fields, field)
}

// cardMode is the keymap mode of the card on top, "" when there is none.
func (m Model) cardMode() Mode {
	if entry, ok := m.topEntry(); ok {
		return entry.mode(m)
	}
	return ""
}

// quitChord is the one global chord every popup card lets through, in every state, busy ones included: ctrl+c quits at once.
const quitChord = "ctrl+c"

// cardLetsThrough reports whether mode belongs to a popup card and, if so, whether the card lets
// chord through to its global binding.
func cardLetsThrough(mode Mode, chord string) (isCard, through bool) {
	for _, entry := range cards {
		if slices.Contains(entry.modes, mode) {
			return true, chord == quitChord
		}
	}
	return false, false
}

// updateTopCardKey gives a key to the card on top; it reports false when no card is open.
func updateTopCardKey(m Model, msg KeyMsg) (Model, []Cmd, bool) {
	entry, ok := m.topEntry()
	if !ok || msg.Chord == quitChord {
		return m, nil, false
	}
	if m, ok := entry.editFields(m, msg); ok {
		return m, nil, true
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
	EditWorkflowBody
)

// editBody applies a native edit or undo to target, if the card on top owns it. It reports whether
// the card took the message, which it does even when the Draft is read-only.
func editBody(m Model, target EditTarget, apply func(TextEditor) TextEditor) (Model, bool) {
	if entry, ok := m.topEntry(); ok && target != NoEditTarget && entry.target == target {
		return entry.edit(m, apply), true
	}
	return m, false
}

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
	if entry, ok := m.topEntry(); ok && entry.focus != nil && !entry.busy(m) && takesFocus(entry.fields, name) && (entry.hidden == nil || !entry.hidden(m, name)) {
		*entry.focus(&m) = name
	}
	return m
}

// chooseField focuses the choice field called name of the open card and gives it its choice at index; a text field, a
// field the card hides or a busy card changes nothing.
func chooseField(m Model, name Field, index int) Model {
	m = focusField(m, name)
	if entry, ok := m.topEntry(); ok && entry.choose != nil && !entry.busy(m) && *entry.focus(&m) == name && !takesText(entry.fields, name) {
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

// editFields is the one step that applies the shared field-editing actions before the card's own key handler: a bound
// NextField, PreviousField, NextChoice, PreviousChoice, DeleteInput or ClearInput the card offers in its current mode. A busy
// card takes none of them, and a choice action on a text field is left to the card, whose text takes it (left and right move
// the caret). Delete and clear on a field that takes no text, or that the card hides, are left to the card. It reports whether
// it took the key.
func (e cardEntry) editFields(m Model, msg KeyMsg) (Model, bool) {
	if e.busy(m) {
		return m, false
	}
	mode := e.mode(m)
	binding, bound := LookupBindingForMode(msg.Chord, mode)
	if !bound || binding.Mode != mode || !slices.ContainsFunc(e.editing, func(d fieldEditing) bool { return d.mode == mode && d.offers(binding.Action) }) {
		return m, false
	}
	switch binding.Action {
	case NextField, PreviousField:
		if e.focus == nil {
			return m, false
		}
		step := 1
		if binding.Action == PreviousField {
			step = -1
		}
		field := e.focus(&m)
		n := len(e.fields)
		at := max(slices.IndexFunc(e.fields, func(f FieldSpec) bool { return f.Name == *field }), 0)
		for i := 1; i <= n; i++ {
			next := e.fields[((at+step*i)%n+n)%n].Name
			if e.hidden == nil || !e.hidden(m, next) {
				*field = next
				break
			}
		}
	case NextChoice, PreviousChoice:
		if e.focus == nil || e.step == nil || takesText(e.fields, *e.focus(&m)) {
			return m, false
		}
		m = e.step(m, binding.Action == PreviousChoice)
	case DeleteInput, ClearInput:
		if e.text == nil {
			return m, false
		}
		var field Field
		if e.focus != nil {
			if field = *e.focus(&m); e.hidden != nil && e.hidden(m, field) {
				return m, false
			}
		}
		value, caret := e.text(&m)
		if value == nil {
			return m, false
		}
		var changed bool
		*value, changed = eraseText(*value, caret, field, binding.Action)
		if e.erased != nil {
			m = e.erased(m, changed)
		}
	default:
		return m, false
	}
	if e.clearErr != nil {
		m = e.clearErr(m)
	}
	return m, true
}

// Caret is where the caret sits in a single-line text field: Back runes before the end of the text, so the zero
// value is the end. Field names the field of the card it belongs to ("" for a one-field card); any other field of the
// card has its caret at the end.
type Caret struct {
	Field Field
	Back  int
}

// back is the caret's distance from the end of the text of field, clamped to the n runes it holds.
func (c Caret) back(field Field, n int) int {
	if c.Field != field {
		return 0
	}
	return max(0, min(c.Back, n))
}

// Offset is the caret of field in UTF-16 units into text, as Tern takes it.
func (c Caret) Offset(field Field, text string) int {
	runes := []rune(text)
	return len(utf16.Encode(runes[:len(runes)-c.back(field, len(runes))]))
}

// isCaretChord says the chord moves or deletes at a text field's caret. The headless Tern delivers Ctrl+a and Ctrl+e
// as meta+a and meta+e.
func isCaretChord(chord string) bool {
	switch chord {
	case "left", "right", "home", "end", "ctrl+a", "ctrl+e", "meta+a", "meta+e", "delete":
		return true
	}
	return false
}

// eraseText applies DeleteInput or ClearInput to a single-line text field called field, whose caret is c: delete removes the
// rune before the caret and clear empties the field. It reports whether the text changed; delete at the start does not change it.
func eraseText(value string, c *Caret, field Field, action Action) (string, bool) {
	runes := []rune(value)
	n := len(runes)
	at := n - c.back(field, n)
	*c = Caret{field, n - at}
	if action == ClearInput {
		*c = Caret{}
		return "", true
	}
	if at == 0 {
		return value, false
	}
	return string(slices.Delete(runes, at-1, at)), true
}

// editText applies one key to a single-line text field called field, whose caret is c. The caret chords move
// it or delete after it whatever else they are bound to, since a card calls this only for keys its text field takes; a
// key bound to anything else is not text; a key with no binding inserts its character, a space, or pasted text with its
// line breaks flattened at the caret. It reports whether the text changed. Backspace and ctrl+u are the shared
// DeleteInput and ClearInput actions, which eraseText applies.
func editText(value string, c *Caret, field Field, bound bool, msg KeyMsg) (string, bool) {
	runes := []rune(value)
	n := len(runes)
	at := n - c.back(field, n)
	*c = Caret{field, n - at}
	switch {
	case msg.Chord == "delete":
		if at == n {
			return value, false
		}
		*c = Caret{field, n - at - 1}
		return string(slices.Delete(runes, at, at+1)), true
	case msg.Chord == "left":
		*c = Caret{field, n - max(at-1, 0)}
	case msg.Chord == "right":
		*c = Caret{field, n - min(at+1, n)}
	case msg.Chord == "home" || msg.Chord == "ctrl+a" || msg.Chord == "meta+a":
		*c = Caret{field, n}
	case msg.Chord == "end" || msg.Chord == "ctrl+e" || msg.Chord == "meta+e":
		*c = Caret{}
	case bound:
	default:
		text := typedText(msg)
		return string(slices.Insert(runes, at, []rune(text)...)), text != ""
	}
	return value, false
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
