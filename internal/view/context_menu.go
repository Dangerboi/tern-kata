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

// menuLine is one row of an Issue's menu, or a separator between its groups: act is what a pick sends, reason why the row is
// disabled ("" when it is on offer).
type menuLine struct {
	sep                bool
	act, label, reason string
	keys               []string
}

// lines lists the rows of the menu of the Issue uid, a row of its detail when row is set. refusal says why a row's action is
// disabled, "" when it is on offer; skip leaves a row out.
func (k contextMenuKeys) lines(uid, row string, refusal func(model.Action) string, skip func(model.Action) bool) []menuLine {
	lines := make([]menuLine, 0, len(k.rows)+4)
	group := -1
	for _, r := range k.rows {
		if skip != nil && skip(r.action) {
			continue
		}
		if group >= 0 && r.group != group {
			lines = append(lines, menuLine{sep: true})
		}
		group = r.group
		lines = append(lines, menuLine{act: ContextMenuPickClick(uid, k.mode, row, r.chord), label: r.label, keys: r.keys, reason: refusal(r.action)})
	}
	return lines
}

// entries are lines as the rows of Tern's own menus.
func (k contextMenuKeys) entries(uid, row string, refusal func(model.Action) string, skip func(model.Action) bool) []ui.MenuEntry {
	return menuEntries(k.lines(uid, row, refusal, skip))
}

func menuEntries(lines []menuLine) []ui.MenuEntry {
	menu := make([]ui.MenuEntry, len(lines))
	for i, line := range lines {
		if line.sep {
			menu[i] = ui.MenuSep
			continue
		}
		item := ui.MenuItem{ID: line.act, Label: line.label, Keys: line.keys}
		if line.reason != "" {
			item.Disabled = ui.Because(line.reason)
		}
		menu[i] = item
	}
	return menu
}

// issueLines are the rows of an Issue's own menu: Reopen only for a closed Issue.
func (k contextMenuKeys) issueLines(issue model.Issue) []menuLine {
	closed := model.IsClosed(issue)
	return k.lines(issue.UID, "", func(action model.Action) string { return model.IssueRefusal(issue, action) },
		func(action model.Action) bool { return action == model.ReopenAction && !closed })
}

func (k contextMenuKeys) of(issue model.Issue) []ui.MenuEntry {
	return menuEntries(k.issueLines(issue))
}

func rowMode(m model.Model) model.Mode {
	if m.LaneViewShown() {
		return model.ModeLane
	}
	return model.ModeList
}
