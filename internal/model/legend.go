package model

// ToggleLegendMsg is a tap on the keys chip of the key legend under FoldedLegendCols: it unfolds the legend, or folds it again.
type ToggleLegendMsg struct{}

func (ToggleLegendMsg) msg() {}

// LegendFolded reports whether the key legend shows only the counter, esc and the keys chip. From FoldedLegendCols up the legend
// is always the full grid and LegendOpen is ignored.
func (m Model) LegendFolded() bool {
	return m.Cols < FoldedLegendCols && !m.LegendOpen
}

// LegendChip reports whether the legend has its keys chip.
func (m Model) LegendChip() bool {
	return m.Cols < FoldedLegendCols
}

func toggleLegend(m Model) Model {
	m.LegendOpen = !m.LegendOpen
	m.legendMode = m.CurrentMode()
	return m
}

// foldLegend closes an open legend once a key (a tap on a hint or a card's button arrives as its key) has run, or the Pane has
// changed mode or grown past the width the legend folds at.
func foldLegend(msg Msg, next Model) Model {
	if !next.LegendOpen {
		return next
	}
	switch msg.(type) {
	case KeyMsg:
		next.LegendOpen = false
	default:
		if next.Cols >= FoldedLegendCols || next.CurrentMode() != next.legendMode {
			next.LegendOpen = false
		}
	}
	return next
}
