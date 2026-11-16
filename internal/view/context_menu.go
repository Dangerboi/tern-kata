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

// contextMenuKeys are the rows of one menu with their key caps and chords, read from the bindings of mode once: a list, a Board or
// a detail draws many menus of the one set.
type contextMenuKeys struct {
	mode model.Mode
	rows []contextMenuRow
}

func newMenuKeys(mode model.Mode, rows []contextMenuRow) contextMenuKeys {
	menu := contextMenuKeys{mode: mode}
	for _, row := range rows {
		if row.keys, row.chord = cardKeys(mode, row.action); row.chord != "" {
			menu.rows = append(menu.rows, row)
		}
	}
	return menu
}

func newContextMenuKeys(mode model.Mode, open model.Action) contextMenuKeys {
	return newMenuKeys(mode, []contextMenuRow{
		{action: open, label: "Open"},
		{action: model.OpenAgentCardAction, label: "Agent…"},
		{action: model.OpenClose, label: "Close"},
		{action: model.ReopenAction, label: "Reopen"},
		{action: model.PriorityAction, label: "Priority…"},
		{action: model.OpenDates, label: "Dates…"},
		{action: model.OpenMoveProject, label: "Move to Project"},
	})
}

// entries lists the rows of the menu of the Issue uid, a row of its detail when row is set. refusal says why a row's action is
// disabled, "" when it is on offer; skip leaves a row out.
func (k contextMenuKeys) entries(uid, row string, refusal func(model.Action) string, skip func(model.Action) bool) []ui.MenuEntry {
	menu := make([]ui.MenuEntry, 0, len(k.rows))
	for _, r := range k.rows {
		if skip != nil && skip(r.action) {
			continue
		}
		item := ui.MenuItem{ID: ContextMenuPickClick(uid, k.mode, row, r.chord), Label: r.label, Keys: r.keys}
		if reason := refusal(r.action); reason != "" {
			item.Disabled = ui.Because(reason)
		}
		menu = append(menu, item)
	}
	return menu
}

func (k contextMenuKeys) of(issue model.Issue) []ui.MenuEntry {
	closed := model.IsClosed(issue)
	return k.entries(issue.UID, "", func(action model.Action) string { return model.IssueRefusal(issue, action) },
		func(action model.Action) bool { return action == model.ReopenAction && !closed })
}

func rowMode(m model.Model) model.Mode {
	if m.LaneViewShown() {
		return model.ModeLane
	}
	return model.ModeList
}
