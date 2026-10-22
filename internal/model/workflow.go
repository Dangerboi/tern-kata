package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
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

// Workflow step kinds.
const (
	StepCommand = "command"
	StepPrompt  = "prompt"
)

// Kata's limits on a workflow, mirrored for the local check.
const (
	maxWorkflowRetries    = 100
	maxWorkflowNameBytes  = 256
	maxWorkflowKeyBytes   = 256
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
	return WorkflowDocument{Name: strings.TrimSpace(file.Name), Definition: string(definition)}, nil
}

// check is kata's workflow rules in kata's order: the name, the version, then each step in turn.
func (f workflowFile) check() error {
	if name := strings.TrimSpace(f.Name); name == "" || len(name) > maxWorkflowNameBytes {
		return errors.New(`"name" must be 1 to 256 characters.`)
	}
	d := f.Definition
	if d.Version != 1 {
		return errors.New(`"version" must be 1.`)
	}
	if len(d.Steps) == 0 {
		return errors.New(`"steps" needs at least one step.`)
	}
	earlier := map[string]bool{}
	for i, step := range d.Steps {
		label := fmt.Sprintf("Step %d", i+1)
		if step.Key != "" {
			label = fmt.Sprintf("Step %q", step.Key)
		}
		switch {
		case !portableName(step.Key):
			return fmt.Errorf(`%s: "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`, label)
		case earlier[step.Key]:
			return fmt.Errorf(`%s: "key" is used by an earlier step; keys must be unique.`, label)
		}
		if step.Retries != nil && (*step.Retries < 0 || *step.Retries > maxWorkflowRetries) {
			return fmt.Errorf(`%s: "retries" must be between 0 and 100.`, label)
		}
		for _, after := range step.After {
			if !earlier[after] {
				return fmt.Errorf(`%s: "after" names %q, which is not an earlier step; list steps in the order they run.`, label, after)
			}
		}
		switch step.Kind {
		case StepCommand:
			if strings.TrimSpace(step.Command) == "" || step.Prompt != "" {
				return fmt.Errorf(`%s: a command step needs "command" and no "prompt".`, label)
			}
		case StepPrompt:
			if strings.TrimSpace(step.Prompt) == "" || step.Command != "" {
				return fmt.Errorf(`%s: a prompt step needs "prompt" and no "command".`, label)
			}
		default:
			return fmt.Errorf(`%s: "kind" must be "command" or "prompt".`, label)
		}
		if err := checkOptions(step.Options); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		earlier[step.Key] = true
	}
	return checkOptions(d.Options)
}

// portableName is kata's rule for a step key: 1 to 256 bytes of letters, digits and "-", "_", ".", not "." or "..".
func portableName(s string) bool {
	if s == "" || len(s) > maxWorkflowKeyBytes || s == "." || s == ".." || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_.", r) {
			return false
		}
	}
	return true
}

// checkOptions is kata's rule for adapter options: an object that carries no credential or local path under any key.
func checkOptions(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New(`"options" must be an object.`)
	}
	var walk func(any) bool
	walk = func(value any) bool {
		switch v := value.(type) {
		case map[string]any:
			for key, entry := range v {
				switch strings.ToLower(key) {
				case "token", "api_token", "api_key", "password", "secret", "credentials", "authorization", "cwd", "path", "checkout_path", "local_path":
					return false
				}
				if !walk(entry) {
					return false
				}
			}
		case []any:
			for _, entry := range v {
				if !walk(entry) {
					return false
				}
			}
		}
		return true
	}
	if !walk(object) {
		return errors.New(`"options" cannot contain credentials or local paths.`)
	}
	return nil
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
