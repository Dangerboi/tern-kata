package data

import (
	"context"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// jobsFixture is a Project on kata's real service, with helpers that write Cron jobs the way kata's own CLI does.
type jobsFixture struct {
	gateway   *Gateway
	client    *client.Client
	workspace string
	demo      model.Project
}

func newJobsFixture(t *testing.T) *jobsFixture {
	t.Helper()
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	return &jobsFixture{gateway: gateway, client: kata.target().Client, workspace: workspace, demo: demo}
}

func (f *jobsFixture) put(t *testing.T, name string, def g.CronJobDefinition) g.CronJob {
	t.Helper()
	uid, err := client.NewCronUID()
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.client.ReplaceCronJobWithResponse(context.Background(), &g.ReplaceCronJobRequestOptions{
		PathParams: &g.ReplaceCronJobPath{ProjectID: f.demo.ID, CronUID: uid},
		Body:       &g.PutCronJobBody{Actor: new("tester"), Name: name, Definition: def},
	})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("put job %s: %v", name, err)
	}
	return response.JSON200.Job
}

// native is a recurring-child job: kata's daemon delivers it, so it needs a parent Issue.
func (f *jobsFixture) native(t *testing.T, name, parentUID string) g.CronJob {
	return f.put(t, name, g.CronJobDefinition{
		Version: 1, Kind: "job", Enabled: true, Overlap: "allow", Catchup: "all",
		Trigger: g.Trigger{Kind: "interval", IntervalSeconds: new(int64(3600))},
		Action:  g.Action{Kind: "notify", Recipient: new("current-owner-or-author")},
		Issue:   &g.IssuePolicy{Kind: "per-run", ParentUID: &parentUID, Title: new("Tick")},
	})
}

// adapter is an agent prompt job, which only a worker runs.
func (f *jobsFixture) adapter(t *testing.T, name string) g.CronJob {
	return f.put(t, name, g.CronJobDefinition{
		Version: 1, Kind: "job", Enabled: true, Overlap: "forbid", Catchup: "skip", GraceSeconds: new(int64(60)),
		Trigger: g.Trigger{Kind: "cron", Cron: new("0 9 * * 1-5"), Timezone: new("Europe/Berlin")},
		Action:  g.Action{Kind: "execute", Prompt: new("triage the inbox")},
		Issue:   &g.IssuePolicy{Kind: "per-run", Title: new("Triage")},
		Options: map[string]any{"profile": "fast"},
	})
}

func (f *jobsFixture) list(t *testing.T, archived bool) model.JobsLoaded {
	t.Helper()
	loaded := as[model.JobsLoaded](t, run(t, f.gateway, model.LoadJobs{Workspace: f.workspace, ProjectID: f.demo.ID, Archived: archived, Request: 7}))
	if loaded.Err != nil || loaded.Request != 7 {
		t.Fatalf("list jobs %+v", loaded)
	}
	return loaded
}

func (f *jobsFixture) archive(t *testing.T, job g.CronJob) {
	t.Helper()
	_, err := f.client.ArchiveCronJobWithResponse(context.Background(), &g.ArchiveCronJobRequestOptions{
		PathParams: &g.ArchiveCronJobPath{ProjectID: f.demo.ID, CronUID: job.UID},
		Body:       &g.ArchiveCronJobBody{Actor: new("tester"), ExpectedEventUID: job.DefinitionEventUID},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func jobNamed(t *testing.T, jobs []model.CronJob, name string) model.CronJob {
	t.Helper()
	for _, job := range jobs {
		if job.Name == name {
			return job
		}
	}
	t.Fatalf("no job named %q in %+v", name, jobs)
	return model.CronJob{}
}

func TestGatewayListsNativeAndAdapterJobsWithTheParentsStatusAndProgress(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	started := time.Now()
	nativeWire := f.native(t, "Report", parent)
	f.adapter(t, "Triage")

	jobs := f.list(t, false).Jobs
	if len(jobs) != 2 || jobs[0].Name != "Report" || jobs[1].Name != "Triage" {
		t.Fatalf("jobs %+v, want both in name order", jobs)
	}
	native, adapter := jobs[0], jobs[1]
	if !native.Native() || native.Kind() != model.KindRecurringChild || native.UID != nativeWire.UID || native.DefinitionEventUID != nativeWire.DefinitionEventUID {
		t.Fatalf("native job %+v", native)
	}
	p := native.Parent
	if p == nil || p.UID != parent || p.Title != "Weekly report" || p.ShortID == "" || p.Status != "open" || p.Deleted {
		t.Fatalf("parent %+v", p)
	}
	if p.Progress == nil || p.Progress.Anchor.Before(started.Add(-time.Minute)) || p.Progress.Anchor.After(time.Now()) || !p.Progress.Through.Equal(p.Progress.Anchor) {
		t.Fatalf("progress %+v: a new job's delivery starts at its creation", p.Progress)
	}
	if got := native.Definition.Trigger; got.Kind != cronjob.TriggerInterval || got.IntervalSeconds != 3600 {
		t.Fatalf("trigger %+v", got)
	}
	if adapter.Native() || adapter.Parent != nil || adapter.Kind() != model.KindAgentPrompt || adapter.Definition.Profile != "fast" {
		t.Fatalf("adapter job %+v", adapter)
	}
	if d := adapter.Definition; d.Action.Prompt != "triage the inbox" || d.Trigger.Cron != "0 9 * * 1-5" || d.Trigger.Timezone != "Europe/Berlin" || d.GraceSeconds != 60 || d.Issue == nil || d.Issue.Title != "Triage" {
		t.Fatalf("adapter definition %+v", d)
	}
}

func TestGatewayReadsTheParentsCurrentStatus(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	f.native(t, "Report", parent)
	helper := &childCountFixture{gateway: f.gateway, workspace: f.workspace}

	helper.setClosed(t, parent, f.demo, true)
	if p := f.list(t, false).Jobs[0].Parent; p == nil || p.Status != "closed" || p.Deleted || p.Progress == nil {
		t.Fatalf("closed parent %+v, want its status and the progress still readable", p)
	}
	helper.remove(t, parent, f.demo)
	if p := f.list(t, false).Jobs[0].Parent; p == nil || !p.Deleted || p.UID != parent {
		t.Fatalf("deleted parent %+v", p)
	}
}

func TestGatewayListsArchivedJobsOnlyWhenAsked(t *testing.T) {
	f := newJobsFixture(t)
	f.adapter(t, "Live")
	f.archive(t, f.adapter(t, "Gone"))

	if jobs := f.list(t, false).Jobs; len(jobs) != 1 || jobs[0].Name != "Live" || jobs[0].Archived() {
		t.Fatalf("default list %+v", jobs)
	}
	jobs := f.list(t, true).Jobs
	if len(jobs) != 2 || !jobNamed(t, jobs, "Gone").Archived() || jobNamed(t, jobs, "Live").Archived() {
		t.Fatalf("list with archived %+v", jobs)
	}
}

func TestGatewayAnArchivedProjectHasNoReadableJobsAndSaysSo(t *testing.T) {
	f := newJobsFixture(t)
	f.adapter(t, "Triage")
	if _, err := f.client.RemoveProjectWithResponse(context.Background(), &g.RemoveProjectRequestOptions{
		PathParams: &g.RemoveProjectPath{ProjectID: f.demo.ID}, Query: &g.RemoveProjectQuery{Actor: "tester"},
	}); err != nil {
		t.Fatal(err)
	}

	loaded := as[model.JobsLoaded](t, run(t, f.gateway, model.LoadJobs{Workspace: f.workspace, ProjectID: f.demo.ID, Request: 3}))
	if loaded.Err != nil || !loaded.ProjectArchived || len(loaded.Jobs) != 0 || loaded.Request != 3 {
		t.Fatalf("archived Project %+v", loaded)
	}
	unknown := as[model.JobsLoaded](t, run(t, f.gateway, model.LoadJobs{Workspace: f.workspace, ProjectID: f.demo.ID + 100, Request: 4}))
	if unknown.Err == nil || unknown.ProjectArchived || unknown.Request != 4 {
		t.Fatalf("unknown Project %+v, want kata's error and no archive claim", unknown)
	}
}
