package view

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/model"
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
	if count := m.DetailRowCount(m.ActiveTab()); count > 0 {
		row, _ := m.DetailRow(m.ActiveTab(), count-1)
		last = DetailActivityID + "." + row.Key
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
		parts := []tern.Element{ui.Col{Common: ui.Common{Key: "heading"}, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title", Actions: &ui.Actions{Click: "focus-detail"}}, Spans: title},
			ui.Badge{Common: ui.Common{Key: "status", Tone: statusTone(issue.Status)}, Text: statusText(issue)},
		)}, ui.Spacer{Common: ui.Common{Key: "gap"}, Size: ui.GapMD}}
		body := issue.Body
		if parent, ok := parentLink(m); ok {
			body = withoutParentSection(body, parent.Ref)
		}
		if strings.TrimSpace(body) == "" {
			body = "No body yet."
		}
		parts = append(parts, properties(m, issue), ui.Md{Common: ui.Common{Key: "body"}, Text: body})
		kids := m.DetailChildren()
		if len(kids) > 0 {
			parts = append(parts, ui.Spacer{Common: ui.Common{Key: "gap-children"}, Size: ui.GapMD}, childrenTable(m, kids, now))
		}
		parts = append(parts,
			ui.Spacer{Common: ui.Common{Key: "gap-activity"}, Size: ui.GapMD},
			ui.Col{Common: ui.Common{Key: "activity"}, Gap: ui.GapSM, Children: append(ui.Nodes(activityTabs(m)), detailRows(m, m.ActiveTab(), len(kids))...)})
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
		tabs = append(tabs, ui.Text{Common: ui.Common{Key: "tab" + strconv.Itoa(int(tab)), Actions: &ui.Actions{Click: "activity-tab=" + strconv.Itoa(int(tab))}}, Spans: []ui.Span{{T: name, S: style}}})
	}
	return ui.Row{Common: ui.Common{Key: "tabs"}, Gap: ui.GapMD, Children: tabs}
}

func detailRows(m model.Model, tab model.ActivityTab, cursorOffset int) tern.Nodes {
	count := m.DetailRowCount(tab)
	if count == 0 {
		return ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: "No " + strings.ToLower(tab.String()) + "."})
	}
	rows := make(tern.Nodes, 0, count)
	for i := range count {
		row, _ := m.DetailRow(tab, i)
		action := "detail-item=" + strconv.Itoa(int(tab)) + ":" + row.Key
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailIndex() == cursorOffset+i {
			mark = ui.MarkPick
		}
		if tab == model.TabLinks {
			rows = append(rows, ui.Col{Common: ui.Common{Key: row.Key, Mark: mark}, Children: ui.Nodes(linkRow(row, &ui.Actions{Click: action}))})
			continue
		}
		var parts tern.Nodes
		if tab == model.TabEvents {
			parts = ui.Nodes(historySentence(row, action))
		}
		if row.Body != "" {
			parts = append(parts, ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		}
		head := ui.Spans(ui.Span{T: row.Author, S: "strong"}, ui.Span{T: "  "}, ui.Span{T: row.Time, S: "muted"})
		rows = append(rows, ui.Card{Common: ui.Common{Key: row.Key, Mark: mark, Actions: &ui.Actions{Click: action}}, Variant: ui.CardBare, Head: head, Children: parts})
	}
	return rows
}

func parentLink(m model.Model) (model.DetailRow, bool) {
	for i := range m.DetailRowCount(model.TabLinks) {
		if link, _ := m.DetailRow(model.TabLinks, i); link.Relation == "parent" {
			return link, true
		}
	}
	return model.DetailRow{}, false
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
func properties(m model.Model, issue *model.Issue) tern.Element {
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
	add("Created", text(model.DetailTime(issue.CreatedAt)))
	add("Updated", text(model.DetailTime(issue.UpdatedAt)))
	if issue.ScheduledOnDate != nil {
		add("Scheduled", text(*issue.ScheduledOnDate))
	}
	if issue.DeadlineOnDate != nil {
		add("Deadline", text(*issue.DeadlineOnDate))
	}
	owner := "Unassigned"
	if issue.Owner != nil && *issue.Owner != "" {
		owner = *issue.Owner
	}
	add("Owner", text(owner))
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
	if link, ok := parentLink(m); ok {
		add("Parent", ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapNone, Wrap: true, Children: referenceTail(link.ReferenceUID, link.Ref, link.RefTitle)})
	}
	if counts := m.ChildCounts[issue.UID]; counts.Total > 0 {
		add("Children", text(fmt.Sprintf("%d open / %d total", counts.Open, counts.Total)))
	}
	keyWidth := 0
	for _, r := range rows {
		keyWidth = max(keyWidth, len(r.name)+1)
	}
	children := make(tern.Nodes, len(rows))
	for i, r := range rows {
		key := ui.Text{Common: ui.Common{Key: "k"}, Text: pad(r.name+":", keyWidth-len(r.name)-1)}
		children[i] = ui.Row{Common: ui.Common{Key: strings.ToLower(r.name)}, Gap: ui.GapSM, Children: ui.Nodes(key, r.value)}
	}
	return ui.Col{Common: ui.Common{Key: "properties"}, Gap: ui.GapNone, Children: children}
}
