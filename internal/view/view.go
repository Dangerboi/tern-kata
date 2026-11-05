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
)

// Cell widths of an Issue row, in characters. The style sheet gives each cell a box of its column's width (cellBoxCSS), so
// a cell lines up with the header above it. The title takes the rest of the row and is cut with an ellipsis when it does
// not fit.
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
	owners := make([]string, len(rows))
	for i, row := range rows {
		owners[i] = listOwner(m, row, now)
		cols.fit(utf8.RuneCountInString(idCell(row)), displayWidth(owners[i]))
	}
	lookups := rowLookups{lanes: m.LaneList(), badges: m.AgentBadges(), today: model.NewCalendarDay(m.Calendar.Date), dateRoom: dateRoom(m, cols)}
	items := make([]tern.Element, 0, len(rows)+len(m.Groups))
	groups := make(map[string]model.IssueGroup, len(m.Issues))
	for _, group := range m.Groups {
		for _, issue := range group.Issues {
			groups[issue.UID] = group
		}
	}
	shownGroups := make(map[string]bool, len(m.Groups))
	menu := newContextMenuKeys(rowMode(m), model.DrillIn)
	for i, row := range rows {
		group := groups[row.UID]
		if row.Depth == 0 && group.Heading && !shownGroups[group.ID] {
			items = append(items, ui.Text{Common: ui.Common{Key: "group." + group.ID}, Spans: []ui.Span{{T: group.Title, S: "strong"}}})
			shownGroups[group.ID] = true
		}
		items = append(items, issueRow(row, owners[i], lookups, row.UID == m.SelectedUID, stripeRole(i), cols, menu, now))
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
	empty := emptyState("No Issues", "This Collection has none. Select another Collection or create one with kata create.")
	switch {
	case m.Filters != (model.Filters{}):
		empty = emptyState("No Issues match", "These filters leave none.", emptyButton(m, "clear-filters", model.ClearFilters, "Clear filters", false)...)
	case m.Collection.IsProject():
		empty = emptyState("No Issues", "This Collection has none.", emptyButton(m, "new", model.NewIssue, "New Issue", true)...)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: fillMax, Selected: selected, Empty: ui.T("Loading Issues…"), Children: items}
	children = append(children, listAndEmpty(list, m.Loading, empty)...)
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, "", children)
}

// laneHeader is the Lane view's header line: the Lane's title and card count, as on the Board.
func laneHeader(m model.Model) tern.Element {
	for _, column := range m.BoardColumns() {
		if column.Lane == m.Board.Lane {
			return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: column.Title, S: "strong"}, {T: " " + strconv.Itoa(len(column.Cards)), S: mono("muted")}}}
		}
	}
	return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: model.LaneTitle(m.Board.Lane), S: "strong"}}}
}

// searchInput is the Search field above the Issues, in the list and on the Board.
func searchInput(m model.Model) tern.Element {
	placeholder := "Search Issues in " + m.Collection.Title
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search Issues"}, Field: ui.Field{Text: m.Filters.Search, Cursor: caretAt(m.Search.Caret, "", m.Filters.Search), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: m.Search.Results, Prompt: []ui.Span{{T: "/ "}}}}
}

// pageSearchInput is the search field above the Jobs, Runs or Workflows list, drawn like the Issues' while it is being typed into
// and read-only while the keys are back on the list.
func pageSearchInput(m model.Model) tern.Element {
	noun := m.PageSearchNoun()
	text := m.PageSearch.Text
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search " + noun}, Field: ui.Field{Text: text, Cursor: caretAt(m.PageSearch.Caret, "", text), Mode: "insert", Placeholder: ui.T("Search " + noun), Readonly: !m.PageSearch.Open, Prompt: []ui.Span{{T: "/ "}}}}
}

// searchEmpty is the empty state of a Page whose search leaves no row; it stays under the search input.
func searchEmpty(m model.Model) ui.Empty {
	return emptyState("No "+m.PageSearchNoun()+" match", "Nothing matches "+strconv.Quote(m.PageSearch.Text)+". Press esc to end the search.")
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
// markers on the left, the counts in the middle, and the Daemon then the Menu button at the right. The Menu button opens the
// Menu. When the row is too narrow Tern's flex shrinks and truncates the Daemon first,
// then the counts; the left cell and the button never shrink. The title keeps the accent whichever column has focus: it names
// what the Pane shows, and the columns' own borders show focus.
func topBar(m model.Model) tern.Element {
	title := barTitle(m)
	heading := []ui.Span{{T: title, S: "title accent"}}
	if m.Loading {
		heading = append(heading, loadingMarker)
	}
	flexCell := func(key string, tone ui.Tone, shrink float64) ui.Common {
		return ui.Common{Key: key, Tone: tone, Shrink: &shrink}
	}
	headingCell := flexCell("heading", ui.ToneAccent, 0)
	rightCell := flexCell("right", ui.ToneNeutral, 100)
	// The right cell never narrows below "… · Menu" with the Menu button's keycap, so the button is never pushed off the bar; the
	// Daemon inside gives way first.
	rightCell.Min = &ui.Bounds{W: ui.Ch(14)}
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
		ui.Text{Common: flexCell("counts", ui.ToneMuted, 1), Spans: monoSpans(m.CountsLine(), "muted"), Wrap: ui.WrapNone},
		ui.Row{Common: rightCell, Gap: ui.GapNone, Children: append(zoneNote, ui.Nodes(
			ui.Text{Common: flexCell("daemon", ui.ToneMuted, 100), Text: m.DaemonLine(), Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
			menuButton(m),
		)...)},
	)}
}

// filterLine lists the active filters under the top bar: the Filter card's funnel, a chip per filter and, muted at the end,
// where they came from (the default or a saved view); nil when it has nothing to show.
func filterLine(m model.Model) []tern.Element {
	terms, note := m.FilterLine()
	if len(terms) == 0 && note == "" || m.OnJobs() || m.OnRuns() {
		return nil
	}
	cells := ui.Nodes(ui.Icon{Common: ui.Common{Key: "icon", Tone: ui.ToneMuted, Aria: "Filters"}, Name: ui.IconFunnel})
	for _, term := range terms {
		cells = append(cells, ui.Badge{Common: ui.Common{Key: string(term.Field)}, Text: filterChipText(term)})
	}
	if note != "" {
		cells = append(cells, ui.Text{Common: ui.Common{Key: "note"}, Spans: []ui.Span{{T: note, S: "muted"}}, Wrap: ui.WrapNone})
	}
	return ui.Nodes(ui.Row{Common: ui.Common{Key: "filters", Hidden: m.HelpOpen}, Gap: ui.GapSM, Wrap: true, Children: cells})
}

// filterChipText is a filter as its chip says it, "status open": the field's name and its value, a Relationship in the Filter
// card's words.
func filterChipText(term model.FilterTerm) string {
	value := term.Value
	if term.Field == model.FilterRelationship {
		value = strings.ToLower(model.FilterChoiceLabel(model.FilterRelationships, value))
	}
	return string(term.Field) + " " + value
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

// rowLookups are what every row of a frame draws from alike, read once: the Lane list and each Issue's Agent badge.
type rowLookups struct {
	lanes    []string
	badges   model.AgentBadgeIndex
	today    model.CalendarDay
	dateRoom int // cells a row with no Agent badge or hand-off word has for its whole title and its Date badge together
}

// dateRoom is the cells left on a row for its title and Date badge: the list column's width less the other cells, the row's
// padding and the gaps before the title and before the badge.
func dateRoom(m model.Model, cols listColumns) int {
	fixed := cols.id + len(cellGap)*3 + prioWidth + statusWidth + kidsWidth + len(cellGap) + updatedWidth + rowPadding
	if cols.ownerShown {
		fixed += len(cellGap) + cols.owner
	}
	return listWidth(m) - fixed - 2*len(cellGap)
}

// Flex rules of the columns Row (see issues and detail), which Tern lays out and the plugin never sees: the list and detail
// grow equally from bases of 0.4 and 0.5 of the Row, the Collections column (when shown) is as wide as its longest entry
// and does not grow, and a gap of columnGap cells sits between columns.
const (
	listBasis     = 0.4
	detailBasis   = 0.5
	columnGap     = 2
	sidebarChrome = 4 // the Collections column's border and padding
)

// listWidth is the list column's width in cells: all of the Pane when it is the only column, otherwise its basis plus its
// share of what the other columns leave, as Tern's flex layout gives it.
func listWidth(m model.Model) int {
	columns := m.VisibleColumns()
	if len(columns) < 2 {
		return m.Cols
	}
	fixed := columnGap * (len(columns) - 1)
	if slices.Contains(columns, model.ColumnCollections) {
		widest := 0
		for _, entry := range m.SidebarEntries() {
			widest = max(widest, displayWidth(m.EntryTitle(entry)))
		}
		fixed += widest + sidebarChrome
	}
	free := float64(m.Cols-fixed) - float64(m.Cols)*(listBasis+detailBasis)
	if slices.Contains(columns, model.ColumnDetail) {
		if free >= 0 {
			return int(float64(m.Cols)*listBasis + free/2)
		}
		return int(float64(m.Cols) * listBasis * float64(m.Cols-fixed) / (float64(m.Cols) * (listBasis + detailBasis)))
	}
	return m.Cols - fixed
}

// rowPadding is the cells a row spends on its selection mark and edges, beside its cells.
const rowPadding = 4

// issueRow is one Issue: the id, prio, status and title on the left, the
// title being the cell that gives way, and kids, owner and updated pinned to
// the right so each column lines up with the header. A blank cell keeps its width.
func issueRow(row model.IssueRow, owner string, lookups rowLookups, selected bool, role string, cols listColumns, menu contextMenuKeys, now time.Time) tern.Element {
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
		{T: cell(idCell(row), cols.id), S: mono(toneStyle(issueLaneToneIn(lookups.lanes, row.Issue)))},
		{T: cellGap + cell(prio, prioWidth), S: mono(prioStyle)},
		{T: cellGap + cell(row.Status, statusWidth), S: statusStyle},
	}
	// The Agent badge is a span: an Item has one click, and Spans have none, so the badge on a row is not clickable.
	gap, beforeTitle := cellGap, 0 // beforeTitle is the cells ahead of the title beyond the one cellGap dateRoom counts
	if badge, ok := lookups.badges.For(row.UID); ok {
		left = append(left, ui.Span{T: gap + badge.Text, S: string(badge.Tone)})
		beforeTitle += displayWidth(gap + badge.Text)
		gap = " "
	}
	// An agent's hand-off (work.attention) reads beside the badge, whoever wrote it.
	if w, ok := model.IssueWorkAttention(row.Issue); ok {
		left = append(left, ui.Span{T: gap + w.Word(), S: string(attentionTone(w))})
		beforeTitle += displayWidth(gap + w.Word())
		gap = " "
	}
	beforeTitle += len(gap) - len(cellGap) // dateRoom already holds the title's own gap of cellGap
	left = append(left, ui.Span{T: gap + title})
	// An open Issue's Date badge reads after the title, as in kata tui. The title has priority: the badge goes when the whole
	// title and the badge do not fit the row.
	if !model.IsClosed(row.Issue) {
		if date, ok := lookups.today.ChooseDateBadge(row.Issue); ok && displayWidth(title)+displayWidth(date.Text) <= lookups.dateRoom-beforeTitle {
			left = append(left, ui.Span{T: "  " + date.Text, S: mono(toneStyle(dateTones[date.Tone]))})
		}
	}
	// A parent's click folds it; a double click on any row opens it. Tern sends the two clicks first, so a parent's fold ends where it began.
	actions := &ui.Actions{DblClick: OpenRowClick(row.UID), Menu: menu.of(row.Issue)}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Role: strings.TrimSpace(role + " " + cols.role()), Tone: tone, Actions: actions}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells(kids, owner, model.RelativeTime(row.UpdatedAt, now), cols, mono("muted"), toneStyle(ownerTone))...)}
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

// footerHint is one key of the current mode's legend: the chord a click presses, the key as drawn and what it does.
type footerHint struct{ chord, key, description string }

// footerHints are the current mode's hints in reading order, and position is the selected row's place in the list.
func footerHints(m model.Model) (hints []footerHint, position string) {
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
		if (binding.Action == model.DrillOut || binding.Action == model.ShowMenu) && m.EscGoesBack() {
			help = "back"
		}
		if binding.Action == model.ShowMenu && m.PageSearching() {
			help = "end search"
		}
		if job := m.SelectedJob(); binding.Action == model.ArchiveJobAction && job != nil && job.Archived() {
			help = "restore"
		}
		hints = append(hints, footerHint{chord: binding.Chord, key: binding.Key, description: help})
	}
	selected, total := m.Position()
	return hints, fmt.Sprintf("%d / %d", selected, total)
}

// footer is the key legend: each hint a muted text with a key span, wrapping onto more lines in a narrow Pane instead of
// losing hints, with the selected row's place in the list at the right.
func footer(m model.Model) tern.Element {
	hints, position := footerHints(m)
	cells := make([]tern.Element, len(hints))
	for i, hint := range hints {
		cells[i] = ui.Text{Common: ui.Common{Key: hint.chord, Actions: &ui.Actions{Click: KeyClick(hint.chord)}},
			Spans: []ui.Span{{T: keycap(hint.chord, hint.key), S: "key"}, {T: " " + hint.description, S: "muted"}}, Wrap: ui.WrapNone}
	}
	return ui.Nodes(ui.Row{Common: ui.Common{Key: "hints"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Row{Common: ui.Common{Key: "grid", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: cells},
		ui.Text{Common: ui.Common{Key: "position", Shrink: new(float64), Tone: ui.ToneMuted}, Spans: []ui.Span{{T: position, S: mono("muted")}}, Wrap: ui.WrapNone})})
}

// headerRow labels the Issue rows. It is an Item like the rows, so its cells sit
// where theirs do, and a node of its own, outside the scrolling list. The labels over the mono cells (id, prio, kids,
// updated) take the mono token too: the boxes are in em, so a label in another face would start its column at another x.
func headerRow(cols listColumns) tern.Element {
	left := []ui.Span{
		{T: cell("#", cols.id), S: "muted mono"},
		{T: cellGap + cell("prio", prioWidth), S: "muted mono"},
		{T: cellGap + cell("status", statusWidth), S: "muted"},
		{T: cellGap + "title", S: "muted"},
	}
	return ui.Item{Common: ui.Common{Key: "header", Role: cols.role(), Tone: ui.ToneMuted}, Label: ui.Spans(left...), Detail: ui.T(" "), Value: ui.Spans(rightCells("kids", "owner", "updated", cols, "muted mono", "muted")...)}
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
