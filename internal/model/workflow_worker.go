package model

import (
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Worker check: what this machine's Worker would refuse of a Workflow kata accepts. The Daemon gateway asks the Worker's own
// code (the model never imports it); the Workflow card shows the reading under its text and the Workflows Page under the selected
// Workflow. The reading never stops a save.

// workflowReadingDelay is how long the card waits after a change of its text before it asks for the Worker's reading.
const workflowReadingDelay = 400 * time.Millisecond

// WorkerReading is the Worker's reading of one Workflow definition on this machine.
type WorkerReading struct {
	// Refusals are why the Worker would not run it, in the Worker's words, all of those that need no worker.toml and the prompt
	// budget when it could be checked.
	Refusals []string
	// BudgetUnchecked says the prompt budget could not be checked: BudgetWhy is why, and ConfigMissing that the reason is only that
	// this machine has no worker.toml (which is no fault, so it is not drawn as a warning).
	BudgetUnchecked bool
	BudgetWhy       string
	ConfigMissing   bool
}

// Refuses says the Worker would not run the Workflow.
func (r WorkerReading) Refuses() bool { return len(r.Refusals) > 0 }

// ReadingLine is one line of a reading and whether it is drawn as a warning.
type ReadingLine struct {
	Text string
	Warn bool
}

// Lines are the reading in words: the verdict, then one line per refusal, then what could not be checked. Refusals warn; the
// unchecked budget warns only when worker.toml exists and could not be read, not when it is simply missing.
func (r WorkerReading) Lines() []ReadingLine {
	var lines []ReadingLine
	switch {
	case r.Refuses():
		lines = append(lines, ReadingLine{"This machine's Worker will not run it:", true})
		for _, why := range r.Refusals {
			lines = append(lines, ReadingLine{"• " + why, true})
		}
	case r.BudgetUnchecked:
		lines = append(lines, ReadingLine{"Nothing here that does not depend on worker.toml would stop this machine's Worker.", false})
	default:
		lines = append(lines, ReadingLine{"This machine's Worker would run it.", false})
	}
	if r.BudgetUnchecked {
		lines = append(lines, ReadingLine{"The prompt budget could not be checked: " + r.BudgetWhy + ".", !r.ConfigMissing})
	}
	return lines
}

// ReadWorkflowText asks for the Worker's reading of a definition the card's text holds, After a pause when After is set. Request
// numbers the asks: an answer to an older one is dropped.
type ReadWorkflowText struct {
	Request    int64
	Definition cronjob.WorkflowDefinition
	After      time.Duration
}

func (ReadWorkflowText) cmd() {}

// WorkflowTextRead answers ReadWorkflowText with the same Request.
type WorkflowTextRead struct {
	Request int64
	Reading WorkerReading
}

func (WorkflowTextRead) msg() {}

// Definition is the workflow in the shared shape the Worker's check and kata's validator read.
func (w Workflow) Definition() cronjob.WorkflowDefinition {
	out := cronjob.WorkflowDefinition{Version: 1, About: w.About, Input: w.Input, Options: decodeOptions(w.Options), Steps: make([]cronjob.WorkflowStep, len(w.Steps))}
	for i, s := range w.Steps {
		step := cronjob.WorkflowStep{Key: s.Key, Kind: cronjob.StepKind(s.Kind), Command: s.Command, Prompt: s.Prompt, After: s.After, Options: decodeOptions(s.Options)}
		if s.Retries != 0 {
			retries := int64(s.Retries)
			step.Retries = &retries
		}
		out.Steps[i] = step
	}
	return out
}

// settleWorkflowReading asks for the Worker's reading when the open card's text is one it has not asked about: at once when the
// card just opened or its text came back from $EDITOR, after a pause while the text is being typed. Text kata refuses gets no
// ask, and any answer still on its way is dropped by numbering the ask all the same.
func (m Model) settleWorkflowReading() (Model, []Cmd) {
	w := &m.Workflows
	c := &w.Card
	if !c.Open || c.Conflict != nil || (c.asked && c.askedText == c.Editor.Text) {
		return m, nil
	}
	after := workflowReadingDelay
	if c.askNow {
		after = 0
	}
	c.asked, c.askedText, c.askNow, c.Reading = true, c.Editor.Text, false, nil
	w.checkRequest++
	doc, err := ParseWorkflow(c.Editor.Text)
	if err != nil {
		return m, nil
	}
	return m, []Cmd{ReadWorkflowText{Request: w.checkRequest, Definition: doc.Shape, After: after}}
}

// workflowTextRead keeps the answer to the latest ask for the card that is open.
func workflowTextRead(m Model, msg WorkflowTextRead) Model {
	if c := &m.Workflows.Card; c.Open && msg.Request == m.Workflows.checkRequest {
		reading := msg.Reading
		c.Reading = &reading
	}
	return m
}

// WorkflowReading is the Worker's reading of the stored version of the workflow with uid, false until it came with a load or a
// save.
func (m Model) WorkflowReading(uid string) (WorkerReading, bool) {
	r, ok := m.Workflows.readings[uid]
	return r, ok
}
