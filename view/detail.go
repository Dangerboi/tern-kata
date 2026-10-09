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
		parts = append(parts, properties(m, issue), labels(issue.Labels), ui.Md{Common: ui.Common{Key: "body"}, Text: body})
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

// properties is the compact block: the priority badge, the primary fields, the
// parent as a reference and the daemon's child counts, then the de-emphasised author, revision
// and timestamps. The status badge is in the heading.
func properties(m model.Model, issue *model.Issue) tern.Element {
	owner := "Unassigned"
	if issue.Owner != nil && *issue.Owner != "" {
		owner = *issue.Owner
	}
	primary := []ui.Pair{
		{K: ui.T("Owner"), V: ui.T(owner)},
		{K: ui.T("Project"), V: ui.T(issue.ProjectName)},
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"Scheduled", issue.ScheduledOnDate}, {"Deadline", issue.DeadlineOnDate}, {"Closed reason", issue.ClosedReason}} {
		if f.v != nil {
			primary = append(primary, ui.Pair{K: ui.T(f.name), V: ui.T(*f.v)})
		}
	}
	muted := func(k, v string) ui.Pair {
		return ui.Pair{K: ui.Spans(ui.Span{T: k, S: "muted"}), V: ui.Spans(ui.Span{T: v, S: "muted"})}
	}
	secondary := []ui.Pair{
		muted("Author", issue.Author),
		muted("Revision", fmt.Sprint(issue.Revision)),
		muted("Created", issue.CreatedAt.Format(time.RFC3339)),
		muted("Updated", issue.UpdatedAt.Format(time.RFC3339)),
	}
	var children tern.Nodes
	if issue.Priority != nil {
		children = append(children, ui.Row{Common: ui.Common{Key: "badges"}, Gap: ui.GapSM, Children: ui.Nodes(
			ui.Badge{Common: ui.Common{Key: "priority", Tone: priorityTone(*issue.Priority)}, Text: fmt.Sprintf("P%d", *issue.Priority)},
		)})
	}
	children = append(children, ui.KV{Common: ui.Common{Key: "primary"}, Layout: ui.KVGrid, Items: primary})
	var relatives tern.Nodes
	for i := range m.DetailRowCount(model.TabLinks) {
		if row, _ := m.DetailRow(model.TabLinks, i); row.Relation == "parent" {
			relatives = append(relatives, ui.Col{Common: ui.Common{Key: row.Key}, Children: ui.Nodes(linkRow(row, nil))})
		}
	}
	if len(relatives) > 0 {
		children = append(children, ui.Col{Common: ui.Common{Key: "relatives"}, Gap: ui.GapSM, Children: relatives})
	}
	if counts := m.ChildCounts[issue.UID]; counts.Total > 0 {
		children = append(children, ui.Text{Common: ui.Common{Key: "children"}, Text: fmt.Sprintf("Children: %d open / %d total", counts.Open, counts.Total)})
	}
	children = append(children, ui.KV{Common: ui.Common{Key: "secondary"}, Layout: ui.KVGrid, Items: secondary})
	return ui.Col{Common: ui.Common{Key: "properties"}, Gap: ui.GapSM, Children: children}
}

func labels(names []string) tern.Element {
	if len(names) == 0 {
		return ui.Text{Common: ui.Common{Key: "labels"}, Text: "No labels."}
	}
	chips := make(tern.Nodes, len(names))
	for i, name := range names {
		chips[i] = ui.Badge{Common: ui.Common{Key: "label-" + name, Tone: ui.ToneNeutral}, Text: name}
	}
	return ui.Row{Common: ui.Common{Key: "labels"}, Gap: ui.GapSM, Wrap: true, Children: chips}
}
