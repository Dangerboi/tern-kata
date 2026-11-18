package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type contextMenuRow struct {
	action model.Action
	label  string
	// group numbers the rows that belong together: a separator is drawn where it changes.
	group int
	keys  []string
	chord string
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

// newContextMenuKeys lists the rows of an Issue's menu in their groups. A row ends in an ellipsis only where it opens a card; a
// row whose action has no binding in mode is left out. Delete is not here: a destructive row in a quick menu invites a
// misclick, so it stays a key.
func newContextMenuKeys(mode model.Mode, open model.Action) contextMenuKeys {
	return newMenuKeys(mode, []contextMenuRow{
		{action: open, label: "Open", group: 0},
		{action: model.OpenClose, label: "Close…", group: 1},
		{action: model.ReopenAction, label: "Reopen", group: 1},
		{action: model.PriorityAction, label: "Priority…", group: 2},
		{action: model.OpenDates, label: "Dates…", group: 2},
		{action: model.AssignOwnerAction, label: "Owner…", group: 2},
		{action: model.AddLabelAction, label: "Labels…", group: 2},
		{action: model.OpenMoveProject, label: "Move to Project…", group: 3},
		{action: model.OpenAgentCardAction, label: "Agents…", group: 3},
		{action: model.NewChild, label: "New child…", group: 4},
	})
}

// entries lists the rows of the menu of the Issue uid, a row of its detail when row is set. refusal says why a row's action is
// disabled, "" when it is on offer; skip leaves a row out.
func (k contextMenuKeys) entries(uid, row string, refusal func(model.Action) string, skip func(model.Action) bool) []ui.MenuEntry {
	menu := make([]ui.MenuEntry, 0, len(k.rows)+4)
	group := -1
	for _, r := range k.rows {
		if skip != nil && skip(r.action) {
			continue
		}
		if group >= 0 && r.group != group {
			menu = append(menu, ui.MenuSep)
		}
		group = r.group
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
