package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Workflow card is a form: the workflow's name, about and input, then one step at a time (its name, kind, command or
// prompt, the steps it waits for and its retries), with the steps added, removed and moved from the card. The options kata
// accepts on a workflow and on a step have no field here; the form carries them through a save untouched, and "Open in
// editor" shows the whole document as JSON for anyone who wants to change them.

const (
	ModeWorkflowForm Mode = "workflow-form"

	WorkflowStepAdd      Action = "workflow-step-add"
	WorkflowStepRemove   Action = "workflow-step-remove"
	WorkflowStepMoveUp   Action = "workflow-step-up"
	WorkflowStepMoveDown Action = "workflow-step-down"
)

const (
	WorkflowFieldName    Field = "name"
	WorkflowFieldAbout   Field = "about"
	WorkflowFieldInput   Field = "input"
	WorkflowFieldStep    Field = "step"
	WorkflowFieldKey     Field = "key"
	WorkflowFieldKind    Field = "kind"
	WorkflowFieldBody    Field = "body"
	WorkflowFieldAfter   Field = "after"
	WorkflowFieldRetries Field = "retries"
)

// WorkflowStepForm is one step as the card holds it. Body is the command of a command step and the prompt of a prompt step, so
// changing the kind keeps the text. After and Retries are the text typed: after is a comma separated list of step names.
type WorkflowStepForm struct {
	Key     string
	Kind    cronjob.StepKind
	Body    TextEditor
	After   string
	Retries string
	Options json.RawMessage
}

// WorkflowForm is the card's content: the workflow-level texts, the steps, and which step and field have the focus.
type WorkflowForm struct {
	Field              Field
	Caret              Caret
	Name, About, Input string
	Options            json.RawMessage
	Steps              []WorkflowStepForm
	// At is the index of the step whose fields the card shows.
	At int
}

// blankWorkflowForm is the form of a new workflow: no name and one empty prompt step.
func blankWorkflowForm() WorkflowForm {
	return WorkflowForm{Field: WorkflowFieldName, Steps: []WorkflowStepForm{{Key: "step", Kind: cronjob.StepPrompt}}}
}

// newWorkflowForm is the form that edits w.
func newWorkflowForm(w Workflow) WorkflowForm {
	f := WorkflowForm{Field: WorkflowFieldName, Name: w.Name, About: w.About, Input: w.Input, Options: w.Options, Steps: make([]WorkflowStepForm, len(w.Steps))}
	for i, s := range w.Steps {
		body := s.Command
		if s.Kind == cronjob.StepPrompt {
			body = s.Prompt
		}
		step := WorkflowStepForm{Key: s.Key, Kind: s.Kind, Body: NewTextEditor(normalizeLineBreaks(body)), After: strings.Join(s.After, ", "), Options: s.Options}
		if s.Retries > 0 {
			step.Retries = strconv.Itoa(s.Retries)
		}
		f.Steps[i] = step
	}
	if len(f.Steps) == 0 {
		f.Steps = blankWorkflowForm().Steps
	}
	return f
}

// current is the step whose fields are shown, nil only for a form with no step, which the card never holds.
func (f *WorkflowForm) current() *WorkflowStepForm {
	if len(f.Steps) == 0 {
		return nil
	}
	f.At = max(0, min(f.At, len(f.Steps)-1))
	return &f.Steps[f.At]
}

// Step is the step whose fields the card shows.
func (f WorkflowForm) Step() WorkflowStepForm {
	if step := f.current(); step != nil {
		return *step
	}
	return WorkflowStepForm{}
}

// StepLabels name each step in the step choice: its number and name, "1 sort", the number alone while it has no name yet.
func (f WorkflowForm) StepLabels() []string {
	labels := make([]string, len(f.Steps))
	for i, s := range f.Steps {
		labels[i] = strings.TrimSpace(strconv.Itoa(i+1) + " " + s.Key)
	}
	return labels
}

// Text is the field named name of the form: the text of a field that takes text, "" for any other.
func (f WorkflowForm) Text(name Field) string {
	step := f.Step()
	switch name {
	case WorkflowFieldName:
		return f.Name
	case WorkflowFieldAbout:
		return f.About
	case WorkflowFieldInput:
		return f.Input
	case WorkflowFieldKey:
		return step.Key
	case WorkflowFieldAfter:
		return step.After
	case WorkflowFieldRetries:
		return step.Retries
	}
	return ""
}

// afterList is the step names typed in an After field, split on commas and spaces.
func afterList(text string) []string {
	var list []string
	return append(list, strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })...)
}

// workflow is the Workflow the form describes, with the retries it holds read as numbers; an error names a retries text that is
// not a whole number.
func (f WorkflowForm) workflow() (Workflow, error) {
	w := Workflow{Name: f.Name, About: f.About, Input: f.Input, Options: f.Options, Steps: make([]WorkflowStep, len(f.Steps))}
	var problem error
	for i, s := range f.Steps {
		step := WorkflowStep{Key: s.Key, Kind: s.Kind, After: afterList(s.After), Options: s.Options}
		if s.Kind == cronjob.StepPrompt {
			step.Prompt = s.Body.Text
		} else {
			step.Command = s.Body.Text
		}
		if text := strings.TrimSpace(s.Retries); text != "" {
			n, err := strconv.Atoi(text)
			if err != nil && problem == nil {
				problem = fmt.Errorf("step %d: retries must be a whole number.", i+1)
			}
			step.Retries = n
		}
		w.Steps[i] = step
	}
	return w, problem
}

// JSON is the whole workflow as the JSON document kata takes, for the editor and for the checks; a retries text that is not a
// number reads as none.
func (f WorkflowForm) JSON() string {
	w, _ := f.workflow()
	return w.Text()
}

// document is the form checked against kata's rules: what a save sends, or the problem that stops it.
func (f WorkflowForm) document() (WorkflowDocument, error) {
	w, err := f.workflow()
	if err != nil {
		return WorkflowDocument{}, err
	}
	if strings.TrimSpace(w.Name) == "" {
		return WorkflowDocument{}, errors.New("the workflow needs a name.")
	}
	return ParseWorkflow(w.Text())
}

// snapshot is every text and choice of the form, without where the caret is, so a form left as it opened can be told from a
// changed one.
func (f WorkflowForm) snapshot() string {
	type step struct{ Key, Kind, Body, After, Retries, Options string }
	steps := make([]step, len(f.Steps))
	for i, s := range f.Steps {
		steps[i] = step{s.Key, string(s.Kind), s.Body.Text, s.After, s.Retries, string(s.Options)}
	}
	return fmt.Sprintf("%q %q %q %q %q", f.Name, f.About, f.Input, string(f.Options), steps)
}

// uniqueStepKey is a step name no step of the form uses: "step", then "step2" and on.
func (f WorkflowForm) uniqueStepKey() string {
	used := map[string]bool{}
	for _, s := range f.Steps {
		used[s.Key] = true
	}
	for n := 1; ; n++ {
		key := "step"
		if n > 1 {
			key += strconv.Itoa(n)
		}
		if !used[key] {
			return key
		}
	}
}

// addStep puts a new empty prompt step after the one shown and shows it, with the caret in its name.
func (f WorkflowForm) addStep() WorkflowForm {
	at := min(f.At+1, len(f.Steps))
	steps := make([]WorkflowStepForm, 0, len(f.Steps)+1)
	steps = append(steps, f.Steps[:at]...)
	steps = append(steps, WorkflowStepForm{Key: f.uniqueStepKey(), Kind: cronjob.StepPrompt})
	f.Steps = append(steps, f.Steps[at:]...)
	f.At, f.Field, f.Caret = at, WorkflowFieldKey, Caret{}
	return f
}

// removeStep takes the shown step out; the last step stays, since a workflow needs one.
func (f WorkflowForm) removeStep() (WorkflowForm, bool) {
	if len(f.Steps) <= 1 {
		return f, false
	}
	f.current()
	steps := make([]WorkflowStepForm, 0, len(f.Steps)-1)
	steps = append(steps, f.Steps[:f.At]...)
	f.Steps = append(steps, f.Steps[f.At+1:]...)
	f.At = min(f.At, len(f.Steps)-1)
	f.Caret = Caret{}
	return f, true
}

// moveStep swaps the shown step with the one before it (up) or after it, and keeps showing it.
func (f WorkflowForm) moveStep(up bool) (WorkflowForm, bool) {
	f.current()
	to := f.At + 1
	if up {
		to = f.At - 1
	}
	if to < 0 || to >= len(f.Steps) {
		return f, false
	}
	steps := append([]WorkflowStepForm(nil), f.Steps...)
	steps[f.At], steps[to] = steps[to], steps[f.At]
	f.Steps, f.At = steps, to
	return f, true
}

// CanMoveStep says whether the shown step has a neighbour in that direction; CanRemoveStep whether it may go.
func (f WorkflowForm) CanMoveStep(up bool) bool {
	if up {
		return f.At > 0
	}
	return f.At < len(f.Steps)-1
}

func (f WorkflowForm) CanRemoveStep() bool { return len(f.Steps) > 1 }

// workflowTable declares the card's fields in tab order. The shown step's fields reach into the step at the form's index; the
// body is the multi-line editor, which takes text like the others but whose text is the step's Editor, so the table reaches none.
var workflowTable = fieldTable[WorkflowCard]{
	form:  func(m *Model) *WorkflowCard { return &m.Workflows.Card },
	busy:  func(c WorkflowCard) bool { return c.Saving || c.DiscardConfirm || c.Conflict != nil },
	focus: func(c *WorkflowCard) *Field { return &c.Form.Field },
	caret: func(c *WorkflowCard) *Caret { return &c.Form.Caret },
	err:   func(c *WorkflowCard) *error { return &c.Err },
	fields: []formField[WorkflowCard]{
		textField(WorkflowFieldName, func(c *WorkflowCard) *string { return &c.Form.Name }),
		textField(WorkflowFieldAbout, func(c *WorkflowCard) *string { return &c.Form.About }),
		textField(WorkflowFieldInput, func(c *WorkflowCard) *string { return &c.Form.Input }),
		workflowStepField,
		textField(WorkflowFieldKey, func(c *WorkflowCard) *string {
			if step := c.Form.current(); step != nil {
				return &step.Key
			}
			return nil
		}),
		workflowKindField,
		textField(WorkflowFieldBody, func(*WorkflowCard) *string { return nil }),
		textField(WorkflowFieldAfter, func(c *WorkflowCard) *string {
			if step := c.Form.current(); step != nil {
				return &step.After
			}
			return nil
		}),
		textField(WorkflowFieldRetries, func(c *WorkflowCard) *string {
			if step := c.Form.current(); step != nil {
				return &step.Retries
			}
			return nil
		}),
	},
}

// workflowStepField is the choice of the step the card shows.
var workflowStepField = formField[WorkflowCard]{name: WorkflowFieldStep,
	step: func(_ *Model, c *WorkflowCard, back bool) {
		c.Form.At = cycleIndex(c.Form.At, max(len(c.Form.Steps), 1), back)
	},
	choose: func(_ *Model, c *WorkflowCard, index int) bool {
		if index < 0 || index >= len(c.Form.Steps) {
			return false
		}
		c.Form.At = index
		return true
	},
}

// workflowKinds are the kinds of a step in the order the kind choice shows them.
var workflowKinds = []cronjob.StepKind{cronjob.StepCommand, cronjob.StepPrompt}

var workflowKindField = formField[WorkflowCard]{name: WorkflowFieldKind,
	step: func(_ *Model, c *WorkflowCard, back bool) {
		if step := c.Form.current(); step != nil {
			step.Kind = workflowKinds[cycleIndex(kindIndex(step.Kind), len(workflowKinds), back)]
		}
	},
	choose: func(_ *Model, c *WorkflowCard, index int) bool {
		step := c.Form.current()
		if step == nil || index < 0 || index >= len(workflowKinds) {
			return false
		}
		step.Kind = workflowKinds[index]
		return true
	},
}

func kindIndex(kind cronjob.StepKind) int {
	if kind == cronjob.StepCommand {
		return 0
	}
	return 1
}

// WorkflowFields is the card's fields in tab order.
var WorkflowFields = workflowTable.specs()

func workflowChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	f := m.Workflows.Card.Form
	switch name {
	case WorkflowFieldStep:
		labels := f.StepLabels()
		return newChoices(segmentedOrSelect(len(labels)), labels, max(0, min(f.At, len(labels)-1))), true
	case WorkflowFieldKind:
		return newChoices(ControlTabs, []string{"Command", "Prompt"}, kindIndex(f.Step().Kind)), true
	}
	return ChoiceSet{}, false
}

// workflowMode is the mode of the open Workflow card: the form, the question that ends it, or the conflict a save found.
func workflowMode(m Model) Mode {
	switch c := m.Workflows.Card; {
	case c.Conflict != nil:
		return ModeConflict
	case c.DiscardConfirm:
		return ModeDiscard
	}
	return ModeWorkflowForm
}

func workflowBusy(m Model) bool { return workflowTable.busyOf(m.Workflows.Card) }

// workflowStepKey handles the actions that change the steps, and says whether the key was one of them. A step that cannot go or
// move leaves the form as it is.
func workflowStepKey(m Model, action Action) (Model, bool) {
	c := &m.Workflows.Card
	switch action {
	case WorkflowStepAdd:
		c.Form = c.Form.addStep()
	case WorkflowStepRemove:
		form, removed := c.Form.removeStep()
		if !removed {
			c.Err = errors.New("A workflow needs at least one step.")
			return m, true
		}
		c.Form = form
	case WorkflowStepMoveUp, WorkflowStepMoveDown:
		form, moved := c.Form.moveStep(action == WorkflowStepMoveUp)
		if !moved {
			return m, true
		}
		c.Form = form
	default:
		return m, false
	}
	c.Err = nil
	return m, true
}

// decodeWorkflowText reads the JSON document of a workflow into a Workflow without checking kata's rules on it, so text brought
// back from the editor lands in the form whatever is wrong with it and the save says what.
func decodeWorkflowText(text string) (Workflow, error) {
	file, err := decodeWorkflowFile(text)
	if err != nil {
		return Workflow{}, err
	}
	if file.Definition.Version != 1 {
		return Workflow{}, errors.New(`"version" must be 1.`)
	}
	w := Workflow{Name: file.Name, About: file.Definition.About, Input: file.Definition.Input, Options: file.Definition.Options, Steps: make([]WorkflowStep, len(file.Definition.Steps))}
	for i, s := range file.Definition.Steps {
		w.Steps[i] = WorkflowStep{Key: s.Key, Kind: s.Kind, Command: s.Command, Prompt: s.Prompt, After: s.After, Options: s.Options}
		if s.Retries != nil {
			w.Steps[i].Retries = int(*s.Retries)
		}
	}
	return w, nil
}
