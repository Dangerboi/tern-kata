package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type contextMenuRow struct {
	action model.Action
	label  string
	keys   []string
	chord  string
}

type contextMenuKeys struct {
	mode model.Mode
	rows []contextMenuRow
}

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

func rowMode(m model.Model) model.Mode {
	if m.LaneViewShown() {
		return model.ModeLane
	}
	return model.ModeList
}
