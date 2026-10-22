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
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Badge{Common: ui.Common{Key: "status", Tone: tone}, Text: string(row.Run.Status)},
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
		return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD,
			ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select a run to read its detail."}))
	}
	rows := make([]tern.Element, len(facts))
	for i, fact := range facts {
		label := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(18)}}, Spans: []ui.Span{{T: fact.Label, S: "muted"}}, Wrap: ui.WrapNone}
		style := ""
		if fact.Label == "Status" {
			style = toneStyle(runTone(model.RunStatus(fact.Value)))
		}
		value := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: fact.Value, S: style}}}
		rows[i] = ui.Row{Common: ui.Common{Key: "fact" + strconv.Itoa(i)}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(label, value)}
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m, 0), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: rows})}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, ui.Nodes(list))
}
