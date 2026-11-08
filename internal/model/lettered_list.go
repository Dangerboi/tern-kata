package model

// What the Lettered Pages that list rows (Credentials, Federation, Daemons, Config) share: the mode, the back key, the cursor.

func sidebarMode(mode Mode) func(Model) Mode {
	return func(m Model) Mode {
		if m.Focus == ColumnCollections {
			return ""
		}
		return mode
	}
}

func letteredBack(m Model) (Model, []Cmd) { return showPage(m.letteredFrom)(m) }

// moveCursor moves a list's cursor by action: one row, to either end, or a page. An empty list has no cursor to move.
func moveCursor(cursor *int, n int, action Action) {
	if n > 0 {
		*cursor = movedIndex(*cursor, n, listPage, action)
	}
}

func rowAt[T any](rows []T, cursor int) *T {
	if cursor < 0 || cursor >= len(rows) {
		return nil
	}
	return &rows[cursor]
}

func letteredBackRow(mode Mode, handle func(Model) (Model, []Cmd)) actionEntry {
	return actionEntry{action: LetteredBack, modes: []Mode{mode}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{mode},
		sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: handle}
}
