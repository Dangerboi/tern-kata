package data

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
)

func nativeDefinition(title string) cronjob.Definition {
	return cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600},
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, Title: title},
	}
}

func (f *jobsFixture) save(t *testing.T, cmd model.SaveJobCmd) model.JobWritten {
	t.Helper()
	cmd.Workspace, cmd.ProjectID, cmd.Actor = f.workspace, f.demo.ID, "tester"
	return as[model.JobWritten](t, run(t, f.gateway, cmd))
}

func (f *jobsFixture) created(t *testing.T, name, parentRef string) model.CronJob {
	t.Helper()
	uid, err := client.NewCronUID()
	if err != nil {
		t.Fatal(err)
	}
	written := f.save(t, model.SaveJobCmd{UID: uid, Name: name, Definition: nativeDefinition("Tick"), ParentRef: parentRef})
	if written.Err != nil || written.Job == nil {
		t.Fatalf("create %+v", written)
	}
	return *written.Job
}

func TestGatewayCreatesANativeJobAtTheCardsUIDResolvingTheParentFromAShortID(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	first := f.created(t, "Report", parent)
	if !first.Native() || first.Parent == nil || first.Parent.UID != parent || first.Parent.Progress == nil || first.Definition.Issue.ParentUID != parent || first.Revision != 1 || first.DefinitionEventUID == "" {
		t.Fatalf("created job %+v parent %+v", first, first.Parent)
	}
	// The short id, with or without its #, names the same parent.
	for _, ref := range []string{first.Parent.ShortID, "#" + first.Parent.ShortID} {
		if again := f.created(t, "Again "+ref, ref); again.Parent.UID != parent {
			t.Fatalf("ref %q resolved to %q, want %q", ref, again.Parent.UID, parent)
		}
	}
	if jobs := f.list(t, false).Jobs; len(jobs) != 3 {
		t.Fatalf("listed %d jobs, want the three created", len(jobs))
	}
}

func TestGatewayAResentCreateThatLandedIsTheSameJobNotAConflict(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	uid, _ := client.NewCronUID()
	cmd := model.SaveJobCmd{UID: uid, Name: "Report", Definition: nativeDefinition("Tick"), ParentRef: parent}
	first := f.save(t, cmd)
	again := f.save(t, cmd)
	if first.Err != nil || again.Err != nil || again.Changed != nil || again.Job == nil || again.Op != model.JobCreated || again.Job.DefinitionEventUID != first.Job.DefinitionEventUID {
		t.Fatalf("first %+v again %+v, want the second to answer the job the first made", first, again)
	}
	if jobs := f.list(t, false).Jobs; len(jobs) != 1 {
		t.Fatalf("listed %d jobs after a resend, want one", len(jobs))
	}
	// A different create at the same uid is not the same job.
	other := cmd
	other.Name = "Different"
	if clash := f.save(t, other); clash.Err != nil || clash.Changed == nil || clash.Changed.Name != "Report" || clash.Job != nil {
		t.Fatalf("clash %+v, want the existing job as a conflict", clash)
	}
}

func TestGatewayAResentAdapterCreateWithAnEmptySecretRefsMapIsStillTheSameJob(t *testing.T) {
	f := newJobsFixture(t)
	uid, _ := client.NewCronUID()
	def := cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger:    cronjob.Trigger{Kind: cronjob.TriggerManual},
		Action:     cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "on demand"},
		Issue:      &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, Title: "On demand"},
		SecretRefs: map[string]string{},
	}
	cmd := model.SaveJobCmd{UID: uid, Name: "Adapter", Definition: def}
	first := f.save(t, cmd)
	again := f.save(t, cmd)
	if first.Err != nil || again.Err != nil || again.Changed != nil || again.Job == nil || again.Op != model.JobCreated || again.Job.DefinitionEventUID != first.Job.DefinitionEventUID {
		t.Fatalf("first %+v again %+v, want the resend to answer the job the first made", first, again)
	}
}

func TestGatewayReplacesAJobGuardedOnItsDefinitionEventUID(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	job := f.created(t, "Report", parent)

	def := job.Definition
	def.Enabled, def.Issue = false, &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, ParentUID: parent, Title: "Retitled"}
	replaced := f.save(t, model.SaveJobCmd{UID: job.UID, ExpectedEventUID: job.DefinitionEventUID, Name: "Report v2", Definition: def})
	if replaced.Err != nil || replaced.Op != model.JobReplaced || replaced.Job == nil {
		t.Fatalf("replace %+v", replaced)
	}
	got := replaced.Job
	if got.Name != "Report v2" || got.Revision != 2 || got.Definition.Enabled || got.Definition.Issue.Title != "Retitled" || got.DefinitionEventUID == job.DefinitionEventUID ||
		got.Parent == nil || got.Parent.Progress == nil || !got.Parent.Progress.Anchor.Equal(job.Parent.Progress.Anchor) {
		t.Fatalf("replaced job %+v, want the edit with the same parent anchor and a new guard", got)
	}
	if listed := jobNamed(t, f.list(t, false).Jobs, "Report v2"); listed.Definition.Enabled || listed.Parent.Status != "open" {
		t.Fatalf("listed %+v", listed)
	}
}

func TestGatewayAGuardThatMovedWritesNothingAndReturnsTheCurrentJob(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	job := f.created(t, "Report", parent)
	stale := job.DefinitionEventUID
	moved := f.save(t, model.SaveJobCmd{UID: job.UID, ExpectedEventUID: stale, Name: "Renamed elsewhere", Definition: job.Definition})
	if moved.Err != nil || moved.Job == nil {
		t.Fatalf("first replace %+v", moved)
	}

	for name, attempt := range map[string]func() model.JobWritten{
		"replace": func() model.JobWritten {
			return f.save(t, model.SaveJobCmd{UID: job.UID, ExpectedEventUID: stale, Name: "Mine", Definition: job.Definition})
		},
		"archive": func() model.JobWritten {
			return as[model.JobWritten](t, run(t, f.gateway, model.ArchiveJobCmd{Workspace: f.workspace, ProjectID: f.demo.ID, Actor: "tester", UID: job.UID, ExpectedEventUID: stale}))
		},
		"restore": func() model.JobWritten {
			return as[model.JobWritten](t, run(t, f.gateway, model.RestoreJobCmd{Workspace: f.workspace, ProjectID: f.demo.ID, Actor: "tester", UID: job.UID, ExpectedEventUID: stale}))
		},
	} {
		got := attempt()
		if got.Err != nil || got.Job != nil || got.Changed == nil || got.Changed.Name != "Renamed elsewhere" || got.Changed.DefinitionEventUID != moved.Job.DefinitionEventUID || got.Changed.Parent == nil {
			t.Errorf("%s with a stale guard: %+v, want the current job and no write", name, got)
		}
	}
	if jobs := f.list(t, true).Jobs; len(jobs) != 1 || jobs[0].Name != "Renamed elsewhere" || jobs[0].Archived() || jobs[0].Revision != 2 {
		t.Fatalf("after the refused writes %+v, want the job untouched", jobs)
	}
}

func TestGatewayArchivesAndRestoresAJobGuarded(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	job := f.created(t, "Report", parent)

	archived := as[model.JobWritten](t, run(t, f.gateway, model.ArchiveJobCmd{Workspace: f.workspace, ProjectID: f.demo.ID, Actor: "tester", UID: job.UID, ExpectedEventUID: job.DefinitionEventUID}))
	if archived.Err != nil || archived.Op != model.JobArchived || archived.Job == nil || !archived.Job.Archived() || archived.Job.DefinitionEventUID == job.DefinitionEventUID {
		t.Fatalf("archive %+v", archived)
	}
	if jobs := f.list(t, false).Jobs; len(jobs) != 0 {
		t.Fatalf("default list after archive %+v", jobs)
	}
	if jobs := f.list(t, true).Jobs; len(jobs) != 1 || !jobs[0].Archived() {
		t.Fatalf("list with archived %+v", jobs)
	}

	restored := as[model.JobWritten](t, run(t, f.gateway, model.RestoreJobCmd{Workspace: f.workspace, ProjectID: f.demo.ID, Actor: "tester", UID: job.UID, ExpectedEventUID: archived.Job.DefinitionEventUID}))
	if restored.Err != nil || restored.Op != model.JobRestored || restored.Job == nil || restored.Job.Archived() || restored.Job.Parent == nil || restored.Job.Parent.Progress == nil {
		t.Fatalf("restore %+v", restored)
	}
	if jobs := f.list(t, false).Jobs; len(jobs) != 1 || jobs[0].Archived() {
		t.Fatalf("list after restore %+v", jobs)
	}
}

func TestGatewayRefusesWhatKataRefusesWithItsWords(t *testing.T) {
	f := newJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	uid, _ := client.NewCronUID()

	short := nativeDefinition("Tick")
	short.Trigger.IntervalSeconds = 30
	refused := f.save(t, model.SaveJobCmd{UID: uid, Name: "Too fast", Definition: short, ParentRef: parent})
	if refused.Err == nil || !strings.Contains(refused.Err.Error(), "at least 60 seconds") || refused.Job != nil {
		t.Fatalf("interval under a minute: %+v", refused)
	}

	missing := f.save(t, model.SaveJobCmd{UID: uid, Name: "Orphan", Definition: nativeDefinition("Tick"), ParentRef: "zzzz"})
	if missing.Err == nil || !strings.Contains(missing.Err.Error(), `Parent: no Issue "zzzz"`) {
		t.Fatalf("unknown parent: %+v", missing)
	}
	if jobs := f.list(t, true).Jobs; len(jobs) != 0 {
		t.Fatalf("refused writes left jobs %+v", jobs)
	}
}
