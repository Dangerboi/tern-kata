package model_test

import (
	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// withDetail shows the selected Issue's loaded authority the way a user sees it: the step asks for the detail and the daemon
// answers.
func withDetail(m model.Model, authority *g.UISelectedAuthority) model.Model {
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
