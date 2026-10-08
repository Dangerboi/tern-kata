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

// DetailScrollID is the list that scrolls the sections: the title stays above it.
const DetailScrollID = DetailID + ".sections"

func DetailSectionID(section model.DetailSection) string {
	return DetailScrollID + ".section" + strconv.Itoa(int(section))
}

// DetailRevealID is what to scroll into view for the detail's selection: its
// row, or its section when it has no rows.
func DetailRevealID(m model.Model) string {
	if row, ok := m.DetailRow(m.DetailSection, m.DetailIndex); ok {
		return DetailSectionID(m.DetailSection) + "." + row.Key
	}
	return DetailSectionID(m.DetailSection)
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
		sections := make([]tern.Element, 0, 2*int(model.SectionHistory)+1)
		for section := model.SectionProperties; section <= model.SectionHistory; section++ {
			content := ui.Nodes(sectionHeading(m, section))
			switch section {
			case model.SectionBody:
				body := issue.Body
				if body == "" {
					body = "No body yet."
				}
				content = append(content, ui.Md{Common: ui.Common{Key: "body"}, Text: body})
			case model.SectionProperties:
				content = append(content, properties(issue))
			case model.SectionLabels:
				content = append(content, labels(issue.Labels))
			default:
				content = append(content, detailRows(m, section)...)
			}
			if section > model.SectionProperties {
				sections = append(sections, ui.Spacer{Common: ui.Common{Key: "gap" + strconv.Itoa(int(section))}, Size: ui.GapMD})
			}
			sections = append(sections, ui.Col{Common: ui.Common{Key: "section" + strconv.Itoa(int(section))}, Gap: ui.GapSM, Children: content})
		}
		children = append(children, ui.List{CommonWithoutMax: scrollerCommon("sections"), Max: scrollMax(m), Children: sections})
	}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
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

// properties is the compact block: badges, the primary fields, then the
// de-emphasised author, revision and timestamps.
func properties(issue *model.Issue) tern.Element {
	badges := ui.Nodes(ui.Badge{Common: ui.Common{Key: "status", Tone: statusTone(issue.Status)}, Text: issue.Status})
	if issue.Priority != nil {
		badges = append(badges, ui.Badge{Common: ui.Common{Key: "priority", Tone: priorityTone(*issue.Priority)}, Text: fmt.Sprintf("P%d", *issue.Priority)})
	}
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
	return ui.Col{Common: ui.Common{Key: "properties"}, Gap: ui.GapSM, Children: ui.Nodes(
		ui.Row{Common: ui.Common{Key: "badges"}, Gap: ui.GapSM, Children: badges},
		ui.KV{Common: ui.Common{Key: "primary"}, Layout: ui.KVGrid, Items: primary},
		ui.KV{Common: ui.Common{Key: "secondary"}, Layout: ui.KVGrid, Items: secondary},
	)}
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
