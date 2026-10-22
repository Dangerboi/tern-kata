package worker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"slices"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
	"golang.org/x/mod/semver"
)

// Limits on the worker's calls (ADR-0004): at most four one-shot kata calls at once across Projects, each with a
// deadline that starts when it takes its slot. Event streams take no slot.
const (
	maxCalls        = 4
	callTimeout     = 60 * time.Second
	readAttempts    = 3
	firstReadRetry  = time.Second
	secondReadRetry = 2 * time.Second
)

// Kata is the worker's whole view of the daemon: kata's HTTP client behind the calls the worker makes (ADR-0001: the
// worker runs no `kata` binary). The worker's other files speak these model types, never the generated ones.
type Kata struct {
	c       *client.Client
	slots   chan struct{}
	timeout time.Duration
	retries [readAttempts - 1]time.Duration
}

// KataOption changes one of the limits of a Kata; tests use them to shorten the waits.
type KataOption func(*Kata)

// WithCallTimeout sets the deadline every one-shot call gets from the moment it takes a slot.
func WithCallTimeout(d time.Duration) KataOption { return func(k *Kata) { k.timeout = d } }

// WithReadRetryDelays sets the waits before the second and third attempt of a read-only call that ran out of time.
func WithReadRetryDelays(first, second time.Duration) KataOption {
	return func(k *Kata) { k.retries = [readAttempts - 1]time.Duration{first, second} }
}

// NewKata wraps c, which the caller found with client.Discover (as the Pane does) or built for a test.
func NewKata(c *client.Client, opts ...KataOption) *Kata {
	k := &Kata{c: c, slots: make(chan struct{}, maxCalls), timeout: callTimeout, retries: [readAttempts - 1]time.Duration{firstReadRetry, secondReadRetry}}
	for _, o := range opts {
		o(k)
	}
	return k
}

// call runs one kata request inside a slot with its own deadline. A read-only request that ran out of time is tried
// again, up to three times in all, after the first and then the second retry delay; a mutation is never repeated here,
// its journal intent decides (ADR-0006).
func call[T any](ctx context.Context, k *Kata, readOnly bool, f func(context.Context) (T, error)) (T, error) {
	attempts := 1
	if readOnly {
		attempts = readAttempts
	}
	for attempt := 0; ; attempt++ {
		select {
		case k.slots <- struct{}{}:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
		callCtx, cancel := context.WithTimeout(ctx, k.timeout)
		out, err := f(callCtx)
		expired := err != nil && callCtx.Err() != nil && ctx.Err() == nil
		cancel()
		<-k.slots
		if !expired {
			return out, err
		}
		if attempt+1 >= attempts {
			return out, fmt.Errorf("kata did not answer within %s: %w", k.timeout, err)
		}
		select {
		case <-time.After(k.retries[attempt]):
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
}

// Errors the worker tells apart from any other daemon failure.
var (
	// ErrNotFound is kata's `not_found` for a Run, Issue or job that doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict is a 409: a different identity or a stale revision.
	ErrConflict = errors.New("conflict")
	// ErrRevisionConflict is a 412: a conditional write lost to another writer.
	ErrRevisionConflict = errors.New("revision conflict")
)

// APIError is a kata refusal with its status and code, wrapping the sentinel that classifies it.
type APIError struct {
	Status  int
	Code    string
	Message string
	kind    error
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) Unwrap() error { return e.kind }

// Refused reports a 4xx other than a timeout or rate limit: kata understood the request and said no, so repeating
// it changes nothing (ADR-0002 `create_rejected`).
func (e *APIError) Refused() bool {
	return e.Status >= 400 && e.Status < 500 && e.Status != http.StatusRequestTimeout && e.Status != http.StatusTooManyRequests
}

// apiError is err as the user should read it, classified by wrapping one of the sentinels above.
func apiError(err error) error {
	if err == nil {
		return nil
	}
	envelope, ok := errors.AsType[g.ErrorEnvelope](err)
	if !ok {
		return err
	}
	status := client.StatusCode(err)
	if status == 0 {
		status = int(envelope.Status)
	}
	text := envelope.ErrorData.Message
	if hint := envelope.ErrorData.Hint; hint != nil && *hint != "" {
		text += " (" + *hint + ")"
	}
	if text == "" {
		text = http.StatusText(status)
	}
	e := &APIError{Status: status, Code: envelope.ErrorData.Code, Message: text}
	switch {
	case status == http.StatusNotFound && envelope.ErrorData.Code == "not_found":
		e.kind, e.Message = ErrNotFound, ErrNotFound.Error()+": "+text
	case status == http.StatusConflict:
		e.kind, e.Message = ErrConflict, ErrConflict.Error()+": "+text
	case status == http.StatusPreconditionFailed:
		e.kind, e.Message = ErrRevisionConflict, ErrRevisionConflict.Error()+": "+text
	}
	return e
}

// refused reports whether err is a kata refusal that a retry cannot change.
func refused(err error) bool {
	e, ok := errors.AsType[*APIError](err)
	return ok && e.Refused()
}

// MinKataVersion is the oldest daemon with the cron API the worker needs.
const MinKataVersion = "v0.19.0"

// Ping returns the daemon's version, refusing one older than MinKataVersion.
func (k *Kata) Ping(ctx context.Context) (string, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.PingResp, error) { return k.c.PingWithResponse(ctx) })
	if err != nil {
		return "", apiError(err)
	}
	if r.JSON200 == nil {
		return "", errors.New("kata daemon returned no version")
	}
	v := "v" + strings.TrimPrefix(r.JSON200.Version, "v")
	if !semver.IsValid(v) {
		return "", fmt.Errorf("cannot verify kata daemon version %q; the worker requires %s or newer", r.JSON200.Version, MinKataVersion)
	}
	if semver.Compare(v, MinKataVersion) < 0 {
		return "", fmt.Errorf("kata daemon %s is too old; the worker requires %s or newer; upgrade kata", r.JSON200.Version, MinKataVersion)
	}
	return v, nil
}

// Project is a kata Project with its metadata, where the lease lives.
type Project struct {
	ID       int64
	UID      string
	Name     string
	Revision int64
	Metadata map[string]any
}

func projectOf(p g.ProjectOut) Project {
	return Project{ID: p.ID, UID: p.UID, Name: p.Name, Revision: p.Revision, Metadata: p.Metadata}
}

// Projects lists the live Projects.
func (k *Kata) Projects(ctx context.Context) ([]Project, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ListProjectsResp, error) {
		return k.c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
	})
	if err != nil {
		return nil, apiError(err)
	}
	if r.JSON200 == nil {
		return nil, errors.New("kata daemon returned no Projects")
	}
	var out []Project
	for _, p := range r.JSON200.Projects {
		if p.DeletedAt == nil {
			out = append(out, projectOf(p))
		}
	}
	return out, nil
}

// Project reads one Project by id.
func (k *Kata) Project(ctx context.Context, id int64) (Project, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ShowProjectResp, error) {
		return k.c.ShowProjectWithResponse(ctx, &g.ShowProjectRequestOptions{PathParams: &g.ShowProjectPath{ProjectID: id}})
	})
	if err != nil {
		return Project{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Project{}, errors.New("kata daemon returned no Project")
	}
	return projectOf(r.JSON200.Project), nil
}

// PatchProjectMetadata writes patch only while the Project is still at revision (ADR-0003), returning the new revision.
func (k *Kata) PatchProjectMetadata(ctx context.Context, id, revision int64, actor string, patch map[string]any) (int64, error) {
	etag := `"rev-` + strconv.FormatInt(revision, 10) + `"`
	r, err := call(ctx, k, false, func(ctx context.Context) (*g.PatchProjectMetadataResp, error) {
		return k.c.PatchProjectMetadataWithResponse(ctx, &g.PatchProjectMetadataRequestOptions{
			PathParams: &g.PatchProjectMetadataPath{ProjectID: id},
			Body:       &g.PatchProjectMetadataBody{Actor: actor, Patch: patch},
			Header:     &g.PatchProjectMetadataHeaders{IfMatch: &etag},
		})
	})
	if err != nil {
		return 0, apiError(err)
	}
	if r.JSON200 == nil {
		return 0, errors.New("kata daemon returned no Project")
	}
	return r.JSON200.Project.Revision, nil
}

// Capabilities is what a Project's daemon answers about its cron API: the event features it advertises and the uid of
// the Project it answers for.
type Capabilities struct {
	Features   []string
	ProjectUID string
}

// Has reports whether the daemon advertises feature.
func (c Capabilities) Has(feature string) bool { return slices.Contains(c.Features, feature) }

// Capabilities reads the Project's cron capabilities.
func (k *Kata) Capabilities(ctx context.Context, projectID int64) (Capabilities, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.GetCronCapabilitiesResp, error) {
		return k.c.GetCronCapabilitiesWithResponse(ctx, &g.GetCronCapabilitiesRequestOptions{PathParams: &g.GetCronCapabilitiesPath{ProjectID: projectID}})
	})
	if err != nil {
		return Capabilities{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Capabilities{}, errors.New("kata daemon returned no capabilities")
	}
	return Capabilities{Features: r.JSON200.EventFeatures, ProjectUID: r.JSON200.ProjectUID}, nil
}

// Instance is who the daemon is and who it says the worker authenticates as.
type Instance struct {
	UID string
	// Actor is the authenticated actor, empty when the daemon names none.
	Actor string
}

// Instance reads the daemon's instance uid and the actor its credential authenticates as.
func (k *Kata) Instance(ctx context.Context) (Instance, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.InstanceResp, error) { return k.c.InstanceWithResponse(ctx) })
	if err != nil {
		return Instance{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Instance{}, errors.New("kata daemon returned no instance")
	}
	return Instance{UID: r.JSON200.InstanceUID, Actor: deref(r.JSON200.Auth.Actor)}, nil
}

// Trigger, IssuePolicy and JobDefinition are the parts of a kata job definition the worker reads.
type Trigger struct {
	Kind            string
	Cron, At        string
	IntervalSeconds int64
	IssueUID        string
	LeadSeconds     int64
	Timezone        string
}

type IssuePolicy struct {
	Kind      string // "per-run" or "existing"
	Title     string
	Body      string
	UID       string
	ParentUID string
	// ScheduledOffsetSeconds and DeadlineOffsetSeconds are the per-run Issue's planning dates, relative to the run's
	// occurrence; nil leaves the date unset (zero is the occurrence itself).
	ScheduledOffsetSeconds *int64
	DeadlineOffsetSeconds  *int64
}

type JobDefinition struct {
	Enabled        bool
	Trigger        Trigger
	ActionKind     string // "execute" or "notify"
	Prompt         string
	WorkflowUID    string
	Issue          *IssuePolicy
	Overlap        string
	Catchup        string
	GraceSeconds   int64
	TimeoutSeconds int64
	CheckoutKey    string
	SecretRefs     map[string]string
	Profile        string // options.profile
	// Recipient and Message are a notify action's inbox address (or "current-owner-or-author") and text.
	Recipient string
	Message   string
}

// Job is a kata cron job.
type Job struct {
	UID                string
	Name               string
	Revision           int64
	DefinitionEventUID string
	Deleted            bool
	ProjectID          int64
	// CreatedAt anchors interval triggers.
	CreatedAt  time.Time
	Definition JobDefinition
}

// IsNative is kata's own predicate: a notify action with a per-run Issue is delivered by kata's daemon, never by an
// adapter, even while disabled.
func (j Job) IsNative() bool {
	d := j.Definition
	return d.ActionKind == "notify" && d.Issue != nil && d.Issue.Kind == "per-run"
}

func jobOf(j g.CronJob) Job {
	d := j.Definition
	def := JobDefinition{
		Enabled:        d.Enabled,
		ActionKind:     d.Action.Kind,
		Prompt:         deref(d.Action.Prompt),
		WorkflowUID:    deref(d.Action.WorkflowUID),
		Overlap:        d.Overlap,
		Catchup:        d.Catchup,
		GraceSeconds:   derefInt(d.GraceSeconds),
		TimeoutSeconds: derefInt(d.TimeoutSeconds),
		CheckoutKey:    deref(d.CheckoutKey),
		SecretRefs:     d.SecretRefs,
		Recipient:      deref(d.Action.Recipient),
		Message:        deref(d.Action.Message),
		Trigger: Trigger{
			Kind: d.Trigger.Kind, Cron: deref(d.Trigger.Cron), At: deref(d.Trigger.At),
			IntervalSeconds: derefInt(d.Trigger.IntervalSeconds), IssueUID: deref(d.Trigger.IssueUID),
			LeadSeconds: derefInt(d.Trigger.LeadSeconds), Timezone: deref(d.Trigger.Timezone),
		},
	}
	if p, ok := d.Options["profile"].(string); ok {
		def.Profile = p
	}
	if d.Issue != nil {
		def.Issue = &IssuePolicy{Kind: d.Issue.Kind, Title: deref(d.Issue.Title), Body: deref(d.Issue.Body), UID: deref(d.Issue.UID), ParentUID: deref(d.Issue.ParentUID),
			ScheduledOffsetSeconds: d.Issue.ScheduledOffsetSeconds, DeadlineOffsetSeconds: d.Issue.DeadlineOffsetSeconds}
	}
	return Job{UID: j.UID, Name: j.Name, Revision: j.Revision, DefinitionEventUID: j.DefinitionEventUID, Deleted: j.DeletedAt != nil, ProjectID: j.ProjectID, CreatedAt: j.CreatedAt.UTC(), Definition: def}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// Jobs lists the Project's live jobs.
func (k *Kata) Jobs(ctx context.Context, projectID int64) ([]Job, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ListCronJobsResp, error) {
		return k.c.ListCronJobsWithResponse(ctx, &g.ListCronJobsRequestOptions{PathParams: &g.ListCronJobsPath{ProjectID: projectID}})
	})
	if err != nil {
		return nil, apiError(err)
	}
	if r.JSON200 == nil {
		return nil, errors.New("kata daemon returned no jobs")
	}
	jobs := make([]Job, len(r.JSON200.Jobs))
	for i, j := range r.JSON200.Jobs {
		jobs[i] = jobOf(j)
	}
	return jobs, nil
}

// Run is a kata Run observation.
type Run struct {
	UID           string
	Status        string
	Revision      int64
	JobUID        string
	WorkflowUID   string
	IssueUID      string
	ExecutorLabel string
	OccurrenceKey string
	Summary       string
	InputTokens   *int64
	OutputTokens  *int64
	StartedAt     *time.Time
	EndedAt       *time.Time
}

func runOf(r g.CronRun) Run {
	return Run{UID: r.UID, Status: r.Status, Revision: r.Revision, JobUID: deref(r.JobUID), WorkflowUID: deref(r.WorkflowUID), IssueUID: deref(r.IssueUID),
		ExecutorLabel: deref(r.ExecutorLabel), OccurrenceKey: deref(r.OccurrenceKey), Summary: deref(r.Summary.Message), InputTokens: r.Summary.InputTokens, OutputTokens: r.Summary.OutputTokens, StartedAt: r.StartedAt, EndedAt: r.EndedAt}
}

// ShowRun reads a Run; ErrNotFound means kata holds no evidence of it.
func (k *Kata) ShowRun(ctx context.Context, projectID int64, uid string) (Run, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ShowCronRunResp, error) {
		return k.c.ShowCronRunWithResponse(ctx, &g.ShowCronRunRequestOptions{PathParams: &g.ShowCronRunPath{ProjectID: projectID, RunUID: uid}})
	})
	if err != nil {
		return Run{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Run{}, errors.New("kata daemon returned no Run")
	}
	return runOf(r.JSON200.Run), nil
}

// Runs lists the newest runs of the Project (at most 100).
func (k *Kata) Runs(ctx context.Context, projectID int64) ([]Run, error) {
	limit := int64(100)
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ListCronRunsResp, error) {
		return k.c.ListCronRunsWithResponse(ctx, &g.ListCronRunsRequestOptions{PathParams: &g.ListCronRunsPath{ProjectID: projectID}, Query: &g.ListCronRunsQuery{Limit: &limit}})
	})
	if err != nil {
		return nil, apiError(err)
	}
	if r.JSON200 == nil {
		return nil, errors.New("kata daemon returned no Runs")
	}
	runs := make([]Run, len(r.JSON200.Runs))
	for i, run := range r.JSON200.Runs {
		runs[i] = runOf(run)
	}
	return runs, nil
}

// Observation is what the worker tells kata about a Run. Identity fields matter only at revision 0; kata refuses a
// later observation whose identity differs.
type Observation struct {
	RunUID             string
	JobUID             string
	DefinitionEventUID string
	// WorkflowUID and WorkflowEventUID name the workflow a workflow job's run executes; both empty for a prompt job.
	WorkflowUID      string
	WorkflowEventUID string
	OccurrenceKey    string
	IssueUID         string
	Executor         string
	Actor            string
	Status           string
	Message          string
	InputTokens      *int64
	OutputTokens     *int64
	StartedAt        time.Time
	EndedAt          *time.Time
	ExpectedRevision int64
}

// Observe sends an observation and returns the stored Run and whether kata replayed an identical one.
func (k *Kata) Observe(ctx context.Context, projectID int64, o Observation) (Run, bool, error) {
	msg := o.Message
	body := &g.ObserveCronRunBody{
		Actor: &o.Actor, JobUID: &o.JobUID, DefinitionEventUID: &o.DefinitionEventUID, OccurrenceKey: &o.OccurrenceKey,
		IssueUID: &o.IssueUID, ExecutorLabel: &o.Executor, Status: g.ObserveCronRunBodyStatus(o.Status),
		Summary:   g.Summary{Version: 1, Message: &msg, InputTokens: o.InputTokens, OutputTokens: o.OutputTokens},
		StartedAt: &o.StartedAt, EndedAt: o.EndedAt, ExpectedRevision: o.ExpectedRevision,
	}
	if o.WorkflowUID != "" {
		body.WorkflowUID, body.WorkflowDefinitionEventUID = &o.WorkflowUID, &o.WorkflowEventUID
	}
	r, err := call(ctx, k, false, func(ctx context.Context) (*g.ObserveCronRunResp, error) {
		return k.c.ObserveCronRunWithResponse(ctx, &g.ObserveCronRunRequestOptions{PathParams: &g.ObserveCronRunPath{ProjectID: projectID, RunUID: o.RunUID}, Body: body})
	})
	if err != nil {
		return Run{}, false, apiError(err)
	}
	if r.JSON200 == nil {
		return Run{}, false, errors.New("kata daemon returned no Run")
	}
	return runOf(r.JSON200.Run), r.JSON200.Replayed, nil
}

// Issue is the part of a kata Issue the worker reads.
type Issue struct {
	ID        int64
	UID       string
	ShortID   string
	Title     string
	Body      string
	Status    string
	ProjectID int64
	Labels    []string
	Comments  []string
	Owner     string
	Author    string
	// Metadata is the Issue's metadata, where inbox requests live.
	Metadata map[string]any
}

// Open reports whether the Issue is open.
func (i Issue) Open() bool { return i.Status == "open" }

// CreateIssue creates the per-run Issue, always keyed and with force_new (ADR-0002), and says whether kata made a new one.
func (k *Kata) CreateIssue(ctx context.Context, projectID int64, actor, key, title, body string, metadata map[string]any) (Issue, error) {
	force := true
	r, err := call(ctx, k, false, func(ctx context.Context) (*g.CreateIssueResp, error) {
		return k.c.CreateIssueWithResponse(ctx, &g.CreateIssueRequestOptions{
			PathParams: &g.CreateIssuePath{ProjectID: strconv.FormatInt(projectID, 10)},
			Body:       &g.CreateIssueBody{Actor: &actor, Title: title, Body: &body, ForceNew: &force, Metadata: metadata},
			Header:     &g.CreateIssueHeaders{IdempotencyKey: &key},
		})
	})
	if err != nil {
		return Issue{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Issue{}, errors.New("kata daemon returned no Issue")
	}
	i := r.JSON200.Issue
	return Issue{ID: i.ID, UID: i.UID, ShortID: i.ShortID, Title: i.Title, Body: i.Body, Status: i.Status, ProjectID: i.ProjectID}, nil
}

// ShowIssue reads an Issue by uid.
func (k *Kata) ShowIssue(ctx context.Context, projectID int64, uid string) (Issue, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ShowIssueResp, error) {
		return k.c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid}})
	})
	if err != nil {
		return Issue{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Issue{}, errors.New("kata daemon returned no Issue")
	}
	i := r.JSON200.Issue
	issue := Issue{ID: i.ID, UID: i.UID, ShortID: i.ShortID, Title: i.Title, Body: i.Body, Status: i.Status, ProjectID: i.ProjectID, Labels: i.Labels, Owner: deref(i.Owner), Author: i.Author, Metadata: i.Metadata}
	for _, c := range r.JSON200.Comments {
		issue.Comments = append(issue.Comments, c.Body)
	}
	return issue, nil
}

// PlanningDates are an Issue's scheduled and deadline instants as kata resolves them (the Issue's zone, the
// recurrence's, the daemon default, then UTC). A date the Issue lacks is the zero time.
type PlanningDates struct {
	Revision  int64
	Scheduled time.Time
	Deadline  time.Time
}

// PlanningDates reads the Issue's planning-date instants.
func (k *Kata) PlanningDates(ctx context.Context, projectID int64, issueUID string) (PlanningDates, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.IssuePlanningDatesResp, error) {
		return k.c.IssuePlanningDatesWithResponse(ctx, &g.IssuePlanningDatesRequestOptions{PathParams: &g.IssuePlanningDatesPath{ProjectID: projectID, Ref: issueUID}})
	})
	if err != nil {
		return PlanningDates{}, apiError(err)
	}
	if r.JSON200 == nil {
		return PlanningDates{}, errors.New("kata daemon returned no planning dates")
	}
	d := PlanningDates{Revision: r.JSON200.Revision}
	if s := r.JSON200.ScheduledOn; s != nil {
		d.Scheduled = s.Instant.UTC()
	}
	if dl := r.JSON200.DeadlineOn; dl != nil {
		d.Deadline = dl.Instant.UTC()
	}
	return d, nil
}

// inboxSlot is the metadata key of the inbox request for recipient: one slot per Issue and recipient.
func inboxSlot(recipient string) string { return "notify." + base64.RawURLEncoding.EncodeToString([]byte(recipient)) }

// InboxRequest is the request the Issue holds for recipient, ok false when there is none.
func (i Issue) InboxRequest(recipient string) (from, message string, ok bool) {
	slot, _ := i.Metadata[inboxSlot(recipient)].(map[string]any)
	if slot == nil {
		return "", "", false
	}
	from, _ = slot["from"].(string)
	message, _ = slot["message"].(string)
	return from, message, true
}

// Notify puts an inbox request for recipient on the Issue: the ordinary metadata write `kata notify` makes, one slot
// per Issue and recipient, so sending the same request again changes nothing.
func (k *Kata) Notify(ctx context.Context, projectID int64, actor, issueUID, recipient, teammate, message string) error {
	value := map[string]any{"from": actor, "message": message}
	if teammate != "" {
		value["teammate"] = teammate
	}
	_, err := call(ctx, k, false, func(ctx context.Context) (*g.PatchIssueMetadataResp, error) {
		return k.c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
			PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: issueUID},
			Body:       &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{inboxSlot(recipient): value}},
		})
	})
	return apiError(err)
}

// Comment adds a keyed comment, so a retry within kata's window adds nothing.
func (k *Kata) Comment(ctx context.Context, projectID int64, actor, issueUID, key, text string) error {
	_, err := call(ctx, k, false, func(ctx context.Context) (*g.CreateCommentResp, error) {
		return k.c.CreateCommentWithResponse(ctx, &g.CreateCommentRequestOptions{
			PathParams: &g.CreateCommentPath{ProjectID: strconv.FormatInt(projectID, 10), Ref: issueUID},
			Body:       &g.CreateCommentBody{Actor: &actor, Body: text},
			Header:     &g.CreateCommentHeaders{IdempotencyKey: &key},
		})
	})
	return apiError(err)
}

// AddLabel labels an Issue; adding one already there changes nothing.
func (k *Kata) AddLabel(ctx context.Context, projectID int64, actor, issueUID, label string) error {
	_, err := call(ctx, k, false, func(ctx context.Context) (*g.AddLabelResp, error) {
		return k.c.AddLabelWithResponse(ctx, &g.AddLabelRequestOptions{
			PathParams: &g.AddLabelPath{ProjectID: strconv.FormatInt(projectID, 10), Ref: issueUID},
			Body:       &g.AddLabelBody{Actor: &actor, Label: label},
		})
	})
	return apiError(err)
}

// Event is one frame of a Project's event stream.
type Event struct {
	ID   int64
	Type string
}

// ErrResetRequired ends a stream whose cursor kata no longer holds; the caller refreshes everything and reopens.
var ErrResetRequired = errors.New("kata asked for a full refresh (sync.reset_required)")

// Stream reads the Project's event stream from after until it ends, calling handle for each event. It returns
// ErrResetRequired on a stale cursor and the transport's error otherwise; cancelling ctx ends it (ADR-0004).
func (k *Kata) Stream(ctx context.Context, projectID, after int64, opened func(), handle func(Event)) error {
	response, err := k.c.StreamEventsRaw(ctx, &g.StreamEventsRequestOptions{Query: &g.StreamEventsQuery{AfterID: &after, ProjectID: &projectID}})
	if err != nil {
		return apiError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		var envelope g.ErrorEnvelope
		if json.Unmarshal(body, &envelope) == nil && envelope.ErrorData.Message != "" {
			return errors.New(envelope.ErrorData.Message)
		}
		return fmt.Errorf("kata event stream: HTTP %d", response.StatusCode)
	}
	opened()
	reader := bufio.NewReader(response.Body)
	var id, kind string
	var hasData bool
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if hasData || kind != "" {
				if kind == "sync.reset_required" {
					return ErrResetRequired
				}
				n, _ := strconv.ParseInt(id, 10, 64)
				handle(Event{ID: n, Type: kind})
			}
			id, kind, hasData = "", "", false
		case strings.HasPrefix(line, ":"):
		default:
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "id":
				id = value
			case "event":
				kind = value
			case "data":
				hasData = true
				if kind == "" {
					var e struct {
						Type string `json:"type"`
					}
					if json.Unmarshal([]byte(value), &e) == nil {
						kind = e.Type
					}
				}
			}
		}
	}
}

// WorkflowStep is one step of a kata workflow as the worker reads it. Options keeps the step's raw options so the
// worker can refuse the keys it does not run (ADR-0008).
type WorkflowStep struct {
	Key     string
	Kind    string // "prompt" or "command"
	Command string
	Prompt  string
	After   []string
	Retries int64
	Options map[string]any
}

// Workflow is a kata workflow definition at one version.
type Workflow struct {
	UID                string
	Name               string
	DefinitionEventUID string
	Deleted            bool
	Version            int64
	About              string
	Options            map[string]any
	Steps              []WorkflowStep
}

// Workflow reads one workflow of the Project.
func (k *Kata) Workflow(ctx context.Context, projectID int64, uid string) (Workflow, error) {
	r, err := call(ctx, k, true, func(ctx context.Context) (*g.ShowCronWorkflowResp, error) {
		return k.c.ShowCronWorkflowWithResponse(ctx, &g.ShowCronWorkflowRequestOptions{PathParams: &g.ShowCronWorkflowPath{ProjectID: projectID, CronUID: uid}})
	})
	if err != nil {
		return Workflow{}, apiError(err)
	}
	if r.JSON200 == nil {
		return Workflow{}, errors.New("kata daemon returned no workflow")
	}
	w := r.JSON200.Workflow
	d := w.Definition
	out := Workflow{UID: w.UID, Name: w.Name, DefinitionEventUID: w.DefinitionEventUID, Deleted: w.DeletedAt != nil, Version: d.Version, About: deref(d.About), Options: d.Options}
	for _, s := range d.Steps {
		out.Steps = append(out.Steps, WorkflowStep{Key: s.Key, Kind: s.Kind, Command: deref(s.Command), Prompt: deref(s.Prompt), After: s.After, Retries: derefInt(s.Retries), Options: s.Options})
	}
	return out, nil
}
