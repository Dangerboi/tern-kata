package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

var cronRun = time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC)

// cronChild is an Issue the job J1 made for the run at cronRun.
func cronChild() model.Issue {
	child := boardCard("child")
	child.ProjectID = 7
	child.CronOccurrence = &model.CronOccurrence{JobUID: "J1", DefinitionEventUID: "E1", ScheduledAt: cronRun}
	return child
}

// cronParentIssue is the Issue J1 hangs its children from, as kata marks it with the job's progress.
func cronParentIssue(status string) model.Issue {
	parent := boardCard("parent-a")
	parent.ProjectID = 7
	parent.Status = status
	parent.Metadata = map[string]any{"cron.progress.J1": map[string]any{"anchor": "2026-10-01T00:00:00Z"}}
	return parent
}

// detailOf shows issue's detail, the Project's jobs read answered with jobs when the detail asks for it, and reports whether it did.
func detailOf(t *testing.T, issue model.Issue, others []model.Issue, jobs ...model.CronJob) (driver.State, bool) {
	t.Helper()
	s := projectPane(t, 160, append([]model.Issue{issue}, others...)...)
	out := step(t, s, model.DetailLoaded{UID: issue.UID, Selected: &model.IssueDetailData{Issue: &issue}})
	load, asked := askedJobs(out.Cmds)
	if !asked {
		return out.State, false
	}
	if load.ProjectID != 7 || !load.Archived {
		t.Fatalf("jobs read %+v, want the Issue's Project with archived jobs, whose names a detail may need", load)
	}
	return step(t, out.State, model.JobsLoaded{Request: load.Request, Jobs: jobs}).State, true
}

func listRowText(t *testing.T, s driver.State, uid string) string {
	t.Helper()
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == view.IssueID(uid) || strings.HasPrefix(n.ID, view.IssueID(uid)+".") {
			text += wireText(n)
		}
	}
	return text
}

func TestCronMadeIssueShowsTheMarkerInTheListAndOnItsBoardCard(t *testing.T) {
	child := cronChild()
	list := projectPane(t, 160, child, boardCard("plain"))
	wantText(t, "the list row", listRowText(t, list, "child"), "◷ Title of child")
	if strings.Contains(listRowText(t, list, "plain"), "◷") {
		t.Error("an Issue without cron.occurrence has the cron marker")
	}
	board := boardPane(t, child, boardCard("plain"))
	nodes := wireNodes(t, view.Render(board.Model, now))
	wantText(t, "the Board card", wireText(node(t, nodes, view.CardID(board.Model, "child")+".title")), "◷ Title of child")
	if strings.Contains(wireText(node(t, nodes, view.CardID(board.Model, "plain")+".title")), "◷") {
		t.Error("a plain Board card has the cron marker")
	}
}

func TestCronMadeIssueDetailNamesTheJobAndTheScheduledTimeAndUsesTheUidWhenTheJobIsGone(t *testing.T) {
	s, _ := detailOf(t, cronChild(), nil, recurringJob("J1", "Weekly report", jobParent("open", false)))
	wantText(t, "the detail", inspectorText(t, s), "Cron job", "Weekly report", "scheduled "+s.Model.DetailTime(cronRun))

	gone, asked := detailOf(t, cronChild(), nil, promptJob("J9", "Unrelated"))
	if !asked {
		t.Fatal("a cron-made Issue's detail asks for no jobs")
	}
	text := inspectorText(t, gone)
	wantText(t, "the detail of a job that is gone", text, "Cron job", "J1")
	if strings.Contains(text, "Unrelated") {
		t.Errorf("the detail names another job: %s", text)
	}
}

func TestIssueWithoutCronMetadataShowsNothingNewAndReadsNoJobs(t *testing.T) {
	s, asked := detailOf(t, boardCard("plain"), nil)
	if asked {
		t.Error("an Issue no job made and none binds asked for the Project's jobs")
	}
	if text := inspectorText(t, s); strings.Contains(text, "Cron job") {
		t.Errorf("the detail shows a Cron property: %s", text)
	}
}

func TestSelectingTheCronJobPropertyOpensTheJobsPageWithTheJobSelected(t *testing.T) {
	jobs := []model.CronJob{promptJob("J0", "Adapter"), recurringJob("J1", "Weekly report", jobParent("open", false)), promptJob("J2", "Other")}
	for name, open := range map[string]func(driver.State) driver.Output{
		"enter": func(s driver.State) driver.Output { return step(t, feed(t, s, key("enter")).State, key("enter")) },
		"click": func(s driver.State) driver.Output { return step(t, s, click("", view.OpenJobClick("J1"))) },
	} {
		s, _ := detailOf(t, cronChild(), nil, jobs...)
		opened := open(s)
		if opened.State.Model.Page != model.PageJobs {
			t.Fatalf("%s: page %v, want the Jobs Page", name, opened.State.Model.Page)
		}
		read := answerJobs(t, opened, jobs...)
		if got := read.Model.Jobs.Selected; got != "J1" {
			t.Errorf("%s: the Jobs Page selects %q, want J1", name, got)
		}
	}
}

func TestParentsDetailListsItsJobsWithStateAndSaysClosedMeansPaused(t *testing.T) {
	jobs := []model.CronJob{recurringJob("J1", "Weekly report", jobParent("open", false)), promptJob("J0", "Adapter")}
	open, _ := detailOf(t, cronParentIssue("open"), nil, jobs...)
	text := inspectorText(t, open)
	wantText(t, "an open parent's detail", text, "Cron jobs", "Weekly report", "active", "every hour")
	if strings.Contains(text, "Adapter") || strings.Contains(text, "paused") {
		t.Errorf("an open parent lists another job or says paused: %s", text)
	}

	closed, _ := detailOf(t, cronParentIssue("closed"), nil, jobs...)
	wantText(t, "a closed parent's detail", inspectorText(t, closed), "Weekly report", "paused: parent closed", "Closed: its Cron jobs are paused.")
}

func TestOpeningAParentsJobFromItsDetailOpensTheJobsPage(t *testing.T) {
	jobs := []model.CronJob{recurringJob("J1", "Weekly report", jobParent("open", false))}
	s, _ := detailOf(t, cronParentIssue("open"), nil, jobs...)
	opened := step(t, feed(t, s, key("enter")).State, key("enter"))
	if opened.State.Model.Page != model.PageJobs {
		t.Fatalf("page %v, want the Jobs Page", opened.State.Model.Page)
	}
}
