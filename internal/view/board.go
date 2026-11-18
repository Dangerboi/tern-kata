package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const BoardID = "main.columns.list.board"

const LaneHeaderID = "main.columns.list.lane"

func laneKey(column int) string { return "lane" + strconv.Itoa(column) }

func CardID(m model.Model, uid string) string {
	if col, _ := model.CardAt(m.BoardColumns(), uid); col >= 0 {
		return itemID(itemID(itemID(BoardID, laneKey(col)), "cards"), uid)
	}
	return ""
}

func SelectedRowID(m model.Model) string {
	if node, ok := listNodes[m.ListNode()]; ok {
		return node.row(m)
	}
	if m.BoardShown() {
		return CardID(m, m.SelectedUID)
	}
	return IssueID(m.SelectedUID)
}

// board is the Board: one column per Lane, No lane first and Done last, each a header over its cards. It takes the list column's
// place. A Lane is a column, not a Card: a Card's body sizes to its content, so only a column's list fills the Lane down to the
// bottom and scrolls inside it, which keeps the Pane itself from scrolling the top bar away. The Lanes' row is a layout
// transition (Tern 0.9.27+; older Terns snap): a card, whose motion key is its Issue's uid, glides to its new place when its
// Issue moves to another Lane or its Lane reorders.
func board(m model.Model, now time.Time) tern.Element {
	columns := m.BoardColumns()
	laneList := m.LaneList()
	tones := make([]ui.Tone, len(columns))
	for i, column := range columns {
		tones[i] = laneTone(laneList, column.Lane)
	}
	var selected model.Relationships
	if m.SelectedUID != "" {
		selected = m.Relationships.Of(m.SelectedUID)
	}
	today := model.NewCalendarDay(m.Calendar.Date)
	badges := m.AgentBadges()
	lanes := make([]tern.Element, len(columns))
	menu := newContextMenuKeys(model.ModeBoard, model.OpenLane)
	for i, column := range columns {
		cards := make([]tern.Element, len(column.Cards))
		for j, card := range column.Cards {
			cards[j] = boardCard(m, card, tones[i], column.Lane == model.LaneDone, selected, menu, today, badges, now)
		}
		count := strconv.Itoa(len(column.Cards))
		if column.Total > len(column.Cards) {
			count += " of " + strconv.Itoa(column.Total)
		}
		head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Children: ui.Nodes(
			ui.Badge{Common: ui.Common{Key: "title", Tone: tones[i]}, Text: column.Title},
			ui.Text{Common: ui.Common{Key: "count"}, Spans: []ui.Span{{T: count, S: "muted"}}, Wrap: ui.WrapNone},
		)}
		lanes[i] = ui.Col{
			Common: ui.Common{Key: laneKey(i), Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}},
			Gap:    ui.GapSM,
			Children: append(ui.Nodes(
				head,
				ui.Rule{Common: ui.Common{Key: "rule", Tone: tones[i]}},
			), ui.List{CommonWithoutMax: scrollerCommon("cards"), Max: fillMax, Children: cards}),
		}
	}
	children := []tern.Element{}
	if m.Search.Open {
		children = append(children, searchInput(m))
	}
	row := ui.Row{Common: ui.Common{Key: "board", Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{H: ui.Lines(0)}, Transition: ui.TransitionLayout}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: lanes}
	// With no card in any Lane and nothing loading the Board says why, as the Issues list does. The Lanes stay mounted, hidden, so
	// the cards keep their motion when Issues come back.
	if !m.Loading && boardEmpty(columns) {
		row.Hidden = true
		children = append(children, row, boardEmptyState(m))
	} else {
		children = append(children, row)
	}
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1}, "", children)
}

var dateTones = map[model.DateTone]ui.Tone{
	model.DateMuted:   ui.ToneMuted,
	model.DateWarning: ui.ToneWarning,
	model.DateError:   ui.ToneError,
}

// boardEmpty says no Lane holds a card.
func boardEmpty(columns []model.BoardColumn) bool {
	for _, column := range columns {
		if len(column.Cards) > 0 {
			return false
		}
	}
	return true
}

// boardEmptyState is the Issues list's empty state for the same cause: filters that leave none, else a Collection with none. The
// Board ignores the status filter, so it is left out of the judgement.
func boardEmptyState(m model.Model) ui.Empty {
	filters := m.Filters
	filters.Status = ""
	var e ui.Empty
	switch {
	case filters != (model.Filters{}):
		e = emptyState("No Issues match", "These filters leave none.", emptyButton(m, "clear-filters", model.ClearFilters, "Clear filters", false)...)
	case m.Collection.IsProject():
		e = emptyState("No Issues", "This Collection has none.", emptyButton(m, "new", model.NewIssue, "New Issue", true)...)
	default:
		e = emptyState("No Issues", "This Collection has none. Select another Collection or create one with kata create.")
	}
	e.Key = "board-empty"
	return e
}

func boardCard(m model.Model, card model.BoardCard, tone ui.Tone, done bool, selectedRels model.Relationships, menu contextMenuKeys, today model.CalendarDay, badges model.AgentBadgeIndex, now time.Time) tern.Element {
	issue := card.Issue
	selected := issue.UID == m.SelectedUID
	titleStyle := ""
	if done {
		titleStyle = "muted"
	}
	title := ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: issueTitle(issue), S: titleStyle}}, Lines: 2}
	if selected {
		title.Lines = 0
	}
	ids := ui.Nodes(ui.Badge{Common: ui.Common{Key: "short", Tone: tone, Role: MonoRole}, Text: issue.ShortID})
	if text, prioTone := priorityText(issue.Priority); text != "" {
		ids = append(ids, ui.Badge{Common: ui.Common{Key: "prio", Tone: prioTone, Role: MonoRole}, Text: text})
	}
	var badge model.DateBadge
	if !done {
		var ok bool
		if badge, ok = today.ChooseDateBadge(issue); ok {
			ids = append(ids, ui.Badge{Common: ui.Common{Key: "date", Tone: dateTones[badge.Tone]}, Text: badge.Text})
		}
	}
	cardTone := ui.ToneNeutral
	if done {
		cardTone = ui.ToneMuted
	}
	body := ui.Nodes(title)
	if tags := selectedRels.HighlightsOf(issue.UID); len(tags) > 0 && !selected {
		// The ring warns only for a blocking pair; every other relation is told in words alone.
		tagStyle := "muted"
		if tags[0] == model.HighlightBlocksThis || tags[0] == model.HighlightBlockedByThis {
			tagStyle = "warning"
			if !done {
				cardTone = ui.ToneWarning
			}
		}
		words := make([]string, len(tags))
		for i, tag := range tags {
			words[i] = string(tag)
		}
		body = append(body, ui.Text{Common: ui.Common{Key: "tag-" + string(tags[0])}, Spans: []ui.Span{{T: strings.Join(words, ", "), S: tagStyle}}})
	}
	if len(card.Labels) > 0 {
		chips := make([]tern.Element, len(card.Labels))
		for i, label := range card.Labels {
			chips[i] = ui.Badge{Common: ui.Common{Key: label, Tone: ui.ToneNeutral}, Text: label}
		}
		body = append(body, ui.Row{Common: ui.Common{Key: "labels"}, Gap: ui.GapSM, Wrap: true, Children: chips})
	}
	if len(card.Also) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "also"}, Spans: []ui.Span{{T: "also in " + strings.Join(card.Also, ", "), S: "muted"}}})
	}
	if !done {
		if spans := blockedSpans(card); spans != nil {
			body = append(body, ui.Text{Common: ui.Common{Key: "blocked", Hovercard: blockedCard(m, card, now)}, Spans: spans})
		}
	}
	if badge, ok := badges.For(issue.UID); ok {
		state := agentBadgeElement(badge, "agent")
		state.Text = badge.Word
		body = append(body, state)
		if unread, ok := agentUnreadElement(badge, "agent-unread"); ok {
			body = append(body, unread)
		}
	}
	if w, ok := model.IssueWorkAttention(issue); ok {
		body = append(body, ui.Badge{Common: ui.Common{Key: "attention", Tone: attentionTone(w)}, Text: w.Word()})
	}
	if selected {
		if row := cardFacts(m, card, badge.Other, now); row != nil {
			body = append(body, row)
		}
	}
	body = slices.Insert(body, 0, tern.Element(ui.Row{Common: ui.Common{Key: "id"}, Gap: ui.GapSM, Wrap: true, Children: ids}))
	return ui.Card{
		Common:   ui.Common{Key: issue.UID, MotionKey: issue.UID, Tone: cardTone, Actions: rowActions(issue, false, menu, CardClick(issue.UID))},
		Selected: selected,
		Children: body,
	}
}

const maxRelationshipIDs = 3

func blockedSpans(card model.BoardCard) []ui.Span {
	blockers := card.Relationships.OpenBlockers()
	if len(blockers) == 0 {
		return nil
	}
	shown, more := shownRefs(blockers)
	spans := []ui.Span{{T: "blocked by ", S: "warning"}}
	for i, r := range shown {
		if i > 0 {
			spans = append(spans, ui.Span{T: ", ", S: "warning"})
		}
		spans = append(spans, ui.Span{T: r.ID(card.Issue.ProjectName), S: mono("warning")})
	}
	if more > 0 {
		spans = append(spans, ui.Span{T: " ", S: "warning"}, ui.Span{T: "+" + strconv.Itoa(more), S: mono("warning")})
	}
	return spans
}

func blockedCard(m model.Model, card model.BoardCard, now time.Time) ui.Hovercard {
	shown, _ := shownRefs(card.Relationships.OpenBlockers())
	facts := make([]refFacts, len(shown))
	for i, r := range shown {
		facts[i] = relFacts(r)
	}
	return refCards(m, facts, now)
}

func shownRefs(refs []model.RelationshipRef) ([]model.RelationshipRef, int) {
	shown := min(len(refs), maxRelationshipIDs)
	return refs[:shown], len(refs) - shown
}

// cardFacts is the selected card's one muted row of facts, separated by a muted middle dot, each only when present: the owner,
// the date the badge leaves out, the parent, how many children are open, what it blocks, what it relates to and, on a Done card,
// the Lane it reopens in. What blocks it is not here: the Blocked marker already says so.
func cardFacts(m model.Model, card model.BoardCard, otherDate string, now time.Time) tern.Element {
	r, issue := card.Relationships, card.Issue
	var facts []tern.Element
	if owner := m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String(); owner != "" {
		facts = append(facts, ui.Text{Common: ui.Common{Key: "owner"}, Spans: []ui.Span{ownerSpan(owner)}})
	}
	if otherDate != "" {
		facts = append(facts, ui.Text{Common: ui.Common{Key: "other-date"}, Spans: []ui.Span{{T: otherDate, S: "muted"}}})
	}
	add := func(key, label string, refs ...model.RelationshipRef) {
		shown, more := shownRefs(refs)
		items := ui.Nodes(ui.Text{Common: ui.Common{Key: "label"}, Spans: []ui.Span{{T: label, S: "muted"}}})
		for _, ref := range shown {
			items = append(items, ui.Text{Common: ui.Common{Key: ref.UID, Hovercard: refCard(m, relFacts(ref), now), Actions: &ui.Actions{Click: RefClick(ref.UID)}}, Spans: []ui.Span{refSpan(ref.ID(issue.ProjectName), true)}})
		}
		if more > 0 {
			items = append(items, ui.Text{Common: ui.Common{Key: "more"}, Spans: []ui.Span{monoSpan("+"+strconv.Itoa(more), "muted")}})
		}
		facts = append(facts, ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapSM, Wrap: true, Children: items})
	}
	if r.Parent != nil {
		add("parent", "parent", *r.Parent)
	}
	if len(r.Children) > 0 {
		words := fmt.Sprintf("%d of %d %s open", len(r.OpenChildren()), len(r.Children), model.Noun(len(r.Children), "child", "children"))
		facts = append(facts, ui.Text{Common: ui.Common{Key: "children"}, Spans: []ui.Span{{T: words, S: "muted"}}})
	}
	if blocks := r.OpenBlocked(); len(blocks) > 0 {
		add("blocks", "blocks", blocks...)
	}
	if len(r.Related) > 0 {
		add("related", "related", r.Related...)
	}
	if card.Reopen != "" {
		facts = append(facts, ui.Text{Common: ui.Common{Key: "reopen"}, Spans: []ui.Span{{T: "reopens in " + card.Reopen, S: "muted"}}})
	}
	if len(facts) == 0 {
		return nil
	}
	parts := make([]tern.Element, 0, 2*len(facts)-1)
	for i, fact := range facts {
		if i > 0 {
			parts = append(parts, ui.Text{Common: ui.Common{Key: "sep" + strconv.Itoa(i)}, Spans: []ui.Span{{T: "·", S: "muted"}}})
		}
		parts = append(parts, fact)
	}
	return ui.Row{Common: ui.Common{Key: "facts"}, Gap: ui.GapSM, Wrap: true, Children: parts}
}
