package view

import (
	"fmt"
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

func detail(m model.Model, now time.Time) tern.Element {
	var children tern.Nodes
	switch {
	case m.SelectedUID == "":
		children = append(children, ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select an Issue to read its detail."})
	case m.DetailLoading && m.Detail == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Issue detail…"})
	case m.DetailErr != nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.DetailErr.Error()})
	case m.Detail == nil || m.Detail.Issue == nil:
		children = append(children, ui.Text{Common: ui.Common{Key: "unavailable"}, Text: "This Issue is unavailable or no longer visible."})
	default:
		issue := m.Detail.Issue
		title := []ui.Span{{T: "#" + issue.ShortID, S: "accent"}, {T: " - " + issue.Title, S: "strong"}}
		if m.DetailLoading {
			title = append(title, loadingMarker)
		}
		parts := []tern.Element{ui.Row{Common: ui.Common{Key: "heading"}, Gap: ui.GapSM, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "title", Actions: &ui.Actions{Click: "focus-detail"}}, Spans: title},
			ui.Badge{Common: ui.Common{Key: "status", Tone: statusTone(issue.Status)}, Text: issue.Status},
		)}, ui.Spacer{Common: ui.Common{Key: "gap"}, Size: ui.GapMD}}
		body := issue.Body
		if body == "" {
			body = "No body yet."
		}
		parts = append(parts, properties(m, issue), ui.Md{Common: ui.Common{Key: "body"}, Text: body})
		kids := m.DetailChildren()
		if len(kids) > 0 {
			parts = append(parts, ui.Spacer{Common: ui.Common{Key: "gap-children"}, Size: ui.GapMD}, childrenTable(m, kids, now))
		}
		parts = append(parts,
			ui.Spacer{Common: ui.Common{Key: "gap-activity"}, Size: ui.GapMD},
			ui.Col{Common: ui.Common{Key: "activity"}, Gap: ui.GapSM, Children: append(ui.Nodes(activityTabs(m)), detailRows(m, m.ActivityTab, len(kids))...)})
		children = append(children, ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})})
	}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
}

// activityTabs is the Activity heading with a clickable name and count per tab;
// the active one is accent and bracketed, as in kata's TUI.
func activityTabs(m model.Model) tern.Element {
	tabs := ui.Nodes(ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: "Activity", S: "strong"}}})
	for tab := model.TabComments; tab < model.TabCount; tab++ {
		name, style := fmt.Sprintf("%s (%d)", tab, m.DetailRowCount(tab)), ""
		if tab == m.ActivityTab {
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
		if m.Focus == model.ColumnDetail && m.DetailIndex == cursorOffset+i {
			mark = ui.MarkPick
		}
		var heading tern.Element
		switch {
		case tab == model.TabEvents:
			heading = historyHeading(row, action)
		case row.Ref != "":
			heading = linkRow(row, &ui.Actions{Click: action})
		default:
			heading = ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Text: row.Title}
		}
		parts := ui.Nodes(heading)
		if row.Body != "" {
			parts = append(parts, ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		}
		rows = append(rows, ui.Col{Common: ui.Common{Key: row.Key, Mark: mark}, Children: parts})
	}
	return rows
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
	for i := range m.DetailRowCount(model.TabLinks) {
		if link, _ := m.DetailRow(model.TabLinks, i); link.Relation == "parent" {
			follow := &ui.Actions{Click: "jump=" + link.ReferenceUID}
			nodes := ui.Nodes(jumpLink(link.ReferenceUID, link.Ref))
			if link.RefTitle != "" {
				nodes = append(nodes, ui.Text{Common: ui.Common{Key: "dash", Actions: follow}, Text: "\u00a0-\u00a0"}, ui.Text{Common: ui.Common{Key: "title", Actions: follow}, Text: link.RefTitle})
			}
			add("Parent", ui.Row{Common: ui.Common{Key: "v"}, Gap: ui.GapNone, Wrap: true, Children: nodes})
			break
		}
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
	if counts := m.ChildCounts[issue.UID]; counts.Total > 0 {
		add("Children", text(fmt.Sprintf("%d open / %d total", counts.Open, counts.Total)))
	}
	if issue.ClosedReason != nil && *issue.ClosedReason != "" {
		add("Closed", text(*issue.ClosedReason))
	}
	keyWidth := 0
	for _, r := range rows {
		keyWidth = max(keyWidth, len(r.name)+1)
	}
	children := make(tern.Nodes, len(rows))
	for i, r := range rows {
		key := ui.Text{Common: ui.Common{Key: "k"}, Text: r.name + ":"}
		key.Min, key.Max = fixedWidth(keyWidth)
		children[i] = ui.Row{Common: ui.Common{Key: strings.ToLower(r.name)}, Gap: ui.GapSM, Children: ui.Nodes(key, r.value)}
	}
	return ui.Col{Common: ui.Common{Key: "properties"}, Gap: ui.GapNone, Children: children}
}
