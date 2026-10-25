package data

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
)

func (f *jobsFixture) create(t *testing.T, name string, def cronjob.Definition, triggerRef, reportRef string) model.JobWritten {
	t.Helper()
	uid, err := client.NewCronUID()
	if err != nil {
		t.Fatal(err)
	}
	return f.save(t, model.SaveJobCmd{UID: uid, Name: name, Definition: def, IssueRef: triggerRef, ReportRef: reportRef})
}

func TestGatewayCreatesAdapterJobsKataAcceptsAndEachRoundTrips(t *testing.T) {
	f := newJobsFixture(t)
	issue := createIssue(t, f.gateway, f.workspace, f.demo, "Release 1.0")
	other := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	workflow := saveWorkflow(t, f.gateway, f.workspace, f.demo, "", "", reviewThenTest).Workflow
	if workflow == nil {
		t.Fatal("no workflow to run")
	}

	prompt := cronjob.Definition{
		Enabled: true, Overlap: "forbid", Catchup: model.CatchupSkip, GraceSeconds: 300, TimeoutSeconds: 1800,
		Trigger:     cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * 1-5", Timezone: "Europe/Berlin"},
		Action:      cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "triage the inbox\nthen report"},
		Issue:       &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, Title: "Triage", Body: "Look at everything new", ScheduledOffsetSeconds: new(int64(86400)), DeadlineOffsetSeconds: new(int64(-3600))},
		CheckoutKey: "repo", SecretRefs: map[string]string{"GH_TOKEN": "github"}, Options: map[string]any{"profile": "fast", "effort": "high"}, Profile: "fast",
	}
	byWorkflow := cronjob.Definition{
		Enabled: false, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerOnce, At: "2026-11-01T09:00:00+01:00"},
		Action:  cronjob.Action{Kind: cronjob.ActionExecute, WorkflowUID: workflow.UID},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssueExisting},
	}
	notify := cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupAll,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 30},
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "alice/reviewer", Message: "The report is due"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssueExisting},
	}
	deadline := cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, LeadSeconds: 7200},
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"},
	}
	scheduled := cronjob.Definition{
		Enabled: true, Overlap: "forbid", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueScheduled},
		Action:  cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "start on the day"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssueExisting},
	}
	notifyOther := cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline},
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssueExisting},
	}
	manual := cronjob.Definition{
		Enabled: true, Overlap: "forbid", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerManual},
		Action:  cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "on demand"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, Title: "On demand"},
	}

	for _, tt := range []struct {
		name string
		def  cronjob.Definition
		// ref is the Issue the trigger or the policy names; report is a notify job's own Issue when its trigger fires on another's.
		ref, report string
		kind        string
		wantD       func(cronjob.Definition) cronjob.Definition
	}{
		{"prompt", prompt, "", "", model.KindAgentPrompt, nil},
		{"workflow", byWorkflow, issue, "", model.KindWorkflow, func(d cronjob.Definition) cronjob.Definition { d.Issue.UID = issue; return d }},
		{"notify", notify, issue, "", model.KindNotify, func(d cronjob.Definition) cronjob.Definition { d.Issue.UID = issue; return d }},
		{"deadline", deadline, issue, "", model.KindNotify, func(d cronjob.Definition) cronjob.Definition { d.Trigger.IssueUID = issue; return d }},
		{"scheduled", scheduled, issue, "", model.KindAgentPrompt, func(d cronjob.Definition) cronjob.Definition {
			d.Trigger.IssueUID = issue
			d.Issue.UID = issue
			return d
		}},
		{"deadline of one Issue notifying another", notifyOther, issue, other, model.KindNotify, func(d cronjob.Definition) cronjob.Definition {
			d.Trigger.IssueUID = issue
			d.Issue.UID = other
			return d
		}},
		{"manual", manual, "", "", model.KindAgentPrompt, nil},
	} {
		written := f.create(t, "Job "+tt.name, tt.def, tt.ref, tt.report)
		if written.Err != nil || written.Job == nil || written.Op != model.JobCreated {
			t.Fatalf("%s: create %+v", tt.name, written)
		}
		want := tt.def
		if tt.wantD != nil {
			want = tt.wantD(copyDefinition(tt.def))
		}
		listed := jobNamed(t, f.list(t, false).Jobs, "Job "+tt.name)
		for _, got := range []model.CronJob{*written.Job, listed} {
			if got.Kind() != tt.kind || got.Native() || got.Parent != nil || !reflect.DeepEqual(got.Definition, want) {
				t.Errorf("%s round trip:\n got %+v issue %+v\nwant %+v issue %+v", tt.name, got.Definition, got.Definition.Issue, want, want.Issue)
			}
		}
	}

	// An edit replaces the definition; options the card does not edit ride along.
	job := jobNamed(t, f.list(t, false).Jobs, "Job prompt")
	edited := job.Definition
	edited.Options, edited.Profile = map[string]any{"profile": "slow", "effort": "high"}, "slow"
	replaced := f.save(t, model.SaveJobCmd{UID: job.UID, ExpectedEventUID: job.DefinitionEventUID, Name: job.Name, Definition: edited})
	if replaced.Err != nil || replaced.Job == nil || replaced.Job.Definition.Profile != "slow" || replaced.Job.Definition.Options["effort"] != "high" {
		t.Fatalf("replace %+v", replaced)
	}
}

func copyDefinition(d cronjob.Definition) cronjob.Definition {
	if d.Issue != nil {
		p := *d.Issue
		d.Issue = &p
	}
	return d
}

func TestGatewayRefusesAnAdapterJobKataRejectsAndNamesTheIssueThatDoesNotExist(t *testing.T) {
	f := newJobsFixture(t)
	issue := createIssue(t, f.gateway, f.workspace, f.demo, "Release 1.0")

	execute := cronjob.Definition{
		Enabled: true, Overlap: "forbid", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline},
		Action:  cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "run"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssueExisting},
	}
	if refused := f.create(t, "Deadline run", execute, issue, ""); refused.Err == nil || !strings.Contains(refused.Err.Error(), "notification-only") {
		t.Fatalf("an executing deadline job: %+v", refused)
	}
	execute.Trigger = cronjob.Trigger{Kind: cronjob.TriggerIssueScheduled}
	if refused := f.create(t, "Scheduled run", execute, "zzzz", ""); refused.Err == nil || !strings.Contains(refused.Err.Error(), `Target Issue: no Issue "zzzz"`) {
		t.Fatalf("an Issue that does not exist: %+v", refused)
	}
	if jobs := f.list(t, false).Jobs; len(jobs) != 0 {
		t.Fatalf("a refused create left %d jobs", len(jobs))
	}
}

func TestGatewayReadsTheWorkersProfilesAndCheckoutsForThePickers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
	askedFor := func() model.WorkerPickers {
		return as[model.WorkerPickersLoaded](t, run(t, &Gateway{}, model.LoadWorkerPickers{})).Pickers
	}
	if got := askedFor(); got.Read || got.Note != "" || len(got.Profiles) != 0 {
		t.Fatalf("no worker.toml: %+v, want free text and no complaint", got)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(base, "tern-kata")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `executor = "laptop"

[profiles.default]
[profiles.fast]
model = "haiku"

[checkouts]
site = '` + filepath.Join(dir, "site") + `'
repo = '` + filepath.Join(dir, "repo") + `'

[[projects]]
name = "demo"
`
	if err := os.WriteFile(filepath.Join(folder, "worker.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := askedFor(); !got.Read || !reflect.DeepEqual(got.Profiles, []string{"default", "fast"}) || !reflect.DeepEqual(got.Checkouts, []string{"repo", "site"}) {
		t.Fatalf("worker.toml read as %+v", got)
	}

	if err := os.WriteFile(filepath.Join(folder, "worker.toml"), []byte(config+"limits = 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := askedFor(); got.Read || !strings.Contains(got.Note, "limits") {
		t.Fatalf("a config the worker would refuse: %+v, want free text and the reason", got)
	}
}
