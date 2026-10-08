package view

import (
	"fmt"
	"strings"

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
		main = ui.Nodes(ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading " + m.Collection.Title + " Issues…"})
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
	return tern.View{Main: main, Dock: footer(m), Layer: editorLayer(m)}
}

func issues(m model.Model) tern.Element {
	rows := m.VisibleRows()
	items := make([]tern.Element, 0, len(rows)+len(m.Groups))
	groups := make(map[string]model.IssueGroup, len(m.Issues))
	for _, group := range m.Groups {
		for _, issue := range group.Issues {
			groups[issue.UID] = group
		}
	}
	shownGroups := make(map[string]bool, len(m.Groups))
	for _, row := range rows {
		group := groups[row.UID]
		if row.Depth == 0 && m.Collection.View != "project" && !shownGroups[group.ID] {
			items = append(items, ui.Text{Common: ui.Common{Key: "group." + group.ID}, Text: group.Title})
			shownGroups[group.ID] = true
		}
		items = append(items, issueRow(row))
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
			ui.Text{Common: ui.Common{Key: "heading", Tone: tone, Actions: &ui.Actions{Click: "focus-list"}}, Text: m.Collection.Title},
			ui.List{
				CommonWithoutMax: ui.CommonWithoutMax{Key: "issues"},
				Selected: selected,
				Empty: ui.T("No Issues in this Collection. Select another Collection or create one with kata create."),
				Children: items,
			},
		),
	}
}

func issueRow(row model.IssueRow) tern.Element {
	disclosure := ""
	if row.HasChildren {
		disclosure = "▸ "
		if row.Expanded {
			disclosure = "▾ "
		}
	}
	tone := ui.ToneNeutral
	statusStyle := "info"
	if row.Status == "closed" {
		tone = ui.ToneMuted
		statusStyle = "muted"
	}
	spans := []ui.Span{
		{T: strings.Repeat("│\u00a0\u00a0", row.Depth) + disclosure + row.ShortID + "  " + row.Title},
		{T: "  " + row.Status, S: statusStyle},
	}
	if row.Priority != nil {
		style := "muted"
		switch *row.Priority {
		case 0:
			style = "error"
		case 1:
			style = "warning"
		case 2:
			style = "info"
		}
		spans = append(spans, ui.Span{T: fmt.Sprintf("  P%d", *row.Priority), S: style})
	}
	if row.Owner != nil && *row.Owner != "" {
		spans = append(spans, ui.Span{T: "  " + *row.Owner, S: "muted"})
	}
	if len(row.Labels) > 0 {
		spans = append(spans, ui.Span{T: "  " + strings.Join(row.Labels, ", "), S: "muted"})
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Tone: tone}, Label: ui.Spans(spans...)}
}

func footer(m model.Model) tern.Element {
	segments := make([]tern.Element, 0, len(model.Keymap)+1)
	for binding := range model.BindingsForMode(m.CurrentMode(), true) {
		segments = append(segments, ui.Seg{Common: ui.Common{Key: binding.Chord}, Text: binding.Key + " " + binding.Help})
	}
	selected := 0
	rows := m.VisibleRows()
	for i, issue := range rows {
		if issue.UID == m.SelectedUID {
			selected = i + 1
			break
		}
	}
	segments = append(segments, ui.Seg{Common: ui.Common{Key: "position"}, Side: ui.SideRight, Text: fmt.Sprintf("%d / %d", selected, len(rows))})
	return ui.Nodes(ui.Status{Common: ui.Common{Key: "hints"}, Children: segments})
}
