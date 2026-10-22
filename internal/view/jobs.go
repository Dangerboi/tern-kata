package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// JobListID is the Jobs Page's list of job cards; a card is the job's uid under it.
const JobListID = "main.columns.list.jobs"

// JobID is the id of the card of the job with uid.
func JobID(uid string) string { return itemID(JobListID, uid) }

// jobTone is the tone of a job's state: active green, a pause amber, a disabled or archived job muted.
func jobTone(pause model.PauseReason) ui.Tone {
	switch pause {
	case model.NotPaused:
		return ui.ToneSuccess
	case model.PauseDisabled, model.PauseArchived:
		return ui.ToneMuted
	}
	return ui.ToneWarning
}

// jobs is the Jobs Page's list column: a heading, then one card per job with its state, name, kind, schedule, profile and next run.
func jobs(m model.Model, now time.Time) tern.Element {
	rows := m.JobRows(now)
	cards := make([]tern.Element, len(rows))
	for i, row := range rows {
		cards[i] = jobCard(row, row.Job.UID == m.Jobs.Selected)
	}
	archived := "archived hidden"
	if m.Jobs.ShowArchived {
		archived = "archived shown"
	}
	heading := []ui.Span{{T: "Jobs", S: "strong"}, {T: " " + strconv.Itoa(len(rows)) + " · " + archived, S: "muted"}}
	if m.Jobs.Loading {
		heading = append(heading, loadingMarker)
	}
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: heading, Wrap: ui.WrapNone})
	if line, good := m.WorkerLine(); line != "" {
		tone := ui.ToneWarning
		if good {
			tone = ui.ToneSuccess
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "worker", Tone: tone}, Text: line})
	}
	if m.Jobs.ProjectArchived {
		children = append(children, ui.Text{Common: ui.Common{Key: "archived-project", Tone: ui.ToneWarning}, Text: "This Project is archived: kata answers none of its jobs, so those held here are paused."})
	}
	if m.Jobs.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Jobs.Err.Error()})
	}
	empty := "No Cron jobs in this Project. Create one with kata cron job create."
	if m.Jobs.Loading {
		empty = "Loading jobs…"
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("jobs"), Max: scrollMax(m, m.RowPx+6+8), Empty: ui.T(empty), Children: cards})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
}

// jobCard is one job: the state badge and name, the kind and schedule in words, then the profile and next run. A part that
// can't be worked out is left off its line.
func jobCard(row model.JobRow, selected bool) tern.Element {
	tone := jobTone(row.Pause)
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "state", Tone: tone}, Text: row.State},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: row.Job.Name, S: "strong"}}},
	)}
	what := row.Kind
	if row.Schedule != "" {
		what += " · " + row.Schedule
	}
	body := ui.Nodes(head, ui.Text{Common: ui.Common{Key: "what"}, Spans: []ui.Span{{T: what, S: toneStyle(ui.ToneInfo)}}})
	var meta []string
	if row.Profile != "" {
		meta = append(meta, "profile "+row.Profile)
	}
	if row.Next != "" {
		meta = append(meta, "next "+row.Next)
	}
	if len(meta) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "meta"}, Spans: []ui.Span{{T: strings.Join(meta, " · "), S: "muted"}}})
	}
	if row.Unsupported != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "unsupported", Tone: ui.ToneWarning}, Text: "Set aside on this machine: " + row.Unsupported})
	}
	cardTone := ui.ToneNeutral
	if row.Pause == model.PauseDisabled || row.Pause == model.PauseArchived {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Job.UID, Tone: cardTone, Actions: &ui.Actions{Click: JobClick(row.Job.UID)}}, Selected: selected, Children: body}
}

// inspector is the Jobs Page's second column: the selected job's definition and, for a native job, its parent, which Enter opens.
func inspector(m model.Model, now time.Time) tern.Element {
	var children tern.Nodes
	facts := m.JobFacts(now)
	if len(facts) == 0 {
		children = append(children, ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select a job to read its definition."})
		return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
	}
	rows := make([]tern.Element, len(facts))
	for i, fact := range facts {
		valueStyle, mark := "", ui.Mark("")
		if fact.Parent {
			valueStyle = "accent"
			if m.Focus == model.ColumnDetail {
				mark = ui.MarkPick
			}
		}
		label := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(18)}}, Spans: []ui.Span{{T: fact.Label, S: "muted"}}, Wrap: ui.WrapNone}
		value := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: fact.Value, S: valueStyle}}}
		rows[i] = ui.Row{Common: ui.Common{Key: "fact" + strconv.Itoa(i), Mark: mark}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(label, value)}
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m, 0), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: rows})})
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, children)
}
