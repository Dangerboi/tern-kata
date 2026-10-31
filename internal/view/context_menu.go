package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// contextMenuRow is one row of an Issue's context menu: the action whose key it runs, its label and, from the Pane's keymap, the
// keycaps and chord it shows and presses.
type contextMenuRow struct {
	action model.Action
	label  string
	keys   []string
	chord  string
}

// contextMenuKeys are the context menu rows of every Issue shown in mode, with the keys of each worked out once: Open, Agent…, Close,
// Reopen, Priority…, Dates… and Move to Project. A row's keycaps are its action's key labels, so a rebind moves them; an action with
// no key in mode has no row.
type contextMenuKeys struct {
	mode model.Mode
	rows []contextMenuRow
}

// newContextMenuKeys is the menu rows for mode; open is the action that Open runs.
func newContextMenuKeys(mode model.Mode, open model.Action) contextMenuKeys {
	menu := contextMenuKeys{mode: mode}
	for _, row := range []contextMenuRow{
		{action: open, label: "Open"},
		{action: model.OpenAgentCardAction, label: "Agent…"},
		{action: model.OpenClose, label: "Close"},
		{action: model.ReopenAction, label: "Reopen"},
		{action: model.PriorityAction, label: "Priority…"},
		{action: model.OpenDates, label: "Dates…"},
		{action: model.OpenMoveProject, label: "Move to Project"},
	} {
		if row.keys, row.chord = cardKeys(mode, row.action); row.chord != "" {
			menu.rows = append(menu.rows, row)
		}
	}
	return menu
}

// of is the right-click menu of issue's row or card. Each row's id picks the Issue and runs the chord of the action's first key in
// the menu's mode, and a row the model refuses on this Issue is off with its reason. Reopen is there only on a closed Issue.
func (k contextMenuKeys) of(issue model.Issue) []ui.MenuEntry {
	closed := model.IsClosed(issue)
	menu := make([]ui.MenuEntry, 0, len(k.rows))
	for _, row := range k.rows {
		if row.action == model.ReopenAction && !closed {
			continue
		}
		item := ui.MenuItem{ID: ContextMenuPickClick(issue.UID, k.mode, row.chord), Label: row.label, Keys: row.keys}
		if reason := model.IssueRefusal(issue, row.action); reason != "" {
			item.Disabled = ui.Because(reason)
		}
		menu = append(menu, item)
	}
	return menu
}

// rowMode is the mode the list's rows are keyed in: the Lane view's or the list's.
func rowMode(m model.Model) model.Mode {
	if m.LaneViewShown() {
		return model.ModeLane
	}
	return model.ModeList
}
