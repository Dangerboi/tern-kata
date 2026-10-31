package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// The reasons a context menu row is off.
const (
	closedReason = "The Issue is already closed"
	agentReason  = "The Issue is closed; reopen it first"
)

// menuRow is one row of an Issue's context menu: the action whose key it presses, its label and whether it is off.
type menuRow struct {
	action   model.Action
	label    string
	disabled ui.Disabled
}

// issueMenu is the right-click menu of an Issue's row or card in mode: Open, Agent…, Close, Reopen, Priority…, Dates… and Move to
// Project. Each row's id picks the Issue and presses the chord of the action's first key in mode, as a card button does, and its
// keycaps are the action's key labels, so a rebind moves both. An action with no key in mode has no row.
func issueMenu(mode model.Mode, issue model.Issue, open model.Action) []ui.MenuEntry {
	closed := model.IsClosed(issue)
	rows := []menuRow{
		{open, "Open", ui.Disabled{}},
		{model.OpenAgentCardAction, "Agent…", reasonIf(closed, agentReason)},
		{model.OpenClose, "Close", reasonIf(closed, closedReason)},
	}
	if closed {
		rows = append(rows, menuRow{model.ReopenAction, "Reopen", ui.Disabled{}})
	}
	rows = append(rows,
		menuRow{model.PriorityAction, "Priority…", ui.Disabled{}},
		menuRow{model.OpenDates, "Dates…", ui.Disabled{}},
		menuRow{model.OpenMoveProject, "Move to Project", ui.Disabled{}},
	)
	menu := make([]ui.MenuEntry, 0, len(rows))
	for _, row := range rows {
		bindings := model.CardKeys(mode, row.action)
		if len(bindings) == 0 {
			continue
		}
		keys := make([]string, len(bindings))
		for i, b := range bindings {
			keys[i] = b.Key
		}
		menu = append(menu, ui.MenuItem{ID: MenuPickClick(issue.UID, bindings[0].Chord), Label: row.label, Keys: keys, Disabled: row.disabled})
	}
	return menu
}

func reasonIf(on bool, reason string) ui.Disabled {
	if on {
		return ui.Because(reason)
	}
	return ui.Disabled{}
}

// rowMode is the mode the list's rows are keyed in: the Lane view's or the list's.
func rowMode(m model.Model) model.Mode {
	if m.LaneViewShown() {
		return model.ModeLane
	}
	return model.ModeList
}
