package view

import (
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const IssueListID = "main.issues"

func IssueID(uid string) string { return IssueListID + "." + uid }

func Render(m model.Model) tern.View {
	main := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Text: m.Project.Name + " / Open Issues"})
	switch {
	case m.Err != nil:
		main = ui.Nodes(ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Err.Error()})
	case m.Loading:
		main = ui.Nodes(ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Project's open Issues…"})
	default:
		rows := m.VisibleRows()
		items := make([]tern.Element, 0, len(rows))
		for _, row := range rows {
			items = append(items, issueRow(row))
		}
		selected := ""
		if m.SelectedUID != "" {
			selected = IssueID(m.SelectedUID)
		}
		main = append(main, ui.List{
			CommonWithoutMax: ui.CommonWithoutMax{Key: "issues"},
			Selected:         selected,
			Empty:            ui.T("No open Issues in this Project. Create one with kata create."),
			Children:         items,
		})
	}
	return tern.View{Main: main, Dock: footer(m), Layer: help(m)}
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
		{T: strings.Repeat("\u00a0\u00a0", row.Depth) + disclosure + row.ShortID + "  " + row.Title},
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
