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

var loadingMarker = ui.Span{T: "  loading…", S: "muted"}

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
		main = ui.Nodes(plainText("loading", ui.TonePending, "Loading "+m.Collection.Title+" Issues…"))
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
	cols := listColumns{ownerShown: !(slices.Contains(columns, model.ColumnList) && slices.Contains(columns, model.ColumnDetail)), id: 1, owner: ownerWidth}
	owners := make([]string, len(rows))
	for i, row := range rows {
		owners[i] = listOwner(m, row, now)
		cols.fit(displayWidth(idCell(row)), displayWidth(owners[i]))
	}
	lookups := rowLookups{lanes: m.LaneList(), badges: m.AgentBadges(), today: model.NewCalendarDay(m.Calendar.Date), width: listWidth(m), twoLine: twoLineRows(m)}
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
	if !lookups.twoLine {
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
	empty := emptyState("No Issues", "This Collection has none. Select another Collection or create one with kata create.")
	// The Board ignores the status filter, so the Lane view judges the filters without it and names an empty Lane, not the Collection.
	filters := m.Filters
	if m.LaneViewShown() {
		filters.Status = ""
	}
	switch {
	case filters != (model.Filters{}):
		empty = emptyState("No Issues match", "These filters leave none.", emptyButton(m, "clear-filters", model.ClearFilters, "Clear filters", false)...)
	case m.LaneViewShown():
		empty = emptyState("No cards", "This Lane is empty.")
	case m.Collection.IsProject():
		empty = emptyState("No Issues", "This Collection has none.", emptyButton(m, "new", model.NewIssue, "New Issue", true)...)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("issues"), Max: fillMax, Selected: selected, Empty: ui.T("Loading Issues…"), Children: items}
	children = append(children, listAndEmpty(list, m.Loading, empty)...)
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.4)}, "", children)
}

func laneHeader(m model.Model) tern.Element {
	for _, column := range m.BoardColumns() {
		if column.Lane == m.Board.Lane {
			return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: column.Title, S: "strong"}, {T: " " + strconv.Itoa(len(column.Cards)), S: mono("muted")}}}
		}
	}
	return ui.Text{Common: ui.Common{Key: "lane"}, Spans: []ui.Span{{T: model.LaneTitle(m.Board.Lane), S: "strong"}}}
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
	narrow := m.NarrowerThan(model.FoldedLegendCols)
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
	cells := ui.Nodes(ui.Row{Common: flexCell("left", ui.ToneNeutral, 0), Gap: ui.GapMD, Children: leftCells})
	right := zoneNote
	// Below FoldedLegendCols the counts and the Daemon would only be drawn as fragments, so they are left out.
	if !narrow {
		cells = append(cells, ui.Text{Common: flexCell("counts", ui.ToneMuted, 1), Spans: monoSpans(m.CountsLine(), "muted"), Wrap: ui.WrapNone})
		right = append(right,
			ui.Text{Common: flexCell("daemon", ui.ToneMuted, 100), Text: m.DaemonLine(), Wrap: ui.WrapNone},
			ui.Text{Common: flexCell("sep", ui.ToneMuted, 0), Text: model.DaemonSeparator, Wrap: ui.WrapNone},
		)
	}
	right = append(right, menuButton(m))
	cells = append(cells, ui.Row{Common: rightCell, Gap: ui.GapNone, Children: right})
	return ui.Row{Common: ui.Common{Key: "topbar", Hidden: m.HelpOpen}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: cells}
}

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

// plainText is a line of text in tone, unstyled: the tone only marks its role. See toneText for one drawn in the tone.
func plainText(key string, tone ui.Tone, text string) ui.Text {
	return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Text: text}
}

func heading(spans ...ui.Span) ui.Text {
	return ui.Text{Common: ui.Common{Key: "heading"}, Spans: spans, Wrap: ui.WrapNone}
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
	// The Agent badge is a span: an Item has one click, and Spans have none, so the badge on a row is not clickable.
	var badges []ui.Span
	if badge, ok := lookups.badges.For(row.UID); ok {
		badges = append(badges, ui.Span{T: badge.Text, S: string(badge.Tone)})
	}
	// An agent's hand-off (work.attention) reads beside the badge, whoever wrote it.
	if w, ok := model.IssueWorkAttention(row.Issue); ok {
		badges = append(badges, ui.Span{T: w.Word(), S: string(attentionTone(w))})
	}
	// An open Issue's Date badge reads after the title, as in kata tui.
	var dateBadge ui.Span
	if !model.IsClosed(row.Issue) {
		if date, ok := lookups.today.ChooseDateBadge(row.Issue); ok {
			dateBadge = ui.Span{T: date.Text, S: mono(toneStyle(dateTones[date.Tone]))}
		}
	}
	parts := rowParts{
		id:         ui.Span{T: idCell(row), S: mono(toneStyle(issueLaneToneIn(lookups.lanes, row.Issue)))},
		prio:       ui.Span{T: prio, S: mono(prioStyle)},
		status:     ui.Span{T: row.Status, S: statusStyle},
		badges:     badges,
		title:      title,
		date:       dateBadge,
		kids:       kids,
		owner:      owner,
		updated:    model.RelativeTime(row.UpdatedAt, now),
		cellStyle:  mono("muted"),
		ownerStyle: toneStyle(ownerTone),
	}
	if lookups.twoLine {
		return twoLineRow(row, parts, selected, role, menu)
	}
	laid := layoutRow(cols, lookups.width, parts)
	// A parent's click folds it; a double click on any row opens it. Tern sends the two clicks first, so a parent's fold ends where it began.
	entries := menu.of(row.Issue)
	actions := &ui.Actions{DblClick: OpenRowClick(row.UID), Menu: entries, Hold: entries}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
	return ui.Item{Common: ui.Common{Key: row.UID, Role: strings.TrimSpace(role + " " + cols.role()), Tone: tone, Actions: actions}, Label: ui.Spans(laid.label...), Detail: ui.T(strings.Repeat(" ", itemDetailWidth)), Value: ui.Spans(laid.value...)}
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
	entries := menu.of(row.Issue)
	actions := &ui.Actions{Click: CardClick(row.UID), DblClick: OpenRowClick(row.UID), Menu: entries, Hold: entries}
	if row.HasChildren {
		actions.Click = ToggleClick(row.UID)
	}
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
	selected, total := m.Position()
	return hints, fmt.Sprintf("%d / %d", selected, total)
}

func footer(m model.Model) tern.Element {
	hints, position := footerHints(m)
	if m.LegendFolded() {
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.chord != "escape" })
	}
	cells := make([]tern.Element, len(hints))
	for i, hint := range hints {
		cells[i] = ui.Text{Common: ui.Common{Key: hint.chord, Actions: &ui.Actions{Click: KeyClick(hint.chord)}},
			Spans: []ui.Span{{T: keycap(hint.chord, hint.key), S: "key"}, {T: " " + hint.description, S: "muted"}}, Wrap: ui.WrapNone}
	}
	row := ui.Nodes(ui.Row{Common: ui.Common{Key: "grid", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: cells})
	if m.LegendChip() {
		row = append(row, ui.Button{Common: ui.Common{Key: "keys", Shrink: new(float64), Actions: &ui.Actions{Click: LegendClick()}}, Text: "keys"})
	}
	row = append(row, ui.Text{Common: ui.Common{Key: "position", Shrink: new(float64), Tone: ui.ToneMuted}, Spans: []ui.Span{{T: position, S: mono("muted")}}, Wrap: ui.WrapNone})
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
	if displayWidth(owner.Name) <= ownerWidth {
		return owner.String()
	}
	return cell(owner.Name, ownerWidth) + owner.Until
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
