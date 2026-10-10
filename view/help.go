package view

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// helpTitles name each group's card; the empty mode is global.
var helpTitles = map[model.Mode]string{
	"":                      "Everywhere",
	model.ModeList:          "Issue list",
	model.ModeDetail:        "Issue detail",
	model.ModeCollections:   "Menu sidebar",
	model.ModePicker:        "Menu",
	model.ModeSearch:        "Search",
	model.ModeSearchResults: "Search results",
	model.ModeFilterForm:    "Filters",
	model.ModeCloseForm:     "Close Issue",
	model.ModeCreate:        "Create Issue",
	model.ModeEditor:        "Draft editor",
	model.ModeConflict:      "Draft conflict",
	model.ModeDiscard:       "Discard prompt",
	model.ModeFieldForm:     "Label / owner / priority",
	model.ModeProjectForm:   "New Project",
	model.ModeHelp:          "Help",
	model.ModeError:         "Error",
}

type helpGroup struct {
	mode model.Mode
	rows []ui.TableRow
}

// help is the keybinding sheet that fills the Pane while help is open: the groups side by side in a
// wrapping row, each a title over `key  action` lines with keys padded to the group's longest key.
func help(m model.Model) tern.Nodes {
	columns := make([]tern.Element, 0, len(model.Keymap))
	for gi, group := range helpGroups() {
		name := helpTitles[group.mode]
		if name == "" {
			name = string(group.mode)
		}
		width := 0
		for _, row := range group.rows {
			width = max(width, utf8.RuneCountInString(row.Cells["key"].(ui.Rich).String()))
		}
		lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "title", Tone: ui.ToneAccent}, Text: name})
		for _, row := range group.rows {
			key := row.Cells["key"].(ui.Rich).String()
			line := key + strings.Repeat(" ", width-utf8.RuneCountInString(key)) + "  " + row.Cells["action"].(ui.Rich).String()
			lines = append(lines, ui.Text{Common: ui.Common{Key: "row-" + row.ID}, Text: line, Wrap: ui.WrapNone})
		}
		columns = append(columns, ui.Col{Common: ui.Common{Key: "group-" + strconv.Itoa(gi)}, Children: lines})
	}
	return ui.Nodes(ui.List{CommonWithoutMax: scrollerCommon("help"), Max: ui.MaxFrac(1), Children: ui.Nodes(ui.Col{
		Common: ui.Common{Key: "sheet"},
		Gap:    ui.GapMD,
		Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title"}, Text: "tern-kata — keybindings"},
			ui.Row{Common: ui.Common{Key: "groups"}, Gap: ui.GapLG, Align: ui.AlignStart, Wrap: true, Children: columns},
			ui.Text{Common: ui.Common{Key: "return", Tone: ui.ToneMuted}, Text: "press ? to return"},
		),
	})})
}

// helpGroups groups the Keymap by mode, global first, merging bindings that
// share an action and help into one row; Tern-headless-only chords are left out.
func helpGroups() []helpGroup {
	var groups []helpGroup
	index := map[model.Mode]int{}
	rowOf := map[string]int{}
	add := func(mode model.Mode) int {
		if i, ok := index[mode]; ok {
			return i
		}
		groups = append(groups, helpGroup{mode: mode})
		index[mode] = len(groups) - 1
		return len(groups) - 1
	}
	add("")
	for _, binding := range model.Keymap {
		if strings.Contains(binding.Help, "Tern headless") {
			continue
		}
		gi := add(binding.Mode)
		id := string(binding.Action) + "|" + binding.Help
		rid := strconv.Itoa(gi) + "|" + id
		if ri, ok := rowOf[rid]; ok {
			row := groups[gi].rows[ri]
			keys := strings.Split(row.Cells["key"].(ui.Rich).String(), "/")
			for _, k := range strings.Split(binding.Key, "/") {
				if !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
			row.Cells["key"] = ui.T(strings.Join(keys, "/"))
			continue
		}
		rowOf[rid] = len(groups[gi].rows)
		groups[gi].rows = append(groups[gi].rows, ui.TableRow{
			ID:    strconv.Itoa(len(groups[gi].rows)),
			Cells: map[string]ui.Cell{"key": ui.T(binding.Key), "action": ui.T(binding.Help)},
		})
	}
	return groups
}
