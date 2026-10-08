package view

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const DetailID = "main.columns.detail"

func DetailSectionID(section model.DetailSection) string {
	return DetailID + ".section" + strconv.Itoa(int(section))
}

func detail(m model.Model) tern.Element {
	children := ui.Nodes(ui.Row{Common: ui.Common{Key: "navigation"}, Gap: ui.GapSM, Children: ui.Nodes(
		el.Button{Common: ui.Common{Key: "back", Actions: &ui.Actions{Click: "key=escape"}}, Text: "Back"},
		el.Button{Common: ui.Common{Key: "toggle", Actions: &ui.Actions{Click: "key=shift+l"}}, Text: "Hide detail"},
	)})
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
		heading := []ui.Span{{T: issue.ShortID + " / " + issue.Title, S: "strong"}}
		if m.DetailLoading {
			heading = append(heading, loadingMarker)
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "title", Actions: &ui.Actions{Click: "focus-detail"}}, Spans: heading})
		for section := model.SectionBody; section <= model.SectionHistory; section++ {
			content := ui.Nodes(sectionHeading(m, section))
			switch section {
			case model.SectionBody:
				body := issue.Body
				if body == "" {
					body = "No body yet."
				}
				content = append(content, ui.Md{Common: ui.Common{Key: "body"}, Text: body})
			case model.SectionProperties:
				owner, priority := "Unassigned", "None"
				if issue.Owner != nil && *issue.Owner != "" {
					owner = *issue.Owner
				}
				if issue.Priority != nil {
					priority = fmt.Sprint(*issue.Priority)
				}
				pairs := []ui.Pair{
					{K: ui.T("Project"), V: ui.T(issue.ProjectName)},
					{K: ui.T("Status"), V: ui.T(issue.Status)},
					{K: ui.T("Priority"), V: ui.T(priority)},
					{K: ui.T("Owner"), V: ui.T(owner)},
					{K: ui.T("Author"), V: ui.T(issue.Author)},
					{K: ui.T("Revision"), V: ui.T(fmt.Sprint(issue.Revision))},
					{K: ui.T("Created"), V: ui.T(issue.CreatedAt.Format(time.RFC3339))},
					{K: ui.T("Updated"), V: ui.T(issue.UpdatedAt.Format(time.RFC3339))},
				}
				if issue.ScheduledOnDate != nil {
					pairs = append(pairs, ui.Pair{K: ui.T("Scheduled"), V: ui.T(*issue.ScheduledOnDate)})
				}
				if issue.DeadlineOnDate != nil {
					pairs = append(pairs, ui.Pair{K: ui.T("Deadline"), V: ui.T(*issue.DeadlineOnDate)})
				}
				if issue.ClosedReason != nil {
					pairs = append(pairs, ui.Pair{K: ui.T("Closed reason"), V: ui.T(*issue.ClosedReason)})
				}
				content = append(content, ui.KV{Common: ui.Common{Key: "properties"}, Layout: ui.KVGrid, Items: pairs})
			case model.SectionLabels:
				labels := strings.Join(issue.Labels, ", ")
				if labels == "" {
					labels = "No labels."
				}
				content = append(content, ui.Text{Common: ui.Common{Key: "labels"}, Text: labels})
			default:
				content = append(content, detailRows(m, section)...)
			}
			children = append(children, ui.Col{Common: ui.Common{Key: "section" + strconv.Itoa(int(section))}, Gap: ui.GapSM, Children: content})
		}
	}
	return ui.Col{Common: ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, Gap: ui.GapMD, Children: children}
}

func sectionHeading(m model.Model, section model.DetailSection) tern.Element {
	style := "strong"
	if m.Focus == model.ColumnDetail && m.DetailSection == section {
		style = "accent strong"
	}
	return ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: "section=" + strconv.Itoa(int(section))}}, Spans: []ui.Span{{T: section.String(), S: style}}}
}

func detailRows(m model.Model, section model.DetailSection) tern.Nodes {
	count := m.DetailRowCount(section)
	if count == 0 {
		return ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: "No " + strings.ToLower(section.String()) + "."})
	}
	rows := make(tern.Nodes, 0, count)
	for i := range count {
		row, _ := m.DetailRow(section, i)
		action := "detail-item=" + strconv.Itoa(int(section)) + ":" + row.Key
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailSection == section && m.DetailIndex == i {
			mark = ui.MarkPick
		}
		heading, body := tern.Element(ui.Text{Common: ui.Common{Key: "heading", Actions: &ui.Actions{Click: action}}, Text: row.Title}), tern.Element(ui.Md{Common: ui.Common{Key: "body"}, Text: row.Body})
		if section == model.SectionHistory {
			heading, body = historyHeading(row, action), historyDetail(row)
		}
		parts := ui.Nodes(heading)
		if row.Body != "" {
			parts = append(parts, body)
		}
		if row.ReferenceUID != "" {
			parts = append(parts, el.Button{Common: ui.Common{Key: "jump", Actions: &ui.Actions{Click: "jump=" + row.ReferenceUID}}, Text: "Open referenced Issue"})
		}
		rows = append(rows, ui.Col{Common: ui.Common{Key: row.Key, Mark: mark}, Children: parts})
	}
	return rows
}
