package cronjob

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StepKind is how kata names what a Workflow step runs. A kind outside the constants is held as it came and fails the
// validator's kind rule.
type StepKind string

// The step kinds kata knows.
const (
	StepCommand StepKind = "command"
	StepPrompt  StepKind = "prompt"
)

// Kata's limits on a Workflow definition and its names.
const (
	// MaxWorkflowRetries is the most `retries` a step may ask for.
	MaxWorkflowRetries = 100
	// MaxPortableNameBytes is the longest a portable name may be: a step key, a checkout key or a secret name.
	MaxPortableNameBytes = 256
)

// WorkflowDefinition is a kata Workflow definition decoded: what the Workflow card's JSON text and kata's client Workflow
// both become before ValidateWorkflow reads them. The Workflow's name and the size of its text are not part of it.
type WorkflowDefinition struct {
	Version int64
	About   string
	Input   string
	// Options is the adapter's options as decoded JSON: nil when absent, a map[string]any when an object, anything else
	// when kata was handed a value that is not an object.
	Options any
	Steps   []WorkflowStep
}

// WorkflowStep is one step of a definition. A command step carries Command, a prompt step Prompt; After names earlier steps.
type WorkflowStep struct {
	Key     string
	Kind    StepKind
	Command string
	Prompt  string
	After   []string
	// Retries is nil when the definition leaves it out, which is no retries.
	Retries *int64
	Options any
}

// RetryCount is how many times the step runs again after a failure: Retries, or zero when it is left out.
func (s WorkflowStep) RetryCount() int64 {
	if s.Retries == nil {
		return 0
	}
	return *s.Retries
}

// WorkflowRule names the rule of kata's Workflow format a definition breaks.
type WorkflowRule string

// The rules, in the order ValidateWorkflow checks them.
const (
	RuleVersion      WorkflowRule = "version"
	RuleNoSteps      WorkflowRule = "no-steps"
	RuleKey          WorkflowRule = "key"
	RuleDuplicateKey WorkflowRule = "duplicate-key"
	RuleRetries      WorkflowRule = "retries"
	RuleAfter        WorkflowRule = "after"
	RuleKind         WorkflowRule = "kind"
	RuleShape        WorkflowRule = "shape"
	RuleOptions      WorkflowRule = "options"
)

// WorkflowProblem is the first rule a definition breaks. Step is the 1-based position of the step it concerns, 0 for a
// problem of the Workflow itself; Key is that step's key as written, possibly empty.
type WorkflowProblem struct {
	Rule   WorkflowRule
	Step   int
	Key    string
	Detail string
}

// Message is the canonical wording of the problem, the one the Workflow card shows: a step's problem starts with which step.
func (p WorkflowProblem) Message() string {
	switch {
	case p.Step == 0:
		return p.Detail
	case p.Key != "":
		return fmt.Sprintf("Step %q: %s", p.Key, p.Detail)
	}
	return fmt.Sprintf("Step %d: %s", p.Step, p.Detail)
}

// Error makes a problem usable as an error; it is Message.
func (p *WorkflowProblem) Error() string { return p.Message() }

// ValidateWorkflow checks a definition against kata's rules in kata's order: the version, at least one step, then each step
// in turn (portable key, unique key, retries bound, `after` naming an earlier step, kind and shape, options), then the
// Workflow's own options. It returns the first problem, or nil when kata would take the definition.
func ValidateWorkflow(d WorkflowDefinition) *WorkflowProblem {
	if d.Version != 1 {
		return &WorkflowProblem{Rule: RuleVersion, Detail: `"version" must be 1.`}
	}
	if len(d.Steps) == 0 {
		return &WorkflowProblem{Rule: RuleNoSteps, Detail: `"steps" needs at least one step.`}
	}
	earlier := map[string]bool{}
	for i, step := range d.Steps {
		problem := func(rule WorkflowRule, detail string, args ...any) *WorkflowProblem {
			return &WorkflowProblem{Rule: rule, Step: i + 1, Key: step.Key, Detail: fmt.Sprintf(detail, args...)}
		}
		switch {
		case !PortableName(step.Key):
			return problem(RuleKey, `"key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most %d bytes.`, MaxPortableNameBytes)
		case earlier[step.Key]:
			return problem(RuleDuplicateKey, `"key" is used by an earlier step; keys must be unique.`)
		}
		if step.Retries != nil && (*step.Retries < 0 || *step.Retries > MaxWorkflowRetries) {
			return problem(RuleRetries, `"retries" must be between 0 and %d.`, MaxWorkflowRetries)
		}
		for _, after := range step.After {
			if !earlier[after] {
				return problem(RuleAfter, `"after" names %q, which is not an earlier step; list steps in the order they run.`, after)
			}
		}
		switch step.Kind {
		case StepCommand:
			if strings.TrimSpace(step.Command) == "" || step.Prompt != "" {
				return problem(RuleShape, `a command step needs "command" and no "prompt".`)
			}
		case StepPrompt:
			if strings.TrimSpace(step.Prompt) == "" || step.Command != "" {
				return problem(RuleShape, `a prompt step needs "prompt" and no "command".`)
			}
		default:
			return problem(RuleKind, `"kind" must be "command" or "prompt".`)
		}
		if detail := optionsProblem(step.Options); detail != "" {
			return problem(RuleOptions, "%s", detail)
		}
		earlier[step.Key] = true
	}
	if detail := optionsProblem(d.Options); detail != "" {
		return &WorkflowProblem{Rule: RuleOptions, Detail: detail}
	}
	return nil
}

// PortableName is kata's rule for a step key, and the Pane's for a checkout key or secret name: 1 to MaxPortableNameBytes bytes of letters, digits and "-", "_", ".", not "." or "..".
func PortableName(s string) bool {
	if s == "" || len(s) > MaxPortableNameBytes || s == "." || s == ".." || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_.", r) {
			return false
		}
	}
	return true
}

// optionsProblem is kata's rule for adapter options: an object that carries no credential or local path under any key.
// It returns the problem's wording, empty when the options are fine.
func optionsProblem(options any) string {
	if options == nil {
		return ""
	}
	object, ok := options.(map[string]any)
	if !ok {
		return `"options" must be an object.`
	}
	if forbiddenOption(object) {
		return `"options" cannot contain credentials or local paths.`
	}
	return ""
}

// forbiddenOption reports a credential or local-path key anywhere under value.
func forbiddenOption(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, entry := range v {
			switch strings.ToLower(key) {
			case "token", "api_token", "api_key", "password", "secret", "credentials", "authorization", "cwd", "path", "checkout_path", "local_path":
				return true
			}
			if forbiddenOption(entry) {
				return true
			}
		}
	case []any:
		for _, entry := range v {
			if forbiddenOption(entry) {
				return true
			}
		}
	}
	return false
}
