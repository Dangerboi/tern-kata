package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"go.kenn.io/kit/tui/helplayout"
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
				if m.Board.Mode == model.BoardColumns {
					columns = append(columns, board(m, now))
				} else {
					columns = append(columns, issues(m, now))
				}
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
	for _, text := range footerTexts(m) {
		main = append(main, text)
	}
	var layer tern.Element
	switch m.TopCard() {
	case model.CardCreate:
		layer = createLayer(m)
	case model.CardDraft:
		layer = editorLayer(m)
	case model.CardField:
		layer = fieldLayer(m)
	case model.CardClose:
		layer = closeLayer(m)
	case model.CardDates:
		layer = datesLayer(m)
	case model.CardRecurrence:
		layer = recurrenceLayer(m)
	case model.CardProject:
		layer = projectLayer(m)
	case model.CardLanes:
		layer = lanesLayer(m)
	case model.CardFilter:
		layer = filterLayer(m)
	case model.CardPicker:
		layer = pickerLayer(m)
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
	rows := m.VisibleRows()
	cols := listColumns{ownerShown: !(slices.Contains(columns, model.ColumnList) && slices.Contains(columns, model.ColumnDetail)), id: 1, owner: ownerWidth}
	for _, row := range rows {
		cols.id = max(cols.id, utf8.RuneCountInString(idCell(row)))
		cols.owner = max(cols.owner, utf8.RuneCountInString(listOwner(m, row, now)))
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
		items = append(items, issueRow(m, row, row.UID == m.SelectedUID, stripeRole(i), cols, now))
	}
	selected := ""
	if m.SelectedUID != "" {
		selected = IssueID(m.SelectedUID)
	}
	// above is what the card draws over the rows, each with the 8px gap under it: the Lane header's text row, the column
	// header's list row and the Search field's 24px.
	children, above := ui.Nodes(), m.RowPx+6+8
	if m.Board.Mode == model.BoardLane {
		children, above = append(children, laneHeader(m)), above+m.RowPx+8
	}
	children = append(children, headerRow(cols))
	if m.Search.Open {
		children, above = append(children, searchInput(m)), above+24+8
	}
	empty := "No Issues in this Collection. Select another Collection or create one with kata create."
	if m.Filters != (model.Filters{}) {
		empty = "No Issues match these filters. Press c to clear filters."
	}
	if m.Loading {
		empty = "Loading Issues…"
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: scrollMax(m, above), Selected: selected, Empty: ui.T(empty), Children: items})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, "", children)
}

// laneHeader is the Lane view's header line: the Lane's title and card count, as on the Board.
func laneHeader(m model.Model) tern.Element {
	for _, column := range m.BoardColumns() {
		if column.Lane == m.Board.Lane {
			return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: column.Title, S: "strong"}, {T: " " + strconv.Itoa(len(column.Cards)), S: "muted"}}}
		}
	}
	return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: model.LaneTitle(m.Board.Lane), S: "strong"}}}
}

// searchInput is the Search field above the Issues, in the list and on the Board.
func searchInput(m model.Model) tern.Element {
	placeholder := "Search Issues in " + m.Collection.Title
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search Issues"}, Field: ui.Field{Text: m.Filters.Search, Cursor: caretAt(m.Search.Caret, "", m.Filters.Search), Mode: "insert", Placeholder: placeholder, Readonly: m.Search.Results, Prompt: []ui.Span{{T: "/ "}}}}
}

// scrollerCommon is a list that scrolls its own content inside its column, so
// the Pane has nothing to scroll and the top bar stays still.
func scrollerCommon(key string) ui.CommonWithoutMax {
	return ui.CommonWithoutMax{Key: key, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}
}

// scrollMax caps a scrolling list. Without a card the list fills the space its column has left. A card's body sizes to its
// content, not to the card, so there the list is capped at the rows its column has (columnsPx) less the card's 10px padding above
// and below and the above px the card draws over the list.
// Until Tern says the Pane's size there is nothing to measure and the list takes half the window.
func scrollMax(m model.Model, above int) ui.ListMax {
	if len(m.VisibleColumns()) < 2 {
		return ui.MaxFrac(1)
	}
	px, ok := columnsPx(m)
	if !ok {
		return ui.MaxFrac(0.5)
	}
	return linesIn(m, px-20-above)
}

// columnsPx is the height the Pane leaves its columns, between the top bar and the footer, and false until Tern says the Pane's
// size. Tern has no fill for a list, only a height cap in list rows, so a list that should fill its column is capped at the
// Pane's pixel height less everything above and below the columns: the top bar's text row and 50px of padding and gaps, a text
// row per footer line, two for the filter line, and the disconnected line and the Notice as footerTextRows counts them.
// ponytail: the chrome is measured at Tern's default density; a denser theme leaves more room unused, a looser one clips the
// lowest row. The Pane's rows are only read when Tern reports a new width, so a height-only resize keeps the old cap until then.
// Upgrade: a Tern prop for a list that fills its column.
func columnsPx(m model.Model) (int, bool) {
	if m.Rows == 0 || m.RowPx == 0 {
		return 0, false
	}
	grid, _, _ := footerGrid(m)
	chrome := m.RowPx + 50 + m.RowPx*len(grid) + m.RowPx*footerTextRows(m)
	if m.FilterSummary() != "" {
		chrome += 2 * m.RowPx
	}
	return m.Rows*m.RowPx - chrome, true
}

// linesIn is the cap of a list that fills px: Tern counts a list row as a text row and 6px, and the list ends up to one list row
// short of px.
func linesIn(m model.Model, px int) ui.ListMax {
	return ui.MaxLines(max(px/(m.RowPx+6), 1))
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
	headingCell.Title, headingCell.Actions = "Menu", &ui.Actions{Click: MenuClick}
	rightCell := flexCell("right", ui.ToneNeutral, 100)
	var zoneNote []tern.Element
	if m.Calendar.Note != "" {
		zoneNote = ui.Nodes(
			ui.Text{Common: flexCell("zone-note", ui.ToneWarning, 0), Text: m.Calendar.Note, Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("zone-sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
		)
	}
	return ui.Row{Common: ui.Common{Key: "topbar", Hidden: m.HelpOpen}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: ui.Nodes(
		ui.Row{Common: flexCell("left", ui.ToneNeutral, 0), Gap: ui.GapSM, Children: ui.Nodes(
			ui.Text{Common: headingCell, Spans: heading, Wrap: ui.WrapNone},
		)},
		ui.Text{Common: flexCell("counts", ui.ToneMuted, 1), Text: m.CountsLine(), Wrap: ui.WrapNone},
		ui.Row{Common: rightCell, Gap: ui.GapNone, Children: append(zoneNote, ui.Nodes(
			ui.Text{Common: flexCell("daemon", ui.ToneMuted, 100), Text: m.DaemonLine(), Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
			menuButton(),
		)...)},
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

// idCell is the first cell: the tree connectors, the fold marker and the short id.
func idCell(row model.IssueRow) string {
	disclosure := ""
	if row.HasChildren {
		disclosure = "▸ "
		if row.Expanded {
			disclosure = "▾ "
		}
	}
	return row.Guide + disclosure + row.ShortID
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

// listColumns are the list's cell widths, and whether the owner column is drawn.
type listColumns struct {
	ownerShown bool
	id, owner  int
}

// issueRow is one Issue: the id, prio, status and title on the left, the
// title being the cell that gives way, and kids, owner and updated pinned to
// the right so each column lines up with the header. A blank cell keeps its width.
func issueRow(m model.Model, row model.IssueRow, selected bool, role string, cols listColumns, now time.Time) tern.Element {
	tone := ui.ToneNeutral
	statusStyle := "accent"
	if model.IsClosed(row.Issue) {
		tone = ui.ToneMuted
		statusStyle = "muted"
	}
	if selected {
		tone = ui.ToneAccent
	}
	prio, prioTone := priorityText(row.Priority)
	prioStyle := string(prioTone)
	kids := ""
	if row.TotalKids > 0 {
		kids = fmt.Sprintf("%d/%d", row.OpenKids, row.TotalKids)
	}
	title := issueTitle(row.Issue)
	left := []ui.Span{
		{T: cell(idCell(row), cols.id), S: toneStyle(issueLaneTone(m, row.Issue))},
		{T: cellGap + cell(prio, prioWidth), S: prioStyle},
		{T: cellGap + cell(row.Status, statusWidth), S: statusStyle},
		{T: cellGap + title},
	}
	owner := listOwner(m, row, now)
	// A parent's click folds it; a double click on any row opens it. Tern sends the two clicks first, so a parent's fold ends where it began.
	actions := &ui.Actions{DblClick: OpenRowClick(row.UID)}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Role: role, Tone: tone, Actions: actions}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells(kids, owner, model.RelativeTime(row.UpdatedAt, now), cols, "muted", toneStyle(ownerTone))...)}
}

// rightCells are the kids, owner and updated cells, the owner only when shown.
func rightCells(kids, owner, updated string, cols listColumns, style, ownerStyle string) []ui.Span {
	spans := []ui.Span{{T: cell(kids, kidsWidth), S: style}}
	if cols.ownerShown {
		spans = append(spans, ui.Span{T: cellGap + cell(owner, cols.owner), S: ownerStyle})
	}
	return append(spans, ui.Span{T: cellGap + cell(updated, updatedWidth), S: style})
}

// footerSeparator stands between two columns of footer hints, and footerMargin is the Dock's padding, in columns.
const (
	footerSeparator = " ▕ "
	footerMargin    = 4
)

// footerGrid is the current mode's hints laid out as kata's TUI lays out its footer: a grid with as many columns as fit the
// Pane, so a narrow Pane gets more lines instead of losing hints. chords are the hints' chords in reading order, and
// position is the selected row's place in the list, drawn at the grid's top right.
func footerGrid(m model.Model) (grid [][]helplayout.HelpItem, chords []string, position string) {
	var items []helplayout.HelpItem
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
		items = append(items, helplayout.HelpItem{Key: binding.Key, Description: help})
		chords = append(chords, binding.Chord)
	}
	selected := 0
	rows := m.VisibleRows()
	for i, issue := range rows {
		if issue.UID == m.SelectedUID {
			selected = i + 1
			break
		}
	}
	position = fmt.Sprintf("%d / %d", selected, len(rows))
	gap := utf8.RuneCountInString(footerSeparator)
	grid = helplayout.ReflowRows([][]helplayout.HelpItem{items}, m.Cols-footerMargin-gap-utf8.RuneCountInString(position), gap)
	return grid, chords, position
}

func footer(m model.Model) tern.Element {
	grid, chords, position := footerGrid(m)
	widths := helplayout.ColumnWidths(grid)
	lines := make([]tern.Element, len(grid))
	next := 0
	for r, row := range grid {
		cells := make([]tern.Element, 0, 2*len(row))
		for c, item := range row {
			if c > 0 {
				cells = append(cells, ui.Text{Common: ui.Common{Key: "sep" + strconv.Itoa(c)}, Spans: []ui.Span{{T: footerSeparator, S: "muted"}}, Wrap: ui.WrapNone})
			}
			chord, pad := chords[next], strings.Repeat(" ", widths[c]-helplayout.ItemWidth(item))
			next++
			cells = append(cells, ui.Text{Common: ui.Common{Key: chord, Actions: &ui.Actions{Click: KeyClick(chord)}},
				Spans: []ui.Span{{T: item.Key, S: "strong"}, {T: " " + item.Description + pad, S: "muted"}}, Wrap: ui.WrapNone})
		}
		lines[r] = ui.Row{Common: ui.Common{Key: strconv.Itoa(r)}, Gap: ui.GapNone, Children: cells}
	}
	return ui.Nodes(ui.Row{Common: ui.Common{Key: "hints"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Col{Common: ui.Common{Key: "grid", Grow: 1}, Children: lines},
		ui.Text{Common: ui.Common{Key: "position", Shrink: new(float64)}, Text: position, Wrap: ui.WrapNone})})
}

// headerRow labels the Issue rows. It is an Item like the rows, so its cells sit
// where theirs do, and a node of its own, outside the scrolling list.
func headerRow(cols listColumns) tern.Element {
	left := []ui.Span{
		{T: cell("#", cols.id), S: "muted"},
		{T: cellGap + cell("prio", prioWidth), S: "muted"},
		{T: cellGap + cell("status", statusWidth), S: "muted"},
		{T: cellGap + "title", S: "muted"},
	}
	return ui.Item{Common: ui.Common{Key: "header", Tone: ui.ToneMuted}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells("kids", "owner", "updated", cols, "muted", "muted")...)}
}

// listOwner is the owner cell text: the name cut to ownerWidth as before, then " until …" whole when the claim expires.
func listOwner(m model.Model, row model.IssueRow, now time.Time) string {
	owner := m.OwnerText(row.Owner, row.AssignmentExpiresOn, now)
	if utf8.RuneCountInString(owner.Name) <= ownerWidth {
		return owner.String()
	}
	return cell(owner.Name, ownerWidth) + owner.Until
}

// issueTitle is the title as the list and the Board draw it: a recurring Issue's starts with "↻ ".
func issueTitle(issue model.Issue) string {
	if issue.RecurrenceID != nil {
		return "↻ " + issue.Title
	}
	return issue.Title
}
