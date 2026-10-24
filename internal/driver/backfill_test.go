package driver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// owing is a native job whose parent progress stopped 3.5 hours before now: its hourly interval owes the runs at 09, 10, 11
// and 12 o'clock, four in all, when it runs again.
func owing(uid string, parent string, catchup string) model.CronJob {
	p := jobParent(parent, false)
	p.Progress = &cronjob.Progress{Anchor: jobAnchor, Through: now.Add(-3*time.Hour - 30*time.Minute)}
	job := nativeJob(uid, "Weekly "+uid, p)
	job.DefinitionEventUID = "EV1"
	job.Definition.Catchup = catchup
	return job
}

func disabled(job model.CronJob) model.CronJob {
	job.Definition.Enabled = false
	return job
}

func archived(job model.CronJob) model.CronJob {
	at := now
	job.DeletedAt = &at
	return job
}

func backfillText(t *testing.T, s driver.State) string {
	t.Helper()
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "layer.backfill" || strings.HasPrefix(n.ID, "layer.backfill.") {
			text += wireText(n)
		}
	}
	return text
}

// backfillOpen reports whether the Backfill card is on top.
func backfillOpen(s driver.State) bool { return s.Model.TopCard() == model.CardBackfill }

func sentRestore(cmds []model.Cmd) (model.RestoreJobCmd, bool) {
	for _, cmd := range cmds {
		if restore, ok := cmd.(model.RestoreJobCmd); ok {
			return restore, true
		}
	}
	return model.RestoreJobCmd{}, false
}

func sentReopen(cmds []model.Cmd) (model.ReopenIssueCmd, bool) {
	for _, cmd := range cmds {
		if reopen, ok := cmd.(model.ReopenIssueCmd); ok {
			return reopen, true
		}
	}
	return model.ReopenIssueCmd{}, false
}

// archivedJobsPane is the Jobs Page with the archived jobs listed.
func archivedJobsPane(t *testing.T, jobs ...model.CronJob) driver.State {
	t.Helper()
	listed := step(t, jobsPane(t, 160), key("a"))
	return answerJobs(t, listed, jobs...)
}

// closedParentPane is the Issues Page on the closed parent of jobs, its detail shown, the jobs read for it answered.
func closedParentPane(t *testing.T, jobs ...model.CronJob) driver.State {
	t.Helper()
	s, asked := detailOf(t, cronParentIssue("closed"), nil, jobs...)
	if !asked {
		t.Fatal("the parent's detail read no jobs")
	}
	return s
}

// pressReopen is `r` on the closed parent: the Pane reads the jobs afresh, and the daemon answers with jobs.
func pressReopen(t *testing.T, s driver.State, jobs ...model.CronJob) driver.Output {
	t.Helper()
	out := step(t, s, key("r"))
	if _, reopened := sentReopen(out.Cmds); reopened {
		t.Fatalf("r reopened before it knew what the jobs owe: %#v", out.Cmds)
	}
	load, ok := askedJobs(out.Cmds)
	if !ok || load.ProjectID != 7 {
		t.Fatalf("r asked %#v, want a read of the Project's jobs", out.Cmds)
	}
	return step(t, out.State, model.JobsLoaded{Request: load.Request, Jobs: jobs})
}

func TestResumingANativeJobThatMissedRunsAsksBeforeWritingAndCountsThem(t *testing.T) {
	s := jobsPane(t, 160, disabled(owing("J1", "open", "all")))
	out := step(t, s, key("p"))
	if len(out.Cmds) != 0 || !backfillOpen(out.State) || out.State.Model.CurrentMode() != model.ModeBackfill {
		t.Fatalf("p: cmds %#v, card on top %v, mode %s", out.Cmds, backfillOpen(out.State), out.State.Model.CurrentMode())
	}
	wantText(t, "card", backfillText(t, out.State), "Resume job?", "4 child Issues", "4 missed runs", "Weekly J1", "Create them", "Switch to latest and resume", "Cancel")

	create := step(t, out.State, key("c"))
	save, ok := savedJob(create.Cmds)
	if !ok || save.Definition.Catchup != model.CatchupAll || !save.Definition.Enabled || save.ExpectedEventUID != "EV1" || backfillOpen(create.State) {
		t.Fatalf("create them sent %#v, card on top %v", create.Cmds, backfillOpen(create.State))
	}

	latest := step(t, out.State, key("l"))
	save, ok = savedJob(latest.Cmds)
	if !ok || save.Definition.Catchup != model.CatchupLatest || !save.Definition.Enabled || save.ExpectedEventUID != "EV1" || save.UID != "J1" || len(latest.Cmds) != 1 {
		t.Fatalf("switch to latest sent %#v", latest.Cmds)
	}
	job := owing("J1", "open", "latest")
	job.Definition, job.DefinitionEventUID = save.Definition, "EV2"
	done := step(t, latest.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobReplaced, Job: &job})
	if backfillOpen(done.State) || len(done.Cmds) != 0 || !strings.Contains(done.State.Model.Notice, "latest") {
		t.Fatalf("after the write: card on top %v, cmds %#v, notice %q", backfillOpen(done.State), done.Cmds, done.State.Model.Notice)
	}
	wantText(t, "row", jobCardText(t, done.State, "J1"), "active")

	cancel := step(t, out.State, key("escape"))
	if len(cancel.Cmds) != 0 || backfillOpen(cancel.State) || cancel.State.Model.Jobs.Writing != "" || cancel.State.Model.SelectedJob().Definition.Enabled {
		t.Fatalf("cancel: cmds %#v, card on top %v, job %+v", cancel.Cmds, backfillOpen(cancel.State), cancel.State.Model.SelectedJob())
	}
}

func TestRestoringAnArchivedNativeJobThatMissedRunsAsksBeforeRestoring(t *testing.T) {
	s := archivedJobsPane(t, archived(owing("J1", "open", "all")))
	out := step(t, s, key("x"))
	if len(out.Cmds) != 0 || !backfillOpen(out.State) {
		t.Fatalf("x: cmds %#v, card on top %v", out.Cmds, backfillOpen(out.State))
	}
	wantText(t, "card", backfillText(t, out.State), "Restore job?", "4 child Issues", "Switch to latest and restore")

	create := step(t, out.State, key("c"))
	restore, ok := sentRestore(create.Cmds)
	if !ok || restore.UID != "J1" || restore.ExpectedEventUID != "EV1" || backfillOpen(create.State) {
		t.Fatalf("create them sent %#v", create.Cmds)
	}

	// A replace of the archived job on its own version restores it, so the catch-up changes in the write that restores.
	latest := step(t, out.State, key("l"))
	save, ok := savedJob(latest.Cmds)
	if !ok || save.Definition.Catchup != model.CatchupLatest || save.ExpectedEventUID != "EV1" || len(latest.Cmds) != 1 {
		t.Fatalf("switch to latest sent %#v", latest.Cmds)
	}

	cancel := step(t, out.State, key("escape"))
	if len(cancel.Cmds) != 0 || backfillOpen(cancel.State) || cancel.State.Model.Jobs.Writing != "" {
		t.Fatalf("cancel: cmds %#v, card on top %v", cancel.Cmds, backfillOpen(cancel.State))
	}
}

func TestReopeningAParentWithAnEnabledNativeJobThatMissedRunsAsksBeforeReopening(t *testing.T) {
	job := owing("J1", "closed", "all")
	asked := pressReopen(t, closedParentPane(t, job), job)
	if len(asked.Cmds) != 0 || !backfillOpen(asked.State) {
		t.Fatalf("after the read: cmds %#v, card on top %v", asked.Cmds, backfillOpen(asked.State))
	}
	wantText(t, "card", backfillText(t, asked.State), "Reopen Issue?", "4 child Issues", "Weekly J1", "4 missed runs", "Switch to latest and reopen")

	create := step(t, asked.State, key("c"))
	reopen, ok := sentReopen(create.Cmds)
	if !ok || reopen.UID != "parent-a" || reopen.ProjectID != 7 || backfillOpen(create.State) {
		t.Fatalf("create them sent %#v", create.Cmds)
	}

	latest := step(t, asked.State, key("l"))
	save, ok := savedJob(latest.Cmds)
	if !ok || save.Definition.Catchup != model.CatchupLatest || save.ExpectedEventUID != "EV1" || len(latest.Cmds) != 1 {
		t.Fatalf("switch to latest sent %#v", latest.Cmds)
	}
	switched := owing("J1", "closed", "latest")
	switched.Definition, switched.DefinitionEventUID = save.Definition, "EV2"
	done := step(t, latest.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobReplaced, Job: &switched})
	if reopen, ok = sentReopen(done.Cmds); !ok || reopen.UID != "parent-a" || backfillOpen(done.State) {
		t.Fatalf("after the catch-up write: cmds %#v, card on top %v", done.Cmds, backfillOpen(done.State))
	}

	cancel := step(t, asked.State, key("escape"))
	if len(cancel.Cmds) != 0 || backfillOpen(cancel.State) || cancel.State.Model.CloseForm.Saving {
		t.Fatalf("cancel: cmds %#v, card on top %v", cancel.Cmds, backfillOpen(cancel.State))
	}
}

func TestReopeningAParentWithSeveralOwingJobsSwitchesEachBeforeReopening(t *testing.T) {
	jobs := []model.CronJob{owing("J1", "closed", "all"), owing("J2", "closed", "all"), owing("J3", "closed", "skip")}
	asked := pressReopen(t, closedParentPane(t, jobs...), jobs...)
	text := backfillText(t, asked.State)
	wantText(t, "card", text, "8 child Issues", "Weekly J1", "Weekly J2")
	if strings.Contains(text, "Weekly J3") {
		t.Error("a skip job is listed as owing a Backfill")
	}
	out := step(t, asked.State, key("l"))
	for _, uid := range []string{"J1", "J2"} {
		save, ok := savedJob(out.Cmds)
		if !ok || save.UID != uid || save.Definition.Catchup != model.CatchupLatest {
			t.Fatalf("write for %s: %#v", uid, out.Cmds)
		}
		if _, reopened := sentReopen(out.Cmds); reopened {
			t.Fatalf("reopened before %s was switched", uid)
		}
		job := owing(uid, "closed", "latest")
		job.Definition, job.DefinitionEventUID = save.Definition, "EV2"
		out = step(t, out.State, model.JobWritten{ProjectID: 7, UID: uid, Op: model.JobReplaced, Job: &job})
	}
	if _, ok := sentReopen(out.Cmds); !ok || backfillOpen(out.State) {
		t.Fatalf("after both writes: cmds %#v", out.Cmds)
	}
}

func TestASwitchToLatestThatFailsOrFindsTheJobChangedDoesNotGoOn(t *testing.T) {
	s := jobsPane(t, 160, disabled(owing("J1", "open", "all")))
	latest := step(t, step(t, s, key("p")).State, key("l"))

	failed := step(t, latest.State, model.JobWritten{ProjectID: 7, UID: "J1", Err: errors.New("daemon said no")})
	if !backfillOpen(failed.State) || failed.State.Model.Backfill.Err == nil || len(failed.Cmds) != 0 {
		t.Fatalf("a failed write: card on top %v, error %v, cmds %#v", backfillOpen(failed.State), failed.State.Model.Backfill.Err, failed.Cmds)
	}
	if retry := step(t, failed.State, key("l")); len(retry.Cmds) != 1 {
		t.Fatalf("the choice could not be made again: %#v", retry.Cmds)
	}

	newer := owing("J1", "open", "all")
	newer.DefinitionEventUID = "EV9"
	moved := step(t, latest.State, model.JobWritten{ProjectID: 7, UID: "J1", Changed: &newer})
	if backfillOpen(moved.State) || len(moved.Cmds) != 0 || !strings.Contains(moved.State.Model.Notice, "changed meanwhile") {
		t.Fatalf("a moved guard: card on top %v, cmds %#v, notice %q", backfillOpen(moved.State), moved.Cmds, moved.State.Model.Notice)
	}

	job := owing("J1", "closed", "all")
	reopening := step(t, pressReopen(t, closedParentPane(t, job), job).State, key("l"))
	stale := owing("J1", "closed", "all")
	stale.DefinitionEventUID = "EV9"
	back := step(t, reopening.State, model.JobWritten{ProjectID: 7, UID: "J1", Changed: &stale})
	if _, reopened := sentReopen(back.Cmds); reopened || backfillOpen(back.State) {
		t.Fatalf("a moved guard still reopened: %#v", back.Cmds)
	}
}

func TestNothingIsAskedWhenNothingWasMissedOrTheJobCatchesUpWithLatestOrSkip(t *testing.T) {
	for _, tt := range []struct {
		name string
		job  func() model.CronJob
	}{
		{"latest", func() model.CronJob { return owing("J1", "open", "latest") }},
		{"skip", func() model.CronJob { return owing("J1", "open", "skip") }},
		{"nothing missed", func() model.CronJob {
			job := owing("J1", "open", "all")
			job.Parent.Progress.Through = now
			return job
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resume := step(t, jobsPane(t, 160, disabled(tt.job())), key("p"))
			if save, ok := savedJob(resume.Cmds); !ok || !save.Definition.Enabled || backfillOpen(resume.State) {
				t.Errorf("resume: cmds %#v, card on top %v", resume.Cmds, backfillOpen(resume.State))
			}
			restore := step(t, archivedJobsPane(t, archived(tt.job())), key("x"))
			if _, ok := sentRestore(restore.Cmds); !ok || backfillOpen(restore.State) {
				t.Errorf("restore: cmds %#v, card on top %v", restore.Cmds, backfillOpen(restore.State))
			}
			closedJob := tt.job()
			closedJob.Parent.Status = "closed"
			reopen := pressReopen(t, closedParentPane(t, closedJob), closedJob)
			if _, ok := sentReopen(reopen.Cmds); !ok || backfillOpen(reopen.State) {
				t.Errorf("reopen: cmds %#v, card on top %v", reopen.Cmds, backfillOpen(reopen.State))
			}
		})
	}
}

func TestNothingIsAskedWhenTheJobWouldStayPausedOrDoesNotRun(t *testing.T) {
	for name, job := range map[string]model.CronJob{
		"parent closed": archived(owing("J1", "closed", "all")),
		"job disabled":  disabled(archived(owing("J1", "open", "all"))),
	} {
		out := step(t, archivedJobsPane(t, job), key("x"))
		if _, ok := sentRestore(out.Cmds); !ok || backfillOpen(out.State) {
			t.Errorf("restore, %s: cmds %#v, card on top %v", name, out.Cmds, backfillOpen(out.State))
		}
	}
	off := disabled(owing("J1", "closed", "all"))
	reopen := pressReopen(t, closedParentPane(t, off), off)
	if _, ok := sentReopen(reopen.Cmds); !ok || backfillOpen(reopen.State) {
		t.Errorf("reopen with every job disabled: cmds %#v, card on top %v", reopen.Cmds, backfillOpen(reopen.State))
	}
}

func TestEnablingAPausedJobThroughTheJobCardAsksAboutTheBackfillAndSendsTheCardsDraft(t *testing.T) {
	s := step(t, jobsPane(t, 160, disabled(owing("J1", "open", "all"))), key("e")).State
	s = set(t, s, model.JobFieldName, "Renamed")
	s = choose(t, s, model.JobFieldEnabled, 0)
	out := step(t, s, key("ctrl+o"))
	if len(out.Cmds) != 0 || !backfillOpen(out.State) || !out.State.Model.JobForm.Open {
		t.Fatalf("saving sent %#v, Backfill card on top %v", out.Cmds, backfillOpen(out.State))
	}
	wantText(t, "card", backfillText(t, out.State), "4 child Issues", "4 missed runs", "Create them", "Switch to latest and save")

	create := step(t, out.State, key("c"))
	save, ok := savedJob(create.Cmds)
	if !ok || save.Name != "Renamed" || save.Definition.Catchup != model.CatchupAll || !save.Definition.Enabled || save.ExpectedEventUID != "EV1" || !create.State.Model.JobForm.Saving {
		t.Fatalf("create them sent %#v", create.Cmds)
	}

	latest := step(t, out.State, key("l"))
	save, ok = savedJob(latest.Cmds)
	if !ok || save.Name != "Renamed" || save.Definition.Catchup != model.CatchupLatest || !save.Definition.Enabled || len(latest.Cmds) != 1 {
		t.Fatalf("switch to latest sent %#v", latest.Cmds)
	}

	// Cancel leaves the card and its draft as they were.
	cancel := step(t, out.State, key("escape"))
	if len(cancel.Cmds) != 0 || backfillOpen(cancel.State) || !cancel.State.Model.JobForm.Open || cancel.State.Model.JobForm.Name != "Renamed" || cancel.State.Model.JobForm.Saving {
		t.Fatalf("cancel: cmds %#v, form %+v", cancel.Cmds, cancel.State.Model.JobForm)
	}
}

func TestSavingAPausedJobThatStaysPausedOrCatchesUpWithLatestAsksNothing(t *testing.T) {
	s := step(t, jobsPane(t, 160, disabled(owing("J1", "open", "all"))), key("e")).State
	s = set(t, s, model.JobFieldName, "Renamed")
	if _, ok := savedJob(step(t, s, key("ctrl+o")).Cmds); !ok {
		t.Fatal("a rename that leaves the job paused asked about a Backfill")
	}
	s = choose(t, choose(t, s, model.JobFieldEnabled, 0), model.JobFieldCatchup, 0)
	if backfillOpen(step(t, s, key("ctrl+o")).State) {
		t.Fatal("a catch-up of latest asked about a Backfill")
	}
}
