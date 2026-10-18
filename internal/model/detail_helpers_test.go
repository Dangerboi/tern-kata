package model_test

import (
	"github.com/Dangerboi/tern-kata/internal/model"
)

// cursorKey is the key of the Issue detail's row under the cursor, "" when the cursor is on no row.
func cursorKey(m model.Model) string {
	rows := m.DetailRows()
	if i := m.DetailIndex(); i >= 0 && i < len(rows) {
		return rows[i].Key
	}
	return ""
}

// activityRow is the i-th row of a tab as the Pane lists it: the tab is shown, then its rows are read from the detail's rows.
func activityRow(m model.Model, tab model.ActivityTab, i int) (model.DetailRow, bool) {
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: tab})
	var rows []model.DetailRow
	for _, item := range m.DetailRows() {
		if item.Kind != model.RowChild {
			rows = append(rows, item.Activity)
		}
	}
	if i < 0 || i >= len(rows) {
		return model.DetailRow{}, false
	}
	return rows[i], true
}

// withDetail shows the selected Issue's loaded authority the way a user sees it: the step asks for the detail and the daemon
// answers.
func withDetail(m model.Model, authority *model.IssueDetailData) model.Model {
	m, _ = model.Update(m, model.ResizeMsg{Cols: m.Cols})
	m, _ = model.Update(m, model.DetailLoaded{UID: m.SelectedUID, Selected: authority})
	return m
}

// cursorAt walks the focused detail's row cursor to index.
func cursorAt(m model.Model, index int) model.Model {
	m, _ = model.Update(m, model.KeyMsg{Chord: "g"})
	for range index {
		m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	}
	return m
}
