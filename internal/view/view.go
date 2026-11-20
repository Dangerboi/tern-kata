package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/mattn/go-runewidth"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// Cell widths of an Issue row, in characters.
// The style sheet gives each cell a box of its column's width (cellBoxCSS), so
// a cell lines up with the header above it. The title takes the rest of the row and is cut with an ellipsis when it does
// not fit.
const (
	prioWidth     = 8
	statusWidth   = 6
	kidsWidth     = 8
	ownerWidth    = 8  // the Owner column's least width
	ownerMaxWidth = 14 // the longest owner name a row shows whole
	updatedWidth  = 8
	cellGap       = "  "
)

const IssueListID = "main.columns.list.issues"

var loadingMarker = ui.Span{T: " loading…", S: "muted"}

func Render(m model.Model, now time.Time) tern.View {
	if len(m.VisibleColumns()) == 0 {
		return narrowView()
	}
	var main tern.Nodes
	switch {
	case m.HelpOpen && (m.Err != nil || m.FirstLoad()):
		main = help(m)
	case m.OnErrorScreen():
		main = failureView(m)
	case m.FirstLoad():
		// The top bar stays while the first load is out, so the Pane is not blank; the list area says what it waits for.
		main = append(append(ui.Nodes(topBar(m)), pageLine(m)...), plainText("loading", ui.TonePending, model.Progress("Loading", "Issues")))
	default:
		columns := make([]tern.Element, 0, 3)
		for _, column := range m.VisibleColumns() {
			switch column {
			case model.ColumnCollections:
				columns = append(columns, Collections(m))
			case model.ColumnList:
				switch {
				case m.OnConfig():
					columns = append(columns, configPage(m))
				case m.OnDaemons():
					columns = append(columns, daemons(m))
				case m.OnFederation():
					columns = append(columns, federation(m, now))
				case m.OnAttention():
					columns = append(columns, attention(m, now))
				case m.OnCredentials():
					columns = append(columns, credentials(m, now))
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
		main = append(append(ui.Nodes(topBar(m)), pageLine(m)...), contextLine(m)...)
		main = append(main, ui.Row{Common: ui.Common{Key: "columns", Hidden: m.HelpOpen, Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: columns})
		if m.HelpOpen {
			// The top bar, filters and columns stay mounted but hidden, so Tern keeps their scroll offsets for when the sheet closes.
			main = append(main, help(m)...)
		}
	}
	main = append(main, footerNodes(m)...)
	main = append(main, toasts(m)...)
	layer := topLayer(m, now)
	if m.CloseForm.Err != nil && !m.CloseForm.Open {
		main = append(main, plainText("close-error", ui.ToneError, m.CloseForm.Err.Error()))
	}
	if m.HelpOpen {
		return tern.View{Main: main}
	}
	return tern.View{Main: main, Dock: footer(m), Layer: layer}
}

func issues(m model.Model, now time.Time) tern.Element {
	columns := m.VisibleColumns()
	rows := m.VisibleRows()
	lookups := rowLookups{lanes: m.LaneList(), badges: m.AgentBadges(), today: model.NewCalendarDay(m.Calendar.Date), width: listWidth(m), twoLine: twoLineRows(m)}
	// A column is drawn only while some visible row has a value for it; Status is the Lane view's own Lane, so it is left out there.
	cols := listColumns{statusShown: !m.LaneViewShown(), id: 2, owner: ownerWidth}
	owners := make([]string, len(rows))
	for i, row := range rows {
		owners[i] = listOwner(m, row, now)
		cols.fit(displayWidth(idCell(row)), displayWidth(owners[i]), displayWidth(rowDate(lookups.today, row).T))
		cols.prioShown = cols.prioShown || row.Priority != nil
		cols.kidsShown = cols.kidsShown || row.TotalKids > 0
		cols.ownerShown = cols.ownerShown || owners[i] != ""
	}
	// Beside the detail the list is too narrow for the owner.
	if slices.Contains(columns, model.ColumnList) && slices.Contains(columns, model.ColumnDetail) {
		cols.ownerShown = false
	}
	cols.fitDate(lookups.width)
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
	selected := selectedItem(IssueListID, m.SelectedUID)
	children := ui.Nodes()
	if m.LaneViewShown() {
		children = append(children, laneHeader(m))
	}
	// A settled empty list has nothing to label, so no header is drawn over its empty state.
	if !lookups.twoLine && len(rows) > 0 {
		children = append(children, headerRow(cols))
	}
	if m.Search.Open {
		children = append(children, searchInput(m))
	}
	if line := m.SearchLine(); line != "" {
		children = append(children, plainText("search-line", ui.ToneMuted, line))
		if m.SearchTurnOnOffered() {
			children = append(children, emptyButton(m, "search-turn-on", model.TurnOnSearchMeaning, "Turn on…", false)...)
		}
	}
	empty := ""
	if m.Loading {
		empty = model.Progress("Loading", "Issues")
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: fillMax, Selected: selected, Empty: ui.T(empty), Children: items}
	children = append(children, listAndEmpty(list, m.Loading, listEmpty(m))...)
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, "", children)
}

// listEmpty is the Issues list's empty state, for the Issues Page and the Board's Lane view: what is missing and how to get one.
// The Board ignores the status filter, so the Lane view judges the filters without it and names an empty Lane, not the
// Collection. A search alone says what it did not find; any other filter says to change or clear it.
func listEmpty(m model.Model) ui.Empty {
	filters := m.Filters
	if m.LaneViewShown() {
		filters.Status = ""
	}
	switch {
	case filters != (model.Filters{}) && filters == (model.Filters{Search: filters.Search}):
		return emptyState("No Issues match", "Nothing matches "+strconv.Quote(filters.Search)+".")
	case filters != (model.Filters{}):
		return emptyState("No Issues match", "Change or clear the filters.", emptyButton(m, "clear-filters", model.ClearFilters, "Clear filters", false)...)
	case m.LaneViewShown():
		return emptyState("No cards", "This Lane is empty.")
	case m.Collection.IsProject():
		return emptyState("No Issues", "Create the first one.", emptyButton(m, "new", model.NewIssue, "New Issue", true)...)
	}
	return emptyState("No Issues", "Pick another Project or view from the Menu.")
}

// laneHeader names the Lane the Lane view lists: its Lane tone Badge and the muted count of its cards, as the Board's header does.
func laneHeader(m model.Model) tern.Element {
	title, count, tone := model.LaneTitle(m.Board.Lane), "", laneTone(m.LaneList(), m.Board.Lane)
	for _, column := range m.BoardColumns() {
		if column.Lane == m.Board.Lane {
			title, count = column.Title, strconv.Itoa(len(column.Cards))
		}
	}
	cells := ui.Nodes(ui.Badge{Common: ui.Common{Key: "title", Tone: tone}, Text: title})
	if count != "" {
		cells = append(cells, ui.Text{Common: ui.Common{Key: "count"}, Spans: []ui.Span{{T: count, S: "muted"}}, Wrap: ui.WrapNone})
	}
	return ui.Row{Common: ui.Common{Key: "lane"}, Gap: ui.GapSM, Children: cells}
}

func searchInput(m model.Model) tern.Element {
	placeholder := "Search Issues in " + m.Collection.Title
	if m.Search.Meaning {
		placeholder = "Search Issues by meaning in " + m.Collection.Title
	}
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search Issues"}, Field: ui.Field{Text: m.Filters.Search, Cursor: caretAt(m.Search.Caret, "", m.Filters.Search), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: m.Search.Results, Prompt: []ui.Span{{T: "/ "}}}, Clear: true}
}

func pageSearchInput(m model.Model) tern.Element {
	noun := m.PageSearchNoun()
	text := m.PageSearch.Text
	return ui.Input{Common: ui.Common{Key: "search", Tone: ui.ToneAccent, Aria: "Search " + noun}, Field: ui.Field{Text: text, Cursor: caretAt(m.PageSearch.Caret, "", text), Mode: "insert", Placeholder: ui.T("Search " + noun), Readonly: !m.PageSearch.Open, Prompt: []ui.Span{{T: "/ "}}}, Clear: true}
}

func searchEmpty(m model.Model) ui.Empty {
	return emptyState("No "+m.PageSearchNoun()+" match", "Nothing matches "+strconv.Quote(m.PageSearch.Text)+".")
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

// topBar is the pinned row above the columns: the Collection title on the left with its markers (loading, and the update chip
// while a version notice holds), the Page bar, and at the right the muted facts line (the Daemon, then this binary's version)
// and the Menu button. When the row is too narrow Tern's flex shrinks and truncates the facts, this binary's version first; the
// left cell and the button never shrink. The title keeps the accent whichever column has focus: it names what the Pane shows,
// and the columns' own borders show focus.
func topBar(m model.Model) tern.Element {
	heading := []ui.Span{{T: barTitle(m), S: "title accent"}}
	if m.Loading {
		heading = append(heading, loadingMarker)
	}
	flexCell := func(key string, tone ui.Tone, shrink float64) ui.Common {
		return ui.Common{Key: key, Tone: tone, Shrink: &shrink}
	}
	headingCell := flexCell("heading", ui.ToneAccent, 0)
	rightCell := flexCell("right", ui.ToneNeutral, 100)
	// The right cell never narrows below the Menu button, so the button is never pushed off the bar; the facts inside give way first.
	rightCell.Min = &ui.Bounds{W: ui.Ch(10)}
	leftCells := ui.Nodes(ui.Text{Common: headingCell, Spans: heading, Wrap: ui.WrapNone})
	if chip := m.VersionChip(); chip != "" {
		update := flexCell("update", ui.ToneWarning, 0)
		update.Aria, update.Actions = "Updates available", &ui.Actions{Click: click(MenuClick, model.MenuOnNotice)}
		leftCells = append(leftCells, ui.Text{Common: update, Spans: []ui.Span{{T: chip, S: "warning"}}, Wrap: ui.WrapNone})
	}
	if fit := topBarFit(m); fit != barOwnLine {
		leftCells = append(leftCells, pageBar(m, flexCell, fit == barCompact))
	}
	cells := ui.Nodes(ui.Row{Common: flexCell("left", ui.ToneNeutral, 0), Gap: ui.GapMD, Children: leftCells})
	right := ui.Nodes(topFacts(m), menuButton(m))
	cells = append(cells, ui.Row{Common: rightCell, Gap: ui.GapMD, Children: right})
	return ui.Row{Common: ui.Common{Key: "topbar", Hidden: m.HelpOpen}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: cells}
}

// topFactCell is one cell of the top bar's facts line: its spans without the separator, and how soon Tern's flex shrinks it.
type topFactCell struct {
	key    string
	spans  []ui.Span
	shrink float64
}

// topFacts is the muted facts line: the zone note when the Pane's zone is unknown, then the Daemon's facts and this binary's
// version, separated by " · " that belongs to the cell after it, so it shrinks away with its cell and none is left dangling. The
// versions are mono, the rest sans. The squeeze order is this binary's version, then kata's, then the account; below
// FoldedLegendCols only a named or remote Daemon is told and no version is.
func topFacts(m model.Model) tern.Element {
	narrow := m.NarrowerThan(model.FoldedLegendCols)
	var cells []topFactCell
	if m.Calendar.Note != "" {
		cells = append(cells, topFactCell{key: "zone-note", spans: []ui.Span{{T: m.Calendar.Note, S: "warning"}}})
	}
	for i, fact := range m.DaemonFacts() {
		shrink := 1.0
		switch {
		case fact.Version != "":
			shrink = 50
		case strings.HasPrefix(fact.Text, "as "):
			shrink = 10
		}
		if narrow && (fact.Version != "" || fact.Text == string(model.DaemonLocal) || strings.HasPrefix(fact.Text, "as ")) {
			continue
		}
		cells = append(cells, topFactCell{key: "fact" + strconv.Itoa(i), spans: factSpans(fact), shrink: shrink})
	}
	if m.Versions.Running != "" && !narrow {
		cells = append(cells, topFactCell{key: "version", spans: factSpans(model.Fact{Text: "tern-kata", Version: m.Versions.Running}), shrink: 100})
	}
	nodes := make([]tern.Element, len(cells))
	for i, c := range cells {
		spans := c.spans
		if i > 0 {
			spans = append([]ui.Span{{T: " · ", S: "muted"}}, spans...)
		}
		nodes[i] = ui.Text{Common: ui.Common{Key: c.key, Tone: ui.ToneMuted, Shrink: &c.shrink}, Spans: spans, Wrap: ui.WrapNone}
	}
	return ui.Row{Common: ui.Common{Key: "facts", Shrink: new(float64)}, Gap: ui.GapNone, Children: nodes}
}

// factSpans draws a fact in muted text, its version in the mono face.
func factSpans(f model.Fact) []ui.Span {
	if f.Version == "" {
		return []ui.Span{{T: f.Text, S: "muted"}}
	}
	return []ui.Span{{T: f.Text + " ", S: "muted"}, monoSpan(f.Version, "muted")}
}

// contextLine is the muted line under the top bar. On the Issues Page it is permanent, so the list does not jump when a filter
// toggles: the counts "5 open · 0 closed", then the filter chips (a muted field name, then its value) and a note naming where
// they came from. Elsewhere, and in the Lane view, it shows only while a filter, a note or a search is active. A search chip is
// left out while the search input is open: the input already shows it.
func contextLine(m model.Model) []tern.Element {
	if m.OnJobs() || m.OnRuns() {
		return nil
	}
	terms, note := m.FilterLine()
	terms = slices.DeleteFunc(slices.Clone(terms), func(term model.FilterTerm) bool {
		return term.Field == model.FilterSearch && m.Search.Open
	})
	counts := ""
	if m.ShowCounts() && !m.LaneViewShown() {
		counts = m.CountsLine()
	}
	if counts == "" && len(terms) == 0 && note == (model.FilterNote{}) {
		return nil
	}
	var cells []tern.Element
	if counts != "" {
		cells = append(cells, ui.Text{Common: ui.Common{Key: "counts", Tone: ui.ToneMuted}, Spans: []ui.Span{{T: counts, S: "muted"}}, Wrap: ui.WrapNone})
	}
	if len(terms) > 0 || note != (model.FilterNote{}) {
		cells = append(cells, ui.Icon{Common: ui.Common{Key: "icon", Tone: ui.ToneMuted, Aria: "Filters"}, Name: ui.IconFunnel})
	}
	for _, term := range terms {
		cells = append(cells, ui.Text{Common: ui.Common{Key: string(term.Field)}, Spans: filterChip(term), Wrap: ui.WrapNone})
	}
	if note != (model.FilterNote{}) {
		spans := []ui.Span{{T: note.Label, S: "muted"}}
		if note.Value != "" {
			spans = append(spans, ui.Span{T: " " + note.Value})
		}
		cells = append(cells, ui.Text{Common: ui.Common{Key: "note"}, Spans: spans, Wrap: ui.WrapNone})
	}
	return ui.Nodes(ui.Row{Common: ui.Common{Key: "filters", Hidden: m.HelpOpen}, Gap: ui.GapMD, Wrap: true, Children: cells})
}

// filterChip is a filter term as its muted field name and then its value, as the Filter card words them.
func filterChip(term model.FilterTerm) []ui.Span {
	value := term.Value
	if term.Field == model.FilterRelationship {
		value = model.FilterChoiceLabel(model.FilterRelationships, value)
	}
	return []ui.Span{{T: model.FilterFieldLabel(term.Field), S: "muted"}, {T: " " + value}}
}

// panel is a column's container. With more than one column showing it is a
// bordered Card: the focused column's ring is Tern's accent (Selected), the
// other's the muted tone. A lone column has no border and spaces its children
// by gap. Either way it is clipped to the height its Row gives it.
func panel(m model.Model, column model.Column, common ui.Common, gap ui.Gap, children []tern.Element) tern.Element {
	common.Max = &ui.Bounds{H: ui.Frac(1)}
	common.Role = ColumnRole
	if len(m.VisibleColumns()) < 2 {
		return ui.Col{Common: common, Gap: gap, Children: children}
	}
	if m.Focus != column {
		common.Tone = ui.ToneMuted
	}
	return ui.Card{Common: common, Selected: m.Focus == column, Children: children}
}

// plainText is a line of text in tone, unstyled: the tone only marks its role. See toneText for one drawn in the tone.
func plainText(key string, tone ui.Tone, text string) ui.Text {
	return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Text: text}
}

func listPanel(m model.Model, basis float64, children []tern.Element) tern.Element {
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(basis)}, ui.GapSM, children)
}

func noticeLines(failed, settled bool, text, detail string) []tern.Element {
	tone := ui.TonePending
	switch {
	case failed:
		tone = ui.ToneError
	case settled:
		tone = ui.ToneMuted
	}
	lines := ui.Nodes(plainText("notice", tone, text))
	if detail != "" {
		lines = append(lines, plainText("detail", ui.ToneMuted, detail))
	}
	return lines
}

func headRow(children ...tern.Element) ui.Row {
	return ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: children}
}

// idCell is an Issue row's id: two cells of indent per level of depth, a one-cell fold gutter (▸ folded, ▾ open, blank when the
// row has no children), then the id.
func idCell(row model.IssueRow) string {
	gutter := " "
	if row.HasChildren {
		gutter = "▸"
		if row.Expanded {
			gutter = "▾"
		}
	}
	return strings.Repeat("  ", row.Depth) + gutter + row.ShortID
}

// rowDate is an open Issue's Date badge, in the Board's words, drawn as plain words in the badge's tone; none for a closed Issue.
func rowDate(today model.CalendarDay, row model.IssueRow) ui.Span {
	if model.IsClosed(row.Issue) {
		return ui.Span{}
	}
	if date, ok := today.ChooseDateBadge(row.Issue); ok {
		return ui.Span{T: date.Text, S: toneStyle(dateTones[date.Tone])}
	}
	return ui.Span{}
}

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

func stripeRole(i int) string {
	if i%2 == 1 {
		return StripeRole
	}
	return ""
}

type rowLookups struct {
	lanes   []string
	badges  model.AgentBadgeIndex
	today   model.CalendarDay
	width   int  // the list column's width in cells
	twoLine bool // whether a row takes two lines, the Pane being under model.TwoLineRowCols wide
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

// twoLineRows is whether the list's Issue rows take two lines: the Pane is under model.TwoLineRowCols wide.
func twoLineRows(m model.Model) bool { return m.NarrowerThan(model.TwoLineRowCols) }

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

const itemDetailWidth = 1

const rowPadding = 4

func issueRow(row model.IssueRow, owner string, lookups rowLookups, selected bool, role string, cols listColumns, menu contextMenuKeys, now time.Time) tern.Element {
	tone := ui.ToneNeutral
	closed := model.IsClosed(row.Issue)
	if closed {
		tone = ui.ToneMuted
	}
	if selected {
		tone = ui.ToneAccent
	}
	prio, prioTone := priorityText(row.Priority)
	kids := ""
	if row.TotalKids > 0 {
		kids = fmt.Sprintf("%d/%d", row.OpenKids, row.TotalKids)
	}
	// The Agent badge is a span: an Item has one click, and Spans have none, so the badge on a row is not clickable.
	var flags []ui.Span
	badge, agent := lookups.badges.For(row.UID)
	agentFlags := 0
	if agent {
		agentFlags = 1
		flags = append(flags, ui.Span{T: badge.Text, S: string(badge.Tone)})
		if badge.Unread {
			agentFlags++
			flags = append(flags, ui.Span{T: model.UnreadMessageWords, S: string(ui.ToneWarning)})
		}
	}
	// An agent's hand-off (work.attention) reads beside the badge, whoever wrote it.
	if w, ok := model.IssueWorkAttention(row.Issue); ok {
		flags = append(flags, ui.Span{T: w.Word(), S: string(attentionTone(w))})
	}
	parts := rowParts{
		id:         monoSpan(idCell(row), "muted"),
		prio:       ui.Span{T: prio, S: mono(toneStyle(prioTone))},
		status:     ui.Span{T: row.Status, S: cellStyleOrDefault(toneStyle(issueStateTone(closed)))},
		flags:      flags,
		agentFlags: agentFlags,
		title:      issueTitle(row.Issue),
		date:       rowDate(lookups.today, row),
		kids:       kids,
		owner:      owner,
		updated:    model.RelativeTime(row.UpdatedAt, now),
		cellStyle:  mono("muted"),
		ownerStyle: "",
	}
	if lookups.twoLine {
		return twoLineRow(row, parts, selected, role, menu)
	}
	laid := layoutRow(cols, lookups.width, parts)
	return ui.Item{Common: ui.Common{Key: row.UID, Title: laid.tip, Role: strings.TrimSpace(role + " " + cols.role()), Tone: tone, Actions: rowActions(row.Issue, row.HasChildren, menu, "")}, Label: ui.Spans(laid.label...), Detail: ui.T(strings.Repeat(" ", itemDetailWidth)), Value: ui.Spans(laid.value...)}
}

// rowActions are an Issue row's or card's actions: click (a parent's click folds it instead), the double click that opens it, and
// its context menu as the right-click menu and the long press. Tern sends a double click's two clicks first, so a parent's fold
// ends where it began.
func rowActions(issue model.Issue, parent bool, menu contextMenuKeys, click string) *ui.Actions {
	entries := menu.of(issue)
	actions := &ui.Actions{Click: click, DblClick: OpenRowClick(issue.UID), Menu: entries, Hold: entries}
	if parent {
		actions.Click = ToggleClick(issue.UID)
	}
	return actions
}

// twoLineRow is an Issue row of two lines, one node: a Col, because an Item has one line and a Card draws a border. The
// selected row takes TwoLineSelectedRole, which TwoLineRowCSS paints like a selected one-line row. Its click selects the row, or
// folds a parent (which selects it first); the double click, menu and hold are the one-line row's.
func twoLineRow(row model.IssueRow, p rowParts, selected bool, role string, menu contextMenuKeys) tern.Element {
	laid := layoutTwoLine(p)
	tone := ui.ToneNeutral
	if model.IsClosed(row.Issue) {
		tone = ui.ToneMuted
	}
	actions := rowActions(row.Issue, row.HasChildren, menu, CardClick(row.UID))
	role = strings.TrimSpace(role + " " + TwoLineRowRole)
	if selected {
		role += " " + TwoLineSelectedRole
	}
	line := ui.Row{Common: ui.Common{Key: "line"}, Gap: ui.GapSM, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "head", Grow: 1}, Spans: laid.head, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "meta", Shrink: new(float64)}, Spans: laid.meta, Wrap: ui.WrapNone},
	)}
	return ui.Col{
		Common:   ui.Common{Key: row.UID, Role: role, Tone: tone, Actions: actions},
		Children: ui.Nodes(line, ui.Text{Common: ui.Common{Key: "title"}, Spans: laid.title, Lines: 2}),
	}
}

type footerHint struct{ chord, key, description string }

func footerHints(m model.Model) (hints []footerHint, position string) {
	for binding := range model.BindingsForMode(m.CurrentMode(), true) {
		if !binding.Offered(m) {
			continue
		}
		if binding.Action == model.FocusDetail && !m.CanFocusDetail() {
			continue
		}
		if binding.Action == model.OverwriteDraft && m.TopCard() == model.CardWorkflow && m.Workflows.Card.Conflict != nil && m.Workflows.Card.Conflict.Archived {
			continue
		}
		if binding.Action == model.MenuBack && m.SidebarLevel == model.MenuTop {
			continue
		}
		help := binding.Described(m)
		if binding.Action == model.ClosePicker && m.Picker.Level != model.MenuTop && m.Picker.Filter == "" {
			help = "back"
		}
		if (binding.Action == model.DrillOut || binding.Action == model.ShowMenu) && m.EscGoesBack() {
			help = "back"
		}
		if (binding.Action == model.DrillOut || binding.Action == model.ShowMenu) && m.EscEndsSearch() {
			help = "end search"
		}
		if job := m.SelectedJob(); binding.Action == model.ArchiveJobAction && job != nil && job.Archived() {
			help = "restore"
		}
		hints = append(hints, footerHint{chord: binding.Chord, key: binding.Key, description: help})
	}
	// Page hints first, then esc, ?, and quit, on every Page and mode.
	rank := func(h footerHint) int {
		switch h.chord {
		case "escape":
			return 1
		case "?":
			return 2
		case "q":
			return 3
		}
		return 0
	}
	slices.SortStableFunc(hints, func(a, b footerHint) int { return rank(a) - rank(b) })
	// The position counter is drawn only while the Page's list has rows, and never on the error screen.
	selected, total := m.Position()
	if total == 0 || m.OnErrorScreen() {
		return hints, ""
	}
	return hints, fmt.Sprintf("%d of %d", selected, total)
}

func footer(m model.Model) tern.Element {
	hints, position := footerHints(m)
	if m.LegendFolded() {
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.chord != "escape" })
	}
	cells := make([]tern.Element, len(hints))
	for i, hint := range hints {
		cells[i] = ui.Text{Common: ui.Common{Key: hint.chord, Actions: &ui.Actions{Click: KeyClick(hint.chord)}},
			Spans: []ui.Span{keySpan(keycap(hint.chord, hint.key)), {T: " " + hint.description, S: "muted"}}, Wrap: ui.WrapNone}
	}
	row := ui.Nodes(ui.Row{Common: ui.Common{Key: "grid", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: cells})
	if m.LegendChip() {
		row = append(row, ui.Button{Common: ui.Common{Key: "keys", Shrink: new(float64), Actions: &ui.Actions{Click: LegendClick()}}, Text: "keys"})
	}
	if position != "" {
		row = append(row, ui.Text{Common: ui.Common{Key: "position", Shrink: new(float64), Tone: ui.ToneMuted}, Spans: []ui.Span{{T: position, S: "muted"}}, Wrap: ui.WrapNone})
	}
	return ui.Nodes(ui.Row{Common: ui.Common{Key: "hints"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: row})
}

// headerRow labels the Issue rows. It is an Item like the rows, so its cells sit
// where theirs do, and a node of its own, outside the scrolling list. The labels over the mono cells (id, prio, kids,
// updated) take the mono token too: the boxes are in em, so a label in another face would start its column at another x.
func headerRow(cols listColumns) tern.Element {
	header := layoutHeader(cols)
	return ui.Item{Common: ui.Common{Key: "header", Role: cols.role(), Tone: ui.ToneMuted}, Label: ui.Spans(header.label...), Detail: ui.T(" "), Value: ui.Spans(header.value...)}
}

func listOwner(m model.Model, row model.IssueRow, now time.Time) string {
	owner := m.OwnerText(row.Owner, row.AssignmentExpiresOn, now)
	if displayWidth(owner.Name) <= ownerMaxWidth {
		return owner.String()
	}
	return cell(owner.Name, ownerMaxWidth) + owner.Until
}

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
