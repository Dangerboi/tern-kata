package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// observe is an executor reporting a Run to kata, as an adapter does: a new Run, which kata stores at revision 1.
func (f *jobsFixture) observe(t *testing.T, body g.ObserveCronRunBody) string {
	t.Helper()
	uid, err := client.NewCronUID()
	if err != nil {
		t.Fatal(err)
	}
	body.Actor = new("tester")
	response, err := f.client.ObserveCronRunWithResponse(context.Background(), &g.ObserveCronRunRequestOptions{
		PathParams: &g.ObserveCronRunPath{ProjectID: f.demo.ID, RunUID: uid}, Body: &body,
	})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("observe run: %v", err)
	}
	return uid
}

func (f *jobsFixture) workflow(t *testing.T, name string) g.CronWorkflow {
	t.Helper()
	uid, err := client.NewCronUID()
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.client.ReplaceCronWorkflowWithResponse(context.Background(), &g.ReplaceCronWorkflowRequestOptions{
		PathParams: &g.ReplaceCronWorkflowPath{ProjectID: f.demo.ID, CronUID: uid},
		Body: &g.PutCronWorkflowBody{Actor: new("tester"), Name: name, Definition: g.CronWorkflowDefinition{
			Version: 1, Steps: []g.CronWorkflowStep{{Key: "build", Kind: "command", Command: new("make")}},
		}},
	})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("put workflow %s: %v", name, err)
	}
	return response.JSON200.Workflow
}

func (f *jobsFixture) runs(t *testing.T, cmd model.LoadRuns) model.RunsLoaded {
	t.Helper()
	cmd.Workspace, cmd.ProjectID, cmd.Request = f.workspace, f.demo.ID, 3
	loaded := as[model.RunsLoaded](t, run(t, f.gateway, cmd))
	if loaded.Err != nil || loaded.Request != 3 {
		t.Fatalf("list runs %+v", loaded)
	}
	return loaded
}

func TestGatewayListsObservedRunsNewestFirstWithNamesIssueAndSummary(t *testing.T) {
	f := newJobsFixture(t)
	issue := createIssue(t, f.gateway, f.workspace, f.demo, "Triage the inbox")
	job := f.adapter(t, "Triage")
	workflow := f.workflow(t, "Nightly build")
	started := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	ended := started.Add(95 * time.Second)
	older := f.observe(t, g.ObserveCronRunBody{
		JobUID: &job.UID, DefinitionEventUID: &job.DefinitionEventUID, IssueUID: &issue, OccurrenceKey: new("2026-10-08T09:00:00Z"),
		ExecutorLabel: new("laptop"), Teammate: new("pip"), Status: "succeeded", StartedAt: &started, EndedAt: &ended,
		Summary: g.Summary{Version: 1, Message: new("triaged 4 items"), InputTokens: new(int64(1200)), OutputTokens: new(int64(340))},
	})
	newer := f.observe(t, g.ObserveCronRunBody{
		WorkflowUID: &workflow.UID, WorkflowDefinitionEventUID: &workflow.DefinitionEventUID, Status: "running",
		StartedAt: &started, Summary: g.Summary{Version: 1},
	})

	runs := f.runs(t, model.LoadRuns{}).Runs
	if len(runs) != 2 || runs[0].UID != newer || runs[1].UID != older {
		t.Fatalf("runs %+v, want the workflow-only run then the job's", runs)
	}
	flow := runs[0]
	if flow.Name() != "Nightly build" || flow.WorkflowUID != workflow.UID || flow.JobUID != "" || flow.Status != model.RunRunning || flow.IssueUID != "" {
		t.Errorf("workflow run %+v", flow)
	}
	got := runs[1]
	if got.Name() != "Triage" || got.JobUID != job.UID || got.DefinitionEventUID != job.DefinitionEventUID || got.Status != model.RunSucceeded {
		t.Errorf("job run %+v", got)
	}
	if got.IssueUID != issue || got.IssueTitle != "Triage the inbox" || got.IssueShortID == "" || got.IssueRef() != "#"+got.IssueShortID+" Triage the inbox" {
		t.Errorf("issue of the run %+v", got)
	}
	if got.Executor() != "laptop" || got.Actor != "tester" || got.Teammate != "pip" || got.OccurrenceKey != "2026-10-08T09:00:00Z" || got.Revision != 1 {
		t.Errorf("who ran it %+v", got)
	}
	if got.Message != "triaged 4 items" || got.InputTokens == nil || *got.InputTokens != 1200 || got.OutputTokens == nil || *got.OutputTokens != 340 {
		t.Errorf("summary %+v", got)
	}
	if d, ok := got.Duration(time.Now()); !ok || d != 95*time.Second || !got.Started().Equal(started) {
		t.Errorf("duration %v %v, started %v", d, ok, got.Started())
	}
}

func TestGatewayJobFilterListsOnlyThatJobsRuns(t *testing.T) {
	f := newJobsFixture(t)
	triage, other := f.adapter(t, "Triage"), f.adapter(t, "Other")
	workflow := f.workflow(t, "Nightly build")
	want := f.observe(t, g.ObserveCronRunBody{JobUID: &triage.UID, DefinitionEventUID: &triage.DefinitionEventUID, Status: "failed", Summary: g.Summary{Version: 1, Message: new("boom")}})
	f.observe(t, g.ObserveCronRunBody{JobUID: &other.UID, DefinitionEventUID: &other.DefinitionEventUID, Status: "succeeded", Summary: g.Summary{Version: 1}})
	f.observe(t, g.ObserveCronRunBody{WorkflowUID: &workflow.UID, WorkflowDefinitionEventUID: &workflow.DefinitionEventUID, Status: "unknown", Summary: g.Summary{Version: 1}})

	runs := f.runs(t, model.LoadRuns{JobUID: triage.UID}).Runs
	if len(runs) != 1 || runs[0].UID != want || runs[0].Status != model.RunFailed {
		t.Fatalf("runs %+v, want only the Triage run", runs)
	}
	if all := f.runs(t, model.LoadRuns{}).Runs; len(all) != 3 {
		t.Fatalf("all runs %+v, want the three", all)
	}
}

func TestGatewayReadsOlderRunsPageByPage(t *testing.T) {
	f := newJobsFixture(t)
	job := f.adapter(t, "Triage")
	var uids []string
	for i := range runsPageSize + 1 {
		uids = append(uids, f.observe(t, g.ObserveCronRunBody{
			JobUID: &job.UID, DefinitionEventUID: &job.DefinitionEventUID, Status: "succeeded", Summary: g.Summary{Version: 1, Message: new(fmt.Sprint("run ", i))},
		}))
	}

	first := f.runs(t, model.LoadRuns{})
	if len(first.Runs) != runsPageSize || first.NextBefore == "" || first.Runs[0].UID != uids[runsPageSize] {
		t.Fatalf("first page %d runs, next %q", len(first.Runs), first.NextBefore)
	}
	second := f.runs(t, model.LoadRuns{Before: first.NextBefore})
	if len(second.Runs) != 1 || second.Runs[0].UID != uids[0] || second.Before != first.NextBefore {
		t.Fatalf("second page %+v", second)
	}
}

func TestGatewayShowsOneRunByUID(t *testing.T) {
	f := newJobsFixture(t)
	job := f.adapter(t, "Triage")
	uid := f.observe(t, g.ObserveCronRunBody{JobUID: &job.UID, DefinitionEventUID: &job.DefinitionEventUID, Status: "cancelled", Summary: g.Summary{Version: 1, Message: new("abandoned")}})

	shown := as[model.RunShown](t, run(t, f.gateway, model.ShowRun{Workspace: f.workspace, ProjectID: f.demo.ID, UID: uid}))
	if shown.Err != nil || shown.Run.UID != uid || shown.Run.Name() != "Triage" || shown.Run.Status != model.RunCancelled || shown.Run.Message != "abandoned" {
		t.Fatalf("shown %+v", shown)
	}
}

func TestGatewayJobsReadWithActivityCarryEachAdapterJobsNewestRuns(t *testing.T) {
	f := newJobsFixture(t)
	busy := f.adapter(t, "Busy")
	quiet := f.adapter(t, "Quiet")
	// The quiet job's only run is older than a whole page of the busy one's: following the pages finds it.
	f.observe(t, g.ObserveCronRunBody{JobUID: &quiet.UID, DefinitionEventUID: &quiet.DefinitionEventUID, Status: "failed", Summary: g.Summary{Version: 1}})
	var uids []string
	for i := range runsPageSize + 3 {
		uids = append(uids, f.observe(t, g.ObserveCronRunBody{
			JobUID: &busy.UID, DefinitionEventUID: &busy.DefinitionEventUID, Status: "succeeded", Summary: g.Summary{Version: 1, Message: new(fmt.Sprint("run ", i))},
		}))
	}

	plain := f.list(t, false)
	if plain.Activity != nil {
		t.Fatalf("a read that did not ask for activity came with %+v", plain.Activity)
	}
	loaded := as[model.JobsLoaded](t, run(t, f.gateway, model.LoadJobs{Workspace: f.workspace, ProjectID: f.demo.ID, Request: 7, Activity: true}))
	if loaded.Err != nil {
		t.Fatal(loaded.Err)
	}
	got := loaded.Activity[busy.UID].Runs
	if len(got) != model.JobActivityShown || got[0].UID != uids[len(uids)-1] || got[1].UID != uids[len(uids)-2] {
		t.Fatalf("busy job's runs %d, first %q, want its %d newest", len(got), got[0].UID, model.JobActivityShown)
	}
	if runs := loaded.Activity[quiet.UID].Runs; len(runs) != 1 || runs[0].Status != model.RunFailed {
		t.Fatalf("quiet job's runs %+v, want its failed run", runs)
	}
}

func TestGatewayRunListAnswerCarriesTheRefreshFlagOfTheAsk(t *testing.T) {
	f := newJobsFixture(t)
	job := f.adapter(t, "Triage")
	f.observe(t, g.ObserveCronRunBody{JobUID: &job.UID, DefinitionEventUID: &job.DefinitionEventUID, Status: "running", Summary: g.Summary{Version: 1}})

	if loaded := f.runs(t, model.LoadRuns{Refresh: true}); !loaded.Refresh {
		t.Errorf("answer to a refresh %+v, want Refresh set", loaded)
	}
	if loaded := f.runs(t, model.LoadRuns{}); loaded.Refresh {
		t.Errorf("answer to a plain read %+v, want Refresh clear", loaded)
	}
}

func TestGatewayNamesArchivedJobsAndWorkflowsAndShowsADeletedIssueByItsReference(t *testing.T) {
	f := newJobsFixture(t)
	issue := createIssue(t, f.gateway, f.workspace, f.demo, "Triage the inbox")
	gone := createIssue(t, f.gateway, f.workspace, f.demo, "Soon gone")
	job := f.adapter(t, "Triage")
	workflow := f.workflow(t, "Nightly build")
	f.observe(t, g.ObserveCronRunBody{JobUID: &job.UID, DefinitionEventUID: &job.DefinitionEventUID, IssueUID: &issue, Status: "succeeded", Summary: g.Summary{Version: 1}})
	f.observe(t, g.ObserveCronRunBody{WorkflowUID: &workflow.UID, WorkflowDefinitionEventUID: &workflow.DefinitionEventUID, IssueUID: &gone, Status: "succeeded", Summary: g.Summary{Version: 1}})
	f.archive(t, job)
	(&childCountFixture{gateway: f.gateway, workspace: f.workspace}).remove(t, gone, f.demo)

	byIssue := map[string]model.Run{}
	for _, r := range f.runs(t, model.LoadRuns{}).Runs {
		byIssue[r.IssueUID] = r
	}
	if r := byIssue[issue]; r.Name() != "Triage" || r.IssueTitle != "Triage the inbox" || r.IssueShortID == "" {
		t.Errorf("run of an archived job %+v, want its name and its Issue's title", r)
	}
	if r := byIssue[gone]; r.Name() != "Nightly build" || r.IssueRef() != gone || r.IssueTitle != "" {
		t.Errorf("run of a deleted Issue %+v, want the workflow's name and the Issue shown by its reference %q", r, gone)
	}
}
