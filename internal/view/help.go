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

// from are the Pages the card opens from, the only ones whose sheet lists the line.
var cardHelp = map[string][]struct {
	text    string
	mode    model.Mode
	actions []model.Action
	from    []model.Page
}{
	"Forms": {
		{"save or apply", model.ModeCreate, []model.Action{model.SaveDraftAction}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"cancel", model.ModeCreate, []model.Action{model.CancelDraft}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"change field", model.ModeCreate, []model.Action{model.NextField, model.PreviousField}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"open editor", model.ModeEditor, []model.Action{model.ExternalEditorAction}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"cycle reply kind", model.ModeEditor, []model.Action{model.CycleReplyKind}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"send reply anyway", model.ModeEditor, []model.Action{model.SendReplyAnyway}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
	},
	"Filters": {
		{"reset filter form", model.ModeFilterForm, []model.Action{model.ResetFilterForm}, []model.Page{model.PageIssues, model.PageBoard}},
		{"save as default filter", model.ModeFilterForm, []model.Action{model.SaveFilterDefault}, []model.Page{model.PageIssues, model.PageBoard}},
	},
}

func helpGroups(m model.Model) []model.SheetGroup {
	page := m.Page
	groups := model.HelpSheetOn(page, m)
	for i := range groups {
		for _, card := range cardHelp[groups[i].Title] {
			if !slices.Contains(card.from, page) {
				continue
			}
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

const helpColumnCols = 64

func help(m model.Model) tern.Nodes {
	var groups []model.SheetGroup
	for _, group := range helpGroups(m) {
		if len(group.Lines) > 0 {
			groups = append(groups, group)
		}
	}
	count := min(max(m.Cols/helpColumnCols, 1), 3, max(len(groups), 1))
	stacks := make([][]int, count)
	heights := make([]int, count)
	widths := make([]int, count)
	for gi, group := range groups {
		shortest := 0
		for i := range heights {
			if heights[i] < heights[shortest] {
				shortest = i
			}
		}
		stacks[shortest] = append(stacks[shortest], gi)
		heights[shortest] += len(group.Lines) + 2 // the head and the gap below the card
		for _, l := range group.Lines {
			widths[shortest] = max(widths[shortest], utf8.RuneCountInString(strings.Join(l.Keys, "/")))
		}
	}
	columns := make([]tern.Element, count)
	for i, stack := range stacks {
		cards := make([]tern.Element, len(stack))
		for j, gi := range stack {
			cards[j] = helpCard("group-"+strconv.Itoa(gi), groups[gi], widths[i])
		}
		columns[i] = ui.Col{Common: ui.Common{Key: "col-" + strconv.Itoa(i), Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{W: ui.Ch(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: cards}
	}
	return ui.Nodes(ui.List{CommonWithoutMax: scrollerCommon("help"), Max: ui.MaxFrac(1), Children: ui.Nodes(ui.Col{
		Common: ui.Common{Key: "sheet"},
		Gap:    ui.GapMD,
		Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title"}, Text: "tern-kata — keybindings"},
			ui.Row{Common: ui.Common{Key: "groups"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: columns},
			ui.Text{Common: ui.Common{Key: "return"}, Spans: []ui.Span{{T: "press ? or esc to return", S: "muted"}}},
		),
	})})
}

func helpCard(key string, group model.SheetGroup, keyCols int) tern.Element {
	rows := make([]tern.Element, len(group.Lines))
	for i, l := range group.Lines {
		rows[i] = ui.Row{Common: ui.Common{Key: "row-" + strconv.Itoa(i)}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "keys", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(float64(keyCols))}}, Spans: []ui.Span{{T: strings.Join(l.Keys, "/"), S: mono("accent")}}, Wrap: ui.WrapNone},
			ui.Text{Common: ui.Common{Key: "action", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Text: l.Text},
		)}
	}
	return ui.Card{Common: ui.Common{Key: key}, Head: ui.T(group.Title), Children: rows}
}
