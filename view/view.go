package view

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// Cell widths of an Issue row, in characters. The Pane is monospaced, so a cell
// padded to its width lines up with the header above it. The title takes the
// rest of the row and is cut with an ellipsis when it does not fit.
const (
	prioWidth    = 4
	statusWidth  = 6
	kidsWidth    = 5
	ownerWidth   = 8
	updatedWidth = 8
	cellGap      = "  "
)

const IssueListID = "main.columns.list.issues"

// loadingMarker sits in a column heading while that column's content is
// replaced; the previous content stays on screen until it arrives.
var loadingMarker = ui.Span{T: "  loading…", S: "muted"}

// Render draws m; now is the time relative "updated" cells are measured from.
func Render(m model.Model, now time.Time) tern.View {
	if len(m.VisibleColumns()) == 0 {
		return narrowView()
	}
	var main tern.Nodes
	switch {
	case m.HelpOpen && (m.Err != nil || m.FirstLoad()):
		main = help(m)
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
				columns = append(columns, issues(m, now))
			case model.ColumnDetail:
				columns = append(columns, detail(m, now))
			}
		}
		// The top bar heads the whole Pane at every width, above the columns and outside their borders.
		// The columns fill what is left of the Pane under it and stretch to equal height: a Row centres its children by default, and a parent's detail (a Link per child) is far taller than the Pane.
		// Min H 0 lets the row shrink to the Pane instead of growing to the tallest column, so the Pane has nothing to scroll and the bar stays put.
		main = append(ui.Nodes(topBar(m)), filterLine(m)...)
		main = append(main, ui.Row{Common: ui.Common{Key: "columns", Hidden: m.HelpOpen, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: columns})
		if m.HelpOpen {
			// The top bar, filters and columns stay mounted but hidden, so Tern keeps their scroll offsets for when the sheet closes.
			main = append(main, help(m)...)
		}
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		main = append(main, staleNotice(m))
	}
	var layer tern.Element
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
	if m.HelpOpen {
		return tern.View{Main: main}
	}
	return tern.View{Main: main, Dock: footer(m), Layer: layer}
}

func issues(m model.Model, now time.Time) tern.Element {
	columns := m.VisibleColumns()
	ownerShown := !(slices.Contains(columns, model.ColumnList) && slices.Contains(columns, model.ColumnDetail))
	rows := m.VisibleRows()
	idWidth := 1
	for _, row := range rows {
		idWidth = max(idWidth, utf8.RuneCountInString(idCell(row)))
	}
	items := make([]tern.Element, 0, len(rows)+len(m.Groups))
	groups := make(map[string]model.IssueGroup, len(m.Issues))
	for _, group := range m.Groups {
		for _, issue := range group.Issues {
			groups[issue.UID] = group
		}
	}
	shownGroups := make(map[string]bool, len(m.Groups))
	for i, row := range rows {
		group := groups[row.UID]
		if row.Depth == 0 && group.Heading && !shownGroups[group.ID] {
			items = append(items, ui.Text{Common: ui.Common{Key: "group." + group.ID}, Spans: []ui.Span{{T: group.Title, S: "strong"}}})
			shownGroups[group.ID] = true
		}
		items = append(items, issueRow(row, row.UID == m.SelectedUID, stripeRole(i), ownerShown, idWidth, now))
	}
	selected := ""
	if m.SelectedUID != "" {
		selected = IssueID(m.SelectedUID)
	}
	children := ui.Nodes(headerRow(ownerShown, idWidth))
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
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: scrollMax(m), Selected: selected, Empty: ui.T(empty), Children: items})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, "", children)
}

// scrollerCommon is a list that scrolls its own content inside its column, so
// the Pane has nothing to scroll and the top bar stays still.
func scrollerCommon(key string) ui.CommonWithoutMax {
	return ui.CommonWithoutMax{Key: key, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}
}

// scrollMax caps a scrolling list. Without a card the list fills the space its
// column has left. A card's body sizes to its content, not to the card, so
// there the cap is a fraction of the window.
// ponytail: Tern gives a card body no fill, so the card tier assumes the window is at least ~600px (about 25 rows) tall; below that the card clips the list's lowest rows instead of the Pane scrolling the top bar away. Upgrade: a Tern prop for a card body that fills its card, or the Pane's rows in tern.Caps to cap in lines.
func scrollMax(m model.Model) ui.ListMax {
	if len(m.VisibleColumns()) < 2 {
		return ui.MaxFrac(1)
	}
	return ui.MaxFrac(0.6)
}

// topBar is the pinned row above the columns: the Collection title with its
// markers on the left, the counts in the middle, and the Daemon then the Menu button at the right. Clicking the title or the
// button opens the Menu. When the row is too narrow Tern's flex shrinks and truncates the Daemon first,
// then the counts; the left cell and the button never shrink.
func topBar(m model.Model) tern.Element {
	tone, style := ui.ToneNeutral, "strong"
	if m.Focus == model.ColumnList {
		tone, style = ui.ToneAccent, "accent strong"
	}
	title := m.Collection.Title
	if m.Collection.IsProject() {
		title = "Project: " + title
	}
	heading := []ui.Span{{T: title, S: style}}
	if m.Loading {
		heading = append(heading, loadingMarker)
	}
	flexCell := func(key string, tone ui.Tone, shrink float64) ui.Common {
		return ui.Common{Key: key, Tone: tone, Shrink: &shrink}
	}
	headingCell := flexCell("heading", tone, 0)
	headingCell.Title, headingCell.Actions = "Menu (p)", &ui.Actions{Click: KeyClick("p")}
	rightCell := flexCell("right", ui.ToneNeutral, 100)
	return ui.Row{Common: ui.Common{Key: "topbar", Hidden: m.HelpOpen}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: ui.Nodes(
		ui.Row{Common: flexCell("left", ui.ToneNeutral, 0), Gap: ui.GapSM, Children: ui.Nodes(
			ui.Text{Common: headingCell, Spans: heading, Wrap: ui.WrapNone},
		)},
		ui.Text{Common: flexCell("counts", ui.ToneMuted, 1), Text: m.CountsLine(), Wrap: ui.WrapNone},
		ui.Row{Common: rightCell, Gap: ui.GapNone, Children: ui.Nodes(
			ui.Text{Common: flexCell("daemon", ui.ToneMuted, 100), Text: m.DaemonLine(), Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
			keyButton("menu", "Menu", "p", true),
		)},
	)}
}

// filterLine lists the active filters under the top bar; nil when none is on.
func filterLine(m model.Model) []tern.Element {
	summary := m.FilterSummary()
	if summary == "" {
		return nil
	}
	return []tern.Element{ui.Text{Common: ui.Common{Key: "filters", Hidden: m.HelpOpen, Tone: ui.ToneMuted}, Text: summary, Wrap: ui.WrapNone}}
}

// panel is a column's container. With more than one column showing it is a
// bordered Card: the focused column's ring is Tern's accent (Selected), the
// other's the muted tone. A lone column has no border and spaces its children
// by gap. Either way it is clipped to the height its Row gives it.
func panel(m model.Model, column model.Column, common ui.Common, gap ui.Gap, children []tern.Element) tern.Element {
	common.Max = &ui.Bounds{H: ui.Frac(1)}
	if len(m.VisibleColumns()) < 2 {
		return ui.Col{Common: common, Gap: gap, Children: children}
	}
	if m.Focus != column {
		common.Tone = ui.ToneMuted
	}
	return ui.Card{Common: common, Selected: m.Focus == column, Children: children}
}

// idCell is the first cell: the tree indent, the fold marker and the short id.
func idCell(row model.IssueRow) string {
	disclosure := ""
	if row.HasChildren {
		disclosure = "▸ "
		if row.Expanded {
			disclosure = "▾ "
		}
	}
	return strings.Repeat("│\u00a0\u00a0", row.Depth) + disclosure + row.ShortID
}

// cell is text padded with spaces to width characters, or cut to it with an ellipsis.
func cell(text string, width int) string {
	n := utf8.RuneCountInString(text)
	if n > width {
		return string([]rune(text)[:width-1]) + "…"
	}
	return text + strings.Repeat(" ", width-n)
}

// stripeRole is the role of the drawn row at index i: every other row is shaded.
func stripeRole(i int) string {
	if i%2 == 1 {
		return StripeRole
	}
	return ""
}

// issueRow is one Issue: the id, prio, status and title on the left, the
// title being the cell that gives way, and kids, owner and updated pinned to
// the right so each column lines up with the header. A blank cell keeps its width.
func issueRow(row model.IssueRow, selected bool, role string, ownerShown bool, idWidth int, now time.Time) tern.Element {
	tone := ui.ToneNeutral
	statusStyle := "accent"
	if model.IsClosed(row.Issue) {
		tone = ui.ToneMuted
		statusStyle = "muted"
	}
	if selected {
		tone = ui.ToneAccent
	}
	prio, prioStyle := "", "muted"
	if row.Priority != nil {
		prio = fmt.Sprintf("P%d", *row.Priority)
		switch *row.Priority {
		case 0:
			prioStyle = "error"
		case 1:
			prioStyle = "warning"
		case 2:
			prioStyle = "info"
		}
	}
	kids := ""
	if row.TotalKids > 0 {
		kids = fmt.Sprintf("%d/%d", row.OpenKids, row.TotalKids)
	}
	left := []ui.Span{
		{T: cell(idCell(row), idWidth)},
		{T: cellGap + cell(prio, prioWidth), S: prioStyle},
		{T: cellGap + cell(row.Status, statusWidth), S: statusStyle},
		{T: cellGap + row.Title},
	}
	owner := ""
	if row.Owner != nil {
		owner = *row.Owner
	}
	// A parent's click folds it; a double click on any row opens it. Tern sends the two clicks first, so a parent's fold ends where it began.
	actions := &ui.Actions{DblClick: OpenRowClick(row.UID)}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Role: role, Tone: tone, Actions: actions}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells(kids, owner, relativeTime(row.UpdatedAt, now), ownerShown, "muted")...)}
}

// rightCells are the kids, owner and updated cells, the owner only when shown.
func rightCells(kids, owner, updated string, ownerShown bool, style string) []ui.Span {
	spans := []ui.Span{{T: cell(kids, kidsWidth), S: style}}
	if ownerShown {
		spans = append(spans, ui.Span{T: cellGap + cell(owner, ownerWidth), S: style})
	}
	return append(spans, ui.Span{T: cellGap + cell(updated, updatedWidth), S: style})
}

func footer(m model.Model) tern.Element {
	segments := make([]tern.Element, 0, len(model.Keymap)+1)
	for binding := range model.BindingsForMode(m.CurrentMode(), true) {
		if binding.Action == model.FocusDetail && !m.CanFocusDetail() {
			continue
		}
		if binding.Action == model.MenuBack && !m.SidebarProjects {
			continue
		}
		help := binding.Help
		if binding.Action == model.ClosePicker && m.Picker.Projects && m.Picker.Filter == "" {
			help = "back"
		}
		segments = append(segments, ui.Seg{Common: ui.Common{Key: binding.Chord, Actions: &ui.Actions{Click: KeyClick(binding.Chord)}}, Text: binding.Key + " " + help})
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

// headerRow labels the Issue rows. It is an Item like the rows, so its cells sit
// where theirs do, and a node of its own, outside the scrolling list.
func headerRow(ownerShown bool, idWidth int) tern.Element {
	left := []ui.Span{
		{T: cell("#", idWidth), S: "muted"},
		{T: cellGap + cell("prio", prioWidth), S: "muted"},
		{T: cellGap + cell("status", statusWidth), S: "muted"},
		{T: cellGap + "title", S: "muted"},
	}
	return ui.Item{Common: ui.Common{Key: "header", Tone: ui.ToneMuted}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells("kids", "owner", "updated", ownerShown, "muted")...)}
}

// relativeTime is kata's TUI "updated" cell: "2h ago", "3d ago", "-" when unset.
func relativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	}
}
