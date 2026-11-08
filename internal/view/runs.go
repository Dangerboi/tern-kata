package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// RunListID is the Runs Page's list of run cards; a card is the Run's uid under it.
const RunListID = "main.columns.list.runs"

// RunID is the id of the card of the Run with uid.
func RunID(uid string) string { return itemID(RunListID, uid) }

// runTone is the tone of a Run's status: running blue, succeeded green, failed red, cancelled muted, unknown amber.
func runTone(status cronjob.RunStatus) ui.Tone {
	switch status {
	case cronjob.RunRunning:
		return ui.ToneInfo
	case cronjob.RunSucceeded:
		return ui.ToneSuccess
	case cronjob.RunFailed:
		return ui.ToneError
	case cronjob.RunCancelled:
		return ui.ToneMuted
	}
	return ui.ToneWarning
}

// runs is the Runs Page's list column: a heading, then one card per Run, newest first.
func runs(m model.Model, now time.Time) tern.Element {
	if m.Runs.Timeline {
		return runsTimeline(m, now)
	}
	rows := m.RunRows(now)
	skips := m.SkipRows()
	cards := make([]tern.Element, 0, len(skips)+len(rows))
	for _, s := range skips {
		cards = append(cards, skippedJob(m, s, now))
	}
	for _, row := range rows {
		cards = append(cards, runCard(row, row.Run.UID == m.Runs.Selected, false))
	}
	scope := "all jobs and workflows"
	if m.Runs.JobUID != "" {
		scope = runsJobWords(m)
	}
	children := runsHeading(m, ui.Span{T: " " + strconv.Itoa(len(rows)), S: mono("muted")}, ui.Span{T: " · " + scope, S: "muted"})
	children = append(children, runsNotices(m)...)
	empty := emptyState("No runs", "An executor reports a run to kata when it starts one.")
	if m.Runs.JobUID != "" {
		empty = emptyState("No runs for this job", "It has not run yet.", emptyButton(m, "all-runs", model.FilterRunsByJob, "Every run", false)...)
	}
	if m.PageSearchShown() {
		children = append(children, pageSearchInput(m))
	}
	if m.PageSearching() {
		empty = searchEmpty(m)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("runs"), Max: fillMax, Empty: ui.T("Loading runs…"), Children: cards}
	children = append(children, listAndEmpty(list, m.Runs.Loading, empty)...)
	return runsPanel(m, children)
}

// runsJobWords is the job the Runs are narrowed to, as the heading calls it.
func runsJobWords(m model.Model) string {
	if m.Runs.JobName == "" {
		return "job " + m.Runs.JobUID
	}
	return "job " + m.Runs.JobName
}

// runsHeading is the Page's heading after "Runs" and the spans that count and scope it, with the marks of older runs to read and
// of a read on its way, as the first child of the list column.
func runsHeading(m model.Model, spans ...ui.Span) []tern.Element {
	heading := append([]ui.Span{{T: "Runs", S: "strong"}}, spans...)
	if m.Runs.More != "" {
		heading = append(heading, ui.Span{T: " · older runs: m", S: "muted"})
	}
	if m.Runs.Loading {
		heading = append(heading, loadingMarker)
	}
	return ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: heading, Wrap: ui.WrapNone})
}

// runsNotices are what the Page says under its heading in either view: an archived Project, why the Runs could not be read and why
// seen.toml could not.
func runsNotices(m model.Model) []tern.Element {
	var notices []tern.Element
	if m.Runs.ProjectArchived {
		notices = append(notices, ui.Text{Common: ui.Common{Key: "archived-project", Tone: ui.ToneWarning}, Text: "This Project is archived: kata answers none of its runs."})
	}
	if m.Runs.Err != nil {
		notices = append(notices, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Runs.Err.Error()})
	}
	if m.Runs.SeenErr != nil {
		notices = append(notices, ui.Text{Common: ui.Common{Key: "seen-error", Tone: ui.ToneError}, Text: m.Runs.SeenErr.Error()})
	}
	return notices
}

// runsPanel is the Runs Page's list column around children.
func runsPanel(m model.Model, children []tern.Element) tern.Element {
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
}

// runCard is one Run on three lines: its status and what it ran, the Issue and who ran it, then when it started, how long it took
// and the summary. A held Run of this machine adds its hold, and another Run whose summary leads with a reason its code. A part
// that can't be worked out is left off its line.
func runCard(row model.RunRow, selected, fresh bool) tern.Element {
	tone := runTone(row.Run.Status)
	if row.Held {
		tone = ui.ToneWarning
	}
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "status", Tone: tone}, Text: row.State},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: row.Name, S: "strong"}}},
	)}
	body := ui.Nodes(head)
	var who []string
	if row.Issue != "" {
		who = append(who, row.Issue)
	}
	if row.Executor != "" {
		who = append(who, "on "+row.Executor)
	}
	if len(who) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "who"}, Spans: []ui.Span{{T: strings.Join(who, " · "), S: mono(toneStyle(ui.ToneInfo))}}})
	}
	switch {
	case row.Held:
		body = append(body, ui.Text{Common: ui.Common{Key: "hold", Tone: ui.ToneWarning}, Text: "held: " + row.Hold})
	case row.Reason != "":
		body = append(body, ui.Text{Common: ui.Common{Key: "reason", Tone: tone}, Text: "reason: " + row.Reason})
	}
	when := []ui.Span{{T: "started ", S: "muted"}, {T: row.Start, S: mono("muted")}}
	if row.Duration != "" {
		when = append(when, ui.Span{T: " · ", S: "muted"}, ui.Span{T: row.Duration, S: mono("muted")})
	}
	if row.Summary != "" {
		when = append(when, ui.Span{T: " · " + row.Summary, S: "muted"})
	}
	body = append(body, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})
	cardTone := ui.ToneNeutral
	switch {
	case fresh:
		cardTone = ui.ToneInfo
	case row.Run.Status == cronjob.RunCancelled:
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Run.UID, Tone: cardTone, Actions: &ui.Actions{Click: RunClick(row.Run.UID), DblClick: RunOpenClick(row.Run.UID)}}, Selected: selected, Children: body}
}

// runInspector is the Runs Page's second column: the selected Run's detail.
func runInspector(m model.Model, now time.Time) tern.Element {
	facts := m.RunFacts(now)
	if len(facts) == 0 {
		return emptyInspector(m, "Select a run to read its detail.")
	}
	lines := make([]factLine, len(facts))
	for i, fact := range facts {
		lines[i] = factLine{Label: fact.Label, Value: fact.Value}
		switch fact.Label {
		case "Status":
			lines[i].Style = toneStyle(runTone(cronjob.RunStatus(fact.Value)))
		case "Held because":
			lines[i].Style = toneStyle(ui.ToneWarning)
		}
	}
	return factInspector(m, lines)
}

// runsTimeline is the Runs Page's since-you-last-looked mode: the Runs ended since the user last looked, highlighted, above a Now
// line and the Project's next runs. With nothing new it says so in one line.
func runsTimeline(m model.Model, now time.Time) tern.Element {
	t := m.Timeline(now)
	loadingRuns, loadingNext := m.TimelineLoading()
	scope := "since you last looked"
	if m.Runs.JobUID != "" {
		scope += " · " + runsJobWords(m)
	}
	children := runsHeading(m, ui.Span{T: " " + strconv.Itoa(t.RunCount), S: mono("muted")}, ui.Span{T: " new · " + scope, S: "muted"})
	children = append(children, runsNotices(m)...)
	if m.PageSearchShown() {
		children = append(children, pageSearchInput(m))
	}
	var items []tern.Element
	for _, s := range m.SkipRows() {
		items = append(items, skippedJob(m, s, now))
	}
	for _, entry := range t.Rows {
		if entry.Group.Folded() {
			items = append(items, foldedRuns(m, entry.Group, now))
			continue
		}
		items = append(items, runCard(entry.Row, entry.Row.Run.UID == m.Runs.Selected, true))
	}
	if t.RunCount == 0 {
		words := "Nothing new yet."
		switch {
		case loadingRuns:
			words = "Loading runs…"
		case m.PageSearching():
			words = "No new run matches the search."
		case !t.Seen.IsZero():
			words = "Nothing new since " + m.Calendar.Read(t.Seen, now) + "."
		}
		items = append(items, ui.Text{Common: ui.Common{Key: "nothing-new", Tone: ui.ToneMuted}, Text: words})
	}
	items = append(items, ui.Rule{Common: ui.Common{Key: "now"}, Label: ui.T("Now")})
	for _, next := range t.Upcoming {
		items = append(items, upcomingRun(m, next))
	}
	if len(t.Upcoming) == 0 {
		words := "No next runs. Native jobs show their next time; adapter jobs show only that the worker starts them."
		if loadingNext {
			words = "Loading next runs…"
		}
		items = append(items, ui.Text{Common: ui.Common{Key: "nothing-next", Tone: ui.ToneMuted}, Text: words})
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("runs"), Max: fillMax, Empty: ui.T("Loading runs…"), Children: items}
	return runsPanel(m, append(children, list))
}

// foldedRuns is one job's ended Runs as a single row: the job, how many Runs and their counts by status, and when the newest
// ended. It can't be selected; the list of every run holds the Runs themselves.
func foldedRuns(m model.Model, group model.RunGroup, now time.Time) tern.Element {
	latest := group.Latest()
	ended, _ := latest.Ended()
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "count", Tone: ui.ToneInfo}, Text: strconv.Itoa(len(group.Runs)) + " runs"},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: latest.Name(), S: "strong"}}},
	)}
	when := []ui.Span{{T: group.Counts(), S: "muted"}, {T: " · last ended ", S: "muted"}, {T: m.Calendar.Read(ended, now), S: mono("muted")}}
	return ui.Card{Common: ui.Common{Key: "fold-" + latest.UID, Tone: ui.ToneInfo}, Children: ui.Nodes(head, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})}
}

// skippedJob is one job's recent skips as a single row: the job, how many occurrences the worker skipped, the condition and reason
// of the latest and when it was skipped. The worker's status holds it, not kata, so it is no Run: it can't be selected or opened.
func skippedJob(m model.Model, s model.JobSkips, now time.Time) tern.Element {
	noun := " skips"
	if s.Count == 1 {
		noun = " skip"
	}
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "count", Tone: ui.ToneWarning}, Text: strconv.Itoa(s.Count) + noun},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: s.Name, S: "strong"}}},
	)}
	latest := []ui.Span{{T: "latest ", S: "muted"}, {T: s.LatestCondition, S: mono("muted")}, {T: ": " + s.LatestReason, S: "muted"}}
	when := []ui.Span{{T: "not run · skipped ", S: "muted"}, {T: m.Calendar.Read(s.LatestAt, now), S: mono("muted")}}
	return ui.Card{Common: ui.Common{Key: "skips-" + s.UID, Tone: ui.ToneWarning}, Children: ui.Nodes(head,
		ui.Text{Common: ui.Common{Key: "latest"}, Spans: latest}, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})}
}

// upcomingRun is one of the Project's next runs: a native job with its next time, an adapter job as started by the worker.
func upcomingRun(m model.Model, next model.UpcomingRun) tern.Element {
	when := "started by the worker"
	if next.Next != "" {
		when = next.Next
	}
	return ui.Text{Common: ui.Common{Key: "next-" + next.Job.UID}, Spans: []ui.Span{{T: next.Job.Name, S: "strong"}, {T: " · ", S: "muted"}, {T: when, S: mono("muted")}}}
}
