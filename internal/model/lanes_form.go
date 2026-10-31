package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	ModeLanesForm    Mode   = "lanes-form"
	LanesSubmit      Action = "lanes-submit"
	LanesCancel      Action = "lanes-cancel"
	LanesStopSharing Action = "lanes-stop-sharing"
)

// The Lanes card's fields.
const (
	LanesNames Field = "names"
	LanesDays  Field = "days"
	LanesCards Field = "cards"
	// LanesShare is where a save writes: this machine's Lane list file or the Project's shared list.
	LanesShare Field = "share"
)

// lanesTable declares the Lanes card's fields, in tab order, and its busy condition. Save to is a choice: 0 is this
// machine, 1 the Project.
var lanesTable = fieldTable[LanesFormState]{
	form:  func(m *Model) *LanesFormState { return &m.LanesForm },
	busy:  func(f LanesFormState) bool { return f.Saving },
	focus: func(f *LanesFormState) *Field { return &f.Field },
	caret: func(f *LanesFormState) *Caret { return &f.Caret },
	err:   func(f *LanesFormState) *error { return &f.Err },
	fields: []formField[LanesFormState]{
		textField(LanesNames, func(f *LanesFormState) *string { return &f.Text }),
		textField(LanesDays, func(f *LanesFormState) *string { return &f.Days }),
		textField(LanesCards, func(f *LanesFormState) *string { return &f.Cards }),
		yesNoField(LanesShare, func(f *LanesFormState) *bool { return &f.Share }),
	},
}

// LanesFields is the Lanes card's field list, in tab order.
var LanesFields = lanesTable.specs()

// LanesFormState is the Lanes card: the Lane list typed as comma-separated names, then Done's days and card count, then where
// a save writes. Saving to this machine writes the Lane list file; sharing writes the Project's tern.lanes metadata.
type LanesFormState struct {
	Open, Saving bool
	Field        Field
	// Text is the Lane list; Days and Cards are Done's limits, blank for the default.
	Text, Days, Cards string
	Caret             Caret
	// Share says a save writes the Project's shared list; it starts on the source in use.
	Share bool
	// ProjectID and Revision are the Project as read with the list when the card opened; a shared write is guarded on
	// Revision, which a conflict moves to the revision read with the Project's list as it stands.
	ProjectID, Revision int64
	Err                 error
	// Sent and SentShare are the write the card is waiting on, so only its own result closes or fails the card.
	Sent      SaveLanes
	SentShare ShareLanes
}

// Value is the text the field called name holds.
func (f LanesFormState) Value(name Field) string { return lanesTable.value(f, name) }

// openLanesForm is `B`. It waits for the saved Lane list to load: a card prefilled with the default would overwrite it on save.
// An unreadable Lane list file keeps it closed unless the Project shares its list, which the card then edits.
func openLanesForm(m Model) Model {
	if !m.Board.loaded {
		m.Notice = "The Lane list is still loading."
		return m
	}
	if m.Board.fileErr != nil && !m.Board.Shared {
		m.Notice = fmt.Sprintf("Lanes not opened: %v", m.Board.fileErr)
		return m
	}
	m.LanesForm = LanesFormState{Open: true, Field: LanesNames, Text: strings.Join(m.LaneList(), ", "), Share: m.Board.Shared,
		ProjectID: m.Board.projectID, Revision: m.Board.revision}
	if m.Board.DoneDays > 0 {
		m.LanesForm.Days = strconv.Itoa(m.Board.DoneDays)
	}
	if m.Board.DoneCards > 0 {
		m.LanesForm.Cards = strconv.Itoa(m.Board.DoneCards)
	}
	return m
}

// SharesLanes says the Project holds a shared Lane list that stopping sharing would clear: one in use, or a value that is not a
// Lane list (the Project was read, yet its value left unused).
func (m Model) SharesLanes() bool {
	return m.Board.Shared || m.Board.projectID != 0 && m.Board.sharedErr != nil
}

// UnusedLabels are the labels on the loaded Issues that are not in the Lane list, sorted.
func (m Model) UnusedLabels() []string {
	lanes := m.LaneList()
	var unused []string
	for _, issue := range m.Issues {
		for _, label := range issue.Labels {
			if !slices.Contains(lanes, label) && !slices.Contains(unused, label) {
				unused = append(unused, label)
			}
		}
	}
	slices.Sort(unused)
	return unused
}

// parseLanes reads the Lanes card's text. An empty field gives no Lanes, which restores the default list; a name that cannot be a Lane is refused with
// an error naming it.
func parseLanes(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var lanes []string
	for i, entry := range strings.Split(text, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("Entry %d is empty.", i+1)
		}
		label, err := CheckLabel(entry)
		if err != nil {
			return nil, fmt.Errorf("%q %w.", shortEntry(entry), err)
		}
		if slices.Contains(lanes, label) {
			return nil, fmt.Errorf("%q appears twice.", label)
		}
		lanes = append(lanes, label)
	}
	return lanes, nil
}

// The largest Done limits the card accepts and the Lane list file counts as set.
const (
	maxDoneDays  = 3650
	maxDoneCards = 500
)

// parseDone reads a Done limit field called name: blank is 0, the default; anything but a whole number from 1 to max is refused naming the field.
func parseDone(text, name string, max int) (int, error) {
	if text = strings.TrimSpace(text); text == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive whole number.", name)
	}
	if n > max {
		return 0, fmt.Errorf("%s must be at most %d.", name, max)
	}
	return n, nil
}

func shortEntry(text string) string {
	const n = 20
	if r := []rune(text); len(r) > n {
		return string(r[:n]) + "…"
	}
	return text
}

func updateLanesFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.LanesForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeLanesForm)
	switch {
	case form.Saving && !(bound && binding.Action == LanesCancel):
		// The save has not answered: the text stays as sent.
	case bound && binding.Action == LanesSubmit:
		lanes, err := parseLanes(form.Text)
		var days, cards int
		if err == nil {
			days, err = parseDone(form.Days, "Done days", maxDoneDays)
		}
		if err == nil {
			cards, err = parseDone(form.Cards, "Done cards", maxDoneCards)
		}
		if err != nil {
			form.Err = err
			return m, nil
		}
		if form.Share {
			return m.shareLaneList(lanes, days, cards, false)
		}
		if m.Board.fileErr != nil {
			form.Err = fmt.Errorf("Lanes not saved: %w", m.Board.fileErr)
			return m, nil
		}
		form.Saving, form.Err = true, nil
		save := SaveLanes{ProjectUID: m.Collection.ProjectUID, ProjectName: m.Collection.Title, Lanes: slices.Clone(lanes), DoneDays: days, DoneCards: cards}
		form.Sent = save
		return m, []Cmd{save}
	case bound && binding.Action == LanesStopSharing:
		if !m.SharesLanes() && form.ProjectID != 0 {
			form.Err = errors.New("The Project shares no Lane list.")
			return m, nil
		}
		return m.shareLaneList(nil, 0, 0, true)
	case bound && binding.Action == LanesCancel:
		m.LanesForm = LanesFormState{}
	default:
		text := lanesTable.textAt(form, form.Field)
		if text == nil { // the Save to field takes no text; space steps its choice
			if bound && binding.Action == InputSpace {
				lanesTable.stepFocused(&m, false)
			}
			break
		}
		if bound && binding.Action == InputSpace {
			bound = false // a space in a text field is typed
		}
		var edited bool
		if *text, edited = editText(*text, &form.Caret, form.Field, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
