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
	title, save := "New job", "Create job"
	if f.Change {
		title, save = "Edit job", "Save job"
	}
	if f.Conflict != nil {
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconTimer}, jobConflict(m)))
	}
	children := make([]tern.Element, 0, len(model.JobFields)+6)
	if p := f.Opened.Parent; f.Change && p != nil {
		parent := []ui.Span{refSpan(p.UID, false)}
		if p.ShortID != "" {
			parent = []ui.Span{refSpan("#"+p.ShortID, false), {T: p.Title}}
		}
		children = append(children, ui.Col{Common: ui.Common{Key: "parent-fixed"}, Gap: ui.GapXS, Children: ui.Nodes(
			propRow("Parent", ui.Text{Common: ui.Common{Key: "value"}, Spans: parent}),
			toneText("hint", ui.ToneMuted, "A job keeps its parent."))})
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
		focused := field.Name == f.Field
		label := model.JobPrompt(field.Name)
		if !field.Text {
			hint := ""
			if field.Name == model.JobFieldWorkflow {
				hint = f.WorkflowsNote()
			}
			children = append(children, choiceControl(m, model.CardJob, field.Name, label, focused, hint, ""))
			continue
		}
		if field.Name == model.JobFieldProfile && f.Worker.Note != "" {
			children = append(children, toneText("worker-note", ui.ToneWarning, "Cannot read the worker's config, so profile and checkout are free text: "+strings.TrimSuffix(f.Worker.Note, ".")+"."))
		}
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		text := f.Text(field.Name)
		var input tern.Element
		if field.Name == model.JobFieldPrompt {
			input = ui.Editor{
				Common:   ui.Common{Key: string(field.Name), Tone: tone, Aria: label},
				Field:    ui.Field{Text: text, Cursor: caretAt(f.Caret, field.Name, text), Mode: "insert", Readonly: f.Saving},
				MaxLines: 8,
			}
		} else {
			input = ui.Input{
				Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: label},
				Field:  ui.Field{Text: text, Cursor: caretAt(f.Caret, field.Name, text), Mode: "insert", Readonly: f.Saving},
			}
		}
		children = append(children, labelledInput(string(field.Name), label, model.JobHint(field.Name), focused, input)...)
	}
	if !previewed {
		children = append(children, jobPreview(f, m.Calendar, now))
	}
	if conditions := jobConditions(f); conditions != nil {
		children = append(children, conditions)
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("save", "the job", f.Err)))
	}
	if f.Saving {
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	} else {
		children = append(children, formFoot(cardButton("save", model.ModeJobForm, model.JobSave, save, true), exitButton("cancel", model.ModeJobForm, model.JobCancel, "Cancel")))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardJob)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconTimer}, children))
}

// jobPreview is what the trigger as typed means: its words, then the next runs it lists, or why it reads as no schedule.
func jobPreview(f model.JobFormState, cal model.PaneCalendar, now time.Time) tern.Element {
	preview := f.Preview(cal, now)
	if preview.Err != nil {
		return toneText("preview", ui.ToneWarning, "Not a valid schedule: "+model.Reason(preview.Err)+".")
	}
	lines := []tern.Element{ui.Text{Common: ui.Common{Key: "words"}, Text: "Runs " + preview.Words}}
	if len(preview.Runs) > 0 {
		runs := make([]tern.Element, len(preview.Runs))
		for i, run := range preview.Runs {
			runs[i] = ui.Text{Common: ui.Common{Key: "run" + string(rune('a'+i))}, Spans: []ui.Span{timeSpan(run)}}
		}
		lines = append(lines, propRow("Next runs", ui.Col{Common: ui.Common{Key: "values"}, Gap: ui.GapXS, Children: ui.Nodes(runs...)}))
	}
	if preview.Note != "" {
		lines = append(lines, toneText("note", ui.ToneMuted, preview.Note))
	}
	return ui.Col{Common: ui.Common{Key: "preview", Tone: ui.ToneMuted}, Gap: ui.GapXS, Children: ui.Nodes(lines...)}
}

// jobConditions lists what the job's conditions hold it back for. They are set with kata, so the card only reads them.
func jobConditions(f model.JobFormState) tern.Element {
	lines, shown, err := f.ConditionLines()
	if !shown {
		return nil
	}
	if err != nil {
		return toneText("conditions", ui.ToneError, "Cannot read the conditions: "+model.Reason(err)+". The worker holds every due run of this job until they are fixed.")
	}
	rows := []tern.Element{toneText("conditions-head", ui.ToneMuted, "Only runs when")}
	for i, line := range lines {
		rows = append(rows, toneText("condition"+string(rune('a'+i)), ui.ToneNeutral, line))
	}
	return ui.Col{Common: ui.Common{Key: "conditions"}, Children: ui.Nodes(rows...)}
}

// jobConflict is the card while the job changed since it was opened: what the newer job holds, and the three ways out. The
// newer job is the primary answer, and the safe exit is last.
func jobConflict(m model.Model) []tern.Element {
	rows := []tern.Element{toneText("conflict", ui.ToneWarning, "Changed since you opened it. Nothing was written; your Draft is kept.")}
	facts := m.JobConflictFacts()
	props := make([]tern.Element, len(facts))
	for i, fact := range facts {
		props[i] = propRow(fact.Label, ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Text: fact.Value})
	}
	if len(props) > 0 {
		rows = append(rows, propGrid(props...))
	}
	return append(rows, formFoot(
		cardButton("newer", model.ModeJobConflict, model.JobTakeNewer, "Use theirs", true),
		exitButton("discard", model.ModeJobConflict, model.JobDiscard, "Discard mine"),
		cardButton("overwrite", model.ModeJobConflict, model.JobOverwrite, "Keep mine", false)))
}
