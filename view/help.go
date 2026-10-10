package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type helpGroup struct {
	mode model.Mode
	rows []ui.TableRow
}

func help(m model.Model) tern.Element {
	if !m.HelpOpen {
		return nil
	}
	children := make([]tern.Element, 0, 2*len(model.Keymap)+1)
	for gi, group := range helpGroups() {
		name := string(group.mode)
		if name == "" {
			name = "Everywhere"
		}
		children = append(children,
			ui.Text{Common: ui.Common{Key: "group-" + strconv.Itoa(gi), Tone: ui.ToneAccent}, Text: name},
			ui.Table{
				Common: ui.Common{Key: "table-" + strconv.Itoa(gi)},
				Cols:   []ui.Column{{ID: "key", Head: ui.T("key")}, {ID: "action", Head: ui.T("action"), Grow: 1}},
				Rows:   group.rows,
			})
	}
	children = append(children, el.Button{Common: ui.Common{Key: "close", Actions: &ui.Actions{Click: "key=escape"}}, Text: "Close (esc)"})
	return ui.Nodes(ui.Overlay{
		Common:   ui.Common{Key: "help"},
		Modal:    true,
		Head:     ui.T("Keyboard help"),
		Size:     ui.SizeLG,
		Children: children,
	})
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
				if !contains(keys, k) {
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

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
