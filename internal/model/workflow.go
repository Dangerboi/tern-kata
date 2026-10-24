package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// Workflow is one of a Project's kata workflows: reusable command and prompt steps with dependencies. EventUID names the
// version the Pane read; a write is guarded on it.
type Workflow struct {
	UID, Name, Author string
	// EventUID is kata's definition_event_uid: it changes on every write, archive and restore.
	EventUID string
	Revision int64
	Archived bool
	About    string
	Input    string
	// Options is the workflow's adapter options as kata holds them, nil when there are none.
	Options json.RawMessage
	Steps   []WorkflowStep
}

// WorkflowStep is one step of a Workflow. A command step carries Command, a prompt step Prompt; After names the earlier
// steps it waits for; Retries is how often a failing step runs again.
type WorkflowStep struct {
	Key, Kind       string
	Command, Prompt string
	After           []string
	Retries         int
	Options         json.RawMessage
}

// Workflow step kinds, as the Pane's text holds them.
const (
	StepCommand = string(cronjob.StepCommand)
	StepPrompt  = string(cronjob.StepPrompt)
)

// The limits of the document the Pane sends; the definition's own rules live in package cronjob.
const (
	maxWorkflowNameBytes  = 256
	maxWorkflowDefinition = 256 << 10
)

// The wire shape of a workflow definition file, as kata's own `{"name", "definition"}` document has it. Field order is the
// order the text shows.
type workflowFile struct {
	Name       string             `json:"name"`
	Definition workflowDefinition `json:"definition"`
}

type workflowDefinition struct {
	Version int64           `json:"version"`
	About   string          `json:"about,omitempty"`
	Input   string          `json:"input,omitempty"`
	Options json.RawMessage `json:"options,omitempty"`
	Steps   []workflowStep  `json:"steps"`
}

type workflowStep struct {
	Key     string          `json:"key"`
	Kind    string          `json:"kind"`
	Command string          `json:"command,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
	After   []string        `json:"after,omitempty"`
	Retries *int64          `json:"retries,omitempty"`
	Options json.RawMessage `json:"options,omitempty"`
}

// NewWorkflowText is the text a new workflow starts from: one prompt step to fill in.
const NewWorkflowText = `{
  "name": "",
  "definition": {
    "version": 1,
    "steps": [
      {
        "key": "step",
        "kind": "prompt",
        "prompt": ""
      }
    ]
  }
}
`

// Text is the workflow as the JSON document the editor shows: its name and its definition, in the shape kata takes.
func (w Workflow) Text() string {
	file := workflowFile{Name: w.Name, Definition: workflowDefinition{Version: 1, About: w.About, Input: w.Input, Options: w.Options, Steps: make([]workflowStep, len(w.Steps))}}
	for i, step := range w.Steps {
		out := workflowStep{Key: step.Key, Kind: step.Kind, Command: step.Command, Prompt: step.Prompt, After: step.After, Options: step.Options}
		if step.Retries != 0 {
			retries := int64(step.Retries)
			out.Retries = &retries
		}
		file.Definition.Steps[i] = out
	}
	var text bytes.Buffer
	encoder := json.NewEncoder(&text)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	// A document of strings, numbers and options kata already accepted always encodes.
	_ = encoder.Encode(file)
	return text.String()
}

// WorkflowDocument is a workflow text that passed the local check: the name and the definition to send, compact.
type WorkflowDocument struct {
	Name       string
	Definition string
	// Shape is the same definition decoded, for the Worker's check.
	Shape cronjob.WorkflowDefinition
}

// ParseWorkflow checks text, the JSON document of a workflow, against kata's rules and returns what to send. The error says
// what is wrong and where; the text stays the caller's to keep.
func ParseWorkflow(text string) (WorkflowDocument, error) {
	var file workflowFile
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return WorkflowDocument{}, describeJSONError(text, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return WorkflowDocument{}, errors.New("Not valid JSON: more text follows the workflow document.")
	}
	if err := file.check(); err != nil {
		return WorkflowDocument{}, err
	}
	definition, err := json.Marshal(file.Definition)
	if err != nil {
		return WorkflowDocument{}, fmt.Errorf("Not valid JSON: %v", err)
	}
	if len(definition) > maxWorkflowDefinition {
		return WorkflowDocument{}, errors.New("The workflow definition is over kata's 256 KiB limit.")
	}
	return WorkflowDocument{Name: strings.TrimSpace(file.Name), Definition: string(definition), Shape: file.Definition.shape()}, nil
}

// check is the document rules in kata's order: the name first, then the definition against the shared validator.
func (f workflowFile) check() error {
	if name := strings.TrimSpace(f.Name); name == "" || len(name) > maxWorkflowNameBytes {
		return errors.New(`"name" must be 1 to 256 characters.`)
	}
	if problem := cronjob.ValidateWorkflow(f.Definition.shape()); problem != nil {
		return problem
	}
	return nil
}

// shape is the decoded definition the shared validator reads. The options are valid JSON already, so they decode.
func (d workflowDefinition) shape() cronjob.WorkflowDefinition {
	out := cronjob.WorkflowDefinition{Version: d.Version, About: d.About, Input: d.Input, Options: decodeOptions(d.Options), Steps: make([]cronjob.WorkflowStep, len(d.Steps))}
	for i, s := range d.Steps {
		out.Steps[i] = cronjob.WorkflowStep{Key: s.Key, Kind: cronjob.StepKind(s.Kind), Command: s.Command, Prompt: s.Prompt, After: s.After, Retries: s.Retries, Options: decodeOptions(s.Options)}
	}
	return out
}

func decodeOptions(raw json.RawMessage) any {
	var options any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &options)
	}
	return options
}

// describeJSONError says what is wrong with text in words that point at the place: the line of a syntax error, the field of
// a value of the wrong type.
func describeJSONError(text string, err error) error {
	switch e := err.(type) {
	case *json.SyntaxError:
		line := 1 + strings.Count(text[:min(int(e.Offset), len(text))], "\n")
		return fmt.Errorf("Not valid JSON on line %d: %v.", line, e)
	case *json.UnmarshalTypeError:
		return fmt.Errorf("%s must be %s, not %s.", jsonPath(e.Field), jsonKind(e.Type), e.Value)
	}
	if errors.Is(err, io.EOF) {
		return errors.New("Not valid JSON: the document is empty.")
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("Not valid JSON: the document ends early; a bracket or quote is missing.")
	}
	if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return fmt.Errorf("Unknown field %s; kata takes only name, version, about, input, options and steps (key, kind, command, prompt, after, retries, options).", field)
	}
	return fmt.Errorf("Not valid JSON: %v.", err)
}

func jsonPath(field string) string {
	if field == "" {
		return "The document"
	}
	return `"` + field + `"`
}

func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Int, reflect.Int64, reflect.Int32:
		return "a whole number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Struct, reflect.Map:
		return "an object"
	}
	return t.String()
}
