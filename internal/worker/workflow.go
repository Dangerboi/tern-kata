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

// PlanStep is one step of the plan. A prompt step carries the profile its omp process is started with.
type PlanStep struct {
	Key         string   `json:"key"`
	Kind        string   `json:"kind"`
	Command     string   `json:"command,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	After       []string `json:"after,omitempty"`
	Retries     int      `json:"retries,omitempty"`
	ProfileName string   `json:"profile_name,omitempty"`
	Profile     Profile  `json:"profile,omitempty"`
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
	Dir     string `json:"dir,omitempty"`
	Summary string `json:"summary,omitempty"`
	// LastFailure is why the previous attempt failed, kept for the retry's prompt.
	LastFailure  string        `json:"last_failure,omitempty"`
	StartedAt    time.Time     `json:"started_at,omitzero"`
	EndedAt      time.Time     `json:"ended_at,omitzero"`
	Launch       *LaunchRecord `json:"launch,omitempty"`
	Exit         *ExitRecord   `json:"exit,omitempty"`
	InputTokens  *int64        `json:"input_tokens,omitempty"`
	OutputTokens *int64        `json:"output_tokens,omitempty"`
}

func terminalStep(state string) bool {
	return state == StepSucceeded || state == StepFailed || state == StepSkipped
}

// activeStep is the index of the step an attempt is running or held on, or -1.
func (j *Journal) activeStep() int {
	if j.Workflow == nil {
		return -1
	}
	return slices.IndexFunc(j.Steps, func(s StepRecord) bool { return s.State == StepRunning })
}

// activeIsCommand reports whether the run's current attempt is a command step's, which has no session to continue.
func (j *Journal) activeIsCommand() bool {
	i := j.activeStep()
	return i >= 0 && j.Workflow.Steps[i].Kind == "command"
}

// attemptDir is the folder of the run's current (or last) attempt: the run's own for a prompt job, the step's for a workflow.
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

// workflowRefusal is why the worker does not run a workflow, empty when it does (ADR-0008). Order, retries and the
// shape of a step are kata's own rules, checked again here because a worker must not trust a definition it did not
// validate; the rest are the herdr-kata extensions this design rejects, which are refused rather than ignored because
// ignoring one changes what the author asked for.
func workflowRefusal(wf Workflow, maxPrompts int) string {
	if wf.Version != 1 {
		return fmt.Sprintf("it is workflow version %d, which this worker does not know", wf.Version)
	}
	if len(wf.Steps) == 0 {
		return "it has no steps"
	}
	if why := unsupportedOption(wf.Options); why != "" {
		return why
	}
	seen := map[string]bool{}
	prompts := 0
	for _, s := range wf.Steps {
		switch {
		case seen[s.Key]:
			return fmt.Sprintf("step key %q is used twice", s.Key)
		case s.Retries < 0 || s.Retries > 100:
			return fmt.Sprintf("step %q has retries %d, outside 0 to 100", s.Key, s.Retries)
		case s.Kind == "command" && (strings.TrimSpace(s.Command) == "" || s.Prompt != ""), s.Kind == "prompt" && (strings.TrimSpace(s.Prompt) == "" || s.Command != ""):
			return fmt.Sprintf("step %q must be a %s step with only its %s", s.Key, s.Kind, s.Kind)
		case s.Kind != "command" && s.Kind != "prompt":
			return fmt.Sprintf("step %q has kind %q, neither prompt nor command", s.Key, s.Kind)
		}
		for _, dep := range s.After {
			if !seen[dep] {
				return fmt.Sprintf("step %q runs after %q, which is not an earlier step", s.Key, dep)
			}
		}
		if why := unsupportedOption(s.Options); why != "" {
			return fmt.Sprintf("step %q: %s", s.Key, why)
		}
		if _, ok := s.Options["profile"]; ok && s.Kind == "command" {
			return fmt.Sprintf("command step %q names a profile, which only prompt steps use", s.Key)
		}
		seen[s.Key] = true
		if s.Kind == "prompt" {
			prompts += 1 + int(s.Retries)
		}
	}
	if prompts > maxPrompts {
		return fmt.Sprintf("prompt_budget %d > %d: its prompt steps and their retries may send more prompts than max_run_prompts allows", prompts, maxPrompts)
	}
	return ""
}

// unsupportedOption names the options of a workflow or step that the worker refuses to run.
func unsupportedOption(options map[string]any) string {
	if _, ok := options["on_fail"]; ok {
		return "options.on_fail (maker/checker loops) is not supported"
	}
	if v, ok := options["on_failure"].(string); ok && v == "park" {
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
		return nil, fmt.Errorf("job %q: its workflow could not be read: %w", job.Name, err)
	case wf.Deleted:
		return nil, fmt.Errorf("job %q runs workflow %q, which is archived", job.Name, wf.Name)
	}
	refuse := func(why string) error {
		return fmt.Errorf("job %q runs workflow %q, which the worker does not run: %s", job.Name, wf.Name, why)
	}
	if why := workflowRefusal(wf, w.opt.Config.maxRunPrompts()); why != "" {
		return nil, refuse(why)
	}
	if p, ok := wf.Options["profile"].(string); ok && p != "" && p != jobProfile {
		return nil, refuse(fmt.Sprintf("it names profile %q but the job runs with %q", p, jobProfile))
	}
	plan := &WorkflowPlan{UID: wf.UID, EventUID: wf.DefinitionEventUID, Name: wf.Name, About: wf.About}
	for _, s := range wf.Steps {
		step := PlanStep{Key: s.Key, Kind: s.Kind, Command: s.Command, Prompt: s.Prompt, After: s.After, Retries: int(s.Retries)}
		if s.Kind == "prompt" {
			step.ProfileName = jobProfile
			if p, ok := s.Options["profile"].(string); ok && p != "" {
				step.ProfileName = p
			}
			profile, ok := w.opt.Config.Profiles[step.ProfileName]
			if !ok || !ps.cfg.allowsProfile(step.ProfileName) {
				return nil, refuse(fmt.Sprintf("step %q names profile %q, which is not available to Project %s in worker.toml", s.Key, step.ProfileName, ps.cfg.Name))
			}
			step.Profile = profile
		}
		plan.Steps = append(plan.Steps, step)
	}
	return plan, nil
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

// attemptSpec is what one process attempt runs: where its files go, what it is told, and which profile starts it.
type attemptSpec struct {
	dir     string
	task    string
	profile Profile
	// plan, index and tries are set for a workflow step: the step, its position and the attempt count of the step.
	plan  *PlanStep
	index int
	tries int
	// failure is why the step's previous attempt failed, for a retry's prompt.
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
		return w.finishWorkflow(j)
	}
	rec := j.Steps[i]
	// A step still saved running here has no launch record (a launched one is held at recovery), so it is checked like a
	// pending one; a resumed step carries a fresh expiry.
	if !j.ExpiresAt.IsZero() && !w.clock.Now().Before(j.ExpiresAt) {
		return w.timeOutWorkflow(j)
	}
	if rec.State != StepRunning {
		j, err = w.store.Update(j.RunUID, "step-started", func(j *Journal) error {
			r := &j.Steps[i]
			r.State, r.Tries = StepRunning, r.Tries+1
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
	if plan.Kind == "command" {
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

// concludeStep records how the current step's attempt ended and what comes next: the next step, a retry after a certain
// failure, or the end of the run. The attempt's start record and receipt move into the step's record.
func (w *Worker) concludeStep(j *Journal, outcome *Outcome) error {
	_, err := w.store.Update(j.RunUID, "step-concluded", func(j *Journal) error {
		return j.concludeStep(outcome, w.clock.Now().UTC())
	})
	return err
}

// timeOutWorkflow ends a run whose expiry passed before its next step started: every step that never ran is skipped
// and the run fails as timed out, with no process started.
func (w *Worker) timeOutWorkflow(j *Journal) error {
	_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
		return j.timeOutWorkflow(w.clock.Now().UTC())
	})
	return err
}

// sumStepTokens is the tokens every attempt of every step used; nil when no attempt reported any.
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

// finishWorkflow decides the run's outcome once every step is succeeded, failed or skipped: failed when any step failed.
func (w *Worker) finishWorkflow(j *Journal) error {
	_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
		return j.finishWorkflow(w.clock.Now().UTC())
	})
	return err
}

// workflowSummary is the run's summary in kata: the workflow's result, then one entry per step with what it ended as
// and the first of what it said (a failed step gets more room, since that is what the reader needs).
func workflowSummary(j *Journal, status string) string {
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
		fmt.Fprintf(&b, "\n%d. %s [%s]: %s", i+1, r.Key, j.Workflow.Steps[i].Kind, state)
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

// stepStateText is a step's state for the run detail: the live state of the step an attempt is on.
func stepStateText(j *Journal, i int) string {
	r := j.Steps[i]
	if r.State != StepRunning {
		return cmp.Or(r.State, StepPending)
	}
	switch {
	case j.held():
		return j.stateName()
	case j.Outcome != nil && j.Outcome.Status == StatusCancelled:
		return "cancelled"
	}
	return StepRunning
}

// commandEnv is the environment of a command step (ADR-0008): the worker's, without its kata credentials and routing
// selectors or the worker's own internals, plus the job's secrets and the run's identity.
func commandEnv(j *Journal, runDir string, secrets map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "TERN_KATA_") || strings.HasPrefix(kv, "KATA_") {
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
	if attempt := j.attempt(); attempt > 1 && j.Resume != nil {
		keepAttemptOutput(sp.dir, attempt-1)
	}
	out := openHeadTail(w.store.hooks, filepath.Join(sp.dir, "output.log"), commandHead, commandTail)
	cmd := exec.Command(program, "-c", sp.task)
	cmd.Dir = cwd
	cmd.Env = commandEnv(j, runDir, secrets)
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

// classifyCommand is the outcome of a command step that has a receipt: succeeded only on exit status 0.
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
		o.Status, o.Summary = StatusSucceeded, "the command exited with status 0"
	} else {
		o.Status, o.Summary = StatusFailed, reason
	}
	if tail != "" {
		o.Summary += "\n\n" + tail
	}
	o.Summary = truncate(o.Summary, maxSummaryBytes)
	return o
}

// Output limits of a command step: the first 1 MiB and the last 256 KiB are kept, with a marker between (ADR-0008).
const (
	commandHead = 1 << 20
	commandTail = 256 << 10
)

// headTail drains a command's output to a capture: the first head bytes as they come, then only the last tail bytes,
// written with a marker when it is closed.
type headTail struct {
	*capture
	head     int
	tailMax  int
	written  int
	tail     []byte
	overflow int64
	closed   bool
}

func openHeadTail(hooks Hooks, path string, head, tail int) *headTail {
	return &headTail{capture: openCapture(hooks, path), head: head, tailMax: tail}
}

func (h *headTail) Write(p []byte) (int, error) {
	n := len(p)
	if room := h.head - h.written; room > 0 {
		k := min(room, len(p))
		_, _ = h.capture.Write(p[:k])
		h.written += k
		p = p[k:]
	}
	if len(p) > 0 {
		h.overflow += int64(len(p))
		h.tail = append(h.tail, p...)
		if len(h.tail) > 2*h.tailMax {
			h.tail = append(h.tail[:0], h.tail[len(h.tail)-h.tailMax:]...)
		}
	}
	return n, nil
}

// Close writes the marker and the kept tail when output went past the head, then closes the file.
func (h *headTail) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	if h.overflow > 0 {
		tail := h.tail
		if len(tail) > h.tailMax {
			tail = tail[len(tail)-h.tailMax:]
		}
		if omitted := h.overflow - int64(len(tail)); omitted > 0 {
			_, _ = fmt.Fprintf(h.capture, "\n[... %d bytes omitted ...]\n", omitted)
		}
		_, _ = h.capture.Write(tail)
	}
	return h.capture.Close()
}

// commandTail1K is the end of a command step's output, kept in the step's result.
func commandTail1K(dir string) string {
	return fileTail(filepath.Join(dir, "output.log"), 1024)
}

// workflowExpiry is when a workflow run prepared at now expires: its frozen timeout from preparation. A prompt job has none here.
func workflowExpiry(plan *WorkflowPlan, now time.Time, timeout time.Duration) time.Time {
	if plan == nil {
		return time.Time{}
	}
	return now.Add(timeout)
}
