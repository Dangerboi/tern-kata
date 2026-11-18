package view

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const WorkflowListID = "main.columns.list.page.names.rows"

func workflows(m model.Model, now time.Time) tern.Element {
	sideBySide := m.Cols >= model.SideBySideCols
	var panels []tern.Element
	if !m.Workflows.Inspecting || sideBySide {
		panels = append(panels, workflowList(m, sideBySide && m.Workflows.Inspecting, now))
	}
	if m.Workflows.Inspecting {
		panels = append(panels, workflowInspector(m, sideBySide, now))
	}
	var top []tern.Element
	if m.PageSearchShown() {
		top = append(top, pageSearchInput(m))
	}
	return ui.Col{Common: ui.Common{Key: "list", Grow: 1, Max: &ui.Bounds{H: ui.Frac(1)}}, Gap: ui.GapSM, Children: append(top,
		ui.Row{Common: ui.Common{Key: "page", Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: panels},
	)}
}

// workflowsHead is the list's head: the title, the number of workflows shown, what is hidden, and the loading marker last.
func workflowsHead(m model.Model, shown int) tern.Element {
	var hidden string
	if !m.Workflows.ShowArchived {
		if n := countArchivedWorkflows(m.Workflows.List); n > 0 {
			hidden = strconv.Itoa(n) + " archived hidden"
		}
	}
	head := pageHead("heading", "Workflows", strconv.Itoa(shown), hidden)
	if row, ok := head.(ui.Row); ok && m.Workflows.Loading {
		row.Children = append(row.Children, ui.Text{Common: ui.Common{Key: "loading"}, Spans: []ui.Span{loadingMarker}, Wrap: ui.WrapNone})
		head = row
	}
	return head
}

func countArchivedWorkflows(list []model.Workflow) int {
	n := 0
	for _, w := range list {
		if w.Archived {
			n++
		}
	}
	return n
}

func workflowList(m model.Model, paired bool, now time.Time) tern.Element {
	visible := m.VisibleWorkflows()
	cards := make([]tern.Element, len(visible))
	for i, w := range visible {
		cards[i] = workflowCard(m, w, w.UID == m.Workflows.Selected, now)
	}
	empty := emptyState("No workflows", "A workflow is a list of steps a job runs.", emptyButton(m, "new", model.NewWorkflow, "New workflow", true)...)
	if len(m.Workflows.List) > 0 {
		empty = emptyState("No workflows", "All of them are archived.", emptyButton(m, "show-archived", model.ToggleArchivedWorkflows, "Show archived", true)...)
	}
	if m.PageSearching() {
		empty = searchEmpty(m)
	}
	selected := selectedItem(WorkflowListID, m.Workflows.Selected)
	list := listAndEmpty(ui.List{CommonWithoutMax: scrollerCommon("rows"), Max: fillMax, Selected: selected, Empty: ui.T("Loading workflows…"), Children: cards}, m.Workflows.Loading, empty)
	common := ui.Common{Key: "names", Grow: 1, Basis: ui.BasisFrac(0.4), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}}
	children := ui.Nodes(workflowsHead(m, len(visible)))
	if m.Workflows.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("read", "workflows", m.Workflows.Err)))
	}
	children = append(children, list...)
	if paired {
		common.Role = ColumnRole
		return ui.Card{Common: common, Selected: true, Children: children}
	}
	return ui.Col{Common: common, Children: children}
}

// workflowCard is a workflow in the list: its name and step count, the jobs that use it, and its latest run.
func workflowCard(m model.Model, w model.Workflow, selected bool, now time.Time) tern.Element {
	name := ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: w.Name, S: "strong"}}, Wrap: ui.WrapNone}
	head := ui.Nodes(name)
	if w.Archived {
		head = append(head, ui.Badge{Common: ui.Common{Key: "archived", Tone: ui.ToneMuted}, Text: "archived"})
	}
	head = append(head, ui.Text{Common: ui.Common{Key: "steps"}, Spans: []ui.Span{{T: stepCount(len(w.Steps)), S: "muted"}}, Wrap: ui.WrapNone})
	body := ui.Nodes(ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: head})
	if jobs := m.WorkflowJobs(w.UID); len(jobs) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "jobs"}, Spans: []ui.Span{{T: "Used by " + jobNames(jobs), S: "muted"}}})
	}
	if run, ok := m.LatestWorkflowRun(w.UID); ok {
		body = append(body, ui.Text{Common: ui.Common{Key: "latest"}, Spans: []ui.Span{
			{T: string(run.Status), S: toneStyle(runStateTone(string(run.Status)))},
			{T: " · ", S: "muted"},
			timeSpan(m.Calendar.Read(run.Started(), now)),
		}})
	}
	cardTone := ui.ToneNeutral
	if w.Archived {
		cardTone = ui.ToneMuted
	}
	return ui.Card{Common: ui.Common{Key: w.UID, Tone: cardTone, Actions: &ui.Actions{Click: WorkflowClick(w.UID), DblClick: WorkflowOpenClick(w.UID)}}, Selected: selected, Children: body}
}

func jobNames(jobs []model.CronJob) string {
	if len(jobs) == 1 {
		return jobs[0].Name
	}
	return fmt.Sprintf("%s +%d", jobs[0].Name, len(jobs)-1)
}

func stepCount(n int) string { return model.Count(n, "step", "steps") }

// sectionHead is a section's title with its count after it: "Steps 2", the count a muted bare number left out at zero.
func sectionHead(key, title string, n int) tern.Element {
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: title, S: "strong"}}, Wrap: ui.WrapNone})
	if count := countSpan(n); count.T != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "count"}, Spans: []ui.Span{count}, Wrap: ui.WrapNone})
	}
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: children}
}

func workflowInspector(m model.Model, paired bool, now time.Time) tern.Element {
	common := ui.Common{Key: "inspector", Grow: 1, Basis: ui.BasisFrac(0.6), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}}
	w, ok := m.SelectedWorkflow()
	if !ok {
		return ui.Col{Common: common, Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select a workflow to read it."})}
	}
	head := ui.Nodes(ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: w.Name, S: "strong"}}, Wrap: ui.WrapNone})
	if w.Archived {
		head = append(head, ui.Text{Common: ui.Common{Key: "archived"}, Spans: []ui.Span{{T: "archived", S: "muted"}}, Wrap: ui.WrapNone})
	}
	parts := ui.Nodes(ui.Row{Common: ui.Common{Key: "title"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: head})
	if w.About != "" {
		parts = append(parts, ui.Text{Common: ui.Common{Key: "about"}, Spans: []ui.Span{{T: w.About, S: "muted"}}})
	}
	if w.Input != "" {
		parts = append(parts, propGrid(propRow("Input", ui.Text{Common: ui.Common{Key: "v"}, Text: w.Input})))
	}
	if reading, ok := m.WorkflowReading(w.UID); ok {
		parts = append(parts, workerReading(reading))
	}
	parts = append(parts, sectionHead("steps-title", "Steps", len(w.Steps)))
	for i, step := range w.Steps {
		parts = append(parts, workflowStep(i, step))
	}
	parts = append(parts, workflowJobs(m, w)...)
	parts = append(parts, workflowRuns(m, w, now)...)
	if w.Author != "" {
		parts = append(parts, ui.Text{Common: ui.Common{Key: "by"}, Spans: []ui.Span{{T: "Created by " + w.Author, S: "muted"}}})
	}
	body := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: fillMax, Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})}
	if paired {
		common.Role = ColumnRole
		return ui.Card{Common: common, Children: ui.Nodes(body)}
	}
	return ui.Col{Common: common, Children: ui.Nodes(body)}
}

// workerReading is the worker's verdict on a workflow: the good verdict in success, each refusal a warning line, and what could
// not be checked in warning or muted text.
func workerReading(r model.WorkerReading) tern.Element {
	lines := r.Lines()
	children := make([]tern.Element, len(lines))
	for i, line := range lines {
		tone := ui.ToneMuted
		switch {
		case line.Warn:
			tone = ui.ToneWarning
		case line.Good:
			tone = ui.ToneSuccess
		}
		children[i] = toneText("line."+strconv.Itoa(i), tone, line.Text)
	}
	return ui.Col{Common: ui.Common{Key: "worker-reading"}, Children: children}
}

// workflowStep is one step: its number and key, what kind it is, what it runs after and its retries, over its command or prompt.
func workflowStep(i int, step model.WorkflowStep) tern.Element {
	text := ui.Text{Common: ui.Common{Key: "text"}, Spans: []ui.Span{monoSpan(step.Command, "")}}
	if step.Kind == cronjob.StepPrompt {
		text = ui.Text{Common: ui.Common{Key: "text"}, Text: step.Prompt}
	}
	head := ui.Nodes(
		ui.Row{Common: ui.Common{Key: "key"}, Gap: ui.GapXS, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "number"}, Spans: []ui.Span{{T: strconv.Itoa(i + 1), S: "muted"}}, Wrap: ui.WrapNone},
			ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: step.Key, S: "strong"}}, Wrap: ui.WrapNone},
		)},
		ui.Text{Common: ui.Common{Key: "kind"}, Spans: []ui.Span{{T: string(step.Kind), S: "muted"}}, Wrap: ui.WrapNone},
	)
	if len(step.After) > 0 {
		head = append(head, ui.Text{Common: ui.Common{Key: "after"}, Spans: []ui.Span{{T: "after " + strings.Join(step.After, ", "), S: "muted"}}, Wrap: ui.WrapNone})
	}
	if step.Retries > 0 {
		head = append(head, ui.Badge{Common: ui.Common{Key: "retries", Tone: ui.ToneMuted}, Text: "retries " + strconv.Itoa(step.Retries)})
	}
	return ui.Col{Common: ui.Common{Key: "step." + strconv.Itoa(i)}, Gap: ui.GapSM, Children: ui.Nodes(
		ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Wrap: true, Align: ui.AlignCenter, Children: head},
		text,
	)}
}

func workflowLayer(m model.Model) tern.Element {
	c := m.Workflows.Card
	title, meta := "New workflow", ui.Rich{}
	if c.UID != "" {
		title, meta = "Edit workflow", ui.T(c.Name)
	}
	var children []tern.Element
	switch {
	case c.Conflict != nil:
		children = workflowConflict(c)
	case c.DiscardConfirm:
		children = ui.Nodes(TextEditor(inputLeaf, c.Editor, true), toneText("question", ui.ToneWarning, "Discard this Draft?"), discardButtons())
	default:
		children = ui.Nodes(TextEditor(inputLeaf, c.Editor, c.Saving))
		if c.Reading != nil {
			children = append(children, workerReading(*c.Reading))
		}
		if c.Saving {
			children = append(children, toneText("saving", ui.TonePending, model.Progress("Saving", "")))
		}
		if problem := cmp.Or(c.Err, c.Problem); problem != nil {
			children = append(children, toneText("error", ui.ToneError, problem.Error()))
		}
		children = append(children, workflowFoot(c.Saving))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardWorkflow)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Meta: meta, Icon: ui.IconPen}, children))
}

// workflowFoot is the editing card's buttons: Save, then Open in editor, then the safe exit. While a save is in flight they stay drawn
// and cannot be pressed.
func workflowFoot(saving bool) tern.Element {
	reason := ""
	if saving {
		reason = model.Progress("Saving", "")
	}
	return formFoot(
		unavailable(cardButton("save", model.ModeEditor, model.SaveDraftAction, "Save", true), reason),
		unavailable(exitButton("cancel", model.ModeEditor, model.CancelDraft, "Cancel"), reason),
		unavailable(cardButton("editor", model.ModeEditor, model.ExternalEditorAction, "Open in editor", false), reason),
	)
}

// workflowConflict is the card after a save found the workflow changed: what happened, the two texts side by side, and the
// choice. The current version being archived leaves only Discard mine, since a save would bring it back.
func workflowConflict(c model.WorkflowCard) []tern.Element {
	current := c.Conflict
	versions := ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
		ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: "Yours"}, TextEditor("body", c.Editor, true))},
		ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: "Current"}, TextEditor("now", model.NewTextEditor(current.Text()), true))},
	)}
	if current.Archived {
		return ui.Nodes(
			toneText("choice", ui.ToneWarning, "Archived since you opened it, so your text cannot be saved over it."),
			versions,
			buttonRow(cardButton("discard", model.ModeConflict, model.DiscardDraft, "Discard mine", true)),
		)
	}
	return ui.Nodes(
		toneText("choice", ui.ToneWarning, "Changed since you opened it. Nothing was written; your Draft is kept."),
		versions,
		formFoot(cardButton("overwrite", model.ModeConflict, model.OverwriteDraft, "Keep mine", true), exitButton("discard", model.ModeConflict, model.DiscardDraft, "Discard mine")),
	)
}

func workflowRuns(m model.Model, w model.Workflow, now time.Time) []tern.Element {
	runs := m.WorkflowRuns(w.UID)
	if len(runs) == 0 {
		return nil
	}
	parts := []tern.Element{sectionHead("runs-title", "Runs", len(runs))}
	for _, r := range runs[:min(len(runs), model.WorkflowRunsShown)] {
		spans := []ui.Span{{T: string(r.Status), S: toneStyle(runStateTone(string(r.Status)))}, {T: " · ", S: "muted"}, timeSpan(m.Calendar.Stamp(r.Started()))}
		if d, ok := r.Duration(now); ok {
			spans = append(spans, ui.Span{T: " · ", S: "muted"}, timeSpan(model.ShortDuration(d)))
		}
		if msg := r.Brief(); msg != "" {
			spans = append(spans, ui.Span{T: " · ", S: "muted"}, ui.Span{T: msg})
		}
		parts = append(parts, ui.Text{Common: ui.Common{Key: "run." + r.UID}, Spans: spans})
	}
	return parts
}

func workflowJobs(m model.Model, w model.Workflow) []tern.Element {
	jobs := m.WorkflowJobs(w.UID)
	if len(jobs) == 0 {
		return nil
	}
	parts := []tern.Element{sectionHead("jobs-title", "Jobs", len(jobs))}
	for _, j := range jobs {
		state, style := "enabled", "success"
		if !j.Definition.Enabled {
			state, style = "disabled", "muted"
		}
		parts = append(parts, ui.Row{Common: ui.Common{Key: "job." + j.UID}, Gap: ui.GapMD, Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: j.Name}}},
			ui.Text{Common: ui.Common{Key: "state"}, Spans: []ui.Span{{T: state, S: style}}},
		)})
	}
	return parts
}
