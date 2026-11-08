package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// Step states, as the journal records them. A step nobody has started is pending (the empty string).
const (
	StepPending   = "pending"
	StepRunning   = "running"
	StepSucceeded = "succeeded"
	StepFailed    = "failed"
	StepSkipped   = "skipped"
)

// WorkflowPlan is the workflow a run executes, frozen when the run was prepared: a later edit of the workflow never
// changes a prepared run (ADR-0008).
type WorkflowPlan struct {
	UID      string     `json:"uid"`
	EventUID string     `json:"event_uid"`
	Name     string     `json:"name"`
	About    string     `json:"about,omitempty"`
	Steps    []PlanStep `json:"steps"`
}

type PlanStep struct {
	Key         string           `json:"key"`
	Kind        cronjob.StepKind `json:"kind"`
	Command     string           `json:"command,omitempty"`
	Prompt      string           `json:"prompt,omitempty"`
	After       []string         `json:"after,omitempty"`
	Retries     int              `json:"retries,omitempty"`
	ProfileName string           `json:"profile_name,omitempty"`
	Profile     Profile          `json:"profile,omitempty"`
}

// StepRecord is what the journal knows of one step. Launch and Exit are the start record and receipt of the step's
// last attempt once it ended; while an attempt runs they live in the run's own Launch and Exit (ADR-0006), which the
// recovery code reads, and move here when the step concludes.
type StepRecord struct {
	Key   string `json:"key"`
	State string `json:"state,omitempty"`
	// Tries counts the attempts started for this step, a retry after a certain failure being a new one.
	Tries int `json:"tries,omitempty"`
	// Dir is the attempt's folder under the run's, holding its prompt, session and output.
	Dir         string        `json:"dir,omitempty"`
	Summary     string        `json:"summary,omitempty"`
	LastFailure string        `json:"last_failure,omitempty"`
	StartedAt   time.Time     `json:"started_at,omitzero"`
	EndedAt     time.Time     `json:"ended_at,omitzero"`
	Launch      *LaunchRecord `json:"launch,omitempty"`
	Exit        *ExitRecord   `json:"exit,omitempty"`
	// StderrTail is the end of the last attempt's stderr, kept when the step concludes: stderr.log holds only its start.
	StderrTail   string   `json:"stderr_tail,omitempty"`
	InputTokens  *int64   `json:"input_tokens,omitempty"`
	OutputTokens *int64   `json:"output_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

func terminalStep(state string) bool {
	return state == StepSucceeded || state == StepFailed || state == StepSkipped
}

func (j *Journal) activeStep() int {
	if j.Workflow == nil {
		return -1
	}
	return slices.IndexFunc(j.Steps, func(s StepRecord) bool { return s.State == StepRunning })
}

// activeIsCommand reports whether the run's current attempt is a command step's, which has no session to continue.
func (j *Journal) activeIsCommand() bool {
	i := j.activeStep()
	return i >= 0 && stepKind(j, i) == cronjob.StepCommand
}

func (w *Worker) attemptDir(j *Journal) string {
	dir := w.store.RunDir(j.RunUID)
	if j.Workflow == nil {
		return dir
	}
	if i := j.activeStep(); i >= 0 && j.Steps[i].Dir != "" {
		return filepath.Join(dir, j.Steps[i].Dir)
	}
	for i := len(j.Steps) - 1; i >= 0; i-- {
		if j.Steps[i].Dir != "" {
			return filepath.Join(dir, j.Steps[i].Dir)
		}
	}
	return dir
}

// workflowRefusal is why the worker does not run a workflow, empty when it does. A worker must not trust a definition it
// did not validate, so kata's own rules come first, through the validator the Workflow card uses and in its canonical words;
// then CheckWorkflow's refusals, the first of them.
func workflowRefusal(wf Workflow, maxPrompts int) string {
	if problem := cronjob.ValidateWorkflow(wf.WorkflowDefinition); problem != nil {
		return problem.Message()
	}
	if check := CheckWorkflow(wf.WorkflowDefinition, &maxPrompts); len(check.Refusals) > 0 {
		return check.Refusals[0]
	}
	return ""
}

// WorkflowCheck is the worker's reading of a definition kata accepts: why it would not run it, in the order it decides, and
// whether the prompt budget could not be checked.
type WorkflowCheck struct {
	Refusals        []string
	BudgetUnchecked bool
}

// CheckWorkflow is ADR-0008's refusals of a definition, then the prompt budget: the herdr-kata extensions this design rejects
// (on_fail, on_failure "park", a profile on a command step), which are refused rather than ignored because ignoring one changes
// what the author asked for, and a count of prompts its prompt steps and their retries may send above maxPrompts. A nil
// maxPrompts is a limit nobody could read (no worker.toml): the budget is left unchecked and the check says so. The definition
// is taken to have passed cronjob.ValidateWorkflow. The planning of a run and the job problem report use this same function.
func CheckWorkflow(d cronjob.WorkflowDefinition, maxPrompts *int) WorkflowCheck {
	var check WorkflowCheck
	if why := unsupportedOption(d.Options); why != "" {
		check.Refusals = append(check.Refusals, why)
	}
	prompts := 0
	for _, s := range d.Steps {
		if why := unsupportedOption(s.Options); why != "" {
			check.Refusals = append(check.Refusals, fmt.Sprintf("step %q: %s", s.Key, why))
		}
		if _, ok := optionMap(s.Options)["profile"]; ok && s.Kind == cronjob.StepCommand {
			check.Refusals = append(check.Refusals, fmt.Sprintf("command step %q names a profile, which only prompt steps use", s.Key))
		}
		if s.Kind == cronjob.StepPrompt {
			prompts += 1 + int(s.RetryCount())
		}
	}
	switch {
	case maxPrompts == nil:
		check.BudgetUnchecked = true
	case prompts > *maxPrompts:
		check.Refusals = append(check.Refusals, fmt.Sprintf("prompt_budget %d > %d: its prompt steps and their retries may send more prompts than max_run_prompts allows", prompts, *maxPrompts))
	}
	return check
}

func optionMap(options any) map[string]any {
	object, _ := options.(map[string]any)
	return object
}

func unsupportedOption(options any) string {
	object := optionMap(options)
	if _, ok := object["on_fail"]; ok {
		return "options.on_fail (maker/checker loops) is not supported"
	}
	if v, ok := object["on_failure"].(string); ok && v == "park" {
		return `options.on_failure "park" is not supported: a failed step fails the run`
	}
	return ""
}

// planWorkflow reads the job's workflow and freezes it with each prompt step's profile resolved against worker.toml.
// An error is a refusal: the run is not prepared.
func (w *Worker) planWorkflow(ctx context.Context, ps *projectState, job cronjob.Job, jobProfile string) (*WorkflowPlan, error) {
	d := job.Definition
	wf, err := w.opt.Kata.Workflow(ctx, ps.project.ID, d.Action.WorkflowUID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, fmt.Errorf("job %q runs workflow %s, which kata does not hold", job.Name, d.Action.WorkflowUID)
	case err != nil:
		return nil, &transientError{fmt.Errorf("job %q: its workflow could not be read: %w", job.Name, err)}
	case wf.Deleted:
		return nil, fmt.Errorf("job %q runs workflow %q, which is archived", job.Name, wf.Name)
	}
	refuse := func(why string) error {
		return fmt.Errorf("job %q runs workflow %q, which the worker does not run: %s", job.Name, wf.Name, why)
	}
	if why, _ := workflowFit(w.opt.Config, ps.cfg, wf, jobProfile); why != "" {
		return nil, refuse(why)
	}
	plan := &WorkflowPlan{UID: wf.UID, EventUID: wf.DefinitionEventUID, Name: wf.Name, About: wf.About}
	for _, s := range wf.Steps {
		step := PlanStep{Key: s.Key, Kind: s.Kind, Command: s.Command, Prompt: s.Prompt, After: s.After, Retries: int(s.RetryCount())}
		if s.Kind == cronjob.StepPrompt {
			step.ProfileName = stepProfile(s, jobProfile)
			step.Profile = w.opt.Config.Profiles[step.ProfileName]
		}
		plan.Steps = append(plan.Steps, step)
	}
	return plan, nil
}

// workflowFit is why a workflow cannot run for a job with this profile in this Project, empty when it can: the
// refusals of workflowRefusal, a workflow profile the job does not run with, a command step in a Project that does not
// allow commands (upgrade: worker.toml can change that), a prompt step whose profile worker.toml does not map to the
// Project. The planning of a run and the job problem report both ask this, so they give the same reason.
func workflowFit(cfg Config, pc ProjectConfig, wf Workflow, jobProfile string) (why string, upgrade bool) {
	if why := workflowRefusal(wf, cfg.MaxRunPrompts()); why != "" {
		return why, false
	}
	if p := optionProfile(wf.Options); p != "" && p != jobProfile {
		return fmt.Sprintf("it names profile %q but the job runs with %q", p, jobProfile), false
	}
	for _, s := range wf.Steps {
		switch {
		case s.Kind == cronjob.StepCommand && !pc.Commands:
			return fmt.Sprintf("step %q is a command, and Project %s does not allow command steps in worker.toml (commands = true)", s.Key, pc.Name), true
		case s.Kind == cronjob.StepPrompt:
			name := stepProfile(s, jobProfile)
			if _, ok := cfg.Profiles[name]; !ok || !pc.allowsProfile(name) {
				return fmt.Sprintf("step %q names profile %q, which is not available to Project %s in worker.toml", s.Key, name, pc.Name), false
			}
		}
	}
	return "", false
}

func stepProfile(s cronjob.WorkflowStep, jobProfile string) string {
	return cmp.Or(optionProfile(s.Options), jobProfile)
}

func optionProfile(options any) string {
	p, _ := optionMap(options)["profile"].(string)
	return p
}

// stepsFor is the empty state of each step of a plan; a prompt job has no plan and no steps.
func stepsFor(plan *WorkflowPlan) []StepRecord {
	if plan == nil {
		return nil
	}
	recs := make([]StepRecord, len(plan.Steps))
	for i, s := range plan.Steps {
		recs[i].Key = s.Key
	}
	return recs
}

type attemptSpec struct {
	dir     string
	task    string
	profile Profile
	plan    *PlanStep
	index   int
	tries   int
	failure string
}

// workflowStep runs the run's next step to its end, or finishes the run when no step is left. It returns once the
// journal says so, then advance calls it again. Steps run one at a time in the order of the plan, which is already
// dependency order (ADR-0008).
func (w *Worker) workflowStep(ctx context.Context, j *Journal) error {
	j, err := w.skipBlocked(j)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(j.Steps, func(s StepRecord) bool { return !terminalStep(s.State) })
	if i < 0 {
		return w.record(j.RunUID, "finished", (*Journal).finishWorkflow)
	}
	rec := j.Steps[i]
	// A step still saved running here has no launch record (a launched one is held at recovery), so it is checked like a
	// pending one; a resumed step carries a fresh expiry.
	if !j.ExpiresAt.IsZero() && !w.clock.Now().Before(j.ExpiresAt) {
		return w.record(j.RunUID, "finished", (*Journal).timeOutWorkflow)
	}
	if rec.State != StepRunning {
		j, err = w.store.Update(j.RunUID, "step-started", func(j *Journal) error {
			r := &j.Steps[i]
			r.State, r.Tries, r.StderrTail = StepRunning, r.Tries+1, "" // the earlier attempt's tail is not this one's
			r.Dir = fmt.Sprintf("steps/%d-%d", i+1, r.Tries)
			if r.StartedAt.IsZero() {
				r.StartedAt = w.clock.Now().UTC()
			}
			return nil
		})
		if err != nil {
			return err
		}
		rec = j.Steps[i]
	}
	plan := j.Workflow.Steps[i]
	sp := attemptSpec{dir: filepath.Join(w.store.RunDir(j.RunUID), rec.Dir), plan: &plan, index: i, tries: rec.Tries, failure: rec.LastFailure}
	if err := os.MkdirAll(sp.dir, 0o700); err != nil {
		return w.hold(j, HoldExecutionError, err.Error())
	}
	if plan.Kind == cronjob.StepCommand {
		sp.task = plan.Command
		return w.runCommand(ctx, j, sp)
	}
	sp.task, sp.profile = plan.Prompt, plan.Profile
	return w.runPrompt(ctx, j, sp)
}

// skipBlocked marks every pending step that depends on a failed or skipped step as skipped: it never runs, while
// steps that do not depend on the failure still do (ADR-0008).
func (w *Worker) skipBlocked(j *Journal) (*Journal, error) {
	blocked := func(j *Journal) []int {
		var out []int
		state := map[string]string{}
		for i, r := range j.Steps {
			state[r.Key] = r.State
			if r.State != "" && r.State != StepPending {
				continue
			}
			for _, dep := range j.Workflow.Steps[i].After {
				if s := state[dep]; s == StepFailed || s == StepSkipped {
					out = append(out, i)
					state[r.Key] = StepSkipped
					break
				}
			}
		}
		return out
	}
	if len(blocked(j)) == 0 {
		return j, nil
	}
	return w.store.Update(j.RunUID, "steps-skipped", func(j *Journal) error {
		for _, i := range blocked(j) {
			j.Steps[i].State, j.Steps[i].Summary = StepSkipped, "skipped: a step it runs after did not succeed"
		}
		return nil
	})
}

func sumStepTokens(steps []StepRecord) (in, out *int64) {
	var sumIn, sumOut int64
	var saw bool
	for _, r := range steps {
		if r.InputTokens == nil && r.OutputTokens == nil {
			continue
		}
		saw = true
		if r.InputTokens != nil {
			sumIn += *r.InputTokens
		}
		if r.OutputTokens != nil {
			sumOut += *r.OutputTokens
		}
	}
	if !saw {
		return nil, nil
	}
	return &sumIn, &sumOut
}

// workflowSummary is the run's summary in kata: the workflow's result, then one entry per step with what it ended as
// and the first of what it said (a failed step gets more room, since that is what the reader needs).
func workflowSummary(j *Journal, status cronjob.RunStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workflow %q %s", j.Workflow.Name, status)
	done := 0
	for _, r := range j.Steps {
		if r.State == StepSucceeded {
			done++
		}
	}
	fmt.Fprintf(&b, " (%d of %d steps succeeded)\n", done, len(j.Steps))
	for i, r := range j.Steps {
		state := cmp.Or(r.State, StepPending)
		fmt.Fprintf(&b, "\n%d. %s [%s]: %s", i+1, r.Key, stepKind(j, i), state)
		if r.Tries > 1 {
			fmt.Fprintf(&b, " after %d attempts", r.Tries)
		}
		limit := 200
		if r.State == StepFailed {
			limit = 1500
		}
		if s := strings.TrimSpace(r.Summary); s != "" && r.State != StepSkipped {
			text := truncate(s, limit)
			if r.State != StepFailed {
				text, _, _ = strings.Cut(text, "\n")
			}
			b.WriteString("\n   " + strings.ReplaceAll(text, "\n", "\n   "))
		} else if r.State == StepSkipped {
			b.WriteString(" (a step it runs after did not succeed)")
			if strings.Contains(r.Summary, "timed out") {
				b.WriteString(" - the run timed out")
			}
		}
	}
	return b.String()
}

func stepStateText(j *Journal, i int) string {
	r := j.Steps[i]
	if r.State != StepRunning {
		return cmp.Or(r.State, StepPending)
	}
	switch {
	case j.held():
		return j.stateName()
	case j.Outcome != nil && j.Outcome.Status == cronjob.RunCancelled:
		return "cancelled"
	}
	return StepRunning
}

// commandEnv is the environment of a command step (ADR-0008): the worker's, without its kata credentials and routing
// selectors or the worker's own internals, plus the job's secrets and the run's identity.
// Names compare case-insensitively when caseInsensitive (Windows).
func commandEnv(environ []string, j *Journal, runDir string, secrets map[string]string, caseInsensitive bool) []string {
	var env []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if reservedEnvName(name, caseInsensitive) {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range secrets {
		env = append(env, k+"="+v)
	}
	return append(env, "TERN_KATA_RUN="+j.RunUID, "TERN_KATA_RUN_DIR="+runDir, "KATA_PROJECT="+j.ProjectName)
}

// runCommand runs one command step through `sh -c` in the checkout, as the worker's user, in its own process group.
// The start record is saved after the process starts and the receipt (its status) right after it ends, before anything
// else reads either (ADR-0006, ADR-0008). A missing receipt, an interruption or a descendant that outlives the shell
// holds the run: the worker never guesses what a command did.
func (w *Worker) runCommand(ctx context.Context, j *Journal, sp attemptSpec) error {
	secrets, err := w.secrets(j)
	if err != nil {
		return w.hold(j, HoldSecretUnavailable, err.Error())
	}
	runDir := w.store.RunDir(j.RunUID)
	cwd := cmp.Or(j.Checkout, runDir)
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return w.failAttempt(j, fmt.Sprintf("cannot start the command: checkout %s is not a directory", cwd))
	}
	program, err := exec.LookPath("sh")
	if err != nil {
		return w.failAttempt(j, "cannot start the command: "+err.Error())
	}
	out := openHeadTail(w.store.hooks, filepath.Join(sp.dir, "output.log"), commandHead, commandTail)
	cmd := exec.Command(program, "-c", sp.task)
	cmd.Dir = cwd
	cmd.Env = commandEnv(os.Environ(), j, runDir, secrets, envCaseInsensitive)
	cmd.Stdin = nil
	cmd.WaitDelay = 5 * time.Second
	isolate(cmd)
	cmd.Stdout, cmd.Stderr = out, out
	exit, err := w.supervise(ctx, j, cmd, "", "")
	out.Close()
	if se, ok := errors.AsType[startError](err); ok {
		return w.failAttempt(j, "cannot start the command: "+se.Error())
	}
	if err != nil {
		return err
	}
	exit.Unrecorded = out.Loss()
	if _, err := w.store.Update(j.RunUID, "exited", func(j *Journal) error {
		return j.markExited(exit, nil)
	}); err != nil {
		return err
	}
	w.logUnrecorded(j, exit)
	j, err = w.store.Load(j.RunUID)
	if err != nil {
		return err
	}
	return w.conclude(ctx, j)
}

func classifyCommand(j *Journal, tail string) *Outcome {
	e := j.Exit
	var reason string
	switch {
	case e.TimedOut:
		reason = fmt.Sprintf("timed out after %s", time.Duration(j.TimeoutSeconds)*time.Second)
	case e.Signal != "":
		reason = "the command was ended by " + e.Signal
	case e.Code != 0:
		reason = fmt.Sprintf("the command exited with status %d", e.Code)
	}
	o := &Outcome{}
	if reason == "" {
		o.Status, o.Summary = cronjob.RunSucceeded, "the command exited with status 0"
	} else {
		o.Status, o.Summary = cronjob.RunFailed, reason
	}
	if tail != "" {
		o.Summary += "\n\n" + tail
	}
	o.Summary = truncate(o.Summary, maxSummaryBytes)
	return o
}

// workflowExpiry is when a workflow run prepared at now expires: its frozen timeout from preparation. A prompt job has none here.
func workflowExpiry(plan *WorkflowPlan, now time.Time, timeout time.Duration) time.Time {
	if plan == nil {
		return time.Time{}
	}
	return now.Add(timeout)
}

func sumStepCost(steps []StepRecord) *float64 {
	var sum *float64
	for _, r := range steps {
		sum = addPtr(sum, r.CostUSD)
	}
	return sum
}
