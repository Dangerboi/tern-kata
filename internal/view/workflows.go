package view

import (
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

// workflows is the Page: wide, the list and the selected workflow's inspector side by side from the first frame, as the Issues
// Page shows its list and detail; narrow, the list alone until enter opens the inspector in its place.
func workflows(m model.Model, now time.Time) tern.Element {
	sideBySide := m.Cols >= model.SideBySideCols
	var panels []tern.Element
	if sideBySide || !m.Workflows.Inspecting {
		panels = append(panels, workflowList(m, sideBySide, now))
	}
	if sideBySide || m.Workflows.Inspecting {
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
		ui.Text{Common: ui.Common{Key: "key"}, Spans: []ui.Span{{T: strconv.Itoa(i+1) + " ", S: "muted"}, {T: step.Key, S: "strong"}}, Wrap: ui.WrapNone},
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
	if c.Conflict != nil {
		children = workflowConflict(c)
	} else {
		children = workflowForm(m, c)
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardWorkflow)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Meta: meta, Icon: ui.IconPen}, children))
}

// workflowForm is the card while it is edited: the workflow's own fields, then the shown step, then the worker's reading; the
// buttons that add, remove and move steps sit in the Steps head, the error and the other buttons go to the foot.
func workflowForm(m model.Model, c model.WorkflowCard) []tern.Element {
	f := c.Form
	readonly := c.Saving || c.DiscardConfirm
	text := func(name model.Field, label, hint string) []tern.Element {
		focused := f.Field == name
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		value := f.Text(name)
		input := ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: value, Cursor: caretAt(f.Caret, name, value), Mode: "insert", Readonly: readonly}}
		nodes := labelledInput(string(name), label, "", focused, input)
		return append(nodes, workflowHint(string(name), hint))
	}
	step := f.Step()
	children := text(model.WorkflowFieldName, "Name", "")
	children = append(children, text(model.WorkflowFieldAbout, "About", aboutHint)...)
	children = append(children, text(model.WorkflowFieldInput, "Input", inputHint)...)
	children = append(children, stepsHead(f, readonly))
	if pick := choiceControl(m, model.CardWorkflow, model.WorkflowFieldStep, "Step", f.Field == model.WorkflowFieldStep, "", ""); pick != nil {
		children = append(children, pick)
	}
	children = append(children, text(model.WorkflowFieldKey, "Step name", stepNameHint)...)
	if kind := choiceControl(m, model.CardWorkflow, model.WorkflowFieldKind, "Kind", f.Field == model.WorkflowFieldKind, kindHint, ""); kind != nil {
		children = append(children, kind)
	}
	bodyLabel := "Prompt"
	if step.Kind == cronjob.StepCommand {
		bodyLabel = "Command"
	}
	focused := f.Field == model.WorkflowFieldBody
	editor := TextEditorLines(string(model.WorkflowFieldBody), step.Body, readonly, 8)
	if e, ok := editor.(ui.Editor); ok {
		e.Aria = bodyLabel
		if !focused {
			e.Tone = ui.ToneNeutral
		}
		editor = e
	}
	bodyHint := promptHint
	if step.Kind == cronjob.StepCommand {
		bodyHint = commandHint
	}
	children = append(children, labelledInput(string(model.WorkflowFieldBody), bodyLabel, "", focused, editor)...)
	children = append(children, workflowHint(string(model.WorkflowFieldBody), bodyHint))
	children = append(children, text(model.WorkflowFieldAfter, "After", afterHint)...)
	children = append(children, text(model.WorkflowFieldRetries, "Retries", retriesHint)...)
	if c.Reading != nil {
		children = append(children, workerReading(*c.Reading))
	}
	if c.Saving {
		children = append(children, toneText("saving", ui.TonePending, model.Progress("Saving", "")))
	}
	if c.Err != nil {
		children = append(children, toneText("error", ui.ToneError, c.Err.Error()))
	}
	if c.DiscardConfirm {
		return append(children, toneText("question", ui.ToneWarning, "Discard this Draft?"), discardButtons())
	}
	busy := busyReason(c.Saving)
	return append(children, formFoot(
		unavailable(cardButton("save", model.ModeWorkflowForm, model.SaveDraftAction, "Save", true), busy),
		unavailable(exitButton("cancel", model.ModeWorkflowForm, model.CancelDraft, "Cancel"), busy),
		unavailable(cardButton("editor", model.ModeWorkflowForm, model.ExternalEditorAction, "Open in editor", false), busy)))
}

// stepsHead is the Steps section's head: the title and count, with the buttons that add, remove and move steps beside them.
func stepsHead(f model.WorkflowForm, busy bool) tern.Element {
	title := ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: "Steps", S: "strong"}}, Wrap: ui.WrapNone}
	children := ui.Nodes(title)
	if count := countSpan(len(f.Steps)); count.T != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "count"}, Spans: []ui.Span{count}, Wrap: ui.WrapNone})
	}
	if busy {
		// While the card saves or asks about discarding, the form is read-only and the step buttons are not offered.
		return ui.Row{Common: ui.Common{Key: "steps-head"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: children}
	}
	children = append(children,
		unavailable(cardButton("add", model.ModeWorkflowForm, model.WorkflowStepAdd, "Add step", false), busyReason(busy)),
		unavailable(cardButton("remove", model.ModeWorkflowForm, model.WorkflowStepRemove, "Remove step", false), stepReason(busy, !f.CanRemoveStep(), "A workflow needs at least one step.")),
		unavailable(cardButton("up", model.ModeWorkflowForm, model.WorkflowStepMoveUp, "Move up", false), stepReason(busy, !f.CanMoveStep(true), "This is the first step.")),
		unavailable(cardButton("down", model.ModeWorkflowForm, model.WorkflowStepMoveDown, "Move down", false), stepReason(busy, !f.CanMoveStep(false), "This is the last step.")))
	return ui.Row{Common: ui.Common{Key: "steps-head"}, Gap: ui.GapMD, Wrap: true, Align: ui.AlignCenter, Children: children}
}

// The descriptions under the Workflow card's fields. Each says what kata or the worker does with the field's text, as their own
// documentation has it; none invents a format.
const (
	aboutHint    = "Free text. The worker puts it at the top of every prompt step, so the agent knows what the workflow is for."
	inputHint    = "Free text noting what a run is given. kata keeps it with the workflow and the worker does not read it."
	stepNameHint = "Letters, digits, - _ and . only. Each step's name is unique, and After refers to it."
	kindHint     = "A command runs in a shell; a prompt goes to the agent."
	commandHint  = "Runs with sh -c in the job's checkout, as written (no placeholders). The environment has TERN_KATA_RUN, TERN_KATA_RUN_DIR and KATA_PROJECT."
	promptHint   = "Sent to the agent as the task, as written (no placeholders); the worker adds the Issue, run and step details around it."
	afterHint    = "Earlier steps this one needs, separated by commas. If one fails, this step is skipped."
	retriesHint  = "How many times a failed step runs again, 0 to 100. A timeout is never retried."
)

// workflowHint is the muted line under a Workflow card field that says what it takes. Unlike the hints of other forms it stays
// while the field has no focus, so the whole card can be read through.
func workflowHint(key, text string) tern.Element {
	return toneText(key+"-hint", ui.ToneMuted, text)
}

// busyReason is the reason a card's buttons cannot be pressed while it works, "" when they can.
func busyReason(busy bool) string {
	if busy {
		return model.Progress("Saving", "")
	}
	return ""
}

// stepReason is why a step button cannot be pressed: the card is busy, or the step has nothing to move or go.
func stepReason(busy, blocked bool, why string) string {
	if busy {
		return busyReason(true)
	}
	if blocked {
		return why
	}
	return ""
}

// workflowConflict is the card after a save found the workflow changed: what happened, the two texts side by side, and the
// choice. The current version being archived leaves only Discard mine, since a save would bring it back.
func workflowConflict(c model.WorkflowCard) []tern.Element {
	current := c.Conflict
	versions := ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
		ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: "Yours"}, TextEditorLines("body", model.NewTextEditor(c.Form.JSON()), true, 12))},
		ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: "Current"}, TextEditorLines("now", model.NewTextEditor(current.Text()), true, 12))},
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
