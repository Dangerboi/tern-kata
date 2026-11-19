package view

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const DetailID = "main.columns.detail"

// DetailScrollID is the one scroll container of the detail column: title,
// properties, body and Activity all live inside it, in DetailContentID.
const DetailScrollID = DetailID + ".viewport"

// DetailContentID is the container's only child. Tern's scroll op names a node
// inside the scroller, not the scroller itself.
const DetailContentID = DetailScrollID + ".content"

// BodyID is the Issue body's node when it is Markdown. On a Tern that draws a doc the body is the doc BodyDocID names.
const BodyID = DetailContentID + ".body"

const bodyDocPrefix = DetailContentID + ".bodydoc-"

// BodyDocID is the id of the doc that shows Issue uid's body. Tern keeps a node while its id stays, and the doc's margin and its
// open edit belong to the node, so a doc of its own for each Issue keeps them from following the detail onto another Issue. The
// uid follows a dash, not a dot, so an Issue row's selector `[data-id$='.<uid>']` does not match the doc.
func BodyDocID(uid string) string { return bodyDocPrefix + uid }

// IsBodyDoc reports whether id is some Issue's body doc.
func IsBodyDoc(id string) bool { return strings.HasPrefix(id, bodyDocPrefix) }

// BodyDocUID is the uid of the Issue whose body the doc id names.
func BodyDocUID(id string) string { return strings.TrimPrefix(id, bodyDocPrefix) }

const (
	activityKey = "activity"
	tabsKey     = "tabs"
)

const DetailActivityID = DetailContentID + "." + activityKey

// ActivityTabsID is the row of tab names, revealed at "start" after a tab switch: the tab names go to the top of the detail,
// or, when the tab below them is shorter than the detail, the detail stops at its end with the whole tab in view (Activity
// is the last part of the detail). It is one reveal on purpose: while every credit is in use the SDK sends the queued ops in
// one frame, and Tern runs a second reveal of a frame against the scroll offset from before the first. For the same reason the
// driver never sends a step more than one op to the detail scroller.
const ActivityTabsID = DetailActivityID + "." + tabsKey

func detail(m model.Model, now time.Time) tern.Element {
	var children tern.Nodes
	switch {
	case m.SelectedUID == "":
		// An empty list says so itself; a detail column beside it would only repeat that nothing is there.
		if len(m.Issues) > 0 || m.Loading {
			noIssue := emptyState("No Issue selected", "Pick one from the list.")
			noIssue.Key = "empty"
			children = append(children, noIssue)
		}
	case m.DetailLoading() && m.DetailAuthority() == nil:
		loading := emptyState("Loading Issue…", "")
		loading.Key, loading.Tone = "loading", ui.TonePending
		children = append(children, loading)
	case m.DetailErr() != nil:
		failed := emptyState("Cannot load this Issue", model.Reason(m.DetailErr()))
		failed.Key = "error"
		children = append(children, failed)
	case m.DetailAuthority() == nil || m.DetailAuthority().Issue == nil:
		gone := emptyState("Issue not available", "It may have been deleted or moved.")
		gone.Key = "unavailable"
		children = append(children, gone)
	default:
		issue := m.DetailAuthority().Issue
		title := []ui.Span{monoSpan(detailID(m, *issue), "muted"), {T: " " + issue.Title, S: "strong"}}
		if m.DetailLoading() {
			title = append(title, loadingMarker)
		}
		parts := []tern.Element{ui.Row{Common: ui.Common{Key: "heading"}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title", Grow: 1, Actions: &ui.Actions{Click: FocusDetailClick}}, Spans: title},
			ui.Text{Common: ui.Common{Key: "status", Tone: statusTone(issue.Status), Shrink: new(float64)}, Spans: []ui.Span{{T: statusText(issue), S: toneStyle(statusTone(issue.Status))}}, Wrap: ui.WrapNone},
		)}, ui.Spacer{Common: ui.Common{Key: "gap"}, Size: ui.GapMD}}
		parts = append(parts, properties(m, issue, now))
		parts = append(parts, bodyNodes(m, *issue)...)
		kids, activity := detailRows(m, now)
		if len(kids) > 0 {
			parts = append(parts, ui.Spacer{Common: ui.Common{Key: "gap-children"}, Size: ui.GapMD}, childrenTable(m, kids, now))
		}
		if len(activity) == 0 {
			activity = ui.Nodes(emptyTab("empty", m.EmptyDetail()))
		}
		parts = append(parts,
			ui.Spacer{Common: ui.Common{Key: "gap-activity"}, Size: ui.GapMD},
			ui.Col{Common: ui.Common{Key: activityKey}, Gap: ui.GapSM, Children: append(ui.Nodes(activityTabs(m)), activity...)})
		children = append(children, ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: fillMax, Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})})
	}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
}

func statusText(issue *model.Issue) string {
	if issue.ClosedReason != nil && *issue.ClosedReason != "" && issue.Status == "closed" {
		return issue.Status + " · " + strings.ToLower(model.CloseReasonWords(*issue.ClosedReason))
	}
	return issue.Status
}

// activityTabs draws the tab row: each tab is its name and a muted count, with no brackets or parentheses. The active tab is
// accent and strong and carries ActiveTabRole, which underlines it, so it is not told by colour alone. A zero count is not drawn.
// Below FoldedLegendCols the tabs never wrap or shrink, so the four fit a phone's width without a word broken.
func activityTabs(m model.Model) tern.Element {
	narrow := m.NarrowerThan(model.FoldedLegendCols)
	var tabs tern.Nodes
	for tab := model.TabComments; tab < model.TabCount; tab++ {
		label := ui.Span{T: tab.String()}
		common := ui.Common{Key: "tab" + strconv.Itoa(int(tab)), Actions: &ui.Actions{Click: ActivityTabClick(tab)}}
		if tab == m.ActiveTab() {
			label.S, common.Role = "accent strong", ActiveTabRole
		}
		spans := []ui.Span{label}
		if count := countSpan(m.DetailRowCount(tab)); count.T != "" {
			spans = append(spans, ui.Span{T: " "}, count)
		}
		text := ui.Text{Common: common, Spans: spans}
		if narrow {
			text.Shrink, text.Wrap = new(float64), ui.WrapNone
		}
		tabs = append(tabs, text)
	}
	gap := ui.GapMD
	if narrow {
		gap = ui.GapSM
	}
	return ui.Row{Common: ui.Common{Key: tabsKey}, Gap: gap, Children: tabs}
}

// emptyTab, under key, is what an Activity tab without rows says: a muted title and one muted sentence whose "%s" marks are the keycaps of
// the tab's actions.
func emptyTab(key string, empty model.EmptyDetail) tern.Element {
	return ui.Col{Common: ui.Common{Key: key}, Gap: ui.GapXS, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: empty.Title, S: "muted"}}},
		ui.Text{Common: ui.Common{Key: "hint"}, Spans: hintSpans(empty.Hint, empty.Actions)},
	)}
}

// hintSpans is hint in muted text with each "%s" replaced by the keycap of the next action in the detail's mode. An action the
// mode does not bind leaves its mark out rather than a key that does nothing.
func hintSpans(hint string, actions []model.Action) []ui.Span {
	var spans []ui.Span
	pieces := strings.Split(hint, "%s")
	for i, piece := range pieces {
		if piece != "" {
			spans = append(spans, ui.Span{T: piece, S: "muted"})
		}
		if i < len(pieces)-1 && i < len(actions) {
			if keys, _ := cardKeys(model.ModeDetail, actions[i]); len(keys) > 0 {
				spans = append(spans, keySpan(keys[0]))
			}
		}
	}
	return spans
}

func detailRows(m model.Model, now time.Time) (children []detailChild, activity tern.Nodes) {
	for i, item := range m.DetailRows() {
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailIndex() == i {
			mark = ui.MarkPick
		}
		if item.Kind == model.RowCron {
			continue // drawn by the properties
		}
		if item.Kind == model.RowChild {
			children = append(children, detailChild{item.Child, mark})
			continue
		}
		row := item.Activity
		action := DetailItemClick(item.Kind.Tab(), row.Key)
		if item.Kind == model.RowAttachment {
			menu := attachmentMenu(m.SelectedUID, row.Key, row.File)
			activity = append(activity, ui.Text{Common: ui.Common{Key: row.Key, Mark: mark, Title: attachmentTitle(row), Actions: &ui.Actions{Click: action, Menu: menu, Hold: menu}}, Spans: attachmentSpans(row)})
			continue
		}
		if item.Kind == model.RowLink {
			if i == 0 || m.DetailRows()[i-1].Kind != model.RowLink || m.DetailRows()[i-1].Activity.Relation != row.Relation {
				activity = append(activity, linkGroupHeading(m, row))
			}
			activity = append(activity, ui.Col{Common: ui.Common{Key: row.Key, Mark: mark}, Children: ui.Nodes(linkRow(m, row, &ui.Actions{Click: action}, now))})
			continue
		}
		var parts tern.Nodes
		var head ui.Rich
		var headNode tern.Element
		if item.Kind == model.RowEvent {
			parts = ui.Nodes(historySentence(m, row, action, now))
		} else {
			spans := make([]ui.Span, len(row.Head))
			for i, part := range row.Head {
				spans[i] = ui.Span{T: part.Text, S: string(part.Style)}
			}
			if item.ReferenceUID != "" {
				// a reply to a comment in another Issue: the head names that Issue, and hovering it shows its card
				facts := refFacts{UID: item.ReferenceUID, Qualified: row.Qualified}
				headNode = ui.Text{Common: ui.Common{Key: "head", Hovercard: refCard(m, facts, now)}, Spans: spans}
			} else if item.Depth > 0 {
				headNode = ui.Text{Common: ui.Common{Key: "head"}, Spans: spans}
			} else {
				head = ui.Spans(spans...)
			}
			if item.Depth > 0 {
				// a reply sits under the comment it answers: its head and its body share one indent of two columns per level
				headNode = indented("head-indent", item.Depth, headNode)
			}
		}
		if row.Creation != "" {
			parts = append(parts, plainText("creation", ui.ToneMuted, row.Creation))
		}
		if len(row.Tags) > 0 {
			parts = append(parts, commentTagLine(row.Tags))
		}
		if item.Kind == model.RowComment {
			if line, ok := commentLinksLine(m, row.Key, mark == ui.MarkPick, now); ok {
				parts = append(parts, line)
			}
		}
		if row.Body != "" {
			parts = append(parts, ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		}
		if item.Depth > 0 {
			parts = ui.Nodes(indented("indented", item.Depth, ui.Col{Common: ui.Common{Key: "parts", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Children: parts}))
		}
		role := ""
		if item.Kind == model.RowComment {
			role = CommentRole
		}
		activity = append(activity, ui.Card{Common: ui.Common{Key: row.Key, Role: role, Mark: mark, Actions: &ui.Actions{Click: action}}, Variant: ui.CardBare, Head: head, HeadNode: headNode, Children: parts})
		for n, leaf := range item.Leaves {
			activity = append(activity, commentLeaf(leafKey(row.Key, n), leaf))
		}
	}
	return children, activity
}

func leafKey(commentKey string, n int) string { return commentKey + "-leaf" + strconv.Itoa(n) }

// commentLinksLine is a comment's links as one muted line, "Links: a, b", always drawn so the card does not change height when the
// cursor arrives; the chosen link is accent while the cursor is on the comment.
func commentLinksLine(m model.Model, uid string, onCursor bool, now time.Time) (ui.Text, bool) {
	labels := m.CommentLinkLabels(uid)
	if len(labels) == 0 {
		return ui.Text{}, false
	}
	chosen, hasChoice := model.CommentLinkChoice{}, false
	if onCursor {
		chosen, hasChoice = m.DetailCommentLink()
	}
	spans := []ui.Span{{T: "Links: ", S: "muted"}}
	for i, label := range labels {
		if i > 0 {
			spans = append(spans, ui.Span{T: ", ", S: "muted"})
		}
		style := "muted"
		if hasChoice && chosen.Index == i+1 {
			style = "accent"
		}
		spans = append(spans, ui.Span{T: label, S: style})
	}
	line := ui.Text{Common: ui.Common{Key: "link"}, Spans: spans}
	if hasChoice && chosen.IssueUID != "" {
		line.Hovercard = refCard(m, refFacts{UID: chosen.IssueUID, Qualified: chosen.Qualified}, now)
	}
	return line, true
}

func commentTagLine(tags []model.RowSpan) ui.Text {
	var spans []ui.Span
	for i, tag := range tags {
		if i > 0 {
			spans = append(spans, ui.Span{T: " · ", S: "muted"})
		}
		spans = append(spans, ui.Span{T: tag.Text, S: string(tag.Style)})
	}
	return ui.Text{Common: ui.Common{Key: "tags"}, Spans: spans}
}

func indented(key string, depth int, node tern.Element) tern.Element {
	if depth == 0 {
		return node
	}
	pad := ui.Text{Common: ui.Common{Key: "indent", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(float64(2 * depth))}}}
	return ui.Row{Common: ui.Common{Key: key}, Children: ui.Nodes(pad, node)}
}

func commentLeaf(key string, leaf model.CommentLeaf) tern.Element {
	lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "leaf", Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: leaf.Text, S: "muted"}}})
	if leaf.Edited {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "edited"}, Spans: []ui.Span{{T: "Target edited after this reply", S: "muted"}}})
	}
	return indented(key, leaf.Depth, ui.Col{Common: ui.Common{Key: key, Grow: 1}, Children: lines})
}

type detailChild struct {
	model.ChildRow
	mark ui.Mark
}

// detailID is the Issue's id as the Pane's Collection sees it, the rule of the Board's Relationships line: "#id" inside the
// Project Collection on screen, "project#id" for an Issue of another Project and in Collections that span Projects.
func detailID(m model.Model, issue model.Issue) string {
	return m.RefName(issue.ProjectName, issue.ShortID)
}

// statusTone is the tone of an Issue's status: open is plain text, anything else (closed) is muted. It is empty, not accent or
// success: where you are and how a run went are not what an Issue's status says.
func statusTone(status string) ui.Tone {
	if status == "open" {
		return ""
	}
	return ui.ToneMuted
}

// properties is one key/value grid in propRow's shared key column, so the values start at one x in the mono and the sans face
// alike. A row with nothing to show is left out. The status is in the heading. Under the grid is one muted line with who
// created the Issue and when, then the attribution line when kata gave one.
func properties(m model.Model, issue *model.Issue, now time.Time) tern.Element {
	var rows []tern.Element
	add := func(key string, value ...tern.Element) { rows = append(rows, propRow(key, value...)) }
	text := func(s string) tern.Element { return ui.Text{Common: ui.Common{Key: "v"}, Text: s} }
	owner := ui.Text{Common: ui.Common{Key: "v"}, Spans: []ui.Span{ownerSpan(m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String())}}
	add("Owner", owner)
	if badge, ok := agentBadge(m, issue.UID, "v"); ok {
		add("Agent", badge)
	}
	if issue.Priority != nil {
		prio, tone := priorityText(issue.Priority)
		add("Priority", ui.Badge{Common: ui.Common{Key: "v", Tone: tone, Title: fmt.Sprintf("Priority %d (0 is highest)", *issue.Priority)}, Text: prio})
	}
	scheduled, deadline := model.IssueDates(*issue)
	// Dates are facts here, not warnings: the Board and list badges carry the urgency.
	dateBadge := func(day string) tern.Element {
		return ui.Badge{Common: ui.Common{Key: "v", Tone: ui.ToneMuted, Role: MonoRole}, Text: day}
	}
	if scheduled != "" {
		add("Scheduled", dateBadge(scheduled))
	}
	if deadline != "" {
		add("Deadline", dateBadge(deadline))
	}
	if len(issue.Labels) > 0 {
		chips := make(tern.Nodes, len(issue.Labels))
		for i, name := range issue.Labels {
			chips[i] = labelChip(name)
		}
		add("Labels", ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapSM, Wrap: true, Children: chips})
	}
	if lane := model.LaneOf(*issue, m.LaneList()); lane != model.LaneNone && lane != model.LaneDone {
		add("Lane", text(lane))
	}
	if w, ok := model.IssueWorkAttention(*issue); ok {
		add("Work attention", workAttention(w))
	}
	if issue.RecurrenceID != nil {
		recurring := "Yes" // kata did not say more: it withholds recurrences from a scoped access
		if d := m.DetailAuthority(); d != nil && d.Recurrence != nil {
			recurring = d.Recurrence.Text()
		}
		add("Recurring", text(recurring))
	}
	for _, r := range cronProperties(m) {
		add(r.name, r.value)
	}
	if link, ok := m.DetailParent(); ok {
		add("Parent", referenceText("v", link.ReferenceUID, link.Ref, link.RefTitle, refCard(m, rowFacts(link), now)))
	}
	rows = append(rows, createdLine(m, issue))
	if line := issue.Attribution.Line(issue.Author); line != "" {
		rows = append(rows, plainText("creation", ui.ToneMuted, line))
	}
	return propGrid(rows...)
}

// createdLine is the muted line under the properties: "dana created 2026-11-18 18:21 · updated 18:40". The author is left out
// when kata gave none; the times are mono, the words around them muted sans.
func createdLine(m model.Model, issue *model.Issue) tern.Element {
	var spans []ui.Span
	if issue.Author != "" {
		spans = append(spans, ui.Span{T: issue.Author + " ", S: "muted"})
	}
	spans = append(spans,
		ui.Span{T: "created ", S: "muted"}, timeSpan(m.DetailTime(issue.CreatedAt)),
		ui.Span{T: " · updated ", S: "muted"}, timeSpan(m.DetailTime(issue.UpdatedAt)))
	return ui.Text{Common: ui.Common{Key: "meta"}, Spans: spans}
}

// message is read to its last word instead of being cut at the edge of a one-line chip.
func workAttention(w model.WorkAttention) tern.Element {
	tone := attentionTone(w)
	if w.Msg == "" {
		return ui.Badge{Common: ui.Common{Key: "v", Tone: tone}, Text: w.Word()}
	}
	noShrink := 0.0
	return ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "word", Tone: tone, Shrink: &noShrink}, Text: w.Word()},
		ui.Text{Common: ui.Common{Key: "message"}, Text: w.Msg},
	)}
}

type propertyRow struct {
	name  string
	value tern.Element
}

func cronProperties(m model.Model) []propertyRow {
	var job tern.Element
	var jobs tern.Nodes
	for i, item := range m.DetailRows() {
		if item.Kind != model.RowCron {
			continue
		}
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailIndex() == i {
			mark = ui.MarkPick
		}
		spans := []ui.Span{{T: item.Cron.Name, S: "accent link"}}
		common := ui.Common{Key: item.Key, Mark: mark, Actions: &ui.Actions{Click: OpenJobClick(item.JobUID)}}
		if item.Cron.Property == "Cron job" {
			if item.Cron.Scheduled != "" {
				spans = append(spans, ui.Span{T: " · scheduled ", S: "muted"}, ui.Span{T: item.Cron.Scheduled, S: mono("muted")})
			}
			common.Key = "v"
			job = ui.Text{Common: common, Spans: spans}
			continue
		}
		spans = append(spans, ui.Span{T: " · " + item.Cron.State, S: "muted"})
		if item.Cron.Schedule != "" {
			spans = append(spans, ui.Span{T: " · ", S: "muted"}, ui.Span{T: item.Cron.Schedule, S: mono("muted")})
		}
		jobs = append(jobs, ui.Text{Common: common, Spans: spans})
	}
	var rows []propertyRow
	if job != nil {
		rows = append(rows, propertyRow{"Cron job", job})
	}
	if len(jobs) > 0 {
		if m.CronJobsClosed() {
			jobs = append(jobs, plainText("paused", ui.ToneMuted, "Closed: its Cron jobs are paused."))
		}
		rows = append(rows, propertyRow{"Cron jobs", ui.Col{Common: ui.Common{Key: "v"}, Gap: ui.GapNone, Children: jobs}})
	}
	return rows
}

// bodyNodes draws the Issue body. On a Tern that draws a doc the body is one the user edits in place and comments on a selection of.
// An empty body stays Markdown with its placeholder: `e` writes the first one. A body with a picture stays Markdown too, which
// keeps the picture at its size and opens it large on a click; `e` edits it.
//
// Tern ends an in-place edit when the program sends the doc a text op, and sends the edit as `change` while the user still types (a
// second or so after the last key), so the doc keeps the stored body while the Draft is only held or saving: it already holds the
// edit. Once the save failed or conflicted, the doc shows the Draft, keeps showing it while a retry or an overwrite saves, and a
// discard then takes it away again.
func bodyNodes(m model.Model, issue model.Issue) []tern.Element {
	body, _ := m.BodyShown(issue)
	var state model.BodyDoc
	var line []tern.Element // what explains a held Draft, whatever the body draws as
	if m.InPlaceBody {
		state = m.BodyDoc(issue)
		switch {
		case state.Conflicted:
			line = []tern.Element{toneText("body-error", ui.ToneError, model.ConflictSentence(state.What, ""))}
		case state.Err != nil:
			line = []tern.Element{toneText("body-error", ui.ToneError, state.Err.Error()+model.DraftKeptHint(state.Err, ""))}
		}
	}
	if strings.TrimSpace(body) == "" {
		empty := emptyTab("body", model.EmptyDetail{Title: "No body", Hint: "Press %s to write one.", Actions: []model.Action{model.EditBody}})
		return append([]tern.Element{empty}, line...)
	}
	if !m.InPlaceBody || model.HasPicture(body) {
		return append([]tern.Element{ui.Md{Common: ui.Common{Key: "body"}, Text: body}}, line...)
	}
	doc := ui.Doc{Common: ui.Common{Key: strings.TrimPrefix(BodyDocID(issue.UID), DetailContentID+".")}, Text: state.Text, Edit: state.Editable, Comment: state.Editable}
	return append([]tern.Element{doc}, line...)
}
