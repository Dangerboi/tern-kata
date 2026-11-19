package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const JobListID = "main.columns.list.jobs"

// loadFailure is the failure line of a Page's read: "Cannot load <thing>: kata is not reachable." when kata did not answer, with
// the raw error in its Hovercard.
func loadFailure(thing string, err error) ui.Text {
	line := toneText("error", ui.ToneError, model.LoadFailure(thing, err))
	if model.Unreachable(err) {
		line.Hovercard = ui.Hovercard{Element: plainText("reason", ui.ToneNeutral, model.Reason(err))}
	}
	return line
}

func jobTone(pause model.PauseReason) ui.Tone {
	switch pause {
	case model.NotPaused:
		return ui.ToneSuccess
	case model.PauseDisabled, model.PauseArchived:
		return ui.ToneMuted
	}
	return ui.ToneWarning
}

func jobs(m model.Model, now time.Time) tern.Element {
	rows := m.JobRows(now)
	cards := make([]tern.Element, len(rows))
	for i, row := range rows {
		cards[i] = jobCard(row, row.Job.UID == m.Jobs.Selected)
	}
	hidden, loading := "", ""
	if n := m.HiddenArchivedJobs(); n > 0 {
		hidden = strconv.Itoa(n) + " archived hidden"
	}
	if m.Jobs.Loading {
		loading = "loading…"
	}
	children := ui.Nodes(pageHead("heading", "Jobs", strconv.Itoa(len(rows)), hidden, loading))
	if banner := m.WorkerBanner(); !banner.Empty() && m.Jobs.Err == nil {
		children = append(children, workerBanner(banner))
	}
	if m.Jobs.ProjectArchived {
		children = append(children, toneText("archived-project", ui.ToneWarning, "This Project is archived: kata answers none of its jobs, so those held here are paused."))
	}
	if m.Jobs.Err != nil {
		children = append(children, loadFailure("jobs", m.Jobs.Err))
	}
	if m.PageSearchShown() {
		children = append(children, pageSearchInput(m))
	}
	empty := emptyState("No jobs", "A job makes an Issue or runs an agent on a schedule.", emptyButton(m, "new", model.NewJobAction, "New job", true)...)
	if m.PageSearching() {
		empty = searchEmpty(m)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("jobs"), Max: fillMax, Empty: ui.T("Loading jobs…"), Children: cards}
	if m.Jobs.Err != nil && len(cards) == 0 {
		// A failed read is no empty list: the failure line above is the only text.
		return listPanel(m, listBasis, children)
	}
	children = append(children, listAndEmpty(list, m.Jobs.Loading, empty)...)
	return listPanel(m, listBasis, children)
}

// workerBanner is the one-line state of this machine's worker: a badge (success while it serves the Project, pending while one
// is about to start, else warning) and a muted detail. A worker that cannot be read is one error line.
func workerBanner(b model.WorkerBanner) tern.Element {
	if b.Failed {
		return toneText("worker", ui.ToneError, b.Detail)
	}
	tone := ui.ToneWarning
	switch {
	case b.Good:
		tone = ui.ToneSuccess
	case b.Pending:
		tone = ui.TonePending
	}
	return ui.Row{Common: ui.Common{Key: "worker", Tone: tone}, Gap: ui.GapMD, Align: ui.AlignCenter, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "badge", Tone: tone}, Text: b.Badge},
		ui.Text{Common: ui.Common{Key: "detail"}, Spans: []ui.Span{{T: b.Detail, S: "muted"}}},
	)}
}

// scheduleSpans is a schedule in muted words; a cron expression the words could not name keeps the mono face.
func scheduleSpans(schedule string) []ui.Span {
	if expression, ok := strings.CutPrefix(schedule, "cron "); ok {
		return []ui.Span{{T: "cron ", S: "muted"}, monoSpan(expression, "muted")}
	}
	return []ui.Span{{T: schedule, S: "muted"}}
}

func jobCard(row model.JobRow, selected bool) tern.Element {
	quiet := row.Pause == model.PauseDisabled || row.Pause == model.PauseArchived
	head := headRow(
		ui.Badge{Common: ui.Common{Key: "state", Tone: jobTone(row.Pause)}, Text: row.State},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: row.Job.Name, S: "strong"}}},
	)
	what := []ui.Span{{T: row.Kind, S: "muted"}}
	if row.Schedule != "" {
		what = append(what, ui.Span{T: " · ", S: "muted"})
		what = append(what, scheduleSpans(row.Schedule)...)
	}
	body := ui.Nodes(head, ui.Text{Common: ui.Common{Key: "what"}, Spans: what})
	if row.Next != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "meta"}, Spans: []ui.Span{{T: "Next run ", S: "muted"}, timeSpan(row.Next)}})
	}
	if row.Last != "" {
		last := []ui.Span{{T: row.Last, S: "muted"}}
		if row.LastStatus != "" {
			status := string(row.LastStatus)
			style := toneStyle(runStateTone(status))
			if quiet {
				style = "muted"
			}
			last = []ui.Span{{T: "Last run ", S: "muted"}, {T: status, S: style}, {T: " · " + row.Last, S: "muted"}}
		}
		body = append(body, ui.Text{Common: ui.Common{Key: "last"}, Spans: last})
	}
	if row.Warning != "" {
		body = append(body, toneText("warning", ui.ToneWarning, row.Warning))
	}
	if row.WorkerPause != "" {
		body = append(body, toneText("worker-pause", ui.ToneWarning, row.WorkerPause))
	}
	if row.Unsupported != "" {
		body = append(body, toneText("unsupported", ui.ToneWarning, "Set aside on this machine: "+row.Unsupported))
	}
	cardTone := ui.ToneNeutral
	if quiet {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Job.UID, Tone: cardTone, Actions: &ui.Actions{Click: JobClick(row.Job.UID), DblClick: JobOpenClick(row.Job.UID)}}, Selected: selected, Children: body}
}

func inspector(m model.Model, now time.Time) tern.Element {
	facts := m.JobFacts(now)
	job := m.SelectedJob()
	if len(facts) == 0 || job == nil {
		return emptyInspector(m, "Select a job to read it.")
	}
	lines := make([]factLine, len(facts))
	for i, fact := range facts {
		lines[i] = factLine{Label: fact.Label, Value: fact.Value}
		switch {
		case fact.Parent:
			lines[i].Style = "accent"
			if m.Focus == model.ColumnDetail {
				lines[i].Mark = ui.MarkPick
			}
		case fact.Status != "":
			status := string(fact.Status)
			lines[i].Spans = []ui.Span{{T: status, S: toneStyle(runStateTone(status))}, {T: " · " + fact.Value, S: "muted"}}
		case fact.Label == "Worker" || fact.Label == "Warning":
			lines[i].Style = toneStyle(ui.ToneWarning)
		case fact.Label == "Created":
			when, by, _ := strings.Cut(fact.Value, " · ")
			lines[i].Spans = []ui.Span{monoSpan(when, "")}
			if by != "" {
				lines[i].Spans = append(lines[i].Spans, ui.Span{T: " · " + by, S: "muted"})
			}
		case isTimeFact(fact.Label):
			lines[i].Spans = []ui.Span{monoSpan(fact.Value, "")}
		case fact.Label == "Checkout":
			lines[i].Spans = []ui.Span{monoSpan(fact.Value, "")}
		}
	}
	return factInspector(m, inspectorTitle(job.Name, m.JobIDs()), lines)
}

// isTimeFact says an inspector row holds a time, which is drawn in the mono face.
func isTimeFact(label string) bool {
	switch label {
	case "Next runs", "Created", "Updated", "Started", "Ended", "Duration", "Delivered through":
		return true
	}
	return false
}

// inspectorTitle is an inspector's strong title line; its hovercard lists the ids a person may need to copy.
func inspectorTitle(name string, ids []model.IDFact) ui.Text {
	return ui.Text{Common: ui.Common{Key: "title", Hovercard: idsCard(ids)}, Spans: []ui.Span{{T: name, S: "strong"}}}
}

// idsCard is a hovercard of label→value rows with the values in the mono face. It is the one home of ids: they are never rows.
func idsCard(ids []model.IDFact) ui.Hovercard {
	if len(ids) == 0 {
		return ui.Hovercard{}
	}
	noShrink := 0.0
	rows := make(tern.Nodes, len(ids))
	for i, id := range ids {
		rows[i] = ui.Row{Common: ui.Common{Key: "id" + strconv.Itoa(i)}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "label", Shrink: &noShrink}, Spans: []ui.Span{{T: id.Label, S: "muted"}}, Wrap: ui.WrapNone},
			ui.Text{Common: ui.Common{Key: "value"}, Spans: []ui.Span{monoSpan(id.Value, "")}},
		)}
	}
	return ui.Hovercard{Element: ui.Col{Common: ui.Common{Key: "ids"}, Gap: ui.GapXS, Children: rows}}
}
