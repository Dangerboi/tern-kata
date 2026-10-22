package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// RunListID is the Runs Page's list of run cards; a card is the Run's uid under it.
const RunListID = "main.columns.list.runs"

// RunID is the id of the card of the Run with uid.
func RunID(uid string) string { return itemID(RunListID, uid) }

// runTone is the tone of a Run's status: running blue, succeeded green, failed red, cancelled muted, unknown amber.
func runTone(status model.RunStatus) ui.Tone {
	switch status {
	case model.RunRunning:
		return ui.ToneInfo
	case model.RunSucceeded:
		return ui.ToneSuccess
	case model.RunFailed:
		return ui.ToneError
	case model.RunCancelled:
		return ui.ToneMuted
	}
	return ui.ToneWarning
}

// runs is the Runs Page's list column: a heading, then one card per Run, newest first.
func runs(m model.Model, now time.Time) tern.Element {
	rows := m.RunRows(now)
	cards := make([]tern.Element, len(rows))
	for i, row := range rows {
		cards[i] = runCard(row, row.Run.UID == m.Runs.Selected)
	}
	scope := "all jobs and workflows"
	if m.Runs.JobUID != "" {
		scope = "job " + m.Runs.JobName
		if m.Runs.JobName == "" {
			scope = "job " + m.Runs.JobUID
		}
	}
	heading := []ui.Span{{T: "Runs", S: "strong"}, {T: " " + strconv.Itoa(len(rows)) + " · " + scope, S: "muted"}}
	if m.Runs.More != "" {
		heading = append(heading, ui.Span{T: " · older runs: m", S: "muted"})
	}
	if m.Runs.Loading {
		heading = append(heading, loadingMarker)
	}
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: heading, Wrap: ui.WrapNone})
	if m.Runs.ProjectArchived {
		children = append(children, ui.Text{Common: ui.Common{Key: "archived-project", Tone: ui.ToneWarning}, Text: "This Project is archived: kata answers none of its runs."})
	}
	if m.Runs.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Runs.Err.Error()})
	}
	empty := "No runs in this Project yet. An executor reports a run to kata when it starts one."
	switch {
	case m.Runs.Loading:
		empty = "Loading runs…"
	case m.Runs.JobUID != "":
		empty = "This job has no runs yet. Press f to see every run."
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("runs"), Max: scrollMax(m, m.RowPx+6+8), Empty: ui.T(empty), Children: cards})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapSM, children)
}

// runCard is one Run on three lines: its status and what it ran, the Issue and who ran it, then when it started, how long it took
// and the summary. A part that can't be worked out is left off its line.
func runCard(row model.RunRow, selected bool) tern.Element {
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
		body = append(body, ui.Text{Common: ui.Common{Key: "who"}, Spans: []ui.Span{{T: strings.Join(who, " · "), S: toneStyle(ui.ToneInfo)}}})
	}
	if row.Held {
		body = append(body, ui.Text{Common: ui.Common{Key: "hold", Tone: ui.ToneWarning}, Text: "held: " + row.Hold})
	}
	when := []string{"started " + row.Start}
	if row.Duration != "" {
		when = append(when, row.Duration)
	}
	if row.Summary != "" {
		when = append(when, row.Summary)
	}
	body = append(body, ui.Text{Common: ui.Common{Key: "when"}, Spans: []ui.Span{{T: strings.Join(when, " · "), S: "muted"}}})
	cardTone := ui.ToneNeutral
	if row.Run.Status == model.RunCancelled {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: row.Run.UID, Tone: cardTone, Actions: &ui.Actions{Click: RunClick(row.Run.UID)}}, Selected: selected, Children: body}
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
			lines[i].Style = toneStyle(runTone(model.RunStatus(fact.Value)))
		case "Held because":
			lines[i].Style = toneStyle(ui.ToneWarning)
		}
	}
	return factInspector(m, lines)
}
