package model

import (
	"errors"
	"fmt"
)

// A Project's default filter is read with the Project when the Pane starts (ProjectLoaded.Default) and written from the Filter
// card. Search text is never part of it.

// SaveFilters asks for Default to be written as the Project's default filter; ProjectName is written beside it for readers of
// the file. An all-empty Default removes it.
type SaveFilters struct {
	ProjectUID, ProjectName string
	Default                 Filters
}

func (SaveFilters) cmd() {}

// FiltersSaved answers SaveFilters.
type FiltersSaved struct {
	ProjectUID string
	Default    Filters
	Err        error
}

func (FiltersSaved) msg() {}

// saveFilterDefault is ctrl+d in the Filter card: it sends the card's values (without Search) as the bound Project's default and
// waits, the card staying open, for the answer.
func saveFilterDefault(m Model) (Model, []Cmd) {
	if m.Project.UID == "" {
		m.FilterForm.Err = errors.New("This folder is bound to no Project, so there is no default filter to save.")
		return m, nil
	}
	values := trimmedFilters(m.FilterForm.Values)
	values.Search = ""
	save := SaveFilters{ProjectUID: m.Project.UID, ProjectName: m.Project.Name, Default: values}
	m.FilterForm.Saving, m.FilterForm.Err, m.FilterForm.Sent = true, nil, save
	return m, []Cmd{save}
}

func filtersSaved(m Model, msg FiltersSaved) (Model, []Cmd) {
	form := m.FilterForm
	if !form.Open || !form.Saving || form.Sent.ProjectUID != msg.ProjectUID || form.Sent.Default != msg.Default {
		if msg.Err == nil && msg.ProjectUID == m.Project.UID {
			m.DefaultFilters = msg.Default // the file holds it now, whichever card asked
		}
		return m, nil
	}
	if msg.Err != nil {
		m.FilterForm.Saving, m.FilterForm.Err = false, fmt.Errorf("Default filter not saved: %w", msg.Err)
		return m, nil
	}
	m.DefaultFilters = msg.Default
	m.Filters = msg.Default
	m.Filters.Search = form.Values.Search
	m.FilterForm = FilterFormState{}
	m.Notice = fmt.Sprintf("Saved as %s's default filter", m.Project.Name)
	if msg.Default == (Filters{}) {
		m.Notice = fmt.Sprintf("Removed %s's default filter", m.Project.Name)
	}
	m.SelectedUID = ""
	return snapshotChanged(m)
}
