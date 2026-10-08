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

// loadingMarker sits in a column heading while that column's content is
// replaced; the previous content stays on screen until it arrives.
var loadingMarker = ui.Span{T: "  loading…", S: "muted"}

func Render(m model.Model) tern.View {
	if m.Cols < model.MinCols {
		return narrowView()
	}
	var main tern.Nodes
	switch {
	case m.Err != nil:
		main = failureView(m)
	case m.FirstLoad():
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
	if m.Stale && m.Err == nil && !m.FirstLoad() {
		main = append(main, staleNotice(m))
	}
	layer := help(m)
	if m.Picker.Open {
		layer = pickerLayer(m)
	}
	if m.FilterForm.Open {
		layer = filterLayer(m)
	}
	if m.ProjectForm.Open {
		layer = projectLayer(m)
	}
	if m.CloseForm.Open {
		layer = closeLayer(m)
	}
	if m.FieldForm.Open {
		layer = fieldLayer(m)
	}
	if _, ok := m.ActiveDraft(); ok {
		layer = editorLayer(m)
	}
	if m.Create.Open {
		layer = createLayer(m)
	}
	if m.CloseForm.Err != nil && !m.CloseForm.Open {
		main = append(main, ui.Text{Common: ui.Common{Key: "close-error", Tone: ui.ToneError}, Text: m.CloseForm.Err.Error()})
	}
	return tern.View{Main: main, Dock: footer(m), Layer: layer}
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
		if row.Depth == 0 && group.Heading && !shownGroups[group.ID] {
			items = append(items, ui.Text{Common: ui.Common{Key: "group." + group.ID}, Spans: []ui.Span{{T: group.Title, S: "strong"}}})
			shownGroups[group.ID] = true
		}
		items = append(items, issueRow(row, row.UID == m.SelectedUID))
	}
	selected := ""
	if m.SelectedUID != "" {
		selected = IssueID(m.SelectedUID)
	}
	tone, style := ui.ToneNeutral, "strong"
	if m.Focus == model.ColumnList {
		tone, style = ui.ToneAccent, "accent strong"
	}
	title := m.ListHeader()
	if m.Collection.View == "project" {
		title = "Project: " + title
	}
	heading := []ui.Span{{T: title, S: style}}
	if m.Loading {
		heading = append(heading, loadingMarker)
	}
	topBar := ui.Col{Common: ui.Common{Key: "topbar"}, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading", Tone: tone, Title: "Collections picker (p)", Actions: &ui.Actions{Click: "key=p"}}, Spans: heading},
		ui.Text{Common: ui.Common{Key: "daemon", Tone: ui.ToneMuted}, Text: m.DaemonLine()},
		ui.Text{Common: ui.Common{Key: "counts", Tone: ui.ToneMuted}, Text: m.CountsLine()},
	)}
	children := ui.Nodes(topBar)
	if m.Search.Open {
		placeholder := "Search Issues in " + m.Collection.Title
		children = append(children, ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search Issues"}, Field: ui.Field{Text: m.Filters.Search, Mode: "insert", Placeholder: placeholder, Readonly: m.Search.Results, Prompt: []ui.Span{{T: "/ "}}}})
	}
	empty := "No Issues in this Collection. Select another Collection or create one with kata create."
	if m.Filters != (model.Filters{}) {
		empty = "No Issues match these filters. Press c to clear filters."
	}
	if m.Loading {
		empty = "Loading Issues…"
	}
	children = append(children, ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "issues"}, Selected: selected, Empty: ui.T(empty), Children: items})
	return ui.Col{Common: ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, Children: children}
}

func issueRow(row model.IssueRow, selected bool) tern.Element {
	disclosure := ""
	if row.HasChildren {
		disclosure = "▸ "
		if row.Expanded {
			disclosure = "▾ "
		}
	}
	tone := ui.ToneNeutral
	statusStyle := "accent"
	if row.Status == "closed" {
		tone = ui.ToneMuted
		statusStyle = "muted"
	}
	if selected {
		tone = ui.ToneAccent
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
	for _, label := range row.Labels {
		spans = append(spans, ui.Span{T: "  " + label, S: "mark"})
	}
	var actions *ui.Actions
	if row.HasChildren {
		actions = &ui.Actions{Click: string(model.ToggleExpansion) + "=" + row.UID}
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Tone: tone, Actions: actions}, Label: ui.Spans(spans...)}
}

func footer(m model.Model) tern.Element {
	segments := make([]tern.Element, 0, len(model.Keymap)+1)
	for binding := range model.BindingsForMode(m.CurrentMode(), true) {
		segments = append(segments, ui.Seg{Common: ui.Common{Key: binding.Chord, Actions: &ui.Actions{Click: "key=" + binding.Chord}}, Text: binding.Key + " " + binding.Help})
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
