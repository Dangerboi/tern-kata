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

// helpEntry is one line of kata's own help: its wording, and the Keymap bindings (mode + actions)
// that supply the keys. An entry with no binding in the Keymap is not shown.
type helpEntry struct {
	text    string
	mode    model.Mode
	actions []model.Action
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

// helpTable mirrors the groups, order and wording of kata's TUI help.
var helpTable = []helpGroup{
	{"Global", []helpEntry{
		{"help", "", []model.Action{model.ToggleHelp}},
		{"quit", "", []model.Action{model.QuitAction}},
		{"inbox", model.ModeList, []model.Action{model.OpenInbox}},
		{"projects", model.ModeList, []model.Action{model.OpenProjects}},
		{"menu", model.ModeList, []model.Action{model.DrillOut}},
		{"toggle layout", model.ModeList, []model.Action{model.ToggleDetail}},
	}},
	{"Graph", []helpEntry{
		{"up", model.ModeList, []model.Action{model.MoveUp}},
		{"down", model.ModeList, []model.Action{model.MoveDown}},
		{"page up", model.ModeList, []model.Action{model.PageUp}},
		{"page down", model.ModeList, []model.Action{model.PageDown}},
		{"first", model.ModeList, []model.Action{model.MoveFirst}},
		{"last", model.ModeList, []model.Action{model.MoveLast}},
		{"open detail", model.ModeList, []model.Action{model.DrillIn}},
		{"expand/collapse", model.ModeList, []model.Action{model.ToggleExpansion}},
		{"expand", model.ModeList, []model.Action{model.ExpandIssue}},
		{"collapse", model.ModeList, []model.Action{model.CollapseIssue}},
		{"expand/collapse all", model.ModeList, []model.Action{model.ToggleAllExpansion}},
		{"new issue (form)", model.ModeList, []model.Action{model.NewIssue}},
		{"toggle flat/nested view", model.ModeList, []model.Action{model.ToggleIssueView}},
		{"close", model.ModeList, []model.Action{model.OpenClose}},
		{"reopen", model.ModeList, []model.Action{model.ReopenAction}},
	}},
	{"Detail", []helpEntry{
		{"next tab", model.ModeDetail, []model.Action{model.NextActivityTab}},
		{"prev tab", model.ModeDetail, []model.Action{model.PreviousActivityTab}},
		{"jump to referenced issue", model.ModeDetail, []model.Action{model.DrillIn}},
		{"back", model.ModeDetail, []model.Action{model.DrillOut}},
		{"edit body", model.ModeDetail, []model.Action{model.EditBody}},
		{"new comment", model.ModeDetail, []model.Action{model.AddComment}},
		{"set parent", model.ModeDetail, []model.Action{model.SetParentAction}},
		{"add label", model.ModeDetail, []model.Action{model.AddLabelAction}},
		{"remove label", model.ModeDetail, []model.Action{model.RemoveLabelAction}},
		{"assign owner", model.ModeDetail, []model.Action{model.AssignOwnerAction}},
		{"clear owner", model.ModeDetail, []model.Action{model.ClearOwnerAction}},
		{"set priority", model.ModeDetail, []model.Action{model.PriorityAction}},
	}},
	{"Children", []helpEntry{
		{"new child", model.ModeDetail, []model.Action{model.NewChild}},
		{"move child cursor", model.ModeDetail, []model.Action{model.MoveDown, model.MoveUp}},
		{"scroll detail viewport", model.ModeDetail, []model.Action{model.ScrollUp, model.ScrollDown}},
		{"open child", model.ModeDetail, []model.Action{model.DrillIn}},
	}},
	{"Forms", []helpEntry{
		{"save or apply", model.ModeCreate, []model.Action{model.SaveDraftAction}},
		{"cancel", model.ModeCreate, []model.Action{model.CancelDraft}},
		{"change field", model.ModeCreate, []model.Action{model.CreateNext, model.CreatePrevious}},
		{"open editor", model.ModeEditor, []model.Action{model.ExternalEditorAction}},
	}},
	{"Filters", []helpEntry{
		{"search", model.ModeList, []model.Action{model.SearchIssues}},
		{"cycle status filter", model.ModeList, []model.Action{model.CycleStatus}},
		{"filter (form)", model.ModeList, []model.Action{model.OpenFilterForm}},
		{"clear filters", model.ModeList, []model.Action{model.ClearFilters}},
		{"reset filter form", model.ModeFilterForm, []model.Action{model.ResetFilterForm}},
	}},
}

// keys is the entry's keys from the Keymap, in action order, "" when nothing is bound.
// Tern-headless-only chords are left out, and so are chords with no label.
func (e helpEntry) keys() string {
	var keys []string
	for _, action := range e.actions {
		for _, b := range model.Keymap {
			if b.Mode != e.mode || b.Action != action || strings.Contains(b.Help, "Tern headless") {
				continue
			}
			for _, k := range strings.Split(b.Key, "/") {
				if k != "" && !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
		}
	}
	return strings.Join(keys, "/")
}

// help is the keybinding sheet that fills the Pane while help is open: kata's groups side by side in a
// wrapping row, each a title over `key  action` lines with keys padded to the group's longest key.
func help(m model.Model) tern.Nodes {
	columns := make([]tern.Element, 0, len(helpTable))
	for gi, group := range helpTable {
		var keys, rows []string
		width := 0
		for _, e := range group.entries {
			if k := e.keys(); k != "" {
				keys, rows = append(keys, k), append(rows, e.text)
				width = max(width, utf8.RuneCountInString(k))
			}
		}
		if len(rows) == 0 {
			continue
		}
		lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "title", Tone: ui.ToneAccent}, Text: group.title})
		for i, text := range rows {
			line := keys[i] + strings.Repeat(" ", width-utf8.RuneCountInString(keys[i])) + "  " + text
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
