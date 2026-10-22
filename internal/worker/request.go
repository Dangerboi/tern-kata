package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// RunAccepted is what a run request answers once the run's journal is durable (ADR-0002): the caller sees the uid only then.
type RunAccepted struct {
	RunUID  string
	JobUID  string
	JobName string
	Project string
}

// RequestRun runs an adapter job now: a manual occurrence with a fresh random uid. It applies the lease, support,
// overlap and capacity rules; a refusal says why and nothing is written to kata. The run itself then proceeds in the background.
func (w *Worker) RequestRun(ctx context.Context, jobUID string) (RunAccepted, error) {
	ps, job, err := w.findJob(ctx, jobUID)
	if err != nil {
		return RunAccepted{}, err
	}
	j, _, err := w.prepare(ctx, ps, job, w.clock.Now(), "")
	if err != nil {
		return RunAccepted{}, err
	}
	w.spawn(ps, j.RunUID)
	return RunAccepted{RunUID: j.RunUID, JobUID: job.UID, JobName: job.Name, Project: ps.cfg.Name}, nil
}

// RunOccurrence prepares the run of one scheduled occurrence of a job in a served Project and starts it. The same
// occurrence always maps to the same run uid; when that run already has a journal, it is returned and created is false.
// The scheduler uses this entry point.
func (w *Worker) RunOccurrence(ctx context.Context, project, jobUID string, occurrence time.Time) (runUID string, created bool, err error) {
	ps := w.projectNamed(project)
	if ps == nil {
		return "", false, fmt.Errorf("Project %q is not served by this worker", project)
	}
	lookup := func() (Job, bool) {
		w.mu.Lock()
		defer w.mu.Unlock()
		job, ok := ps.jobs[jobUID]
		return job, ok
	}
	job, ok := lookup()
	if !ok && w.admitsNothing(ps) == "" {
		_ = w.refreshJobs(ctx, ps)
		job, ok = lookup()
	}
	if !ok {
		return "", false, fmt.Errorf("job %s is not in Project %s", jobUID, project)
	}
	j, created, err := w.prepare(ctx, ps, job, occurrence, OccurrenceKey(occurrence))
	if err != nil {
		return "", false, err
	}
	if created {
		w.spawn(ps, j.RunUID)
	}
	return j.RunUID, created, nil
}

// findJob locates the job in a served Project, refreshing the definitions once before giving up. When it belongs to
// a Project this worker doesn't serve it says so, because unserved Projects never run.
func (w *Worker) findJob(ctx context.Context, jobUID string) (*projectState, Job, error) {
	lookup := func() (*projectState, Job, bool) {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, ps := range w.projects {
			if j, ok := ps.jobs[jobUID]; ok {
				return ps, j, true
			}
		}
		return nil, Job{}, false
	}
	if ps, j, ok := lookup(); ok {
		return ps, j, nil
	}
	for _, ps := range w.projects {
		if w.admitsNothing(ps) == "" {
			_ = w.refreshJobs(ctx, ps)
		}
	}
	if ps, j, ok := lookup(); ok {
		return ps, j, nil
	}
	// Name the Project when it exists but isn't served, and each served Project that can't answer.
	var notes []string
	if projects, err := w.opt.Kata.Projects(ctx); err == nil {
		for _, p := range projects {
			if w.projectNamed(p.Name) != nil {
				continue
			}
			if jobs, err := w.opt.Kata.Jobs(ctx, p.ID); err == nil {
				for _, j := range jobs {
					if j.UID == jobUID {
						return nil, Job{}, fmt.Errorf("job %s (%s) belongs to Project %s, which this worker does not serve; list it in worker.toml to serve it", jobUID, j.Name, p.Name)
					}
				}
			}
		}
	}
	for _, ps := range w.projects {
		if why := w.admitsNothing(ps); why != "" {
			notes = append(notes, fmt.Sprintf("Project %s: %s", ps.cfg.Name, why))
		}
	}
	msg := fmt.Sprintf("no job %s in a Project this worker serves", jobUID)
	if len(notes) > 0 {
		msg += " (" + strings.Join(notes, "; ") + ")"
	}
	return nil, Job{}, errors.New(msg)
}

// admitsNothing is why a Project can't start runs right now, empty when it can.
func (w *Worker) admitsNothing(ps *projectState) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case ps.blocked != "":
		return ps.blocked
	case !ps.resolved:
		return "not resolved yet"
	case !ps.lease.Held:
		return ps.lease.Why()
	case ps.route.err != "":
		return "the kata route is not confirmed: " + ps.route.err
	}
	return ""
}

func newJournalID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// prepare checks a job can run, reserves a slot, applies ADR-0002's veto and saves the run's journal. occurrenceKey is
// empty for a manual run. It returns the existing journal, with created false, when the occurrence was prepared before.
func (w *Worker) prepare(ctx context.Context, ps *projectState, job Job, occurrence time.Time, occurrenceKey string) (*Journal, bool, error) {
	if why := w.admitsNothing(ps); why != "" {
		return nil, false, fmt.Errorf("Project %s admits nothing: %s", ps.cfg.Name, why)
	}
	if job.IsNative() {
		return nil, false, fmt.Errorf("job %q is a native job: kata's daemon delivers it, the worker never runs it", job.Name)
	}
	d := job.Definition
	var notify *NotifySpec
	var issuePolicy IssuePolicy
	var profileName, checkout string
	var profile Profile
	var timeout time.Duration
	if d.ActionKind == "notify" {
		var err error
		if notify, issuePolicy, err = notifyPlan(job); err != nil {
			return nil, false, err
		}
	} else {
		if unsupported != "" { // a notify run starts no process, so only execute runs need to prove one gone
			return nil, false, errors.New(unsupported)
		}
		switch {
		case d.WorkflowUID == "" && strings.TrimSpace(d.Prompt) == "":
			return nil, false, fmt.Errorf("job %q has neither a prompt nor a workflow", job.Name)
		case d.Issue == nil:
			return nil, false, fmt.Errorf("job %q has no Issue policy", job.Name)
		}
		issuePolicy = *d.Issue
		profileName = cmpOr(d.Profile, "default")
		var ok bool
		profile, ok = w.opt.Config.Profiles[profileName]
		if !ok || !ps.cfg.allowsProfile(profileName) {
			return nil, false, fmt.Errorf("job %q names profile %q, which is not available to Project %s in worker.toml", job.Name, profileName, ps.cfg.Name)
		}
		if d.CheckoutKey != "" {
			dir, ok := w.opt.Config.Checkouts[d.CheckoutKey]
			if !ok || !ps.cfg.allowsCheckout(d.CheckoutKey) {
				return nil, false, fmt.Errorf("job %q names checkout %q, which is not available to Project %s in worker.toml", job.Name, d.CheckoutKey, ps.cfg.Name)
			}
			checkout = dir
		}
		timeout = w.opt.Config.defaultTimeout()
		if d.TimeoutSeconds > 0 {
			timeout = time.Duration(d.TimeoutSeconds) * time.Second
		}
		if timeout > w.opt.Config.maxTimeout() {
			return nil, false, fmt.Errorf("job %q asks for a timeout of %s, over max_timeout %s (default_timeout is %s)", job.Name, timeout, w.opt.Config.maxTimeout(), w.opt.Config.defaultTimeout())
		}
	}
	manual := occurrenceKey == ""
	if !manual {
		if j, err := w.store.Load(RunUID(job.UID, occurrenceKey, occurrence)); err == nil {
			return j, false, nil
		}
	}
	if d.Overlap == "forbid" {
		if earlier := w.gateClosed(job.UID, ps.project.UID); earlier != "" {
			return nil, false, &busyError{fmt.Sprintf("job %q forbids overlap and run %s is not settled", job.Name, earlier), false}
		}
	}
	if !w.reserve(ps) {
		return nil, false, &busyError{fmt.Sprintf("Project %s is at capacity (%d); try again when a run ends", ps.cfg.Name, ps.cfg.Capacity), true}
	}
	bound := false
	defer func() {
		if !bound {
			w.unreserve(ps)
		}
	}()
	if issuePolicy.Kind == "existing" {
		issue, err := w.opt.Kata.ShowIssue(ctx, ps.project.ID, issuePolicy.UID)
		if err != nil {
			return nil, false, fmt.Errorf("job %q: its Issue could not be read: %w", job.Name, err)
		}
		if !issue.Open() {
			return nil, false, fmt.Errorf("job %q reports to Issue %s, which is closed; reopen it first", job.Name, issue.ShortID)
		}
	}
	var workflow *WorkflowPlan
	if d.ActionKind != "notify" && d.WorkflowUID != "" {
		var err error
		if workflow, err = w.planWorkflow(ctx, ps, job, profileName); err != nil {
			return nil, false, err
		}
	}
	uid := ""
	if manual {
		uid = newManualUID(occurrence)
		occurrenceKey = "manual:" + uid
	} else {
		uid = RunUID(job.UID, occurrenceKey, occurrence)
	}
	if w.store.IsQuarantined(uid) {
		return nil, false, fmt.Errorf("run %s is quarantined; its damaged journal vetoes it (run `tern-kata worker doctor`)", uid)
	}
	// Existing evidence in kata vetoes preparation; only not_found permits it.
	if run, err := w.opt.Kata.ShowRun(ctx, ps.project.ID, uid); err == nil {
		return nil, false, fmt.Errorf("kata already holds run %s (%s); it is not prepared again", uid, run.Status)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, fmt.Errorf("cannot tell whether kata holds run %s yet: %w", uid, err)
	}
	w.mu.Lock()
	generation := ps.lease.Generation
	w.mu.Unlock()
	j := &Journal{
		Version: 1, RunUID: uid, JournalID: newJournalID(), Executor: w.opt.Config.Executor, Actor: ps.actor, Generation: generation,
		ProjectUID: ps.project.UID, ProjectID: ps.project.ID, ProjectName: ps.project.Name,
		JobUID: job.UID, JobName: job.Name, DefinitionEventUID: job.DefinitionEventUID, Prompt: d.Prompt, IssuePolicy: issuePolicy, Notify: notify, Workflow: workflow,
		SecretRefs: d.SecretRefs, ProfileName: profileName, Profile: profile, Checkout: checkout, TimeoutSeconds: int64(timeout / time.Second),
		OccurrenceKey: occurrenceKey, Occurrence: occurrence.UTC(), Manual: manual,
		Steps: stepsFor(workflow),
		Phase: PhasePrepared, StartedAt: w.clock.Now().UTC(),
	}
	if err := w.store.Create(j, "prepared"); err != nil {
		w.noteHook(err)
		return nil, false, err
	}
	w.bind(ps, uid, job.UID)
	bound = true
	if _, err := os.Stat(w.store.RunDir(uid)); err != nil {
		return nil, false, err
	}
	return j, true, nil
}

// notifyPlan is what a notify job asks of the worker: the request to send and the existing Issue it goes to. The
// Issue is the job's own `existing` policy, else the one its Issue-date trigger watches.
func notifyPlan(job Job) (*NotifySpec, IssuePolicy, error) {
	d := job.Definition
	uid := d.Trigger.IssueUID
	if d.Issue != nil {
		if d.Issue.Kind != "existing" {
			return nil, IssuePolicy{}, fmt.Errorf("job %q creates an Issue per run: kata's daemon delivers it, the worker never runs it", job.Name)
		}
		uid = d.Issue.UID
	}
	if uid == "" || d.Recipient == "" {
		return nil, IssuePolicy{}, fmt.Errorf("job %q does not name an Issue and a recipient to notify", job.Name)
	}
	return &NotifySpec{Recipient: d.Recipient, Message: d.Message}, IssuePolicy{Kind: "existing", UID: uid}, nil
}
