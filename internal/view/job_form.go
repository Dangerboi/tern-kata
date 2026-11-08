package view

import (
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

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
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconTimer}, jobConflict(m)))
	}
	children := make([]tern.Element, 0, len(model.JobFields)+6)
	if p := f.Opened.Parent; f.Change && p != nil {
		ref := p.UID
		if p.ShortID != "" {
			ref = "#" + p.ShortID + " " + p.Title
		}
		children = append(children, toneText("parent-fixed", ui.ToneMuted, "Parent: "+ref+" (a job keeps its parent)"))
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
			hint := ""
			if field.Name == model.JobFieldWorkflow {
				hint = f.WorkflowsNote()
			}
			children = append(children, choiceControl(m, model.CardJob, field.Name, model.JobPrompt(field.Name), field.Name == f.Field, hint, ""))
			continue
		}
		if field.Name == model.JobFieldProfile && f.Worker.Note != "" {
			children = append(children, toneText("worker-note", ui.ToneWarning, "The worker's config could not be read, so profile and checkout are free text: "+f.Worker.Note))
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
	if conditions := jobConditions(f); conditions != nil {
		children = append(children, conditions)
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	if f.Saving {
		children = append(children, toneText("pending", ui.TonePending, pending))
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeJobForm, model.JobSave, save, true), cardButton("cancel", model.ModeJobForm, model.JobCancel, "Cancel", false)))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconTimer}, children))
}

func jobPreview(f model.JobFormState, cal model.PaneCalendar, now time.Time) tern.Element {
	preview := f.Preview(cal, now)
	if preview.Err != nil {
		return toneText("preview", ui.ToneWarning, "Schedule: not yet; "+preview.Err.Error())
	}
	lines := []tern.Element{toneText("words", ui.ToneInfo, "Schedule: "+preview.Words)}
	if len(preview.Runs) > 0 {
		lines = append(lines, toneText("runs", ui.ToneMuted, "Next runs: "+strings.Join(preview.Runs, " · ")))
	}
	if preview.Note != "" {
		lines = append(lines, toneText("note", ui.ToneMuted, preview.Note))
	}
	return ui.Col{Common: ui.Common{Key: "preview"}, Children: ui.Nodes(lines...)}
}

func jobConditions(f model.JobFormState) tern.Element {
	lines, shown, err := f.ConditionLines()
	if !shown {
		return nil
	}
	if err != nil {
		return toneText("conditions", ui.ToneError, "Conditions: not readable; "+err.Error()+". The worker holds every due occurrence of this job, and lists the job as blocked, until options.conditions is fixed.")
	}
	rows := []tern.Element{toneText("conditions-head", ui.ToneInfo, "Only when (set in options.conditions with kata):")}
	for i, line := range lines {
		rows = append(rows, toneText("condition"+string(rune('a'+i)), ui.ToneNeutral, line))
	}
	rows = append(rows, toneText("conditions-note", ui.ToneMuted, "A worker that does not know conditions ignores them."))
	return ui.Col{Common: ui.Common{Key: "conditions"}, Children: ui.Nodes(rows...)}
}

func jobConflict(m model.Model) []tern.Element {
	rows := []tern.Element{toneText("conflict", ui.ToneWarning, "The job changed meanwhile; nothing was written. Your draft is kept. The newer job:")}
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
