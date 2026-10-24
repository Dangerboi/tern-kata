// Package workertest holds what tests of the worker (and of the CLI around it) share: kata in process, a stub agent
// program, a manual clock and builders for Projects and jobs. Production code never imports it.
package workertest

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Daemon is kata running in this process on a temporary database.
type Daemon struct {
	Client *client.Client
	Kata   *worker.Kata
	Server *httptest.Server

	mu       sync.Mutex
	rules    []*rule
	requests []string
	inflight int
	peak     int
	gate     *Gate
	failing  func(*http.Request) bool
}

// Gate holds every request its match accepts until the test lets it through, so a test can freeze the worker at a
// chosen call to kata and run another request meanwhile.
type Gate struct {
	match func(*http.Request) bool
	// Arrived receives one value per held request (never blocks the daemon: it is buffered).
	Arrived chan struct{}
	pass    chan struct{}
	open    chan struct{}
	once    sync.Once
}

// Hold starts holding the requests match accepts.
func (d *Daemon) Hold(match func(*http.Request) bool) *Gate {
	g := &Gate{match: match, Arrived: make(chan struct{}, 64), pass: make(chan struct{}, 64), open: make(chan struct{})}
	d.mu.Lock()
	d.gate = g
	d.mu.Unlock()
	return g
}

// Release lets one held request through.
func (g *Gate) Release() { g.pass <- struct{}{} }

// Open lets every held and later request through.
func (g *Gate) Open() { g.once.Do(func() { close(g.open) }) }

// Fail answers 503 to every request match accepts until Heal.
func (d *Daemon) Fail(match func(*http.Request) bool) {
	d.mu.Lock()
	d.failing = match
	d.mu.Unlock()
}

// Heal stops what Fail started.
func (d *Daemon) Heal() { d.Fail(nil) }

func (d *Daemon) intercept(w http.ResponseWriter, r *http.Request) (served bool) {
	d.mu.Lock()
	g, failing := d.gate, d.failing
	d.mu.Unlock()
	if g != nil && g.match(r) {
		select {
		case g.Arrived <- struct{}{}:
		default:
		}
		select {
		case <-g.pass:
		case <-g.open:
		case <-r.Context().Done():
			return true
		}
	}
	if failing != nil && failing(r) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return true
	}
	return false
}

type rule struct {
	match  func(*http.Request) bool
	handle func(w http.ResponseWriter, r *http.Request, next http.Handler)
}

// StartKata starts kata and points kata's discovery variables away from the real daemon. Its ping reports 0.20.0,
// as the in-process service reports no version.
func StartKata(t *testing.T) *Daemon {
	t.Helper()
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	service, err := kata.New(context.Background(), kata.Config{
		DSN:  filepath.Join(t.TempDir(), "kata.db"),
		Auth: kata.AuthConfig{TrustCallerAuthentication: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	handler := service.Handler()
	d := &Daemon{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": "0.20.0"})
			return
		}
		if d.intercept(w, r) {
			return
		}
		handler.ServeHTTP(w, r)
	})
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream := strings.Contains(r.URL.Path, "/events")
		d.mu.Lock()
		d.requests = append(d.requests, r.Method+" "+r.URL.Path)
		var intercepted func(http.ResponseWriter, *http.Request, http.Handler)
		for _, rule := range d.rules {
			if rule.match(r) {
				intercepted = rule.handle
				break
			}
		}
		if !stream {
			d.inflight++
			d.peak = max(d.peak, d.inflight)
		}
		d.mu.Unlock()
		if !stream {
			defer func() {
				d.mu.Lock()
				d.inflight--
				d.mu.Unlock()
			}()
		}
		if intercepted != nil {
			intercepted(w, r, next)
			return
		}
		next.ServeHTTP(w, r)
	}))
	t.Cleanup(d.Server.Close)
	c, err := client.New(d.Server.URL)
	if err != nil {
		t.Fatal(err)
	}
	d.Client, d.Kata = c, worker.NewKata(c)
	return d
}

// Intercept answers every request match accepts with handle instead of kata, until the returned function is called;
// handle may pass the request on through next. A handler that waits should also watch r.Context(), which ends when
// the caller gives up.
func (d *Daemon) Intercept(match func(*http.Request) bool, handle func(w http.ResponseWriter, r *http.Request, next http.Handler)) (remove func()) {
	r := &rule{match: match, handle: handle}
	d.mu.Lock()
	d.rules = append(d.rules, r)
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.rules = slices.DeleteFunc(d.rules, func(x *rule) bool { return x == r })
	}
}

// Requests counts the requests so far whose method and path ("GET /api/v1/projects") contain text.
func (d *Daemon) Requests(text string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, r := range d.requests {
		if strings.Contains(r, text) {
			n++
		}
	}
	return n
}

// PeakInFlight is the most requests (event streams aside) that were being answered at once.
func (d *Daemon) PeakInFlight() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peak
}

// Project creates (or finds) a Project.
func (d *Daemon) Project(t *testing.T, name string) worker.Project {
	t.Helper()
	actor := "tester"
	r, err := d.Client.InitProjectWithResponse(context.Background(), &g.InitProjectRequestOptions{Body: &g.InitProjectBody{Name: &name, Actor: &actor}})
	if err != nil || r.JSON200 == nil {
		t.Fatalf("creating Project %s: %v", name, err)
	}
	p := r.JSON200.Project
	return worker.Project{ID: p.ID, UID: p.UID, Name: p.Name, Revision: p.Revision}
}

// Job describes a prompt job to create. Zero values give an enabled manual-trigger job with a per-run Issue.
type Job struct {
	Name   string
	Prompt string
	// Workflow makes the job run the workflow with this uid instead of Prompt.
	Workflow    string
	Profile     string
	Checkout    string
	IssueTitle  string
	ExistingUID string // an Issue the job reports to, instead of a per-run Issue
	Overlap     string
	Timeout     int64
	Secrets     map[string]string
	// Trigger defaults to manual.
	Trigger cronjob.Trigger
	// Catchup defaults to skip; GraceSeconds to 60.
	Catchup      string
	GraceSeconds *int64
	Disabled     bool
	// Notify makes the job a notify job on ExistingUID (or on Trigger.IssueUID for an Issue-date trigger) instead of a
	// prompt job.
	Notify *Notify
	// ScheduledOffset and DeadlineOffset are the per-run Issue's date offsets in seconds.
	ScheduledOffset, DeadlineOffset *int64
}

// Notify is a notify action: the inbox address (or "current-owner-or-author") and the text.
type Notify struct{ Recipient, Message string }

// Seconds is a pointer to n, for the optional numeric fields of Job.
func Seconds(n int64) *int64 { return &n }

func (j Job) definition() g.CronJobDefinition {
	if j.Overlap == "" {
		j.Overlap = "allow"
	}
	if j.Catchup == "" {
		j.Catchup = "skip"
	}
	if j.GraceSeconds == nil {
		j.GraceSeconds = ptr(int64(60))
	}
	policy := &g.IssuePolicy{Kind: "per-run", Title: ptr(cmp.Or(j.IssueTitle, "Run of "+j.Name)), Body: ptr("job body"), ScheduledOffsetSeconds: j.ScheduledOffset, DeadlineOffsetSeconds: j.DeadlineOffset}
	if j.ExistingUID != "" {
		policy = &g.IssuePolicy{Kind: "existing", UID: &j.ExistingUID}
	}
	trigger := g.Trigger{Kind: cmp.Or(string(j.Trigger.Kind), "manual")}
	if t := j.Trigger; t.Kind != "" && t.Kind != cronjob.TriggerManual {
		trigger.Cron, trigger.At, trigger.Timezone = optional(t.Cron), optional(t.At), optional(t.Timezone)
		if t.IntervalSeconds != 0 {
			trigger.IntervalSeconds = &t.IntervalSeconds
		}
		trigger.IssueUID = optional(t.IssueUID)
		if t.Kind == cronjob.TriggerIssueDeadline {
			trigger.LeadSeconds = &t.LeadSeconds
		}
	}
	def := g.CronJobDefinition{
		Version: 1, Kind: "job", Enabled: !j.Disabled, Trigger: trigger,
		Action: g.Action{Kind: "execute", Prompt: &j.Prompt}, Issue: policy, Overlap: j.Overlap, Catchup: j.Catchup, GraceSeconds: j.GraceSeconds,
		SecretRefs: j.Secrets,
	}
	if n := j.Notify; n != nil {
		def.Action = g.Action{Kind: "notify", Recipient: &n.Recipient, Message: optional(n.Message)}
		def.Issue, def.GraceSeconds = nil, j.GraceSeconds
		if j.ExistingUID != "" {
			def.Issue = &g.IssuePolicy{Kind: "existing", UID: &j.ExistingUID}
		}
	}
	if j.Workflow != "" {
		def.Action = g.Action{Kind: "execute", WorkflowUID: &j.Workflow}
	}
	if j.Profile != "" {
		def.Options = map[string]any{"profile": j.Profile}
	}
	if j.Checkout != "" {
		def.CheckoutKey = &j.Checkout
	}
	if j.Timeout > 0 {
		def.TimeoutSeconds = &j.Timeout
	}
	return def
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// CreateJob creates an adapter prompt job in the Project.
func (d *Daemon) CreateJob(t *testing.T, p worker.Project, j Job) cronjob.Job {
	t.Helper()
	actor := "tester"
	if _, err := d.Client.CreateCronJobWithResponse(context.Background(), &g.CreateCronJobRequestOptions{
		PathParams: &g.CreateCronJobPath{ProjectID: p.ID}, Body: &g.CreateCronJobBody{Actor: &actor, Name: j.Name, Definition: j.definition()},
	}); err != nil {
		t.Fatalf("creating job %s: %v", j.Name, err)
	}
	return d.findJob(t, p, j.Name)
}

// ReplaceJob edits a job the way the Jobs Page does: the whole definition is replaced by j's, on the version found.
func (d *Daemon) ReplaceJob(t *testing.T, p worker.Project, found cronjob.Job, j Job) cronjob.Job {
	t.Helper()
	actor := "tester"
	if _, err := d.Client.ReplaceCronJobWithResponse(context.Background(), &g.ReplaceCronJobRequestOptions{
		PathParams: &g.ReplaceCronJobPath{ProjectID: p.ID, CronUID: found.UID},
		Body:       &g.ReplaceCronJobBody{Actor: &actor, Name: found.Name, Definition: j.definition(), ExpectedEventUID: &found.DefinitionEventUID},
	}); err != nil {
		t.Fatalf("replacing job %s: %v", found.Name, err)
	}
	return d.findJob(t, p, found.Name)
}

func (d *Daemon) findJob(t *testing.T, p worker.Project, name string) cronjob.Job {
	t.Helper()
	jobs, err := d.Kata.Jobs(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, found := range jobs {
		if found.Name == name {
			return found
		}
	}
	t.Fatalf("job %s not listed", name)
	return cronjob.Job{}
}

// PlanningDates reads an Issue's scheduled and deadline instants; a date the Issue lacks is the zero time.
func (d *Daemon) PlanningDates(t *testing.T, p worker.Project, issueUID string) (scheduled, deadline time.Time) {
	t.Helper()
	r, err := d.Client.IssuePlanningDatesWithResponse(context.Background(), &g.IssuePlanningDatesRequestOptions{PathParams: &g.IssuePlanningDatesPath{ProjectID: p.ID, Ref: issueUID}})
	if err != nil || r.JSON200 == nil {
		t.Fatalf("reading the planning dates of %s: %v", issueUID, err)
	}
	if s := r.JSON200.ScheduledOn; s != nil {
		scheduled = s.Instant
	}
	if dl := r.JSON200.DeadlineOn; dl != nil {
		deadline = dl.Instant
	}
	return scheduled, deadline
}

// SetIssueDate sets the Issue's scheduled_on or deadline_on to the minute containing t, read by kata as UTC.
func (d *Daemon) SetIssueDate(t *testing.T, p worker.Project, issueUID, field string, at time.Time) {
	t.Helper()
	d.patchIssueDate(t, p, issueUID, field, at.UTC().Format("2006-01-02T15:04"))
}

// ClearIssueDate removes one planning date (scheduled_on or deadline) from an Issue.
func (d *Daemon) ClearIssueDate(t *testing.T, p worker.Project, issueUID, field string) {
	t.Helper()
	d.patchIssueDate(t, p, issueUID, field, nil)
}

func (d *Daemon) patchIssueDate(t *testing.T, p worker.Project, issueUID, field string, value any) {
	t.Helper()
	actor := "tester"
	if _, err := d.Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: p.ID, Ref: issueUID}, Body: &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{field: value}},
	}); err != nil {
		t.Fatalf("setting %s of %s: %v", field, issueUID, err)
	}
}

// InboxRequest is the notification an Issue holds for a recipient: its sender and text, ok false when there is none.
func (d *Daemon) InboxRequest(t *testing.T, p worker.Project, issueUID, recipient string) (from, message string, ok bool) {
	t.Helper()
	r, err := d.Client.ShowIssueWithResponse(context.Background(), &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: p.ID, Ref: issueUID}})
	if err != nil || r.JSON200 == nil {
		t.Fatalf("reading Issue %s: %v", issueUID, err)
	}
	slot, _ := r.JSON200.Issue.Metadata["notify."+base64.RawURLEncoding.EncodeToString([]byte(recipient))].(map[string]any)
	if slot == nil {
		return "", "", false
	}
	from, _ = slot["from"].(string)
	message, _ = slot["message"].(string)
	return from, message, true
}

// CreateNativeJob creates a recurring-child job under parent, which kata's daemon delivers and the worker never runs.
func (d *Daemon) CreateNativeJob(t *testing.T, p worker.Project, name, parentUID string) cronjob.Job {
	t.Helper()
	actor := "tester"
	def := g.CronJobDefinition{
		Version: 1, Kind: "job", Enabled: true, Trigger: g.Trigger{Kind: "interval", IntervalSeconds: ptr(int64(3600))},
		Action:  g.Action{Kind: "notify", Recipient: ptr("current-owner-or-author")},
		Issue:   &g.IssuePolicy{Kind: "per-run", ParentUID: &parentUID, Title: ptr("Child")},
		Overlap: "allow", Catchup: "all",
	}
	if _, err := d.Client.CreateCronJobWithResponse(context.Background(), &g.CreateCronJobRequestOptions{
		PathParams: &g.CreateCronJobPath{ProjectID: p.ID}, Body: &g.CreateCronJobBody{Actor: &actor, Name: name, Definition: def},
	}); err != nil {
		t.Fatalf("creating native job: %v", err)
	}
	return d.findJob(t, p, name)
}

// Issue creates an Issue in the Project and returns it.
func (d *Daemon) Issue(t *testing.T, p worker.Project, title string) worker.Issue {
	t.Helper()
	issue, err := d.Kata.CreateIssue(context.Background(), p.ID, "tester", "", title, "body", nil)
	if err != nil {
		t.Fatal(err)
	}
	return issue
}

// CloseIssue closes the Issue as done.
func (d *Daemon) CloseIssue(t *testing.T, p worker.Project, issueUID string) {
	t.Helper()
	reason := g.CloseActionRequestBodyReason("done")
	protocol := g.CloseActionRequestBodyRetryProtocol("close-v1")
	source := g.CloseActionRequestBodySource("tui")
	actor, key := "tester", "close-"+issueUID
	if _, err := d.Client.CloseIssueWithResponse(context.Background(), &g.CloseIssueRequestOptions{
		PathParams: &g.CloseIssuePath{ProjectID: p.ID, Ref: issueUID}, Header: &g.CloseIssueHeaders{IdempotencyKey: &key},
		Body: &g.CloseIssueBody{Actor: &actor, Reason: &reason, RetryProtocol: &protocol, Source: &source},
	}); err != nil {
		t.Fatalf("closing Issue %s: %v", issueUID, err)
	}
}

func ptr[T any](v T) *T { return &v }

// BuildStub compiles the stub agent program and returns its path.
func BuildStub(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	out := filepath.Join(t.TempDir(), "stub")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "./internal/worker/workertest/stub")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the stub agent: %v\n%s", err, output)
	}
	return out
}

// ManualClock is a Clock a test advances by hand.
type ManualClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

// NewManualClock starts at the real now, so timestamps kata also stamps stay comparable.
func NewManualClock() *ManualClock {
	return &ManualClock{now: time.Now().UTC().Truncate(time.Millisecond)}
}

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), ch: ch})
	return ch
}

// Advance moves the clock and releases every wait that is now due.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

// WriteConfig writes worker.toml into a fresh config folder and returns the folders.
func WriteConfig(t *testing.T, toml string) worker.Dirs {
	t.Helper()
	dirs := worker.Dirs{Config: t.TempDir(), State: shortTemp(t)}
	if err := os.WriteFile(dirs.ConfigPath(), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	return dirs
}

// shortTemp is a folder whose path is short enough for a unix socket.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "tk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Step is one step of a workflow to create: a command step when Command is set, else a prompt step.
type Step struct {
	Key     string
	Command string
	Prompt  string
	After   []string
	Retries int64
	Options map[string]any
}

// CreateWorkflow creates a workflow in the Project and returns it as the worker reads it.
func (d *Daemon) CreateWorkflow(t *testing.T, p worker.Project, name string, steps ...Step) worker.Workflow {
	t.Helper()
	actor := "tester"
	def := g.CronWorkflowDefinition{Version: 1, About: ptr("workflow about")}
	for _, s := range steps {
		step := g.CronWorkflowStep{Key: s.Key, Kind: "prompt", Prompt: optional(s.Prompt), After: s.After, Options: s.Options}
		if s.Command != "" {
			step.Kind, step.Command = "command", &s.Command
		}
		if s.Retries > 0 {
			step.Retries = &s.Retries
		}
		def.Steps = append(def.Steps, step)
	}
	r, err := d.Client.CreateCronWorkflowWithResponse(context.Background(), &g.CreateCronWorkflowRequestOptions{
		PathParams: &g.CreateCronWorkflowPath{ProjectID: p.ID}, Body: &g.CreateCronWorkflowBody{Actor: &actor, Name: name, Definition: def},
	})
	if err != nil || r.JSON201 == nil {
		t.Fatalf("creating workflow %s: %v (%d %s)", name, err, r.StatusCode, r.Body)
	}
	wf, err := d.Kata.Workflow(context.Background(), p.ID, r.JSON201.Workflow.UID)
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

// EditIssue changes an Issue's title and owner as a person would; an empty argument leaves that field alone.
func (d *Daemon) EditIssue(t *testing.T, p worker.Project, issueUID, title, owner string) {
	t.Helper()
	actor := "tester"
	body := &g.EditIssueBody{Actor: &actor}
	if title != "" {
		body.Title = &title
	}
	if owner != "" {
		body.Owner = &owner
	}
	r, err := d.Client.EditIssueWithResponse(context.Background(), &g.EditIssueRequestOptions{
		PathParams: &g.EditIssuePath{ProjectID: strconv.FormatInt(p.ID, 10), Ref: issueUID}, Body: body,
	})
	if err != nil || r.JSON200 == nil {
		t.Fatalf("editing Issue %s: %v", issueUID, err)
	}
}
