package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// What the worker says about the definitions it was given (ADR-0011): a job it sets aside is listed with its reason, and a
// per-run Issue job that creates many Issues a day is listed with a warning. Neither stops any other job, and the warning
// stops nothing.

// JobNote is a line of `worker status` and `worker doctor` about one job: Text is why it is set aside, or the warning.
type JobNote struct {
	UID  string
	Name string
	Text string
	// Upgrade is set when the job is set aside only for a secret its Project does not list or a command step its
	// Project does not allow, which `worker upgrade` can add.
	Upgrade bool
}

// definitionNotes reads the Project's adapter jobs against the worker's config: the jobs it sets aside with their
// reason and the warnings. A workflow a job runs is read from kata, so a failure to read it is itself the reason.
func definitionNotes(ctx context.Context, k *Kata, cfg Config, pc ProjectConfig, projectID int64, jobs []cronjob.Job, now time.Time) (aside, warnings []JobNote) {
	workflows := map[string]*Workflow{}
	workflowErrs := map[string]error{}
	for _, job := range jobs {
		d := job.Definition
		if job.Archived() || job.Native() || d.Action.Kind != cronjob.ActionExecute || d.Action.WorkflowUID == "" {
			continue
		}
		if _, done := workflows[d.Action.WorkflowUID]; done || workflowErrs[d.Action.WorkflowUID] != nil {
			continue
		}
		wf, err := k.Workflow(ctx, projectID, d.Action.WorkflowUID)
		if err != nil {
			workflowErrs[d.Action.WorkflowUID] = err
			continue
		}
		workflows[d.Action.WorkflowUID] = &wf
	}
	for _, job := range jobs {
		if job.Archived() || job.Native() {
			continue
		}
		note := JobNote{UID: job.UID, Name: job.Name}
		wf, wfErr := workflows[job.Definition.Action.WorkflowUID], workflowErrs[job.Definition.Action.WorkflowUID]
		if why, upgrade := jobProblem(cfg, pc, job, wf, wfErr); why != "" {
			note.Text, note.Upgrade = why, upgrade
			aside = append(aside, note)
			continue
		}
		if text := cronjob.HighRateWarning(job.IssuesPerDay(now)); text != "" {
			note.Text = text
			warnings = append(warnings, note)
		}
	}
	byName := func(a, b JobNote) int { return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.UID, b.UID)) }
	slices.SortFunc(aside, byName)
	slices.SortFunc(warnings, byName)
	return aside, warnings
}

// jobProblem is why the worker will not run an adapter job as defined, empty when it will: a definition problem of the
// module, a profile, checkout or secret worker.toml does not map, a timeout or prompt over its bound, or a workflow it
// does not run (ADR-0008). wf is the job's workflow when it runs one, wfErr why it was not read.
func jobProblem(cfg Config, pc ProjectConfig, job cronjob.Job, wf *Workflow, wfErr error) (why string, upgrade bool) {
	d := job.Definition
	if why := job.Problem(); why != "" {
		return why, false
	}
	if d.Action.Kind == cronjob.ActionNotify {
		return "", false
	}
	if len(d.Action.Prompt) > MaxPromptBytes {
		return fmt.Sprintf("its prompt is over %d KiB", MaxPromptBytes>>10), false
	}
	profile := cmp.Or(d.Profile, "default")
	if _, ok := cfg.Profiles[profile]; !ok || !pc.allowsProfile(profile) {
		return fmt.Sprintf("it names profile %q, which is not available to Project %s in worker.toml", profile, pc.Name), false
	}
	if key := d.CheckoutKey; key != "" {
		if _, ok := cfg.Checkouts[key]; !ok || !pc.allowsCheckout(key) {
			return fmt.Sprintf("it names checkout %q, which is not available to Project %s in worker.toml", key, pc.Name), false
		}
	}
	for env, name := range d.SecretRefs {
		switch _, ok := cfg.Secrets[name]; {
		case strings.HasPrefix(env, "KATA_") || strings.HasPrefix(env, "TERN_KATA_"):
			return fmt.Sprintf("secret %q would set %s, a reserved name", name, env), false
		case !ok:
			return fmt.Sprintf("it names secret %q, which is not defined in worker.toml", name), false
		case !pc.allowsSecret(name):
			return fmt.Sprintf("it names secret %q, which is not available to Project %s in worker.toml", name, pc.Name), true
		}
	}
	timeout := cfg.defaultTimeout()
	if d.TimeoutSeconds > 0 {
		timeout = time.Duration(d.TimeoutSeconds) * time.Second
	}
	if timeout > cfg.maxTimeout() {
		return fmt.Sprintf("it asks for a timeout of %s, over max_timeout %s", timeout, cfg.maxTimeout()), false
	}
	if d.Action.WorkflowUID == "" {
		return "", false
	}
	switch {
	case errors.Is(wfErr, ErrNotFound):
		return fmt.Sprintf("it runs workflow %s, which kata does not hold", d.Action.WorkflowUID), false
	case wfErr != nil:
		return "its workflow could not be read: " + wfErr.Error(), false
	case wf == nil:
		return "", false
	case wf.Deleted:
		return fmt.Sprintf("it runs workflow %q, which is archived", wf.Name), false
	}
	if why := workflowRefusal(*wf, cfg.MaxRunPrompts()); why != "" {
		return fmt.Sprintf("it runs workflow %q, which the worker does not run: %s", wf.Name, why), false
	}
	if p, ok := optionMap(wf.Options)["profile"].(string); ok && p != "" && p != profile {
		return fmt.Sprintf("it runs workflow %q, which names profile %q where the job runs with %q", wf.Name, p, profile), false
	}
	for _, s := range wf.Steps {
		if s.Kind == cronjob.StepCommand && !pc.Commands {
			return fmt.Sprintf("it runs workflow %q, whose step %q is a command, and Project %s does not allow command steps in worker.toml (commands = true)", wf.Name, s.Key, pc.Name), true
		}
		if s.Kind != cronjob.StepPrompt {
			continue
		}
		name := profile
		if p, ok := optionMap(s.Options)["profile"].(string); ok && p != "" {
			name = p
		}
		if _, ok := cfg.Profiles[name]; !ok || !pc.allowsProfile(name) {
			return fmt.Sprintf("it runs workflow %q, whose step %q names profile %q, which is not available to Project %s in worker.toml", wf.Name, s.Key, name, pc.Name), false
		}
	}
	return "", false
}
