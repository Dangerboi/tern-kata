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

var (
	jobAnchor = time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	jobParent = func(status string, deleted bool) *model.CronJobParent {
		return &model.CronJobParent{UID: "parent-a", ShortID: "pa11", Title: "Weekly report", Status: status, Deleted: deleted,
			Progress: &model.CronProgress{Anchor: jobAnchor, Through: now}}
	}
)

// nativeJob is a native job: kata's daemon makes a child Issue per run of its hourly interval.
func nativeJob(uid, name string, parent *model.CronJobParent) model.CronJob {
	return model.CronJob{
		UID: uid, Name: name, Author: "tester", Revision: 2, CreatedAt: jobAnchor, UpdatedAt: jobAnchor,
		Definition: model.CronDefinition{
			Enabled: true, Overlap: "allow", Catchup: "all",
			Trigger: model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600},
			Action:  model.CronAction{Kind: model.ActionNotify, Recipient: "current-owner-or-author"},
			Issue:   &model.CronIssuePolicy{Kind: model.IssuePerRun, ParentUID: "parent-a", Title: "Tick", Body: "Write the report"},
		},
		Parent: parent,
	}
}

// promptJob is an adapter job: a worker runs its agent prompt every weekday at nine.
func promptJob(uid, name string) model.CronJob {
	return model.CronJob{
		UID: uid, Name: name, Author: "tester", Revision: 1, CreatedAt: jobAnchor, UpdatedAt: jobAnchor,
		Definition: model.CronDefinition{
			Enabled: true, Overlap: "forbid", Catchup: "skip", GraceSeconds: 60, Profile: "fast",
			Trigger: model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * 1-5", Timezone: "Europe/Berlin"},
			Action:  model.CronAction{Kind: model.ActionExecute, Prompt: "triage the inbox\nthen report"},
			Issue:   &model.CronIssuePolicy{Kind: model.IssuePerRun, Title: "Triage"},
		},
	}
}

// jobsPane is a Pane of the given width on the Jobs Page, the jobs read answered with jobs.
func jobsPane(t *testing.T, cols int, jobs ...model.CronJob) driver.State {
	t.Helper()
	s := projectPane(t, cols, boardCard("parent-a"))
	out := step(t, s, key("3"))
	return answerJobs(t, out, jobs...)
}

// answerJobs is the daemon answering the jobs read in out.
func answerJobs(t *testing.T, out driver.Output, jobs ...model.CronJob) driver.State {
	t.Helper()
	load, ok := askedJobs(out.Cmds)
	if !ok {
		t.Fatalf("no jobs read asked for: %#v", out.Cmds)
	}
	return step(t, out.State, model.JobsLoaded{Request: load.Request, Jobs: jobs}).State
}

func askedJobs(cmds []model.Cmd) (model.LoadJobs, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadJobs); ok {
			return load, true
		}
	}
	return model.LoadJobs{}, false
}

// jobCardText is everything the Jobs Page draws under the card of uid.
func jobCardText(t *testing.T, s driver.State, uid string) string {
	t.Helper()
	id := view.JobID(uid)
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id || strings.HasPrefix(n.ID, id+".") {
			text += wireText(n)
		}
	}
	if text == "" {
		t.Fatalf("the Jobs Page draws no card for %s", uid)
	}
	return text
}

func inspectorText(t *testing.T, s driver.State) string {
	t.Helper()
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == view.DetailID || strings.HasPrefix(n.ID, view.DetailID+".") {
			text += wireText(n)
		}
	}
	return text
}

func wantText(t *testing.T, what, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("%s lacks %q: %s", what, w, text)
		}
	}
}

func TestJobsPageKeyAsksForTheProjectsJobsAndThePageBarShowsTheCurrentPage(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"))
	out := step(t, s, key("3"))
	load, ok := askedJobs(out.Cmds)
	if !ok || load.ProjectID != 7 || load.Archived || load.Workspace != "/repo" {
		t.Fatalf("cmds %#v, want a read of the Project's jobs without the archived", out.Cmds)
	}
	if out.State.Model.Page != model.PageJobs {
		t.Fatalf("page %v", out.State.Model.Page)
	}
	if !drawnIDs(t, view.Render(out.State.Model, now))[view.PageBarEntryID("jobs")] {
		t.Error("the Page bar draws no Jobs Page bar entry")
	}
	clicked := step(t, s, click(view.PageBarEntryID("jobs"), view.KeyClick("3")))
	if _, ok := askedJobs(clicked.Cmds); !ok {
		t.Error("clicking the Jobs Page bar entry asks for no jobs")
	}
	back := step(t, out.State, key("1"))
	if back.State.Model.Page != model.PageIssues {
		t.Errorf("1 leaves the Jobs Page on page %v", back.State.Model.Page)
	}
}

func TestJobsPageRowsShowStateNameKindScheduleProfileAndNextRun(t *testing.T) {
	s := jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false)), promptJob("J2", "Triage"))

	wantText(t, "native job row", jobCardText(t, s, "J1"), "active", "Weekly", "recurring child", "every hour", "next 13:00")
	// 2026-10-08 is a Thursday: the next nine o'clock in Berlin is 07:00 UTC on Friday.
	wantText(t, "adapter job row", jobCardText(t, s, "J2"), "active", "Triage", "agent prompt", "every weekday at 09:00 Europe/Berlin", "profile fast", "next 2026-10-09 07:00")
}

func TestJobsPageLeavesOutWhatItCannotWorkOut(t *testing.T) {
	odd := promptJob("J3", "Odd")
	odd.Definition.Trigger = model.CronTrigger{Kind: model.TriggerCron, Cron: "not a schedule"}
	odd.Definition.Profile = ""
	text := jobCardText(t, jobsPane(t, 160, odd), "J3")
	for _, left := range []string{"next", "profile", "every", "Europe"} {
		if strings.Contains(text, left) {
			t.Errorf("a row whose schedule cannot be read still draws %q: %s", left, text)
		}
	}
}

func TestJobsPageShowsEachPauseReason(t *testing.T) {
	disabled := nativeJob("J3", "Off", jobParent("open", false))
	disabled.Definition.Enabled = false
	archivedAt := now.Add(-time.Hour)
	archived := promptJob("J4", "Retired")
	archived.DeletedAt = &archivedAt
	s := jobsPane(t, 160,
		nativeJob("J1", "Closed parent", jobParent("closed", false)),
		nativeJob("J2", "Deleted parent", jobParent("open", true)),
		disabled,
		promptJob("J5", "Running"),
	)
	// The archived job stays hidden until the toggle.
	shown := feed(t, s, key("a"))
	load, ok := askedJobs(shown.Cmds)
	if !ok || !load.Archived {
		t.Fatalf("a asks for %#v, want a read that includes the archived", shown.Cmds)
	}
	s = step(t, shown.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{
		nativeJob("J1", "Closed parent", jobParent("closed", false)),
		nativeJob("J2", "Deleted parent", jobParent("open", true)),
		disabled, archived, promptJob("J5", "Running"),
	}}).State
	for uid, want := range map[string]string{"J1": "paused: parent closed", "J2": "paused: parent deleted", "J3": "disabled", "J4": "archived", "J5": "active"} {
		wantText(t, uid, jobCardText(t, s, uid), want)
	}
	// None of the paused rows promises a next run.
	for _, uid := range []string{"J1", "J2", "J3", "J4"} {
		if text := jobCardText(t, s, uid); strings.Contains(text, "next ") {
			t.Errorf("%s is not running but draws a next run: %s", uid, text)
		}
	}
}

func TestJobsPageOfAnArchivedProjectPausesTheJobsItHolds(t *testing.T) {
	s := jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false)), promptJob("J2", "Triage"))
	out := step(t, s, key("ctrl+r"))
	load, ok := askedJobs(out.Cmds)
	if !ok {
		t.Fatalf("ctrl+r asks for %#v, want a read", out.Cmds)
	}
	s = step(t, out.State, model.JobsLoaded{Request: load.Request, ProjectArchived: true}).State
	for _, uid := range []string{"J1", "J2"} {
		wantText(t, uid, jobCardText(t, s, uid), "paused: Project archived")
	}
	wantText(t, "Jobs Page", wireTextOf(t, s), "This Project is archived")
}

func wireTextOf(t *testing.T, s driver.State) string {
	t.Helper()
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		text += wireText(n)
	}
	return text
}

func TestJobsPageArchivedToggleHidesAndShowsArchivedJobs(t *testing.T) {
	archivedAt := now.Add(-time.Hour)
	retired := promptJob("J2", "Retired")
	retired.DeletedAt = &archivedAt
	s := jobsPane(t, 160, promptJob("J1", "Live"))
	on := feed(t, s, key("a"))
	load, _ := askedJobs(on.Cmds)
	s = step(t, on.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{promptJob("J1", "Live"), retired}}).State
	if !drawnIDs(t, view.Render(s.Model, now))[view.JobID("J2")] {
		t.Fatal("the archived job is not drawn after the toggle")
	}
	off := step(t, s, key("a"))
	load, ok := askedJobs(off.Cmds)
	if !ok || load.Archived {
		t.Fatalf("a again asks for %#v, want a read without the archived", off.Cmds)
	}
	s = step(t, off.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{promptJob("J1", "Live")}}).State
	if drawnIDs(t, view.Render(s.Model, now))[view.JobID("J2")] {
		t.Error("the archived job is still drawn after the toggle is off")
	}
}

func TestJobsPageInspectorShowsTheSelectedJobsDefinitionAndParent(t *testing.T) {
	s := jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false)), promptJob("J2", "Triage"))
	wantText(t, "inspector of the native job", inspectorText(t, s),
		"Weekly", "recurring child", "Child title", "Tick", "Write the report", "current-owner-or-author", "Catchup", "all",
		"#pa11 Weekly report · open", "Delivered through")

	s = feed(t, s, key("j")).State
	wantText(t, "inspector of the prompt job", inspectorText(t, s),
		"Triage", "triage the inbox\\nthen report", "0 9 * * 1-5", "Europe/Berlin", "Profile", "fast", "skip, grace 60 s")
	if strings.Contains(inspectorText(t, s), "Parent") {
		t.Error("an adapter job has no parent to show")
	}

	// A click on a card selects it too.
	s = step(t, s, click(view.JobID("J1"), view.JobClick("J1"))).State
	wantText(t, "inspector after a click", inspectorText(t, s), "Weekly")
}

func TestJobsPageSelectionMovesWithTheKeysAndIsRevealed(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "A"), promptJob("J2", "B"), promptJob("J3", "C"))
	out := step(t, s, key("j"))
	if out.State.Model.Jobs.Selected != "J2" {
		t.Fatalf("selected %q after j, want J2", out.State.Model.Jobs.Selected)
	}
	wantOps(t, out, tern.OpReveal(view.JobID("J2"), "nearest"))
	if got := feed(t, out.State, tern.Key{Name: "g", Shift: true}).State.Model.Jobs.Selected; got != "J3" {
		t.Errorf("G selects %q, want J3", got)
	}
	if got := feed(t, out.State, key("g")).State.Model.Jobs.Selected; got != "J1" {
		t.Errorf("g selects %q, want J1", got)
	}
}

func TestEnterOnANativeJobsParentOpensItOnTheIssuesPage(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J0", "Adapter"), nativeJob("J1", "Weekly", jobParent("open", false)))
	s = feed(t, s, key("j")).State
	inspected := step(t, s, key("enter"))
	if inspected.State.Model.CurrentMode() != model.ModeJobInspector {
		t.Fatalf("enter on the list leaves mode %q, want the inspector", inspected.State.Model.CurrentMode())
	}
	opened := step(t, inspected.State, key("enter"))
	m := opened.State.Model
	if m.Page != model.PageIssues || m.SelectedUID != "parent-a" || m.Focus != model.ColumnDetail {
		t.Fatalf("page %v selected %q focus %v, want the parent's detail on the Issues Page", m.Page, m.SelectedUID, m.Focus)
	}
	loaded := false
	for _, cmd := range opened.Cmds {
		if load, ok := cmd.(model.LoadDetail); ok && load.UID == "parent-a" {
			loaded = true
		}
	}
	if !loaded {
		t.Errorf("cmds %#v, want the parent's detail loaded", opened.Cmds)
	}
}

func TestEnterOnAnAdapterJobsInspectorOpensNothingAndADeletedParentSaysSo(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "Adapter"))
	got := feed(t, s, key("enter"), key("enter")).State.Model
	if got.Page != model.PageJobs {
		t.Errorf("enter on an adapter job's inspector left the Jobs Page for %v", got.Page)
	}
	s = jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", true)))
	got = feed(t, s, key("enter"), key("enter")).State.Model
	if got.Page != model.PageJobs || !strings.Contains(got.Notice, "deleted") {
		t.Errorf("page %v notice %q, want the Jobs Page and a notice that the parent is deleted", got.Page, got.Notice)
	}
}

func TestJobsPageNarrowShowsTheListThenTheInspector(t *testing.T) {
	s := jobsPane(t, 100, nativeJob("J1", "Weekly", jobParent("open", false)))
	ids := drawnIDs(t, view.Render(s.Model, now))
	if !ids[view.JobID("J1")] || ids[view.DetailID] {
		t.Fatalf("narrow list draws the card %v and the inspector %v, want the card only", ids[view.JobID("J1")], ids[view.DetailID])
	}
	s = feed(t, s, key("enter")).State
	ids = drawnIDs(t, view.Render(s.Model, now))
	if ids[view.JobID("J1")] || !ids[view.DetailID] {
		t.Fatalf("narrow inspector draws the card %v and the inspector %v, want the inspector only", ids[view.JobID("J1")], ids[view.DetailID])
	}
}

func TestJobsPageReadsTheJobsAgainOnEveryDaemonEvent(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "A"))
	out := step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "cron.job.created"})
	if _, ok := askedJobs(out.Cmds); !ok {
		t.Fatalf("a daemon event asks for %#v, want the jobs read again", out.Cmds)
	}
}

func TestJobsPageFollowsTheMenuToAnotherCollection(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "A"))
	out := step(t, s, model.CollectionSelected{Collection: model.InboxCollection()})
	if out.State.Model.Page != model.PageIssues {
		t.Fatalf("page %v after leaving the Project, want the Issues Page", out.State.Model.Page)
	}
}

func TestJobsPageNeedsAProject(t *testing.T) {
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}})
	out := step(t, s, key("3"))
	if out.State.Model.Page != model.PageIssues || !strings.Contains(out.State.Model.Notice, "Jobs Page needs a Project") {
		t.Fatalf("page %v notice %q", out.State.Model.Page, out.State.Model.Notice)
	}
}

func TestFooterAndHelpSheetListTheJobsPageKeys(t *testing.T) {
	s := jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false)))
	wantText(t, "Jobs Page footer", footerText(t, s), "archived", "inspect")
	inspector := feed(t, s, key("enter")).State
	wantText(t, "inspector footer", footerText(t, inspector), "open parent", "back")
	help := wireTextOf(t, feed(t, s, key("?")).State)
	wantText(t, "help sheet", help, "Jobs", "show or hide archived jobs", "open a native job's parent Issue", "inspect the job", "switch Page")
}

func TestJobsPageDoesNotRevealACardAReadJustAdded(t *testing.T) {
	// The archived job is new to the list when the toggle's read answers; the selection settles on it and nothing reveals it.
	archivedAt := now.Add(-time.Hour)
	retired := promptJob("J1", "Retired")
	retired.DeletedAt = &archivedAt
	s := jobsPane(t, 160)
	on := step(t, s, key("a"))
	load, _ := askedJobs(on.Cmds)
	answered := step(t, on.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{retired}})
	if answered.State.Model.Jobs.Selected != "J1" {
		t.Fatalf("selected %q, want the archived job", answered.State.Model.Jobs.Selected)
	}
	wantOps(t, answered)
}

func TestJobsPageRowsShowTheLatestRunOrChildAndTheInspectorTheRecentOnes(t *testing.T) {
	out := step(t, projectPane(t, 160, boardCard("parent-a")), key("3"))
	if load, ok := askedJobs(out.Cmds); !ok || !load.Activity {
		t.Fatalf("the Jobs Page asked %#v, want the jobs read with what each has done lately", out.Cmds)
	}
	failed := finishedRun("R2")
	failed.Status, failed.Message = model.RunFailed, "step build failed"
	older := finishedRun("R1")
	load, _ := askedJobs(out.Cmds)
	s := step(t, out.State, model.JobsLoaded{Request: load.Request,
		Jobs: []model.CronJob{nativeJob("N1", "Hourly", jobParent("open", false)), promptJob("J3", "Idle"), promptJob("J1", "Triage")},
		Activity: map[string]model.JobActivity{
			"J1": {Runs: []model.Run{failed, older}},
			"N1": {Children: []model.CronChild{
				{UID: "c2", ShortID: "ab12", Title: "Tick — 2026-10-08T11:00:00Z", Status: "open", ScheduledAt: now.Add(-1 * time.Hour)},
				{UID: "c1", ShortID: "cd34", Title: "Tick — 2026-10-08T10:00:00Z", Status: "closed", ScheduledAt: now.Add(-2 * time.Hour)},
			}},
		}}).State
	wantText(t, "adapter row", jobCardText(t, s, "J1"), "last run failed", "step build failed")
	wantText(t, "native row", jobCardText(t, s, "N1"), "last child #ab12", "open")
	if strings.Contains(jobCardText(t, s, "J3"), "last") {
		t.Errorf("a job that did nothing yet draws a last line: %s", jobCardText(t, s, "J3"))
	}

	wantText(t, "native inspector", inspectorText(t, s), "Recent children", "#ab12", "#cd34", "closed")
	adapter := feed(t, s, key("j"), key("j")).State
	wantText(t, "adapter inspector", inspectorText(t, adapter), "Recent runs", "failed", "step build failed", "succeeded", "triaged 4 items")
}

func TestJobsPageWarnsOfAnAdapterJobMakingMoreThanADayOfIssuesPerDay(t *testing.T) {
	everyMinute := promptJob("J1", "Chatty")
	everyMinute.Definition.Trigger = model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 60}
	existing := promptJob("J4", "Reports to one Issue")
	existing.Definition.Trigger = model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 60}
	existing.Definition.Issue = &model.CronIssuePolicy{Kind: model.IssueExisting, UID: "parent-a"}
	native := nativeJob("N1", "Native", jobParent("open", false))
	native.Definition.Trigger = model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 60}
	disabled := promptJob("J5", "Switched off")
	disabled.Definition.Trigger = model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 60}
	disabled.Definition.Enabled = false

	s := jobsPane(t, 160, everyMinute, existing, native, disabled)
	wantText(t, "every minute", jobCardText(t, s, "J1"), "creates about 1440 Issues a day; consider an existing Issue")
	for uid, why := range map[string]string{"J4": "it reports to an existing Issue", "N1": "a recurring child is made of children", "J5": "it is disabled and makes none"} {
		if strings.Contains(jobCardText(t, s, uid), "Issues a day") {
			t.Errorf("%s warns, but %s", uid, why)
		}
	}
	wantText(t, "inspector", inspectorText(t, s), "creates about 1440 Issues a day")
}
