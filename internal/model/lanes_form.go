package model

import (
	"fmt"
	"slices"
	"strings"
)

const (
	ModeLanesForm Mode   = "lanes-form"
	LanesSubmit   Action = "lanes-submit"
	LanesCancel   Action = "lanes-cancel"
)

// LanesFormState is the Lanes card: the Lane list typed as comma-separated names. Saving writes the Lane list file only.
type LanesFormState struct {
	Open bool
	Text string
	Err  error
}

// openLanesForm is `B`. It waits for the saved Lane list to load: a card prefilled with the default would overwrite it on save.
func openLanesForm(m Model) Model {
	if !m.Board.loaded {
		m.Notice = "The Lane list is still loading."
		return m
	}
	m.LanesForm = LanesFormState{Open: true, Text: strings.Join(m.LaneList(), ", ")}
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
	case bound && binding.Action == LanesSubmit:
		lanes, err := parseLanes(form.Text)
		if err != nil {
			form.Err = err
			return m, nil
		}
		m.LanesForm = LanesFormState{}
		return m.SaveLaneList(lanes)
	case bound && binding.Action == LanesCancel:
		m.LanesForm = LanesFormState{}
	default:
		var edited bool
		if form.Text, edited = editText(form.Text, binding, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
