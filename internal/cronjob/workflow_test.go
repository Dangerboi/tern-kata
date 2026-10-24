package cronjob_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func retried(n int64) cronjob.WorkflowStep {
	s := command("a")
	s.Retries = new(n)
	return s
}

func command(key string) cronjob.WorkflowStep {
	return cronjob.WorkflowStep{Key: key, Kind: cronjob.StepCommand, Command: "c"}
}

func definition(steps ...cronjob.WorkflowStep) cronjob.WorkflowDefinition {
	return cronjob.WorkflowDefinition{Version: 1, Steps: steps}
}

func with(step cronjob.WorkflowStep, edit func(*cronjob.WorkflowStep)) cronjob.WorkflowStep {
	edit(&step)
	return step
}

func TestValidateWorkflowNamesTheFirstRuleBrokenInKatasOrder(t *testing.T) {
	longKey := strings.Repeat("k", cronjob.MaxPortableNameBytes+1)
	for name, tc := range map[string]struct {
		def     cronjob.WorkflowDefinition
		rule    cronjob.WorkflowRule
		step    int
		message string
	}{
		"wrong version": {cronjob.WorkflowDefinition{Version: 2, Steps: []cronjob.WorkflowStep{command("a")}}, cronjob.RuleVersion, 0, `"version" must be 1.`},
		"no steps":      {cronjob.WorkflowDefinition{Version: 1}, cronjob.RuleNoSteps, 0, `"steps" needs at least one step.`},
		"empty key":     {definition(command("")), cronjob.RuleKey, 1, `Step 1: "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"space in key":  {definition(command("a b")), cronjob.RuleKey, 1, `Step "a b": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"dot key":       {definition(command(".")), cronjob.RuleKey, 1, `Step ".": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"dotdot key":    {definition(command("..")), cronjob.RuleKey, 1, `Step "..": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"slash in key":  {definition(command("a/b")), cronjob.RuleKey, 1, `Step "a/b": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"key too long":  {definition(command(longKey)), cronjob.RuleKey, 1, `Step "` + longKey + `": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"duplicate key": {definition(command("a"), command("a")), cronjob.RuleDuplicateKey, 2, `Step "a": "key" is used by an earlier step; keys must be unique.`},
		"retries over the bound": {definition(retried(cronjob.MaxWorkflowRetries + 1)),
			cronjob.RuleRetries, 1, `Step "a": "retries" must be between 0 and 100.`},
		"negative retries": {definition(retried(-1)),
			cronjob.RuleRetries, 1, `Step "a": "retries" must be between 0 and 100.`},
		"forward after": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.After = []string{"b"} }), command("b")),
			cronjob.RuleAfter, 1, `Step "a": "after" names "b", which is not an earlier step; list steps in the order they run.`},
		"self after": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.After = []string{"a"} })),
			cronjob.RuleAfter, 1, `Step "a": "after" names "a", which is not an earlier step; list steps in the order they run.`},
		"missing after": {definition(command("a"), with(command("b"), func(s *cronjob.WorkflowStep) { s.After = []string{"a", "zzz"} })),
			cronjob.RuleAfter, 2, `Step "b": "after" names "zzz", which is not an earlier step; list steps in the order they run.`},
		"command and prompt": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.Prompt = "p" })),
			cronjob.RuleShape, 1, `Step "a": a command step needs "command" and no "prompt".`},
		"command step without command": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.Command = "  " })),
			cronjob.RuleShape, 1, `Step "a": a command step needs "command" and no "prompt".`},
		"prompt and command": {definition(cronjob.WorkflowStep{Key: "a", Kind: cronjob.StepPrompt, Command: "c", Prompt: "p"}),
			cronjob.RuleShape, 1, `Step "a": a prompt step needs "prompt" and no "command".`},
		"prompt step without prompt": {definition(cronjob.WorkflowStep{Key: "a", Kind: cronjob.StepPrompt}),
			cronjob.RuleShape, 1, `Step "a": a prompt step needs "prompt" and no "command".`},
		"unknown kind": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.Kind = "shell" })),
			cronjob.RuleKind, 1, `Step "a": "kind" must be "command" or "prompt".`},
		"credential deep in step options": {definition(with(command("a"), func(s *cronjob.WorkflowStep) {
			s.Options = map[string]any{"deep": map[string]any{"list": []any{map[string]any{"Token": "t"}}}}
		})), cronjob.RuleOptions, 1, `Step "a": "options" cannot contain credentials or local paths.`},
		"local path in step options": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.Options = map[string]any{"cwd": "/tmp"} })),
			cronjob.RuleOptions, 1, `Step "a": "options" cannot contain credentials or local paths.`},
		"step options not an object": {definition(with(command("a"), func(s *cronjob.WorkflowStep) { s.Options = "fast" })),
			cronjob.RuleOptions, 1, `Step "a": "options" must be an object.`},
		"credential in workflow options": {cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{command("a")}, Options: map[string]any{"a": map[string]any{"api_key": "k"}}},
			cronjob.RuleOptions, 0, `"options" cannot contain credentials or local paths.`},
	} {
		t.Run(name, func(t *testing.T) {
			got := cronjob.ValidateWorkflow(tc.def)
			if got == nil {
				t.Fatalf("no problem, want %s", tc.rule)
			}
			if got.Rule != tc.rule || got.Step != tc.step || got.Message() != tc.message || got.Error() != tc.message {
				t.Errorf("got %+v message %q, want rule %s step %d message %q", got, got.Message(), tc.rule, tc.step, tc.message)
			}
		})
	}
}

func TestValidateWorkflowTakesWhatKataTakes(t *testing.T) {
	for name, def := range map[string]cronjob.WorkflowDefinition{
		"retries absent":       definition(command("a")),
		"retries zero":         definition(retried(0)),
		"retries at the bound": definition(retried(cronjob.MaxWorkflowRetries)),
		"key at the bound":     definition(command(strings.Repeat("k", cronjob.MaxPortableNameBytes))),
		"portable keys":        definition(command("build-1"), command("test_2"), command("v1.0"), command("Ünï")),
		"after earlier steps": definition(command("a"), command("b"),
			with(command("c"), func(s *cronjob.WorkflowStep) { s.After = []string{"a", "b"} })),
		"prompt step": definition(cronjob.WorkflowStep{Key: "a", Kind: cronjob.StepPrompt, Prompt: "p", Options: map[string]any{"profile": "fast"}}),
		"unknown option keys": {Version: 1, Steps: []cronjob.WorkflowStep{command("a")}, Options: map[string]any{"mystery": 1.0}},
		"typed nil options":   {Version: 1, Steps: []cronjob.WorkflowStep{command("a")}, Options: map[string]any(nil)},
	} {
		t.Run(name, func(t *testing.T) {
			if got := cronjob.ValidateWorkflow(def); got != nil {
				t.Errorf("problem %q", got.Message())
			}
		})
	}
}

func TestValidateWorkflowReportsTheFirstProblemOnly(t *testing.T) {
	def := cronjob.WorkflowDefinition{Version: 1, Options: map[string]any{"token": "t"}, Steps: []cronjob.WorkflowStep{
		command("a b"),
		retried(500),
		command("a"),
	}}
	if got := cronjob.ValidateWorkflow(def); got == nil || got.Rule != cronjob.RuleKey || got.Step != 1 {
		t.Fatalf("got %+v, want the first step's key", got)
	}
	def.Steps[0].Key = "a"
	if got := cronjob.ValidateWorkflow(def); got == nil || got.Rule != cronjob.RuleDuplicateKey || got.Step != 2 {
		t.Fatalf("got %+v, want step 2's duplicate key before its retries", got)
	}
	def.Steps[1].Key = "b"
	if got := cronjob.ValidateWorkflow(def); got == nil || got.Rule != cronjob.RuleRetries || got.Step != 2 {
		t.Fatalf("got %+v, want step 2's retries", got)
	}
	def.Steps[1].Retries = nil
	def.Steps[2].Key = "c"
	if got := cronjob.ValidateWorkflow(def); got == nil || got.Rule != cronjob.RuleOptions || got.Step != 0 {
		t.Fatalf("got %+v, want the Workflow's options last", got)
	}
}

func TestRetryCountReadsAnAbsentValueAsZero(t *testing.T) {
	if got := (cronjob.WorkflowStep{}).RetryCount(); got != 0 {
		t.Errorf("absent retries counted %d", got)
	}
	if got := (cronjob.WorkflowStep{Retries: new(int64(3))}).RetryCount(); got != 3 {
		t.Errorf("retries 3 counted %d", got)
	}
}
