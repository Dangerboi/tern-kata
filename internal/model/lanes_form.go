package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	ModeLanesForm Mode   = "lanes-form"
	LanesSubmit   Action = "lanes-submit"
	LanesCancel   Action = "lanes-cancel"
)

// The Lanes card's fields.
const (
	LanesNames Field = "names"
	LanesDays  Field = "days"
	LanesCards Field = "cards"
)

// LanesFields is the Lanes card's field list, in tab order.
var LanesFields = []FieldSpec{{LanesNames, true}, {LanesDays, true}, {LanesCards, true}}

// LanesFormState is the Lanes card: the Lane list typed as comma-separated names, then Done's days and card count. Saving
// writes the Lane list file only.
type LanesFormState struct {
	Open, Saving bool
	Field        Field
	// Text is the Lane list; Days and Cards are Done's limits, blank for the default.
	Text, Days, Cards string
	Caret             Caret
	Err               error
	// Sent is the save the card is waiting on, so only its own result closes or fails the card.
	Sent SaveLanes
}

// at is the text of the field called name, nil for a name that is not a Lanes field.
func (f *LanesFormState) at(name Field) *string {
	switch name {
	case LanesNames:
		return &f.Text
	case LanesDays:
		return &f.Days
	case LanesCards:
		return &f.Cards
	}
	return nil
}

// Value is the text the field called name holds.
func (f LanesFormState) Value(name Field) string {
	if text := f.at(name); text != nil {
		return *text
	}
	return ""
}

// openLanesForm is `B`. It waits for the saved Lane list to load: a card prefilled with the default would overwrite it on save.
func openLanesForm(m Model) Model {
	if !m.Board.loaded {
		m.Notice = "The Lane list is still loading."
		return m
	}
	if m.Board.fileErr != nil {
		m.Notice = fmt.Sprintf("Lanes not opened: %v", m.Board.fileErr)
		return m
	}
	m.LanesForm = LanesFormState{Open: true, Field: LanesNames, Text: strings.Join(m.LaneList(), ", ")}
	if m.Board.DoneDays > 0 {
		m.LanesForm.Days = strconv.Itoa(m.Board.DoneDays)
	}
	if m.Board.DoneCards > 0 {
		m.LanesForm.Cards = strconv.Itoa(m.Board.DoneCards)
	}
	return m
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
			return nil, fmt.Errorf("%q %v.", shortEntry(entry), err)
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
		if m.Board.fileErr != nil {
			form.Err = fmt.Errorf("Lanes not saved: %v", m.Board.fileErr)
			return m, nil
		}
		form.Saving, form.Err = true, nil
		return m.SaveLaneList(lanes, days, cards)
	case bound && binding.Action == LanesCancel:
		m.LanesForm = LanesFormState{}
	default:
		text := form.at(form.Field)
		var edited bool
		if *text, edited = editText(*text, &form.Caret, form.Field, binding, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
