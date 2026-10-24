package view

import (
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// jobLayer is the Job card: the fields the kind and trigger show, the schedule in words and the next runs under the trigger,
// and the buttons. After a save found the job changed meanwhile it shows the newer job and asks what to do with the draft.
func jobLayer(m model.Model, now time.Time) tern.Element {
	f := m.JobForm
	if !f.Open {
		return nil
	}
	title, save, pending := "New job", "Create job", "Creating the job…"
	if f.Change {
		title, save, pending = "Edit job", "Save job", "Saving the job…"
	}
	if f.Conflict != nil {
		return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: jobConflict(m)})
	}
	children := make([]tern.Element, 0, len(model.JobFields)+6)
	if p := f.Opened.Parent; f.Change && p != nil {
		ref := p.UID
		if p.ShortID != "" {
			ref = "#" + p.ShortID + " " + p.Title
		}
		children = append(children, ui.Text{Common: ui.Common{Key: "parent-fixed", Tone: ui.ToneMuted}, Text: "Parent: " + ref + " (a job keeps its parent)"})
	}
	previewed := false
	for _, field := range model.JobFields {
		if !f.Shows(field.Name) {
			continue
		}
		if !previewed && field.Name != model.JobFieldKind && field.Name != model.JobFieldName && !f.InTrigger(field.Name) && field.Name != model.JobFieldTrigger {
			children = append(children, jobPreview(f, m.Calendar, now))
			previewed = true
		}
		tone := ui.ToneNeutral
		prompt := model.JobPrompt(field.Name) + ": "
		if field.Name == f.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		if !field.Text {
			choices := f.JobChoices(field.Name)
			labels := make([]string, len(choices))
			for i, c := range choices {
				labels[i] = c.Label
			}
			children = append(children, choiceField(field.Name, tone, model.JobPrompt(field.Name), prompt, labels, func(i int) bool { return choices[i].Chosen }, -1))
			if note := f.WorkflowsNote(); field.Name == model.JobFieldWorkflow && note != "" {
				children = append(children, ui.Text{Common: ui.Common{Key: "workflow-note", Tone: ui.ToneMuted}, Text: note})
			}
			continue
		}
		if field.Name == model.JobFieldProfile && f.Worker.Note != "" {
			children = append(children, ui.Text{Common: ui.Common{Key: "worker-note", Tone: ui.ToneWarning}, Text: "The worker's config could not be read, so profile and checkout are free text: " + f.Worker.Note})
		}
		text := f.Text(field.Name)
		if field.Name == model.JobFieldPrompt {
			children = append(children, ui.Editor{
				Common:   ui.Common{Key: string(field.Name), Tone: tone, Aria: model.JobPrompt(field.Name)},
				Field:    ui.Field{Text: text, Cursor: caretAt(f.Caret, field.Name, text), Mode: "insert", Readonly: f.Saving, Prompt: []ui.Span{{T: prompt}}},
				MaxLines: 8,
			})
			continue
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: model.JobPrompt(field.Name)},
			Field:  ui.Field{Text: text, Cursor: caretAt(f.Caret, field.Name, text), Mode: "insert", Readonly: f.Saving, Prompt: []ui.Span{{T: prompt}}},
		})
	}
	if !previewed {
		children = append(children, jobPreview(f, m.Calendar, now))
	}
	if f.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()})
	}
	if f.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: pending})
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeJobForm, model.JobSave, save, true), cardButton("cancel", model.ModeJobForm, model.JobCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}

// jobPreview is the schedule under the trigger: in words and the next runs, or what keeps it from being told yet.
func jobPreview(f model.JobFormState, cal model.PaneCalendar, now time.Time) tern.Element {
	preview := f.Preview(cal, now)
	if preview.Err != nil {
		return ui.Text{Common: ui.Common{Key: "preview", Tone: ui.ToneWarning}, Text: "Schedule: not yet; " + preview.Err.Error()}
	}
	lines := []tern.Element{ui.Text{Common: ui.Common{Key: "words", Tone: ui.ToneInfo}, Text: "Schedule: " + preview.Words}}
	if len(preview.Runs) > 0 {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "runs", Tone: ui.ToneMuted}, Text: "Next runs: " + strings.Join(preview.Runs, " · ")})
	}
	return ui.Col{Common: ui.Common{Key: "preview"}, Children: ui.Nodes(lines...)}
}

// jobConflict is the card after a save found the job changed: the newer job, the draft kept, and the three ways out.
func jobConflict(m model.Model) []tern.Element {
	rows := []tern.Element{ui.Text{Common: ui.Common{Key: "conflict", Tone: ui.ToneWarning}, Text: "The job changed meanwhile; nothing was written. Your draft is kept. The newer job:"}}
	for i, fact := range m.JobConflictFacts() {
		label := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(14)}}, Spans: []ui.Span{{T: fact.Label, S: "muted"}}, Wrap: ui.WrapNone}
		value := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Text: fact.Value}
		rows = append(rows, ui.Row{Common: ui.Common{Key: "fact" + string(rune('a'+i))}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(label, value)})
	}
	return append(rows, buttonRow(
		cardButton("overwrite", model.ModeJobConflict, model.JobOverwrite, "Save my draft over it", false),
		cardButton("newer", model.ModeJobConflict, model.JobTakeNewer, "Use the newer job", true),
		cardButton("discard", model.ModeJobConflict, model.JobDiscard, "Discard draft", false)))
}
