package view

import (
	"strconv"
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
	heading := []ui.Span{{T: "Jobs", S: "strong"}, {T: " " + strconv.Itoa(len(rows)), S: mono("muted")}, {T: " · " + archived, S: "muted"}}
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
	if m.PageSearchShown() {
		children = append(children, pageSearchInput(m))
	}
	empty := emptyState("No Cron jobs", "This Project has none.", emptyButton(m, "new", model.NewJobAction, "New job", true)...)
	if m.PageSearching() {
		empty = searchEmpty(m)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("jobs"), Max: fillMax, Empty: ui.T("Loading jobs…"), Children: cards}
	children = append(children, listAndEmpty(list, m.Jobs.Loading, empty)...)
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
	what := []ui.Span{{T: row.Kind, S: toneStyle(ui.ToneInfo)}}
	if row.Schedule != "" {
		what = append(what, ui.Span{T: " · ", S: toneStyle(ui.ToneInfo)}, ui.Span{T: row.Schedule, S: mono(toneStyle(ui.ToneInfo))})
	}
	body := ui.Nodes(head, ui.Text{Common: ui.Common{Key: "what"}, Spans: what})
	var meta []ui.Span
	add := func(label, value string) {
		if len(meta) > 0 {
			meta = append(meta, ui.Span{T: " · ", S: "muted"})
		}
		meta = append(meta, ui.Span{T: label, S: "muted"}, ui.Span{T: value, S: mono("muted")})
	}
	if row.Profile != "" {
		add("profile ", row.Profile)
	}
	if row.Next != "" {
		add("next ", row.Next)
	}
	if len(meta) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "meta"}, Spans: meta})
	}
	if row.Last != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "last"}, Spans: []ui.Span{{T: row.Last, S: "muted"}}})
	}
	if row.Warning != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "warning", Tone: ui.ToneWarning}, Text: row.Warning})
	}
	if row.WorkerPause != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "worker-pause", Tone: ui.ToneWarning}, Text: "This machine's worker: " + row.WorkerPause + "; u resumes it"})
	}
	if row.Unsupported != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "unsupported", Tone: ui.ToneWarning}, Text: "Set aside on this machine: " + row.Unsupported})
	}
	cardTone := ui.ToneNeutral
	if row.Pause == model.PauseDisabled || row.Pause == model.PauseArchived {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Job.UID, Tone: cardTone, Actions: &ui.Actions{Click: JobClick(row.Job.UID), DblClick: JobOpenClick(row.Job.UID)}}, Selected: selected, Children: body}
}

// inspector is the Jobs Page's second column: the selected job's definition and, for a native job, its parent, which Enter opens.
func inspector(m model.Model, now time.Time) tern.Element {
	facts := m.JobFacts(now)
	if len(facts) == 0 {
		return emptyInspector(m, "Select a job to read its definition.")
	}
	lines := make([]factLine, len(facts))
	for i, fact := range facts {
		lines[i] = factLine{Label: fact.Label, Value: fact.Value}
		if fact.Parent {
			lines[i].Style = "accent"
			if m.Focus == model.ColumnDetail {
				lines[i].Mark = ui.MarkPick
			}
		}
	}
	return factInspector(m, lines)
}
