package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// finishedRun is a Run of the "Triage" job for Issue "parent-a" that succeeded after 95 seconds.
func finishedRun(uid string) model.Run {
	started := now.Add(-3 * time.Hour)
	ended := started.Add(95 * time.Second)
	return model.Run{
		UID: uid, JobUID: "J1", JobName: "Triage", DefinitionEventUID: "E1", IssueUID: "parent-a", IssueShortID: "pa11", IssueTitle: "Weekly report",
		Actor: "tester", Teammate: "pip", ExecutorLabel: "laptop", OccurrenceKey: "2026-10-08T09:00:00Z", Status: model.RunSucceeded,
		Message: "triaged 4 items\nsecond line", InputTokens: ptr(int64(1200)), OutputTokens: ptr(int64(340)), Revision: 2,
		CreatedAt: started, UpdatedAt: ended, StartedAt: &started, EndedAt: &ended,
	}
}

// workflowRun is a Run of a workflow alone, still running on a machine, with no Issue.
func workflowRun(uid string) model.Run {
	started := now.Add(-30 * time.Second)
	return model.Run{
		UID: uid, WorkflowUID: "W1", WorkflowName: "Nightly build", WorkflowDefinitionEventUID: "WE1", Actor: "worker", Status: model.RunRunning,
		CreatedAt: started, UpdatedAt: started, StartedAt: &started,
	}
}

// runsPane is a Pane on the Runs Page, the runs read answered with runs.
func runsPane(t *testing.T, cols int, runs ...model.Run) driver.State {
	t.Helper()
	out := step(t, projectPane(t, cols, boardCard("parent-a")), key("4"))
	return answerRuns(t, out, "", runs...)
}

func answerRuns(t *testing.T, out driver.Output, next string, runs ...model.Run) driver.State {
	t.Helper()
	load, ok := askedRuns(out.Cmds)
	if !ok {
		t.Fatalf("no runs read asked for: %#v", out.Cmds)
	}
	return step(t, out.State, model.RunsLoaded{Request: load.Request, Before: load.Before, Runs: runs, NextBefore: next}).State
}

func askedRuns(cmds []model.Cmd) (model.LoadRuns, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadRuns); ok {
			return load, true
		}
	}
	return model.LoadRuns{}, false
}

func runCardText(t *testing.T, s driver.State, uid string) string {
	t.Helper()
	id := view.RunID(uid)
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id || strings.HasPrefix(n.ID, id+".") {
			text += wireText(n)
		}
	}
	if text == "" {
		t.Fatalf("the Runs Page draws no card for %s", uid)
	}
	return text
}

func TestRunsPageKeyAsksForTheProjectsRunsAndThePageBarShowsThePage(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"))
	out := step(t, s, key("4"))
	load, ok := askedRuns(out.Cmds)
	if !ok || load.ProjectID != 7 || load.JobUID != "" || load.Before != "" || load.Workspace != "/repo" {
		t.Fatalf("cmds %#v, want a read of the Project's newest runs", out.Cmds)
	}
	if out.State.Model.Page != model.PageRuns || !drawnIDs(t, view.Render(out.State.Model, now))[view.PageBarEntryID("runs")] {
		t.Fatalf("page %v, want the Runs Page with its Page bar entry", out.State.Model.Page)
	}
	if _, ok := askedRuns(step(t, s, click(view.PageBarEntryID("runs"), view.KeyClick("4"))).Cmds); !ok {
		t.Error("clicking the Runs entry asks for no runs")
	}
	if back := step(t, out.State, key("1")); back.State.Model.Page != model.PageIssues {
		t.Errorf("1 leaves the Runs Page on page %v", back.State.Model.Page)
	}
}

func TestRunsPageRowsShowStatusNameIssueExecutorStartDurationAndSummary(t *testing.T) {
	s := runsPane(t, 160, workflowRun("R2"), finishedRun("R1"))

	wantText(t, "job run row", runCardText(t, s, "R1"), "succeeded", "Triage", "#pa11 Weekly report", "on laptop", "started", "1m 35s", "triaged 4 items")
	if strings.Contains(runCardText(t, s, "R1"), "second line") {
		t.Error("a row draws more than the summary's first line")
	}
	wantText(t, "workflow run row", runCardText(t, s, "R2"), "running", "Nightly build", "on worker", "30s")
	// Newest first: the list keeps the order kata gave.
	rows := s.Model.RunRows(now)
	if len(rows) != 2 || rows[0].Run.UID != "R2" || rows[1].Run.UID != "R1" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestEnterShowsTheRunsDetailWithSummaryTokensAndReferences(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R1"))
	out := step(t, s, key("enter"))
	if out.State.Model.CurrentMode() != model.ModeRunInspector {
		t.Fatalf("mode %q, want the run inspector", out.State.Model.CurrentMode())
	}
	shown := false
	for _, cmd := range out.Cmds {
		if show, ok := cmd.(model.ShowRun); ok && show.UID == "R1" && show.ProjectID == 7 {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("cmds %#v, want the run read again", out.Cmds)
	}
	wantText(t, "run detail", inspectorText(t, out.State),
		"succeeded", "Triage", "#pa11 Weekly report", "laptop", "pip", "1m 35s", "triaged 4 items", "second line",
		"1200", "340", "2026-10-08T09:00:00Z", "J1", "E1", "parent-a", "R1")

	// The answer replaces the run held.
	changed := finishedRun("R1")
	changed.Message = "triaged 5 items"
	s = step(t, out.State, model.RunShown{UID: "R1", Run: changed}).State
	wantText(t, "run detail after the read", inspectorText(t, s), "triaged 5 items")

	if back := step(t, s, key("escape")); back.State.Model.CurrentMode() != model.ModeRuns {
		t.Errorf("esc leaves mode %q, want the run list", back.State.Model.CurrentMode())
	}
}

func TestRunDetailLeavesOutWhatTheRunDoesNotHold(t *testing.T) {
	s := feed(t, runsPane(t, 160, workflowRun("R2")), key("enter")).State
	text := inspectorText(t, s)
	for _, left := range []string{"Input tokens", "Output tokens", "Issue", "Job UID", "Ended", "Occurrence"} {
		if strings.Contains(text, left) {
			t.Errorf("a run without it still draws %q: %s", left, text)
		}
	}
	wantText(t, "run detail", text, "Nightly build", "WE1")
}

func TestIOpensTheRunsIssueOnTheIssuesPage(t *testing.T) {
	s := runsPane(t, 160, workflowRun("R2"), finishedRun("R1"))
	s = feed(t, s, key("j")).State
	opened := step(t, s, key("i"))
	m := opened.State.Model
	if m.Page != model.PageIssues || m.SelectedUID != "parent-a" || m.Focus != model.ColumnDetail {
		t.Fatalf("page %v selected %q focus %v, want the Issue's detail on the Issues Page", m.Page, m.SelectedUID, m.Focus)
	}
	loaded := false
	for _, cmd := range opened.Cmds {
		if load, ok := cmd.(model.LoadDetail); ok && load.UID == "parent-a" {
			loaded = true
		}
	}
	if !loaded {
		t.Errorf("cmds %#v, want the Issue's detail loaded", opened.Cmds)
	}
	// From the detail view too.
	inspected := feed(t, runsPane(t, 160, finishedRun("R1")), key("enter")).State
	if got := step(t, inspected, key("i")).State.Model; got.Page != model.PageIssues || got.SelectedUID != "parent-a" {
		t.Errorf("i in the detail leaves page %v selected %q", got.Page, got.SelectedUID)
	}
}

func TestIOnARunWithoutAnIssueSaysSo(t *testing.T) {
	got := feed(t, runsPane(t, 160, workflowRun("R2")), key("i")).State.Model
	if got.Page != model.PageRuns || !strings.Contains(got.Notice, "no Issue") {
		t.Errorf("page %v notice %q, want the Runs Page and a notice", got.Page, got.Notice)
	}
}

func TestRKeyOnTheJobsPageOpensTheRunsFilteredToTheSelectedJob(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "Triage"), promptJob("J2", "Other"))
	s = feed(t, s, key("j")).State
	out := step(t, s, key("r"))
	load, ok := askedRuns(out.Cmds)
	if !ok || load.JobUID != "J2" || out.State.Model.Page != model.PageRuns {
		t.Fatalf("page %v cmds %#v, want the Runs Page reading the runs of J2", out.State.Model.Page, out.Cmds)
	}
	s = answerRuns(t, out, "", finishedRun("R1"))
	wantText(t, "heading", wireTextOf(t, s), "job Other")

	// f lifts the filter and asks for every run; f on an unfiltered list narrows to the selected run's job.
	lifted := step(t, s, key("f"))
	if load, ok := askedRuns(lifted.Cmds); !ok || load.JobUID != "" {
		t.Fatalf("f asks for %#v, want every run", lifted.Cmds)
	}
	s = answerRuns(t, lifted, "", workflowRun("R2"), finishedRun("R1"))
	narrowed := step(t, step(t, s, key("j")).State, key("f"))
	if load, ok := askedRuns(narrowed.Cmds); !ok || load.JobUID != "J1" {
		t.Fatalf("f on a job's run asks for %#v, want the runs of J1", narrowed.Cmds)
	}
	// A workflow's run has no job to narrow to.
	if got := feed(t, s, key("f")).State.Model; got.Runs.JobUID != "" || !strings.Contains(got.Notice, "workflow") {
		t.Errorf("filter %q notice %q, want no filter and a notice", got.Runs.JobUID, got.Notice)
	}
}

func TestSwitchingPagesKeepsEachPagesSelectionLoadedRunsAndFilter(t *testing.T) {
	// Runs: the job filter, the selection and the older page already read survive a visit to another Page.
	out := step(t, jobsPane(t, 160, promptJob("J1", "Triage")), key("r"))
	s := answerRuns(t, out, "cursor-1", finishedRun("R2"))
	s = answerRuns(t, step(t, s, key("m")), "", finishedRun("R2"), finishedRun("R1"))
	s = feed(t, s, key("j")).State
	back := step(t, step(t, s, key("1")).State, key("4"))
	if got := back.State.Model.Runs; got.JobUID != "J1" || got.Selected != "R1" || len(got.Runs) != 2 {
		t.Fatalf("after a visit elsewhere: filter %q selected %q runs %d, want J1, R1 and both pages", got.JobUID, got.Selected, len(got.Runs))
	}
	load, ok := askedRuns(back.Cmds)
	if !ok || load.JobUID != "J1" || !load.Refresh {
		t.Fatalf("cmds %#v, want the newest page of J1's runs read again to join the ones held", back.Cmds)
	}

	// Jobs: the selected job survives too.
	j := feed(t, jobsPane(t, 160, promptJob("J1", "Triage"), promptJob("J2", "Other")), key("j")).State
	again := step(t, step(t, j, key("1")).State, key("3"))
	if got := again.State.Model.Jobs.Selected; got != "J2" {
		t.Fatalf("selected %q after a visit elsewhere, want J2", got)
	}
	if _, ok := askedJobs(again.Cmds); !ok {
		t.Fatalf("cmds %#v, want the jobs read again", again.Cmds)
	}
}

func TestMReadsTheNextOlderPageAndAppendsIt(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R2"))
	out := step(t, s, key("m"))
	if len(out.Cmds) != 0 {
		t.Fatalf("m with no older runs asks for %#v", out.Cmds)
	}
	s = answerRuns(t, step(t, s, key("ctrl+r")), "cursor-1", finishedRun("R2"))
	out = step(t, s, key("m"))
	load, ok := askedRuns(out.Cmds)
	if !ok || load.Before != "cursor-1" {
		t.Fatalf("m asks for %#v, want the page before cursor-1", out.Cmds)
	}
	s = answerRuns(t, out, "", finishedRun("R2"), finishedRun("R1"))
	if runs := s.Model.Runs.Runs; len(runs) != 2 || runs[1].UID != "R1" || s.Model.Runs.More != "" {
		t.Fatalf("runs %+v more %q, want the older page appended once", runs, s.Model.Runs.More)
	}
}

func TestRunsPageReadsAgainOnEveryDaemonEvent(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R1"))
	out := step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "cron.run.observed"})
	if _, ok := askedRuns(out.Cmds); !ok {
		t.Fatalf("a daemon event asks for %#v, want the runs read again", out.Cmds)
	}
}

func TestRunsPageSelectionClickAndMoveAreFollowed(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R3"), finishedRun("R2"), finishedRun("R1"))
	moved := step(t, s, key("j"))
	if moved.State.Model.Runs.Selected != "R2" {
		t.Fatalf("selected %q after j", moved.State.Model.Runs.Selected)
	}
	wantOps(t, moved, tern.OpReveal(view.RunID("R2"), "nearest"))
	clicked := step(t, s, click(view.RunID("R1"), view.RunClick("R1")))
	if clicked.State.Model.Runs.Selected != "R1" {
		t.Errorf("selected %q after a click, want R1", clicked.State.Model.Runs.Selected)
	}
	wantText(t, "detail beside the list", inspectorText(t, clicked.State), "R1")
}

func TestRunsPageNeedsAProject(t *testing.T) {
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}})
	out := step(t, s, key("4"))
	if out.State.Model.Page != model.PageIssues || !strings.Contains(out.State.Model.Notice, "Runs Page needs a Project") {
		t.Fatalf("page %v notice %q", out.State.Model.Page, out.State.Model.Notice)
	}
}

func TestFooterAndHelpSheetListTheRunsPageKeys(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R1"))
	wantText(t, "Runs Page footer", footerText(t, s), "detail", "issue", "job filter", "older")
	help := wireTextOf(t, feed(t, s, key("?")).State)
	wantText(t, "help sheet", help, "Runs", "open the run's Issue", "show the run's detail", "switch Page")
	jobs := wireTextOf(t, feed(t, jobsPane(t, 160, promptJob("J1", "Triage")), key("?")).State)
	wantText(t, "Jobs help", jobs, "open the Runs Page on the job's runs")
}

func TestWorkflowRowAndInspectorShowTheWorkflowsLatestRuns(t *testing.T) {
	out := feed(t, projectPane(t, 160, boardCard("a")), key("5"))
	load, ok := askedRuns(out.Cmds)
	if !ok || load.ProjectID != 7 || load.JobUID != "" {
		t.Fatalf("the Page asked for runs with %#v, want the Project's newest runs", out.Cmds)
	}
	failed := workflowRun("R1")
	failed.Status = model.RunFailed
	failed.WorkflowUID = "W1"
	failed.Message = "step build failed"
	running := workflowRun("R2")
	running.WorkflowUID = "W1"
	other := workflowRun("R3")
	other.WorkflowUID = "W2"
	s := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow(), nightlyFlow()}}).State
	s = step(t, s, model.RunsLoaded{Request: load.Request, Runs: []model.Run{running, failed, other, finishedRun("R4")}}).State

	// The newest run of W1 leads its row; a run of another workflow or of a job does not.
	mustShow(t, s, "row", "running")
	if latest, ok := s.Model.LatestWorkflowRun("W1"); !ok || latest.UID != "R2" {
		t.Fatalf("latest run of W1 %+v", latest)
	}
	s = step(t, s, key("enter")).State
	mustShow(t, s, "inspector", "step build failed", "failed")
	if got := s.Model.WorkflowRuns("W1"); len(got) != 2 {
		t.Errorf("runs of W1 %+v, want its two", got)
	}
}

func TestAnAnswerToTheRunsPagesReadDoesNotLandOnTheWorkflowsPage(t *testing.T) {
	out := feed(t, projectPane(t, 160, boardCard("a")), key("5"))
	load, _ := askedRuns(out.Cmds)
	s := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow()}}).State
	running := workflowRun("R2")
	running.WorkflowUID = "W1"
	stray := workflowRun("R9")
	stray.WorkflowUID = "W1"
	stray.Status = model.RunFailed
	// The Runs Page's own reads are numbered upward; an answer to one of them is not the Workflows Page's.
	s = step(t, s, model.RunsLoaded{Request: 1, Runs: []model.Run{stray}}).State
	if _, ok := s.Model.LatestWorkflowRun("W1"); ok {
		t.Fatal("the Workflows Page took the answer to a read it never asked")
	}
	s = step(t, s, model.RunsLoaded{Request: load.Request, Runs: []model.Run{running}}).State
	if latest, ok := s.Model.LatestWorkflowRun("W1"); !ok || latest.UID != "R2" {
		t.Fatalf("latest run of W1 %+v after its own answer", latest)
	}
}

func TestWorkflowsPageFollowsRunPagesToFindEachWorkflowsLatestRunAndOlderRuns(t *testing.T) {
	out := feed(t, projectPane(t, 160, boardCard("a")), key("5"))
	first, ok := askedRuns(out.Cmds)
	if !ok {
		t.Fatalf("no runs read asked for: %#v", out.Cmds)
	}
	s := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow(), nightlyFlow()}}).State
	onW1 := func(uid string) model.Run {
		r := workflowRun(uid)
		r.WorkflowUID = "W1"
		return r
	}
	onW2 := workflowRun("R9")
	onW2.WorkflowUID = "W2"

	// The newest page holds runs of the first workflow alone, and kata has an older page.
	next := step(t, s, model.RunsLoaded{Request: first.Request, Runs: []model.Run{onW1("R1"), onW1("R2")}, NextBefore: "c1"})
	second, ok := askedRuns(next.Cmds)
	if !ok || second.Before != "c1" || second.Request != first.Request {
		t.Fatalf("cmds %#v, want the page before c1 read on the same request: W2 has no run yet", next.Cmds)
	}
	// W2 turns up there; the selected W1 still lists fewer runs than the inspector shows, so the next page is read too.
	more := step(t, next.State, model.RunsLoaded{Request: first.Request, Before: "c1", Runs: []model.Run{onW2}, NextBefore: "c2"})
	third, ok := askedRuns(more.Cmds)
	if !ok || third.Before != "c2" {
		t.Fatalf("cmds %#v, want the page before c2 read: W1 shows two of its five runs", more.Cmds)
	}
	if latest, ok := more.State.Model.LatestWorkflowRun("W2"); !ok || latest.UID != "R9" {
		t.Fatalf("latest run of W2 %+v, want R9 from the older page", latest)
	}
	done := step(t, more.State, model.RunsLoaded{Request: first.Request, Before: "c2", Runs: []model.Run{onW1("R3"), onW1("R4"), onW1("R5")}, NextBefore: "c3"})
	if _, again := askedRuns(done.Cmds); again {
		t.Fatalf("cmds %#v, want no more pages once every workflow has its latest run and the selection its five", done.Cmds)
	}
	if got := done.State.Model.WorkflowRuns("W1"); len(got) != 5 {
		t.Fatalf("runs of W1 %d, want 5", len(got))
	}
}
