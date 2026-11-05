package view

import (
	"fmt"
	"regexp"
	"slices"
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

// DetailActivityID holds the Activity heading, its tab names and the active tab's rows.
const DetailActivityID = DetailContentID + ".activity"

// ActivityRevealIDs are the nodes to reveal, in this order, to bring the active Activity tab into view:
// the last node of its rows (or its empty line), then the tab names.
func ActivityRevealIDs(m model.Model) []string {
	last := DetailActivityID + ".empty"
	if rows := m.DetailRows(); len(rows) > 0 && rows[len(rows)-1].Kind.InActivity() {
		last = DetailActivityID + "." + rows[len(rows)-1].Key
	}
	return []string{last, DetailActivityID + ".tabs"}
}

func detail(m model.Model, now time.Time) tern.Element {
	var children tern.Nodes
	switch {
	case m.SelectedUID == "":
		noIssue := emptyState("No Issue selected", "Pick one from the list to read its detail.")
		noIssue.Key = "empty"
		children = append(children, noIssue)
	case m.DetailLoading() && m.DetailAuthority() == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Issue detail…"})
	case m.DetailErr() != nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.DetailErr().Error()})
	case m.DetailAuthority() == nil || m.DetailAuthority().Issue == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "unavailable"}, Text: "This Issue is unavailable or no longer visible."})
	default:
		issue := m.DetailAuthority().Issue
		title := []ui.Span{{T: detailID(m, *issue), S: mono(toneStyle(issueLaneTone(m, *issue)))}, {T: " - " + issue.Title, S: "strong"}}
		if m.DetailLoading() {
			title = append(title, loadingMarker)
		}
		parts := []tern.Element{ui.Col{Common: ui.Common{Key: "heading"}, Align: ui.AlignStart, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title", Actions: &ui.Actions{Click: FocusDetailClick}}, Spans: title},
			ui.Badge{Common: ui.Common{Key: "status", Tone: statusTone(issue.Status)}, Text: statusText(issue)},
		)}, ui.Spacer{Common: ui.Common{Key: "gap"}, Size: ui.GapMD}}
		body := issue.Body
		if parent, ok := m.DetailParent(); ok {
			body = withoutParentSection(body, parent.Ref)
		}
		if strings.TrimSpace(body) == "" {
			body = "No body yet."
		}
		parts = append(parts, properties(m, issue, now), ui.Md{Common: ui.Common{Key: "body"}, Text: body})
		kids, activity := detailRows(m, now)
		if len(kids) > 0 {
			parts = append(parts, ui.Spacer{Common: ui.Common{Key: "gap-children"}, Size: ui.GapMD}, childrenTable(m, kids, now))
		}
		if len(activity) == 0 {
			activity = ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: m.EmptyDetailText()})
		}
		parts = append(parts,
			ui.Spacer{Common: ui.Common{Key: "gap-activity"}, Size: ui.GapMD},
			ui.Col{Common: ui.Common{Key: "activity"}, Gap: ui.GapSM, Children: append(ui.Nodes(activityTabs(m)), activity...)})
		children = append(children, ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: fillMax, Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})})
	}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
}

// statusText is the status badge text: the status, plus the close reason of a closed Issue.
func statusText(issue *model.Issue) string {
	if issue.ClosedReason != nil && *issue.ClosedReason != "" && issue.Status == "closed" {
		return issue.Status + " · " + *issue.ClosedReason
	}
	return issue.Status
}

// activityTabs is the Activity heading with a clickable name and count per tab;
// the active one is accent and bracketed, as in kata's TUI.
func activityTabs(m model.Model) tern.Element {
	tabs := ui.Nodes(ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: "Activity", S: "strong"}}})
	for tab := model.TabComments; tab < model.TabCount; tab++ {
		name, style := fmt.Sprintf("%s (%d)", tab, m.DetailRowCount(tab)), ""
		if tab == m.ActiveTab() {
			name, style = "[ "+name+" ]", "accent strong"
		}
		tabs = append(tabs, ui.Text{Common: ui.Common{Key: "tab" + strconv.Itoa(int(tab)), Actions: &ui.Actions{Click: ActivityTabClick(tab)}}, Spans: []ui.Span{{T: name, S: style}}})
	}
	return ui.Row{Common: ui.Common{Key: "tabs"}, Gap: ui.GapMD, Children: tabs}
}

// detailRows draws the model's rows: the children as table rows and the others as Activity cards, the one under the
// cursor marked.
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
			activity = append(activity, ui.Text{Common: ui.Common{Key: row.Key, Mark: mark}, Spans: attachmentSpans(row)})
			continue
		}
		if item.Kind == model.RowLink {
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
			if item.Depth > 0 {
				// a reply sits under the comment it answers: two columns per level, kept as spaces so the card keeps its path
				spans = append([]ui.Span{{T: strings.Repeat("\u00a0\u00a0", item.Depth)}}, spans...)
			}
			if item.ReferenceUID != "" {
				// a reply to a comment in another Issue: the head names that Issue, and hovering it shows its card
				facts := refFacts{UID: item.ReferenceUID, Qualified: row.Qualified}
				headNode = ui.Text{Common: ui.Common{Key: "head", Hovercard: refCard(m, facts, now)}, Spans: spans}
			} else {
				head = ui.Spans(spans...)
			}
		}
		if row.Creation != "" {
			parts = append(parts, ui.Text{Common: ui.Common{Key: "creation", Tone: ui.ToneMuted}, Text: row.Creation})
		}
		if len(row.Tags) > 0 {
			parts = append(parts, commentTagLine(row.Tags))
		}
		if mark == ui.MarkPick && item.Kind == model.RowComment {
			if choice, ok := m.DetailCommentLink(); ok {
				link := ui.Text{Common: ui.Common{Key: "link", Tone: ui.ToneAccent}, Text: fmt.Sprintf("link %d/%d: %s", choice.Index, choice.Count, choice.Label)}
				if choice.IssueUID != "" {
					link.Hovercard = refCard(m, refFacts{UID: choice.IssueUID, Qualified: choice.Qualified}, now)
				}
				parts = append(parts, link)
			}
		}
		if row.Body != "" {
			parts = append(parts, ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		}
		if item.Depth > 0 {
			parts = ui.Nodes(indented("indented", item.Depth, ui.Col{Common: ui.Common{Key: "parts", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Children: parts}))
		}
		activity = append(activity, ui.Card{Common: ui.Common{Key: row.Key, Mark: mark, Actions: &ui.Actions{Click: action}}, Variant: ui.CardBare, Head: head, HeadNode: headNode, Children: parts})
		for n, leaf := range item.Leaves {
			activity = append(activity, commentLeaf(row.Key+"-leaf"+strconv.Itoa(n), leaf))
		}
	}
	return children, activity
}

// commentTagLine is a comment's confirm and refute tags on one line, joined by a middle dot.
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

// indented draws node after two columns of space per level of depth, so a reply sits under the comment it answers.
func indented(key string, depth int, node tern.Element) tern.Element {
	if depth == 0 {
		return node
	}
	pad := ui.Text{Common: ui.Common{Key: "indent", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(float64(2 * depth))}}}
	return ui.Row{Common: ui.Common{Key: key}, Children: ui.Nodes(pad, node)}
}

// commentLeaf is a reply from another Issue: a muted line under the comment it answers, indented to its depth, with a second
// line when the comment it answers changed after the reply.
func commentLeaf(key string, leaf model.CommentLeaf) tern.Element {
	lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "leaf", Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: leaf.Text, S: "muted"}}})
	if leaf.Edited {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "edited"}, Spans: []ui.Span{{T: "Target edited after this reply", S: "muted"}}})
	}
	return indented(key, leaf.Depth, ui.Col{Common: ui.Common{Key: key, Grow: 1}, Children: lines})
}

// detailChild is a child row with its cursor mark.
type detailChild struct {
	model.ChildRow
	mark ui.Mark
}

var (
	atxHeading   = regexp.MustCompile(`^#{1,6}(\s|$)`)
	parentHeader = regexp.MustCompile(`^##\s+Parent\s*$`)
)

// withoutParentSection drops a leading `## Parent` section (up to the next heading) that names the parent
// by ref (#short or project#short) as a whole word; display only, the stored body is untouched.
func withoutParentSection(body, ref string) string {
	trimmed := strings.TrimLeft(body, "\r\n")
	first, rest, _ := strings.Cut(trimmed, "\n")
	if !parentHeader.MatchString(first) {
		return body
	}
	section, after := rest, ""
	for off, line := 0, rest; ; {
		if atxHeading.MatchString(line) {
			section, after = rest[:off], rest[off:]
			break
		}
		nl := strings.IndexByte(line, '\n')
		if nl < 0 {
			break
		}
		off += nl + 1
		line = rest[off:]
	}
	short := ref[strings.LastIndex(ref, "#")+1:]
	named := slices.ContainsFunc(strings.Fields(section), func(word string) bool {
		word = strings.Trim(word, ".,;:()[]<>\"'`*_")
		return word == short || strings.HasSuffix(word, "#"+short)
	})
	if !named {
		return body
	}
	return strings.TrimLeft(after, "\r\n")
}

// detailID is the Issue's id as the Pane's Collection sees it, the rule of the Board's Relationships line: "#id" inside the
// Project Collection on screen, "project#id" for an Issue of another Project and in Collections that span Projects.
func detailID(m model.Model, issue model.Issue) string {
	if issue.ProjectName == "" || m.Collection.IsProject() && m.Collection.Title == issue.ProjectName {
		return "#" + issue.ShortID
	}
	return issue.ProjectName + "#" + issue.ShortID
}

func statusTone(status string) ui.Tone {
	if status == "open" {
		return ui.ToneAccent
	}
	return ui.ToneMuted
}

// properties is one Name: value grid: every row's key sits in a box PropertyKeyCSS gives one width, so the values line up
// in the mono and the sans face alike. A row with nothing to show is left out. The status badge is in the heading.
func properties(m model.Model, issue *model.Issue, now time.Time) tern.Element {
	type row = propertyRow
	text := func(s string) tern.Element { return ui.Text{Common: ui.Common{Key: "v"}, Text: s} }
	machine := func(s string) tern.Element {
		return ui.Text{Common: ui.Common{Key: "v"}, Spans: []ui.Span{{T: s, S: mono("")}}}
	}
	var rows []row
	add := func(name string, value tern.Element) { rows = append(rows, row{name, value}) }
	if issue.Author != "" {
		add("Author", text(issue.Author))
	}
	add("Created", machine(m.DetailTime(issue.CreatedAt)))
	add("Updated", machine(m.DetailTime(issue.UpdatedAt)))
	scheduled, deadline := model.IssueDates(*issue)
	dateBadge := func(day string, rule func(day, today string) model.DateTone) tern.Element {
		tone := ui.ToneMuted
		if issue.Status != "closed" {
			tone = dateTones[rule(day, m.Calendar.Date)]
		}
		return ui.Badge{Common: ui.Common{Key: "v", Tone: tone, Role: MonoRole}, Text: day}
	}
	if scheduled != "" {
		add("Scheduled", dateBadge(scheduled, model.ScheduledTone))
	}
	if deadline != "" {
		add("Deadline", dateBadge(deadline, model.DeadlineTone))
	}
	owner := m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String()
	if owner == "" {
		owner = "Unassigned"
	}
	add("Owner", ui.Badge{Common: ui.Common{Key: "v", Tone: ownerTone}, Text: owner})
	if badge, ok := agentBadge(m, issue.UID, "v"); ok {
		add("Agent", badge)
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
	if issue.Priority != nil {
		prio, tone := priorityText(issue.Priority)
		add("Priority", ui.Badge{Common: ui.Common{Key: "v", Tone: tone, Role: MonoRole}, Text: prio})
	}
	if len(issue.Labels) > 0 {
		chips := make(tern.Nodes, len(issue.Labels))
		for i, name := range issue.Labels {
			chips[i] = ui.Badge{Common: ui.Common{Key: "label-" + name, Tone: labelTone}, Text: name}
		}
		add("Labels", ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapSM, Wrap: true, Children: chips})
	}
	for _, r := range cronProperties(m) {
		add(r.name, r.value)
	}
	if link, ok := m.DetailParent(); ok {
		add("Parent", referenceText("v", link.ReferenceUID, link.Ref, link.RefTitle, refCard(m, rowFacts(link), now)))
	}
	if counts := m.DetailChildCounts(); counts.Total > 0 {
		add("Children", ui.Text{Common: ui.Common{Key: "v"}, Spans: monoSpans(fmt.Sprintf("%d open / %d total", counts.Open, counts.Total), "")})
	}
	noShrink := 0.0
	children := make(tern.Nodes, len(rows))
	for i, r := range rows {
		key := ui.Text{Common: ui.Common{Key: "k", Role: PropertyKeyRole, Shrink: &noShrink}, Text: r.name + ":", Wrap: ui.WrapNone}
		children[i] = ui.Row{Common: ui.Common{Key: strings.ReplaceAll(strings.ToLower(r.name), " ", "-")}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(key, r.value)}
	}
	if line := issue.Attribution.Line(issue.Author); line != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "creation", Tone: ui.ToneMuted}, Text: line})
	}
	return ui.Col{Common: ui.Common{Key: "properties", Role: PropertiesRole}, Gap: ui.GapNone, Children: children}
}

// workAttention is the Work attention value: the word a badge in its tone, then the whole message as wrapping text, so a long
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

// propertyRow is a Name: value row of the properties grid.
type propertyRow struct {
	name  string
	value tern.Element
}

// cronProperties are the Cron job property of a cron-made Issue and the Cron jobs property of a parent, each job a row the
// cursor can stand on and a click opens on the Jobs Page. A closed parent says its jobs are paused.
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
			jobs = append(jobs, ui.Text{Common: ui.Common{Key: "paused", Tone: ui.ToneMuted}, Text: "Closed: its Cron jobs are paused."})
		}
		rows = append(rows, propertyRow{"Cron jobs", ui.Col{Common: ui.Common{Key: "v"}, Gap: ui.GapNone, Children: jobs}})
	}
	return rows
}
