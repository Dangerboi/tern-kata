package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const IssueListID = "main.columns.list.issues"

func IssueID(uid string) string { return IssueListID + "." + uid }

func Render(m model.Model) tern.View {
	var main tern.Nodes
	switch {
	case m.Err != nil:
		main = failureView(m)
	case m.Loading:
		main = ui.Nodes(ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Project's open Issues…"})
	default:
		columns := make([]tern.Element, 0, 3)
		for _, column := range m.VisibleColumns() {
			switch column {
			case model.ColumnCollections:
				columns = append(columns, Collections(m))
			case model.ColumnList:
				columns = append(columns, issues(m))
			case model.ColumnDetail:
				columns = append(columns, detail(m))
			}
		}
		main = ui.Nodes(ui.Row{Common: ui.Common{Key: "columns"}, Gap: ui.GapMD, Children: columns})
	}
	if m.Stale && m.Err == nil && !m.Loading {
		main = append(main, staleNotice(m))
	}
	return tern.View{Main: main, Dock: footer(m), Layer: help(m)}
}

func issues(m model.Model) tern.Element {
	items := make([]tern.Element, 0, len(m.Issues))
	for _, issue := range m.Issues {
		items = append(items, ui.Item{
			Common: ui.Common{Key: issue.UID},
			Label: ui.T(issue.ShortID + "  " + issue.Title),
		})
	}
	selected := ""
	if m.SelectedUID != "" {
		selected = IssueID(m.SelectedUID)
	}
	tone := ui.ToneNeutral
	if m.Focus == model.ColumnList { tone = ui.ToneAccent }
	return ui.Col{
		Common: ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)},
		Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "heading", Tone: tone, Actions: &ui.Actions{Click: "focus-list"}}, Text: m.Project.Name + " / Open Issues"},
			ui.List{
				CommonWithoutMax: ui.CommonWithoutMax{Key: "issues"},
				Selected: selected,
				Empty: ui.T("No open Issues in this Project. Create one with kata create."),
				Children: items,
			},
		),
	}
}

func footer(m model.Model) tern.Element {
	segments := make([]tern.Element, 0, len(model.Keymap)+1)
	for binding := range model.BindingsForMode(m.CurrentMode(), true) {
		segments = append(segments, ui.Seg{Common: ui.Common{Key: binding.Chord}, Text: binding.Key + " " + binding.Help})
	}
	selected := 0
	for i, issue := range m.Issues {
		if issue.UID == m.SelectedUID {
			selected = i + 1
			break
		}
	}
	segments = append(segments, ui.Seg{Common: ui.Common{Key: "position"}, Side: ui.SideRight, Text: fmt.Sprintf("%d / %d", selected, len(m.Issues))})
	return ui.Nodes(ui.Status{Common: ui.Common{Key: "hints"}, Children: segments})
}
