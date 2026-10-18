package view

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// cardHelp is the lines of the help sheet that come from the Popup cards' own keys, by group title: the Keymap bindings
// (mode + actions) that supply the keys, in the order shown after the group's main-surface lines.
var cardHelp = map[string][]struct {
	text    string
	mode    model.Mode
	actions []model.Action
}{
	"Forms": {
		{"save or apply", model.ModeCreate, []model.Action{model.SaveDraftAction}},
		{"cancel", model.ModeCreate, []model.Action{model.CancelDraft}},
		{"change field", model.ModeCreate, []model.Action{model.CreateNext, model.CreatePrevious}},
		{"open editor", model.ModeEditor, []model.Action{model.ExternalEditorAction}},
		{"cycle reply kind", model.ModeEditor, []model.Action{model.CycleReplyKind}},
		{"send reply anyway", model.ModeEditor, []model.Action{model.SendReplyAnyway}},
	},
	"Filters": {
		{"reset filter form", model.ModeFilterForm, []model.Action{model.ResetFilterForm}},
	},
}

// helpGroups is the sheet: the main-surface groups from the action entries, each followed by its card lines.
func helpGroups() []model.SheetGroup {
	groups := model.HelpSheet()
	for i := range groups {
		for _, card := range cardHelp[groups[i].Title] {
			var keys []string
			for _, action := range card.actions {
				for _, b := range model.Keymap {
					if b.Mode != card.mode || b.Action != action || b.Headless() {
						continue
					}
					for _, k := range strings.Split(b.Key, "/") {
						if !slices.Contains(keys, k) {
							keys = append(keys, k)
						}
					}
				}
			}
			if len(keys) > 0 {
				groups[i].Lines = append(groups[i].Lines, model.SheetLine{Text: card.text, Keys: keys})
			}
		}
	}
	return groups
}

// help is the keybinding sheet that fills the Pane while help is open: kata's groups side by side in a
// wrapping row, each a title over `key  action` lines with keys padded to the group's longest key.
func help(m model.Model) tern.Nodes {
	groups := helpGroups()
	columns := make([]tern.Element, 0, len(groups))
	for gi, group := range groups {
		if len(group.Lines) == 0 {
			continue
		}
		keys := make([]string, len(group.Lines))
		width := 0
		for i, l := range group.Lines {
			keys[i] = strings.Join(l.Keys, "/")
			width = max(width, utf8.RuneCountInString(keys[i]))
		}
		lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "title", Tone: ui.ToneAccent}, Text: group.Title})
		for i, l := range group.Lines {
			line := keys[i] + strings.Repeat(" ", width-utf8.RuneCountInString(keys[i])) + "  " + l.Text
			lines = append(lines, ui.Text{Common: ui.Common{Key: "row-" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone})
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
