package model

import (
	"cmp"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// The adapter side of the Job card: the kinds that a Worker runs (an agent prompt, a workflow) or that notify an existing Issue,
// the triggers only they use (once, manual, Issue dates), and the pickers this machine's worker config and the Project's
// workflows give them.

// WorkerPickers is what this machine's worker.toml offers the card's profile and checkout pickers. Read says a config was read:
// without one both fields take free text. Note is why a config that exists could not be read.
type WorkerPickers struct {
	Read                bool
	Profiles, Checkouts []string
	Note                string
}

// LoadWorkerPickers asks for this machine's worker config, which answers WorkerPickersLoaded.
type LoadWorkerPickers struct{}

func (LoadWorkerPickers) cmd() {}

// WorkerPickersLoaded answers LoadWorkerPickers. Names are sorted.
type WorkerPickersLoaded struct{ Pickers WorkerPickers }

func (WorkerPickersLoaded) msg() {}

// LoadJobWorkflows asks for the workflows of the Project with ProjectID for the Job card's workflow picker.
type LoadJobWorkflows struct {
	Workspace string
	ProjectID int64
}

func (LoadJobWorkflows) cmd() {}

// JobWorkflowsLoaded answers LoadJobWorkflows: every workflow of the Project, archived ones included, in kata's order.
type JobWorkflowsLoaded struct {
	ProjectID int64
	Workflows []Workflow
	Err       error
}

func (JobWorkflowsLoaded) msg() {}

// jobChoiceCmds are the reads that fill an opening card's pickers.
func jobChoiceCmds(m Model, projectID int64) []Cmd {
	return []Cmd{LoadWorkerPickers{}, LoadJobWorkflows{Workspace: m.Workspace, ProjectID: projectID}}
}

func workerPickersLoaded(m Model, msg WorkerPickersLoaded) Model {
	f := &m.JobForm
	if !f.Open {
		return m
	}
	f.Worker = msg.Pickers
	if !f.Shows(f.Field) {
		f.Field = JobFieldName // the free-text field the focus was in has given way to a picker
	}
	return m
}

func jobWorkflowsLoaded(m Model, msg JobWorkflowsLoaded) Model {
	f := &m.JobForm
	if !f.Open || f.ProjectID != msg.ProjectID {
		return m
	}
	if f.WorkflowsErr = msg.Err; msg.Err == nil {
		f.Workflows, f.WorkflowsRead = msg.Workflows, true
	}
	return m
}

// WorkflowsNote is the line that stands in for the workflow picker when it has nothing to offer: still reading, unreadable, or
// the Project has no workflow yet. It is empty when the picker has choices.
func (f JobFormState) WorkflowsNote() string {
	switch {
	case len(f.options(JobFieldWorkflow)) > 0:
		return ""
	case f.WorkflowsErr != nil:
		return "the Project's workflows could not be read: " + f.WorkflowsErr.Error()
	case !f.WorkflowsRead:
		return "reading the Project's workflows…"
	}
	return "this Project has no workflow; make one on the Workflows Page."
}

// maxJobSeconds is the longest span kata accepts for an interval, a lead, an offset, a grace or a timeout.
const maxJobSeconds = int64(1<<63-1) / 1e9

// parseSeconds reads a span as whole seconds: a number, a Go duration like 90m, or days like 2d. Negative spans read too.
func parseSeconds(text string) (int64, bool) {
	text = strings.TrimSpace(text)
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n, true
	}
	if days, ok := strings.CutSuffix(text, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		return n * 86400, err == nil && n <= maxJobSeconds/86400 && n >= -maxJobSeconds/86400
	}
	d, err := time.ParseDuration(text)
	return int64(d / time.Second), err == nil && d%time.Second == 0
}

// seconds reads the optional span in the text field name: blank is zero, a span out of kata's range or below min is refused.
func seconds(field Field, text string, min int64) (int64, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	n, ok := parseSeconds(text)
	switch {
	case !ok:
		return 0, fieldError(field, "enter a whole number of seconds, or a span like 30m, 2h or 1d.")
	case n < min:
		return 0, fieldError(field, "can't be negative.")
	case n > maxJobSeconds || n < -maxJobSeconds:
		return 0, fieldError(field, "is longer than kata accepts.")
	}
	return n, nil
}

// parseSecrets reads "ENV=source, ENV2=source2" into the secret_refs map: every name and source portable, no name twice.
func parseSecrets(text string) (map[string]string, error) {
	refs := map[string]string{}
	for _, entry := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		env, source, found := strings.Cut(entry, "=")
		switch {
		case !found || !portableName(env) || !portableName(source):
			return nil, fieldError(JobFieldSecrets, "%q is not ENV=source; names are letters, digits, - _ and . only.", entry)
		case refs[env] != "":
			return nil, fieldError(JobFieldSecrets, "%s is listed twice.", env)
		}
		refs[env] = source
	}
	if len(refs) == 0 {
		return nil, nil
	}
	return refs, nil
}

func formatSecrets(refs map[string]string) string {
	entries := make([]string, 0, len(refs))
	for env, source := range refs {
		entries = append(entries, env+"="+source)
	}
	sort.Strings(entries)
	return strings.Join(entries, ", ")
}

// DateTrigger says the trigger fires on a date of an Issue.
func DateTrigger(kind string) bool {
	return kind == TriggerIssueScheduled || kind == TriggerIssueDeadline
}

func dateName(kind string) string {
	if kind == TriggerIssueDeadline {
		return "deadline"
	}
	return "scheduled date"
}

var onceJobTrigger = jobTrigger{
	id: TriggerOnce, label: "Once", fields: []Field{JobFieldAt},
	load: func(f *JobFormState, t CronTrigger) { f.At = t.At },
	build: func(f JobFormState) (CronTrigger, error) {
		at := strings.TrimSpace(f.At)
		if _, err := time.Parse(time.RFC3339, at); err != nil {
			return CronTrigger{}, fieldError(JobFieldAt, "enter a full instant with its offset, like 2026-11-01T09:00:00+01:00 or 2026-11-01T08:00:00Z.")
		}
		return CronTrigger{Kind: TriggerOnce, At: at}, nil
	},
}

var manualJobTrigger = jobTrigger{
	id: TriggerManual, label: "Manual",
	load:  func(*JobFormState, CronTrigger) {},
	build: func(JobFormState) (CronTrigger, error) { return CronTrigger{Kind: TriggerManual}, nil },
}

var issueScheduledJobTrigger = jobTrigger{
	id: TriggerIssueScheduled, label: "Issue's scheduled date", fields: []Field{JobFieldIssue},
	load:  func(f *JobFormState, t CronTrigger) { f.Issue = t.IssueUID },
	build: func(JobFormState) (CronTrigger, error) { return CronTrigger{Kind: TriggerIssueScheduled}, nil },
}

var issueDeadlineJobTrigger = jobTrigger{
	id: TriggerIssueDeadline, label: "Issue's deadline", fields: []Field{JobFieldIssue, JobFieldLead},
	load: func(f *JobFormState, t CronTrigger) {
		f.Issue = t.IssueUID
		if t.LeadSeconds > 0 {
			f.Lead = strconv.FormatInt(t.LeadSeconds, 10)
		}
	},
	build: func(f JobFormState) (CronTrigger, error) {
		lead, err := seconds(JobFieldLead, f.Lead, 0)
		return CronTrigger{Kind: TriggerIssueDeadline, LeadSeconds: lead}, err
	},
}

// catchupApplies says the trigger can fall due while nothing runs it, so a catch-up policy means something.
func catchupApplies(trigger string) bool {
	return trigger == TriggerCron || trigger == TriggerInterval || trigger == TriggerOnce
}

// adapterTriggers are the triggers an adapter job may have; a deadline fires a notification only, as kata allows.
var adapterTriggers = []string{TriggerCron, TriggerInterval, TriggerOnce, TriggerManual, TriggerIssueScheduled}

var promptJobKind = executeKind("prompt", "Agent prompt", KindAgentPrompt, false)

var workflowJobKind = executeKind("workflow", "Workflow", KindWorkflow, true)

// executeKind is the kind of a job a Worker executes: an agent prompt, or a workflow of the Project.
func executeKind(id, label, kind string, workflow bool) jobKind {
	return jobKind{
		id: id, label: label, triggers: adapterTriggers,
		owns: func(j CronJob) bool { return j.Kind() == kind },
		shown: func(f JobFormState, name Field) bool {
			switch name {
			case JobFieldPrompt:
				return !workflow
			case JobFieldWorkflow:
				return workflow
			case JobFieldProfile, JobFieldCheckout:
				return !f.Worker.Read
			case JobFieldProfilePick, JobFieldCheckoutPick:
				return f.Worker.Read
			case JobFieldSecrets, JobFieldPolicy, JobFieldOverlap, JobFieldTimeout:
				return true
			case JobFieldIssue:
				return f.Policy == IssueExisting
			case JobFieldTitle, JobFieldBody, JobFieldScheduled, JobFieldDeadline:
				return f.Policy != IssueExisting
			}
			return adapterCatchupShown(f, name)
		},
		load:         loadExecute,
		checkTrigger: func(CronTrigger) error { return nil },
		build:        func(f JobFormState) (CronDefinition, string, error) { return buildExecute(f, workflow) },
	}
}

// adapterCatchupShown is whether the catch-up fields apply to an adapter job: its trigger can be missed, and grace belongs to skip.
func adapterCatchupShown(f JobFormState, name Field) bool {
	switch name {
	case JobFieldCatchup:
		return catchupApplies(f.Trigger)
	case JobFieldGrace:
		return catchupApplies(f.Trigger) && f.Catchup == CatchupSkip
	}
	return false
}

func loadExecute(f *JobFormState, job CronJob) {
	d := job.Definition
	f.Prompt, f.Workflow, f.Profile, f.Checkout, f.Secrets = d.Action.Prompt, d.Action.WorkflowUID, d.Profile, d.CheckoutKey, formatSecrets(d.SecretRefs)
	f.Overlap, f.Catchup = cmp.Or(d.Overlap, f.Overlap), cmp.Or(d.Catchup, f.Catchup)
	if d.GraceSeconds > 0 {
		f.Grace = strconv.FormatInt(d.GraceSeconds, 10)
	}
	if d.TimeoutSeconds > 0 {
		f.Timeout = strconv.FormatInt(d.TimeoutSeconds, 10)
	}
	p := d.Issue
	if p == nil {
		return
	}
	f.Policy = p.Kind
	if p.Kind == IssueExisting {
		f.Issue = cmp.Or(f.Issue, p.UID)
		return
	}
	f.Title, f.Body = p.Title, strings.Join(strings.Fields(p.Body), " ")
	for _, offset := range []struct {
		value *int64
		into  *string
	}{{p.ScheduledOffsetSeconds, &f.Scheduled}, {p.DeadlineOffsetSeconds, &f.DeadlineOffset}} {
		if offset.value != nil {
			*offset.into = strconv.FormatInt(*offset.value, 10)
		}
	}
}

// buildExecute is the definition of an agent prompt or workflow job: the action, its profile, checkout and secrets, the Issue
// policy, overlap, catch-up and timeout.
func buildExecute(f JobFormState, workflow bool) (CronDefinition, string, error) {
	def := CronDefinition{Overlap: f.Overlap, Catchup: f.Catchup, Action: CronAction{Kind: ActionExecute}}
	if workflow {
		switch {
		case f.Workflow == "" && f.WorkflowsRead && len(f.options(JobFieldWorkflow)) == 0:
			return def, "", fieldError(JobFieldWorkflow, "this Project has no workflow; make one on the Workflows Page first.")
		case f.Workflow == "":
			return def, "", fieldError(JobFieldWorkflow, "pick the workflow to run.")
		case !f.WorkflowsRead && f.WorkflowsErr != nil:
			return def, "", fieldError(JobFieldWorkflow, "the Project's workflows could not be read (%v).", f.WorkflowsErr)
		case !f.WorkflowsRead:
			return def, "", fieldError(JobFieldWorkflow, "the Project's workflows are still being read; try again in a moment.")
		case !slices.ContainsFunc(f.Workflows, func(w Workflow) bool { return w.UID == f.Workflow && !w.Archived }):
			return def, "", fieldError(JobFieldWorkflow, "no live workflow %s in this Project; pick another.", f.Workflow)
		}
		def.Action.WorkflowUID = f.Workflow
	} else {
		if def.Action.Prompt = strings.TrimSpace(f.Prompt); def.Action.Prompt == "" {
			return def, "", fieldError(JobFieldPrompt, "must not be blank.")
		}
	}
	options := maps.Clone(f.Opened.Definition.Options)
	if profile := strings.TrimSpace(f.Profile); profile != "" {
		if options == nil {
			options = map[string]any{}
		}
		options["profile"], def.Profile = profile, profile
	} else {
		delete(options, "profile")
	}
	if len(options) > 0 {
		def.Options = options
	}
	if def.CheckoutKey = strings.TrimSpace(f.Checkout); def.CheckoutKey != "" && !portableName(def.CheckoutKey) {
		return def, "", fieldError(JobFieldCheckout, "%q is not a checkout key; keys are letters, digits, - _ and . only.", def.CheckoutKey)
	}
	secrets, err := parseSecrets(f.Secrets)
	if err != nil {
		return def, "", err
	}
	def.SecretRefs = secrets
	if def.Issue, err = executePolicy(f); err != nil {
		return def, "", err
	}
	if def.TimeoutSeconds, err = seconds(JobFieldTimeout, f.Timeout, 0); err != nil {
		return def, "", err
	}
	def.GraceSeconds, err = adapterGrace(f)
	return def, "", err
}

// adapterGrace is the grace an adapter job's skip catch-up allows; blank is none, and it counts only where it is shown.
func adapterGrace(f JobFormState) (int64, error) {
	if !adapterCatchupShown(f, JobFieldGrace) {
		return 0, nil
	}
	return seconds(JobFieldGrace, f.Grace, 0)
}

// executePolicy is the Issue policy of a job that executes: a new Issue per run with its title, body and date offsets, or the
// existing Issue the card names.
func executePolicy(f JobFormState) (*CronIssuePolicy, error) {
	if f.Policy == IssueExisting {
		if f.issueRef() == "" {
			return nil, fieldError(JobFieldIssue, "name the Issue every run belongs to, by its short id.")
		}
		return &CronIssuePolicy{Kind: IssueExisting}, nil
	}
	policy := CronIssuePolicy{Kind: IssuePerRun, Title: strings.TrimSpace(f.Title), Body: strings.TrimSpace(f.Body)}
	if policy.Title == "" {
		return nil, fieldError(JobFieldTitle, "must not be blank.")
	}
	for _, offset := range []struct {
		field Field
		text  string
		into  **int64
	}{{JobFieldScheduled, f.Scheduled, &policy.ScheduledOffsetSeconds}, {JobFieldDeadline, f.DeadlineOffset, &policy.DeadlineOffsetSeconds}} {
		if strings.TrimSpace(offset.text) == "" {
			continue
		}
		n, err := seconds(offset.field, offset.text, -maxJobSeconds)
		if err != nil {
			return nil, err
		}
		*offset.into = &n
	}
	return &policy, nil
}

var notifyJobKind = jobKind{
	id: "notify", label: "Notify", triggers: append(slices.Clone(adapterTriggers), TriggerIssueDeadline),
	owns: func(j CronJob) bool { return j.Kind() == KindNotify },
	shown: func(f JobFormState, name Field) bool {
		switch name {
		case JobFieldRecipient, JobFieldMessage:
			return true
		case JobFieldIssue:
			return !DateTrigger(f.Trigger)
		case JobFieldReport:
			return DateTrigger(f.Trigger)
		}
		return adapterCatchupShown(f, name)
	},
	load: func(f *JobFormState, job CronJob) {
		d := job.Definition
		if d.Action.Recipient != recipientDefault {
			f.Recipient = d.Action.Recipient
		}
		f.Message, f.Catchup = d.Action.Message, cmp.Or(d.Catchup, f.Catchup)
		if d.GraceSeconds > 0 {
			f.Grace = strconv.FormatInt(d.GraceSeconds, 10)
		}
		switch {
		case d.Issue != nil && DateTrigger(d.Trigger.Kind):
			f.Report = d.Issue.UID
		case d.Issue != nil:
			f.Issue = d.Issue.UID
		}
	},
	checkTrigger: func(CronTrigger) error { return nil },
	build: func(f JobFormState) (CronDefinition, string, error) {
		def := CronDefinition{Overlap: "allow", Catchup: f.Catchup, Action: CronAction{Kind: ActionNotify, Recipient: recipientDefault}}
		if err := notification(f, &def, true); err != nil {
			return def, "", err
		}
		switch {
		case !DateTrigger(f.Trigger):
			if f.issueRef() == "" {
				return def, "", fieldError(JobFieldIssue, "name the Issue to notify about, by its short id.")
			}
			def.Issue = &CronIssuePolicy{Kind: IssueExisting}
		case f.reportRef() != "":
			def.Issue = &CronIssuePolicy{Kind: IssueExisting}
		}
		grace, err := adapterGrace(f)
		def.GraceSeconds = grace
		return def, "", err
	},
}

// notification fills the notify action's recipient and message from the card and checks them as kata does. needsMessage says a
// named recipient must come with a message: kata asks that of an adapter's notify job, and not of a native one.
func notification(f JobFormState, def *CronDefinition, needsMessage bool) error {
	if recipient := strings.TrimSpace(f.Recipient); recipient != "" {
		if !exactRecipient(recipient) {
			return fieldError(JobFieldRecipient, "use an exact address like alice or alice/reviewer, or leave it blank for the current owner or author.")
		}
		def.Action.Recipient = recipient
	}
	if len(f.Message) > 1024 {
		return fieldError(JobFieldMessage, "at most 1024 bytes.")
	}
	def.Action.Message = strings.TrimSpace(f.Message)
	if needsMessage && def.Action.Recipient != recipientDefault && def.Action.Message == "" {
		return fieldError(JobFieldMessage, "a notification to a named recipient needs a message.")
	}
	return nil
}
