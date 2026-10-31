package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/mattn/go-runewidth"
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
				switch {
				case m.OnDaemons():
					columns = append(columns, daemons(m))
				case m.OnFederation():
					columns = append(columns, federation(m, now))
				case m.OnAttention():
					columns = append(columns, attention(m))
				case m.OnCredentials():
					columns = append(columns, credentials(m))
				case m.OnRuns():
					columns = append(columns, runs(m, now))
				case m.OnJobs():
					columns = append(columns, jobs(m, now))
				case m.BoardShown():
					columns = append(columns, board(m, now))
				case m.OnWorkflows():
					columns = append(columns, workflows(m, now))
				default:
					columns = append(columns, issues(m, now))
				}
			case model.ColumnDetail:
				if m.OnRuns() {
					columns = append(columns, runInspector(m, now))
				} else if m.OnJobs() {
					columns = append(columns, inspector(m, now))
				} else {
					columns = append(columns, detail(m, now))
				}
			}
		}
		// The top bar heads the whole Pane at every width, above the columns and outside their borders.
		// The columns fill what is left of the Pane under it and stretch to equal height: a Row centres its children by default, and a parent's detail (a Link per child) is far taller than the Pane.
		// Min H 0 lets the row shrink to the Pane instead of growing to the tallest column, so the Pane has nothing to scroll and the bar stays put.
		main = append(append(ui.Nodes(topBar(m)), pageLine(m)...), filterLine(m)...)
		main = append(main, ui.Row{Common: ui.Common{Key: "columns", Hidden: m.HelpOpen, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: columns})
		if m.HelpOpen {
			// The top bar, filters and columns stay mounted but hidden, so Tern keeps their scroll offsets for when the sheet closes.
			main = append(main, help(m)...)
		}
	}
	for _, text := range footerTexts(m) {
		main = append(main, text)
	}
	layer := topLayer(m, now)
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
		cols.fit(utf8.RuneCountInString(idCell(row)), displayWidth(listOwner(m, row, now)))
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
	children := ui.Nodes()
	if m.LaneViewShown() {
		children = append(children, laneHeader(m))
	}
	children = append(children, headerRow(cols))
	if m.Search.Open {
		children = append(children, searchInput(m))
	}
	empty := "No Issues in this Collection. Select another Collection or create one with kata create."
	if m.Filters != (model.Filters{}) {
		empty = "No Issues match these filters. Press c to clear filters."
	}
	if m.Loading {
		empty = "Loading Issues…"
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: fillMax, Selected: selected, Empty: ui.T(empty), Children: items})
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
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search Issues"}, Field: ui.Field{Text: m.Filters.Search, Cursor: caretAt(m.Search.Caret, "", m.Filters.Search), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: m.Search.Results, Prompt: []ui.Span{{T: "/ "}}}}
}

// scrollerCommon is a list that scrolls its own content inside its column, so
// the Pane has nothing to scroll and the top bar stays still.
func scrollerCommon(key string) ui.CommonWithoutMax {
	return ui.CommonWithoutMax{Key: key, Role: FillRole, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}
}

// fillMax is every column scroller's cap. ColumnFillCSS stretches a column's card body and its list to the column's height, so
// the list fills its column and scrolls inside it whatever the Pane's size or Tern's density; the cap only gives the list
// Tern's scrolling, never binds, and is the whole window until the style sheet is in.
var fillMax = ui.MaxFrac(1)

// topBar is the pinned row above the columns: the Collection title with its
// markers on the left, the counts in the middle, and the Daemon then the Menu button at the right. Clicking the title or the
// button opens the Menu. When the row is too narrow Tern's flex shrinks and truncates the Daemon first,
// then the counts; the left cell and the button never shrink. The title keeps the accent whichever column has focus: it names
// what the Pane shows, and the columns' own borders show focus.
func topBar(m model.Model) tern.Element {
	title := barTitle(m)
	heading := []ui.Span{{T: title, S: "accent strong"}}
	if m.Loading {
		heading = append(heading, loadingMarker)
	}
	flexCell := func(key string, tone ui.Tone, shrink float64) ui.Common {
		return ui.Common{Key: key, Tone: tone, Shrink: &shrink}
	}
	headingCell := flexCell("heading", ui.ToneAccent, 0)
	rightCell := flexCell("right", ui.ToneNeutral, 100)
	// The right cell never narrows below "… · Menu", so the Menu button is never pushed off the bar; the Daemon inside gives way first.
	rightCell.Min = &ui.Bounds{W: ui.Ch(8)}
	var zoneNote []tern.Element
	if m.Calendar.Note != "" {
		zoneNote = ui.Nodes(
			ui.Text{Common: flexCell("zone-note", ui.ToneWarning, 0), Text: m.Calendar.Note, Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("zone-sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
		)
	}
	leftCells := ui.Nodes(ui.Text{Common: headingCell, Spans: heading, Wrap: ui.WrapNone})
	if fit := topBarFit(m); fit != barOwnLine {
		leftCells = append(leftCells, pageBar(m, flexCell, fit == barCompact))
	}
	return ui.Row{Common: ui.Common{Key: "topbar", Hidden: m.HelpOpen}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: ui.Nodes(
		ui.Row{Common: flexCell("left", ui.ToneNeutral, 0), Gap: ui.GapMD, Children: leftCells},
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
	if summary == "" || m.OnJobs() || m.OnRuns() {
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
	common.Role = ColumnRole
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

// cell is text padded with spaces to width terminal cells, or cut to it with an ellipsis.
func cell(text string, width int) string {
	text = cut(text, width)
	return text + strings.Repeat(" ", max(width-displayWidth(text), 0))
}

// cut is text cut to width terminal cells with an ellipsis, not padded: the owner cell's box (see cellBoxCSS) gives it its
// width, and padding it by cell count would overshoot the box when Tern draws a wide glyph a little wider than its cells.
func cut(text string, width int) string {
	if displayWidth(text) > width {
		return displayCondition.Truncate(text, width, "…")
	}
	return text
}

// displayCondition measures text in terminal cells, wide characters two, whatever the locale: Tern draws them so.
var displayCondition = &runewidth.Condition{StrictEmojiNeutral: true}

func displayWidth(text string) int { return displayCondition.StringWidth(text) }

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

// fit widens the columns to a row's id and owner widths, up to maxBoxWidth: a wider column would have no box rule, so its
// cells are cut to the cap instead (rightCells and issueRow cut to the column's width).
func (c *listColumns) fit(id, owner int) {
	c.id = min(max(c.id, id), maxBoxWidth)
	c.owner = min(max(c.owner, owner), maxBoxWidth)
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
	}
	// The Agent badge is a span: an Item has one click, and Spans have none, so the badge on a row is not clickable.
	if badge, ok := m.AgentBadgeFor(row.UID); ok {
		left = append(left, ui.Span{T: cellGap + badge.Text, S: string(badge.Tone)}, ui.Span{T: " " + title})
	} else {
		left = append(left, ui.Span{T: cellGap + title})
	}
	owner := listOwner(m, row, now)
	// A parent's click folds it; a double click on any row opens it. Tern sends the two clicks first, so a parent's fold ends where it began.
	actions := &ui.Actions{DblClick: OpenRowClick(row.UID)}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Role: strings.TrimSpace(role + " " + cols.role()), Tone: tone, Actions: actions}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells(kids, owner, model.RelativeTime(row.UpdatedAt, now), cols, "muted", toneStyle(ownerTone))...)}
}

// role is the row's boxes in role tokens, which the style sheet turns into the columns' widths (see cellBoxCSS): the row
// kind, the id width and, when the owner shows, the owner width. Their widths are the Issues' own, so they cannot be static.
func (c listColumns) role() string {
	role := ListRowRole + " " + ListIDRole + strconv.Itoa(c.id)
	if c.ownerShown {
		role += " " + ListOwnerRole + strconv.Itoa(c.owner)
	}
	return role
}

// rightCells are the kids, owner and updated cells, the owner only when shown.
func rightCells(kids, owner, updated string, cols listColumns, style, ownerStyle string) []ui.Span {
	spans := []ui.Span{{T: cell(kids, kidsWidth), S: style}}
	if cols.ownerShown {
		spans = append(spans, ui.Span{T: cellGap + cut(owner, cols.owner), S: ownerStyle})
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
		if binding.Action == model.OverwriteDraft && m.TopCard() == model.CardWorkflow && m.Workflows.Card.Conflict != nil && m.Workflows.Card.Conflict.Archived {
			continue
		}
		if binding.Action == model.MenuBack && m.SidebarLevel == model.MenuTop {
			continue
		}
		help := binding.Help
		if binding.Action == model.ClosePicker && m.Picker.Level != model.MenuTop && m.Picker.Filter == "" {
			help = "back"
		}
		if binding.Action == model.DrillOut && m.EscGoesBack() {
			help = "back"
		}
		if job := m.SelectedJob(); binding.Action == model.ArchiveJobAction && job != nil && job.Archived() {
			help = "restore"
		}
		items = append(items, helplayout.HelpItem{Key: binding.Key, Description: help})
		chords = append(chords, binding.Chord)
	}
	selected, total := m.Position()
	position = fmt.Sprintf("%d / %d", selected, total)
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
	return ui.Item{Common: ui.Common{Key: "header", Role: cols.role(), Tone: ui.ToneMuted}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells("kids", "owner", "updated", cols, "muted", "muted")...)}
}

// listOwner is the owner cell text: the name cut to ownerWidth as before, then " until …" whole when the claim expires.
func listOwner(m model.Model, row model.IssueRow, now time.Time) string {
	owner := m.OwnerText(row.Owner, row.AssignmentExpiresOn, now)
	if displayWidth(owner.Name) <= ownerWidth {
		return owner.String()
	}
	return cell(owner.Name, ownerWidth) + owner.Until
}

// issueTitle is the title as the list and the Board draw it: a recurring Issue's starts with "↻ ", one a Cron job made with "◷ ".
func issueTitle(issue model.Issue) string {
	title := issue.Title
	if issue.CronMade() {
		title = "◷ " + title
	}
	if issue.RecurrenceID != nil {
		title = "↻ " + title
	}
	return title
}
