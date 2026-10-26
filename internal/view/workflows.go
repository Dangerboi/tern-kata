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

// WorkflowListID is the Workflows Page's list of workflows; a row is the workflow's uid under it.
const WorkflowListID = "main.columns.list.page.names.rows"

// WorkflowID is the id of the Workflows Page's row for uid.
func WorkflowID(uid string) string { return itemID(WorkflowListID, uid) }

// Tones the Workflows Page gives what it names: a workflow's steps by kind, its retries and its waits.
const (
	commandTone  = ui.ToneInfo
	promptTone   = ui.ToneUser
	retryTone    = ui.ToneWarning
	afterTone    = ui.TonePending
	usingJobTone = ui.ToneAccent
)

// workflows is the Workflows Page: the Project's workflows, and beside them (in a narrow Pane instead of them) the steps of the
// selected one once Enter has opened them. It takes the list column's place.
func workflows(m model.Model, now time.Time) tern.Element {
	sideBySide := m.Cols >= model.SideBySideCols
	var panels []tern.Element
	if !m.Workflows.Inspecting || sideBySide {
		panels = append(panels, workflowList(m, sideBySide && m.Workflows.Inspecting))
	}
	if m.Workflows.Inspecting {
		panels = append(panels, workflowInspector(m, sideBySide, now))
	}
	heading := ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Workflows", S: "strong"}, {T: "  " + m.Collection.Title, S: "accent"}}}
	if m.Workflows.Loading {
		heading.Spans = append(heading.Spans, loadingMarker)
	}
	if !m.Workflows.ShowArchived {
		heading.Spans = append(heading.Spans, ui.Span{T: "  archived hidden", S: "muted"})
	}
	return ui.Col{Common: ui.Common{Key: "list", Grow: 1, Max: &ui.Bounds{H: ui.Frac(1)}}, Gap: ui.GapSM, Children: ui.Nodes(
		heading,
		ui.Row{Common: ui.Common{Key: "page", Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: panels},
	)}
}

// workflowList is the rows: name, then the steps it has, an archived workflow muted and badged. paired says the inspector sits beside.
func workflowList(m model.Model, paired bool) tern.Element {
	visible := m.VisibleWorkflows()
	items := make([]tern.Element, len(visible))
	for i, w := range visible {
		tone, style := ui.ToneNeutral, "strong"
		if w.Archived {
			tone, style = ui.ToneMuted, "muted"
		}
		if w.UID == m.Workflows.Selected {
			tone = ui.ToneAccent
		}
		label := []ui.Span{{T: w.Name, S: style}}
		if w.Archived {
			label = append(label, ui.Span{T: "  archived", S: "warning"})
		}
		value := []ui.Span{{T: stepCount(len(w.Steps)), S: "muted"}}
		if jobs := m.WorkflowJobs(w.UID); len(jobs) > 0 {
			value = append(value, ui.Span{T: "  " + jobNames(jobs), S: toneStyle(usingJobTone)})
		}
		if run, ok := m.LatestWorkflowRun(w.UID); ok {
			value = append(value, ui.Span{T: "  " + string(run.Status) + " " + m.Calendar.Stamp(run.Started()), S: toneStyle(runTone(run.Status))})
		}
		items[i] = ui.Item{Common: ui.Common{Key: w.UID, Role: stripeRole(i), Tone: tone}, Label: ui.Spans(label...), Detail: ui.T(" "), Value: ui.Spans(value...)}
	}
	empty := "No workflows in " + m.Collection.Title + ". Press n to write one."
	switch {
	case m.Workflows.Loading:
		empty = "Loading workflows…"
	case len(m.Workflows.List) > 0:
		empty = "Every workflow here is archived. Press a to show them."
	}
	selected := ""
	if m.Workflows.Selected != "" {
		selected = WorkflowID(m.Workflows.Selected)
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("rows"), Max: scrollMax(m, m.RowPx+6+8), Selected: selected, Empty: ui.T(empty), Children: items}
	common := ui.Common{Key: "names", Grow: 1, Basis: ui.BasisFrac(0.4), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}}
	children := ui.Nodes(list)
	if m.Workflows.Err != nil {
		children = ui.Nodes(ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Workflows.Err.Error()}, list)
	}
	if paired {
		return ui.Card{Common: common, Selected: true, Children: children}
	}
	return ui.Col{Common: common, Children: children}
}

// jobNames is the jobs that use a workflow, as a row's cell: the first name and how many more.
func jobNames(jobs []model.CronJob) string {
	if len(jobs) == 1 {
		return jobs[0].Name
	}
	return fmt.Sprintf("%s +%d", jobs[0].Name, len(jobs)-1)
}

func stepCount(n int) string {
	if n == 1 {
		return "1 step"
	}
	return strconv.Itoa(n) + " steps"
}

// workflowInspector is the selected workflow read as steps: each with its kind, its text, the steps it waits for and its retries.
func workflowInspector(m model.Model, paired bool, now time.Time) tern.Element {
	common := ui.Common{Key: "inspector", Grow: 1, Basis: ui.BasisFrac(0.6), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}}
	w, ok := m.SelectedWorkflow()
	if !ok {
		return ui.Col{Common: common, Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: "Select a workflow to read its steps."})}
	}
	head := []ui.Span{{T: w.Name, S: "strong"}}
	if w.Archived {
		head = append(head, ui.Span{T: "  archived", S: "warning"})
	}
	parts := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: head})
	if w.About != "" {
		parts = append(parts, ui.Text{Common: ui.Common{Key: "about"}, Text: w.About})
	}
	if w.Input != "" {
		parts = append(parts, ui.Text{Common: ui.Common{Key: "input"}, Spans: []ui.Span{{T: "input  ", S: "muted"}, {T: w.Input}}})
	}
	if reading, ok := m.WorkflowReading(w.UID); ok {
		parts = append(parts, workerReading(reading))
	}
	parts = append(parts, ui.Text{Common: ui.Common{Key: "steps-title"}, Spans: []ui.Span{{T: "Steps", S: "strong"}, {T: " " + strconv.Itoa(len(w.Steps)), S: "muted"}}})
	for i, step := range w.Steps {
		parts = append(parts, workflowStep(i, step))
	}
	parts = append(parts, workflowJobs(m, w)...)
	parts = append(parts, workflowRuns(m, w, now)...)
	parts = append(parts, ui.Text{Common: ui.Common{Key: "by"}, Spans: []ui.Span{{T: fmt.Sprintf("by %s · revision %d", w.Author, w.Revision), S: "muted"}}})
	body := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m, 0), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: parts})}
	if paired {
		return ui.Card{Common: common, Children: ui.Nodes(body)}
	}
	return ui.Col{Common: common, Children: ui.Nodes(body)}
}

// workerReading is the Worker's reading of a Workflow: what this machine's Worker would refuse and why, or that it would run it.
// Refusals and a worker.toml that could not be read are drawn as warnings, the rest quietly.
func workerReading(r model.WorkerReading) tern.Element {
	lines := r.Lines()
	children := make([]tern.Element, len(lines))
	for i, line := range lines {
		tone := ui.ToneMuted
		if line.Warn {
			tone = ui.ToneWarning
		}
		children[i] = ui.Text{Common: ui.Common{Key: "line." + strconv.Itoa(i), Tone: tone}, Text: line.Text}
	}
	return ui.Col{Common: ui.Common{Key: "worker-reading"}, Children: children}
}

// workflowStep is one step: its number and key, its kind badge, what it waits for and how often it retries, over its command or
// prompt.
func workflowStep(i int, step model.WorkflowStep) tern.Element {
	tone, text := commandTone, step.Command
	if step.Kind == cronjob.StepPrompt {
		tone, text = promptTone, step.Prompt
	}
	head := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "key"}, Spans: []ui.Span{{T: strconv.Itoa(i+1) + "  ", S: "muted"}, {T: step.Key, S: "accent strong"}}, Wrap: ui.WrapNone},
		ui.Badge{Common: ui.Common{Key: "kind", Tone: tone}, Text: string(step.Kind)},
	)
	if len(step.After) > 0 {
		head = append(head, ui.Text{Common: ui.Common{Key: "after"}, Spans: []ui.Span{{T: "after " + strings.Join(step.After, ", "), S: toneStyle(afterTone)}}, Wrap: ui.WrapNone})
	}
	if step.Retries > 0 {
		head = append(head, ui.Badge{Common: ui.Common{Key: "retries", Tone: retryTone}, Text: "retries " + strconv.Itoa(step.Retries)})
	}
	return ui.Col{Common: ui.Common{Key: "step." + strconv.Itoa(i)}, Gap: ui.GapSM, Children: ui.Nodes(
		ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: head},
		ui.Text{Common: ui.Common{Key: "text"}, Text: text},
	)}
}

// workflowLayer is the Workflow card: the JSON text of the workflow in an editor, the problem a failed check or save names under
// it, and the card's keys. After a save found the workflow changed, the text stays beside the workflow as it stands now.
func workflowLayer(m model.Model) tern.Element {
	c := m.Workflows.Card
	title := "New workflow"
	if c.UID != "" {
		title = "Edit workflow " + c.Name
	}
	readonly := c.Saving || c.DiscardConfirm || c.Conflict != nil
	children := ui.Nodes(TextEditor(inputLeaf, c.Editor, readonly))
	if c.Reading != nil && c.Conflict == nil {
		children = append(children, workerReading(*c.Reading))
	}
	if c.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Saving workflow…"})
	}
	if problem := cmp.Or(c.Err, c.Problem); problem != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: problem.Error()})
	}
	switch {
	case c.Conflict != nil:
		title = "Workflow changed: your text is kept"
		// The choice comes first: the card has a fixed height, and the two texts below can run past its end.
		children = append(conflictChoice(c.Conflict), ui.Row{Common: ui.Common{Key: "versions"}, Gap: ui.GapMD, Wrap: true, Children: ui.Nodes(
			ui.Col{Common: ui.Common{Key: "draft", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: "Your text"}, TextEditor("body", c.Editor, true))},
			ui.Col{Common: ui.Common{Key: "current", Grow: 1, Basis: ui.BasisFrac(0.5)}, Children: ui.Nodes(ui.Text{Text: fmt.Sprintf("As it stands now (revision %d)", c.Conflict.Revision)}, TextEditor("now", model.NewTextEditor(c.Conflict.Text()), true))},
		)})
	case c.DiscardConfirm:
		children = append(children, ui.Text{Common: ui.Common{Key: "discard", Tone: ui.ToneWarning}, Text: "Discard this text? y discards; n or esc keeps editing."}, discardButtons())
	case !c.Saving:
		children = append(children, buttonRow(
			cardButton("save", model.ModeEditor, model.SaveDraftAction, "Save", true),
			cardButton("editor", model.ModeEditor, model.ExternalEditorAction, "$EDITOR", false),
			cardButton("discard", model.ModeEditor, model.CancelDraft, "Discard", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardWorkflow)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}

// conflictChoice is the conflict card's ask: Overwrite and Discard Draft, or only Discard Draft when the current version is
// archived, since a save would bring it back.
func conflictChoice(current *model.Workflow) []tern.Element {
	discard := cardButton("discard", model.ModeConflict, model.DiscardDraft, "Discard Draft", current.Archived)
	if current.Archived {
		return ui.Nodes(ui.Text{Common: ui.Common{Key: "choice", Tone: ui.ToneWarning}, Text: "This workflow was archived meanwhile, so your text cannot be saved over it; d discards only your text."}, buttonRow(discard))
	}
	return ui.Nodes(ui.Text{Common: ui.Common{Key: "choice", Tone: ui.ToneWarning}, Text: "o saves your text over the new version; d discards only your text."},
		buttonRow(cardButton("overwrite", model.ModeConflict, model.OverwriteDraft, "Overwrite", true), discard))
}

// workflowRuns is the inspector's Runs section: the workflow's latest runs among those the Page read, newest first, each with its
// status, start, duration and summary.
func workflowRuns(m model.Model, w model.Workflow, now time.Time) []tern.Element {
	runs := m.WorkflowRuns(w.UID)
	if len(runs) == 0 {
		return nil
	}
	parts := []tern.Element{ui.Text{Common: ui.Common{Key: "runs-title"}, Spans: []ui.Span{{T: "Runs", S: "strong"}, {T: " " + strconv.Itoa(len(runs)), S: "muted"}}}}
	for _, r := range runs[:min(len(runs), model.WorkflowRunsShown)] {
		spans := []ui.Span{{T: string(r.Status), S: toneStyle(runTone(r.Status))}, {T: "  " + m.Calendar.Stamp(r.Started()), S: "muted"}}
		if d, ok := r.Duration(now); ok {
			spans = append(spans, ui.Span{T: "  " + model.ShortDuration(d), S: "muted"})
		}
		if msg, _, _ := strings.Cut(strings.TrimSpace(r.Summary), "\n"); msg != "" {
			spans = append(spans, ui.Span{T: "  " + msg})
		}
		parts = append(parts, ui.Text{Common: ui.Common{Key: "run." + r.UID}, Spans: spans})
	}
	return parts
}

// workflowJobs is the inspector's Jobs section: the live Cron jobs that run the workflow, each with its name and whether it is on.
func workflowJobs(m model.Model, w model.Workflow) []tern.Element {
	jobs := m.WorkflowJobs(w.UID)
	if len(jobs) == 0 {
		return nil
	}
	parts := []tern.Element{ui.Text{Common: ui.Common{Key: "jobs-title"}, Spans: []ui.Span{{T: "Jobs", S: "strong"}, {T: " " + strconv.Itoa(len(jobs)), S: "muted"}}}}
	for _, j := range jobs {
		state, style := "enabled", "success"
		if !j.Definition.Enabled {
			state, style = "disabled", "muted"
		}
		parts = append(parts, ui.Text{Common: ui.Common{Key: "job." + j.UID}, Spans: []ui.Span{{T: j.Name, S: toneStyle(usingJobTone)}, {T: "  " + state, S: style}}})
	}
	return parts
}
