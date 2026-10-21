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
	if rows := m.DetailRows(); len(rows) > 0 && rows[len(rows)-1].Kind != model.RowChild {
		last = DetailActivityID + "." + rows[len(rows)-1].Key
	}
	return []string{last, DetailActivityID + ".tabs"}
}

func detail(m model.Model, now time.Time) tern.Element {
	var children tern.Nodes
	switch {
	case m.SelectedUID == "":
		children = append(children, ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select an Issue to read its detail."})
	case m.DetailLoading() && m.DetailAuthority() == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Issue detail…"})
	case m.DetailErr() != nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.DetailErr().Error()})
	case m.DetailAuthority() == nil || m.DetailAuthority().Issue == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "unavailable"}, Text: "This Issue is unavailable or no longer visible."})
	default:
		issue := m.DetailAuthority().Issue
		title := []ui.Span{{T: "#" + issue.ShortID, S: "accent"}, {T: " - " + issue.Title, S: "strong"}}
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
		kids, activity := detailRows(m)
		if len(kids) > 0 {
			parts = append(parts, ui.Spacer{Common: ui.Common{Key: "gap-children"}, Size: ui.GapMD}, childrenTable(m, kids, now))
		}
		if len(activity) == 0 {
			activity = ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: m.ActiveTab().EmptyText()})
		}
		parts = append(parts,
			ui.Spacer{Common: ui.Common{Key: "gap-activity"}, Size: ui.GapMD},
			ui.Col{Common: ui.Common{Key: "activity"}, Gap: ui.GapSM, Children: append(ui.Nodes(activityTabs(m)), activity...)})
		children = append(children, ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})})
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
func detailRows(m model.Model) (children []detailChild, activity tern.Nodes) {
	for i, item := range m.DetailRows() {
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailIndex() == i {
			mark = ui.MarkPick
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
			activity = append(activity, ui.Col{Common: ui.Common{Key: row.Key, Mark: mark}, Children: ui.Nodes(linkRow(row, &ui.Actions{Click: action}))})
			continue
		}
		var parts tern.Nodes
		var head ui.Rich
		if item.Kind == model.RowEvent {
			parts = ui.Nodes(historySentence(row, action))
		} else {
			spans := make([]ui.Span, len(row.Head))
			for i, part := range row.Head {
				spans[i] = ui.Span{T: part.Text, S: string(part.Style)}
			}
			head = ui.Spans(spans...)
		}
		if row.Body != "" {
			parts = append(parts, ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		}
		activity = append(activity, ui.Card{Common: ui.Common{Key: row.Key, Mark: mark, Actions: &ui.Actions{Click: action}}, Variant: ui.CardBare, Head: head, Children: parts})
	}
	return children, activity
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

func statusTone(status string) ui.Tone {
	if status == "open" {
		return ui.ToneAccent
	}
	return ui.ToneMuted
}

func priorityTone(priority int64) ui.Tone {
	switch priority {
	case 0:
		return ui.ToneError
	case 1:
		return ui.ToneWarning
	case 2:
		return ui.ToneInfo
	}
	return ui.ToneMuted
}

// properties is one Name: value grid: every row's key sits in a column as wide as the longest key, so the
// values line up. A row with nothing to show is left out. The status badge is in the heading.
func properties(m model.Model, issue *model.Issue, now time.Time) tern.Element {
	type row struct {
		name  string
		value tern.Element
	}
	text := func(s string) tern.Element { return ui.Text{Common: ui.Common{Key: "v"}, Text: s} }
	var rows []row
	add := func(name string, value tern.Element) { rows = append(rows, row{name, value}) }
	if issue.Author != "" {
		add("Author", text(issue.Author))
	}
	add("Created", text(m.DetailTime(issue.CreatedAt)))
	add("Updated", text(m.DetailTime(issue.UpdatedAt)))
	if day := model.IssueDate(issue.ScheduledOnDate, issue.Metadata, model.ScheduledKey); day != "" {
		add("Scheduled", text(day))
	}
	if day := model.IssueDate(issue.DeadlineOnDate, issue.Metadata, model.DeadlineKey); day != "" {
		add("Deadline", text(day))
	}
	owner := m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String()
	if owner == "" {
		owner = "Unassigned"
	}
	add("Owner", text(owner))
	if issue.RecurrenceID != nil {
		recurring := "Yes" // kata did not say more: it withholds recurrences from a scoped access
		if d := m.DetailAuthority(); d != nil && d.Recurrence != nil {
			recurring = d.Recurrence.Text()
		}
		add("Recurring", text(recurring))
	}
	if issue.Priority != nil {
		add("Priority", ui.Badge{Common: ui.Common{Key: "v", Tone: priorityTone(*issue.Priority)}, Text: fmt.Sprintf("P%d", *issue.Priority)})
	}
	if len(issue.Labels) > 0 {
		chips := make(tern.Nodes, len(issue.Labels))
		for i, name := range issue.Labels {
			chips[i] = ui.Badge{Common: ui.Common{Key: "label-" + name, Tone: ui.ToneNeutral}, Text: name}
		}
		add("Labels", ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapSM, Wrap: true, Children: chips})
	}
	if link, ok := m.DetailParent(); ok {
		add("Parent", referenceText("v", link.ReferenceUID, link.Ref, link.RefTitle))
	}
	if counts := m.DetailChildCounts(); counts.Total > 0 {
		add("Children", text(fmt.Sprintf("%d open / %d total", counts.Open, counts.Total)))
	}
	keyWidth := 0
	for _, r := range rows {
		keyWidth = max(keyWidth, len(r.name)+1)
	}
	noShrink := 0.0
	children := make(tern.Nodes, len(rows))
	for i, r := range rows {
		key := ui.Text{Common: ui.Common{Key: "k", Shrink: &noShrink}, Text: pad(r.name+":", keyWidth-len(r.name)-1), Wrap: ui.WrapNone}
		children[i] = ui.Row{Common: ui.Common{Key: strings.ToLower(r.name)}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(key, r.value)}
	}
	return ui.Col{Common: ui.Common{Key: "properties"}, Gap: ui.GapNone, Children: children}
}
