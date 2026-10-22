// Package workertest holds what tests of the worker (and of the CLI around it) share: kata in process, a stub agent
// program, a manual clock and builders for Projects and jobs. Production code never imports it.
package workertest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

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
}

// StartKata starts kata and points kata's discovery variables away from the real daemon. Its ping reports 0.19.0,
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": "0.19.0"})
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Daemon{Client: c, Kata: worker.NewKata(c), Server: server}
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
	Name        string
	Prompt      string
	Profile     string
	Checkout    string
	IssueTitle  string
	ExistingUID string // an Issue the job reports to, instead of a per-run Issue
	Overlap     string
	Timeout     int64
	Secrets     map[string]string
	// Trigger defaults to manual.
	Trigger worker.Trigger
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
	policy := &g.IssuePolicy{Kind: "per-run", Title: ptr(orDefault(j.IssueTitle, "Run of "+j.Name)), Body: ptr("job body"), ScheduledOffsetSeconds: j.ScheduledOffset, DeadlineOffsetSeconds: j.DeadlineOffset}
	if j.ExistingUID != "" {
		policy = &g.IssuePolicy{Kind: "existing", UID: &j.ExistingUID}
	}
	trigger := g.Trigger{Kind: orDefault(j.Trigger.Kind, "manual")}
	if t := j.Trigger; t.Kind != "" && t.Kind != "manual" {
		trigger.Cron, trigger.At, trigger.Timezone = optional(t.Cron), optional(t.At), optional(t.Timezone)
		if t.IntervalSeconds != 0 {
			trigger.IntervalSeconds = &t.IntervalSeconds
		}
		trigger.IssueUID = optional(t.IssueUID)
		if t.Kind == "issue-deadline" {
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
func (d *Daemon) CreateJob(t *testing.T, p worker.Project, j Job) worker.Job {
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
func (d *Daemon) ReplaceJob(t *testing.T, p worker.Project, found worker.Job, j Job) worker.Job {
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

func (d *Daemon) findJob(t *testing.T, p worker.Project, name string) worker.Job {
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
	return worker.Job{}
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
	actor := "tester"
	value := at.UTC().Format("2006-01-02T15:04")
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
func (d *Daemon) CreateNativeJob(t *testing.T, p worker.Project, name, parentUID string) worker.Job {
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
	jobs, _ := d.Kata.Jobs(context.Background(), p.ID)
	for _, found := range jobs {
		if found.Name == name {
			return found
		}
	}
	t.Fatalf("job %s not listed", name)
	return worker.Job{}
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

func ptr[T any](v T) *T { return &v }

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

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
