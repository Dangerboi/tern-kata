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

// cardHelp are the lines of the Cards and Filter and search groups that the card modes add, by group title. from are the Pages the
// card opens from, the only ones whose sheet lists the line.
var cardHelp = map[string][]struct {
	text    string
	mode    model.Mode
	actions []model.Action
	from    []model.Page
}{
	"Cards": {
		{"Save or apply", model.ModeCreate, []model.Action{model.SaveDraftAction}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"Cancel", model.ModeCreate, []model.Action{model.CancelDraft}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"Change field", model.ModeCreate, []model.Action{model.NextField, model.PreviousField}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"Open editor", model.ModeEditor, []model.Action{model.ExternalEditorAction}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"Cycle reply kind", model.ModeEditor, []model.Action{model.CycleReplyKind}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
		{"Send reply anyway", model.ModeEditor, []model.Action{model.SendReplyAnyway}, []model.Page{model.PageIssues, model.PageBoard, model.PageWorkflows}},
	},
	"Filter and search": {
		{"Reset the form", model.ModeFilterForm, []model.Action{model.ResetFilterForm}, []model.Page{model.PageIssues, model.PageBoard}},
		{"Save as default filter", model.ModeFilterForm, []model.Action{model.SaveFilterDefault}, []model.Page{model.PageIssues, model.PageBoard}},
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

// emacsNote is named once on the Everywhere card: the Emacs aliases carry no key of their own in the rows.
var emacsNote = "Emacs keys also work: " + model.Key("ctrl+n") + ", " + model.Key("ctrl+p") + ", " + model.Key("ctrl+v") + ", " + model.Key("alt+v") + ", " + model.Key("ctrl+alt+<") + " and " + model.Key("ctrl+alt+>") + "."

// shownKeys is what a sheet row draws for its keys: a run of three or more consecutive digits collapses to "a-b" (1-6), and a
// row of aliases shows its primary key and one alias. Single characters are sibling keys, not aliases (the letters of the More
// Pages), so a row made only of them keeps every key.
func shownKeys(keys []string) []string {
	var out []string
	for i := 0; i < len(keys); {
		j := i
		for j+1 < len(keys) && isDigit(keys[j]) && isDigit(keys[j+1]) && keys[j+1][0] == keys[j][0]+1 {
			j++
		}
		if j-i >= 2 {
			out = append(out, keys[i]+"-"+keys[j])
		} else {
			out = append(out, keys[i:j+1]...)
		}
		i = j + 1
	}
	if len(out) > 2 && slices.ContainsFunc(out, func(k string) bool { return utf8.RuneCountInString(k) > 1 }) {
		out = out[:2]
	}
	return out
}

func isDigit(key string) bool { return len(key) == 1 && key[0] >= '0' && key[0] <= '9' }

// keySpans draws keys as keycaps with a space between them.
func keySpans(keys []string) []ui.Span {
	spans := make([]ui.Span, 0, 2*len(keys))
	for i, k := range keys {
		if i > 0 {
			spans = append(spans, ui.Span{T: " "})
		}
		spans = append(spans, keySpan(KeyWords(k)))
	}
	return spans
}

// keysWidth is the cells keySpans takes for keys, with each keycap's padding left to the renderer.
func keysWidth(keys []string) int {
	width := len(keys) - 1
	for _, k := range keys {
		width += utf8.RuneCountInString(KeyWords(k))
	}
	return max(width, 0)
}

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
			widths[shortest] = max(widths[shortest], keysWidth(shownKeys(l.Keys)))
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
	// The head sits outside the scroller so Close stays in reach however far the sheet is scrolled.
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "title", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: "Keyboard shortcuts", S: "strong"}}},
		exitButton("close", model.ModeHelp, model.CloseHelp, "Close"),
	)}
	return ui.Nodes(head, ui.List{CommonWithoutMax: scrollerCommon("help"), Max: ui.MaxFrac(1), Children: ui.Nodes(ui.Col{
		Common:   ui.Common{Key: "sheet"},
		Gap:      ui.GapMD,
		Children: ui.Nodes(ui.Row{Common: ui.Common{Key: "groups"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: columns}),
	})})
}

func helpCard(key string, group model.SheetGroup, keyCols int) tern.Element {
	rows := make([]tern.Element, len(group.Lines))
	for i, l := range group.Lines {
		keys := shownKeys(l.Keys)
		rows[i] = ui.Row{Common: ui.Common{Key: "row-" + strconv.Itoa(i)}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "keys", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(float64(keyCols))}}, Spans: keySpans(keys), Wrap: ui.WrapNone},
			helpAction(model.Capitalize(l.Text)),
		)}
	}
	if group.Title == model.SheetEverywhere {
		rows = append(rows, ui.Text{Common: ui.Common{Key: "emacs"}, Spans: prose(emacsNote, "muted")})
	}
	return ui.Card{Common: ui.Common{Key: key}, Head: ui.T(group.Title), Children: rows}
}

// helpAction is a row's words: plain text, or spans when the words name a key as a keycap.
func helpAction(words string) ui.Text {
	common := ui.Common{Key: "action", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}
	if !strings.Contains(words, model.KeyOpen) {
		return ui.Text{Common: common, Text: words}
	}
	return ui.Text{Common: common, Spans: prose(words, "")}
}
