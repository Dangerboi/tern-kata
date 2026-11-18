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

const RunListID = "main.columns.list.runs"

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
		cards = append(cards, runCard(row, row.Run.UID == m.Runs.Selected))
	}
	var scope []string
	if m.Runs.JobUID != "" {
		scope = append(scope, runsJobWords(m))
	}
	children := runsHeading(m, strconv.Itoa(len(rows)), scope...)
	children = append(children, runsNotices(m)...)
	empty := emptyState("No runs", "Runs from jobs and workflows appear here.")
	if m.Runs.JobUID != "" {
		empty = emptyState("No runs", "This job has not run yet.", emptyButton(m, "all-runs", model.FilterRunsByJob, "All runs", false)...)
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

func runsJobWords(m model.Model) string {
	if m.Runs.JobName == "" {
		return "job " + m.Runs.JobUID
	}
	return "job " + m.Runs.JobName
}

// runsHeading is the Page head: the title, the count and the scope clauses, then a note that older runs are not read yet and the
// loading marker while a read is in flight.
func runsHeading(m model.Model, count string, scope ...string) []tern.Element {
	facts := append([]string{count}, scope...)
	if m.Runs.More != "" {
		facts = append(facts, "older runs not shown")
	}
	if m.Runs.Loading {
		facts = append(facts, "loading…")
	}
	return ui.Nodes(pageHead("heading", "Runs", facts...))
}

func runsNotices(m model.Model) []tern.Element {
	var notices []tern.Element
	if m.Runs.ProjectArchived {
		notices = append(notices, toneText("archived-project", ui.ToneWarning, "This Project is archived: kata answers none of its runs."))
	}
	if m.Runs.Err != nil {
		notices = append(notices, toneText("error", ui.ToneError, model.Cannot("load", "runs", m.Runs.Err)))
	}
	if m.Runs.SeenErr != nil {
		notices = append(notices, toneText("seen-error", ui.ToneError, model.Cannot("read", "seen.toml", m.Runs.SeenErr)))
	}
	return notices
}

func runsPanel(m model.Model, children []tern.Element) tern.Element {
	return listPanel(m, 0.5, children)
}

func runCard(row model.RunRow, selected bool) tern.Element {
	head := headRow(
		ui.Badge{Common: ui.Common{Key: "status", Tone: runStateTone(row.State)}, Text: row.State},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: row.Name, S: "strong"}}},
	)
	if row.Executor != "" {
		head.Children = append(head.Children, ui.Text{Common: ui.Common{Key: "executor"}, Spans: []ui.Span{{T: row.Executor, S: "muted"}}})
	}
	body := ui.Nodes(head)
	if row.IssueRef != "" {
		who := []ui.Span{refSpan(row.IssueRef, true)}
		if row.IssueTitle != "" {
			who = append(who, ui.Span{T: " " + row.IssueTitle})
		}
		body = append(body, ui.Text{Common: ui.Common{Key: "who"}, Spans: who})
	}
	why := row.Reason
	if row.Held {
		why = row.Hold
	}
	if line := joinWords(why, row.Summary); line != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "reason"}, Spans: []ui.Span{{T: line, S: "muted"}}})
	}
	when := []ui.Span{timeSpan(row.Start)}
	if row.Duration != "" {
		when = append(when, ui.Span{T: " · ", S: "muted"}, timeSpan(row.Duration))
	}
	body = append(body, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})
	cardTone := ui.ToneNeutral
	if row.Run.Status == cronjob.RunCancelled {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Run.UID, Tone: cardTone, Actions: &ui.Actions{Click: RunClick(row.Run.UID), DblClick: RunOpenClick(row.Run.UID)}}, Selected: selected, Children: body}
}

// joinWords joins the parts that are not empty with the middle dot.
func joinWords(parts ...string) string {
	var shown []string
	for _, p := range parts {
		if p != "" {
			shown = append(shown, p)
		}
	}
	return strings.Join(shown, " · ")
}

func runInspector(m model.Model, now time.Time) tern.Element {
	facts := m.RunFacts(now)
	run := m.SelectedRun()
	if len(facts) == 0 || run == nil {
		return emptyInspector(m, "Select a run to read it.")
	}
	lines := make([]factLine, len(facts))
	for i, fact := range facts {
		lines[i] = factLine{Label: fact.Label, Value: fact.Value}
		switch {
		case fact.Label == "Status":
			lines[i].Style = toneStyle(runStateTone(fact.Value))
		case fact.Detail != "":
			lines[i].Spans = []ui.Span{{T: fact.Value}, {T: " · " + fact.Detail, S: "muted"}}
		case isTimeFact(fact.Label):
			lines[i].Spans = []ui.Span{monoSpan(fact.Value, "")}
		}
	}
	title := tern.Element(inspectorTitle(run.Name(), m.RunIDs()))
	if notice := m.RunNotice(); notice != "" {
		title = ui.Col{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Children: ui.Nodes(title, toneText("notice", ui.ToneWarning, notice))}
	}
	return factInspector(m, title, lines)
}

func runsTimeline(m model.Model, now time.Time) tern.Element {
	t := m.Timeline(now)
	loadingRuns, loadingNext := m.TimelineLoading()
	scope := []string{"new"}
	if !t.Seen.IsZero() {
		scope = []string{"new since " + m.Calendar.Read(t.Seen, now)}
	}
	if m.Runs.JobUID != "" {
		scope = append(scope, runsJobWords(m))
	}
	children := runsHeading(m, strconv.Itoa(t.RunCount), scope...)
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
		items = append(items, runCard(entry.Row, entry.Row.Run.UID == m.Runs.Selected))
	}
	if t.RunCount == 0 {
		words := "Nothing new yet."
		switch {
		case loadingRuns:
			words = model.Progress("Loading", "runs")
		case m.PageSearching():
			words = "No new run matches the search."
		case !t.Seen.IsZero():
			words = "Nothing new since " + m.Calendar.Read(t.Seen, now) + "."
		}
		items = append(items, plainText("nothing-new", ui.ToneMuted, words))
	}
	items = append(items, ui.Rule{Common: ui.Common{Key: "now"}, Label: ui.T("Now")})
	for _, next := range t.Upcoming {
		items = append(items, upcomingRun(next))
	}
	if len(t.Upcoming) == 0 {
		words := "No scheduled runs."
		if loadingNext {
			words = model.Progress("Loading", "next runs")
		}
		items = append(items, plainText("nothing-next", ui.ToneMuted, words))
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("runs"), Max: fillMax, Empty: ui.T("Loading runs…"), Children: items}
	return runsPanel(m, append(children, list))
}

func foldedRuns(m model.Model, group model.RunGroup, now time.Time) tern.Element {
	latest := group.Latest()
	ended, _ := latest.Ended()
	head := headRow(
		ui.Badge{Common: ui.Common{Key: "count", Tone: ui.ToneNeutral}, Text: strconv.Itoa(len(group.Runs)) + " runs"},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: latest.Name(), S: "strong"}}},
	)
	when := []ui.Span{{T: group.Counts() + " · last ended ", S: "muted"}, timeSpan(m.Calendar.Read(ended, now))}
	return ui.Card{Common: ui.Common{Key: "fold-" + latest.UID, Tone: ui.ToneNeutral}, Children: ui.Nodes(head, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})}
}

// skippedJob is one job's recent skips as a single row: the job, how many occurrences the worker skipped, the condition and reason
// of the latest and when it was skipped. The worker's status holds it, not kata, so it is no Run: it can't be selected or opened.
func skippedJob(m model.Model, s model.JobSkips, now time.Time) tern.Element {
	head := headRow(
		ui.Badge{Common: ui.Common{Key: "count", Tone: ui.ToneWarning}, Text: model.Count(s.Count, "skip", "skips")},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: s.Name, S: "strong"}}},
	)
	latest := []ui.Span{{T: joinWords(model.ConditionWords(s.LatestCondition), s.LatestReason), S: "muted"}}
	when := []ui.Span{{T: "Not run · ", S: "muted"}, timeSpan(m.Calendar.Read(s.LatestAt, now))}
	return ui.Card{Common: ui.Common{Key: "skips-" + s.UID, Tone: ui.ToneWarning}, Children: ui.Nodes(head,
		ui.Text{Common: ui.Common{Key: "latest"}, Spans: latest}, ui.Text{Common: ui.Common{Key: "when"}, Spans: when})}
}

// upcomingRun is a next run: its time in a fixed cell, then the job. An adapter job has no time, as only the worker knows when it
// starts it.
func upcomingRun(next model.UpcomingRun) tern.Element {
	noShrink := 0.0
	when := model.None
	if next.Next != "" {
		when = next.Next
	}
	name := []ui.Span{{T: next.Job.Name, S: "strong"}}
	if next.Next == "" {
		name = append(name, ui.Span{T: " the worker starts it", S: "muted"})
	}
	return ui.Row{Common: ui.Common{Key: "next-" + next.Job.UID}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "time", Shrink: &noShrink, Min: &ui.Bounds{W: ui.Ch(16)}}, Spans: []ui.Span{timeSpan(when)}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "name"}, Spans: name},
	)}
}
