package driver_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// guarded is the native job every test below edits, at the version its guard names.
func guarded(uid, name, event string) model.CronJob {
	job := nativeJob(uid, name, jobParent("open", false))
	job.DefinitionEventUID = event
	return job
}

// set puts text into the Job card's text field, as a click on it, a clear and typing do.
func set(t *testing.T, s driver.State, field model.Field, text string) driver.State {
	t.Helper()
	s = step(t, s, &tern.Focus{ID: view.JobFieldID(field)}).State
	s = step(t, s, key("ctrl+u")).State
	if text == "" {
		return s
	}
	return step(t, s, tern.Key{Text: text}).State
}

// choose picks a choice of a Job card field, as a click on it does.
func choose(t *testing.T, s driver.State, field model.Field, index int) driver.State {
	t.Helper()
	return step(t, s, model.ChooseMsg{Field: field, Index: index}).State
}

// newJobCard is the Jobs Page with the card open for a new job, filled in so that it saves; parent names the Issue.
func newJobCard(t *testing.T) driver.State {
	t.Helper()
	s := jobsPane(t, 160, guarded("J1", "Weekly", "EV1"))
	s = step(t, s, key("n")).State
	s = set(t, s, model.JobFieldName, "Report")
	s = set(t, s, model.JobFieldParent, "parent-a")
	return set(t, s, model.JobFieldTitle, "Report time")
}

func savedJob(cmds []model.Cmd) (model.SaveJobCmd, bool) {
	for _, cmd := range cmds {
		if save, ok := cmd.(model.SaveJobCmd); ok {
			return save, true
		}
	}
	return model.SaveJobCmd{}, false
}

// conflictedJobCard is a new job's card after its save found a job at the same uid that differs from the draft.
func conflictedJobCard(t *testing.T) driver.State {
	t.Helper()
	out := step(t, newJobCard(t), key("ctrl+o"))
	save, _ := savedJob(out.Cmds)
	newer := guarded(save.UID, "Someone else's", "EV2")
	return step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Changed: &newer}).State
}

func cardText(t *testing.T, s driver.State) string {
	t.Helper()
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "layer.job" || strings.HasPrefix(n.ID, "layer.job.") {
			text += wireText(n)
		}
	}
	return text
}

func drawn(t *testing.T, s driver.State, field model.Field) bool {
	t.Helper()
	return drawnIDs(t, view.Render(s.Model, now))[view.JobFieldID(field)]
}

var ulid = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func TestNewJobFromTheJobsPageSendsAGuardlessCreateAtAMintedUID(t *testing.T) {
	s := newJobCard(t)
	out := step(t, s, key("ctrl+o"))
	save, ok := savedJob(out.Cmds)
	if !ok {
		t.Fatalf("saving sent %#v", out.Cmds)
	}
	if !ulid.MatchString(save.UID) || save.ExpectedEventUID != "" || save.ParentRef != "parent-a" || save.ProjectID != 7 || save.Name != "Report" {
		t.Fatalf("create %+v", save)
	}
	want := cronjob.Definition{
		Enabled: true, Overlap: "allow", Catchup: model.CatchupLatest,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "UTC"},
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"},
		Issue:   &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, Title: "Report time"},
	}
	if got := save.Definition; got.Trigger != want.Trigger || got.Action != want.Action || *got.Issue != *want.Issue || got.Enabled != want.Enabled || got.Overlap != want.Overlap || got.Catchup != want.Catchup || got.GraceSeconds != 0 {
		t.Fatalf("definition %+v issue %+v, want %+v", got, got.Issue, want)
	}
	// Another card mints another uid, and a resend of this card's save keeps its own.
	other := feed(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Err: errTest}, key("escape"), key("n")).State.Model.JobForm.UID
	if other == save.UID || !ulid.MatchString(other) {
		t.Fatalf("second card minted %q after %q", other, save.UID)
	}
	if !step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Err: errTest}).State.Model.JobForm.Open {
		t.Fatal("a failed save closed the card")
	}
}

var errTest = testError("daemon said no")

type testError string

func (e testError) Error() string { return string(e) }

func TestAFailedSaveKeepsTheCardAndTheDraftAndShowsKatasWords(t *testing.T) {
	out := step(t, newJobCard(t), key("ctrl+o"))
	save, _ := savedJob(out.Cmds)
	failed := step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Err: errTest}).State
	form := failed.Model.JobForm
	if !form.Open || form.Saving || form.Name != "Report" || form.Title != "Report time" || form.Err == nil {
		t.Fatalf("form after a failed save %+v", form)
	}
	wantText(t, "card", cardText(t, failed), "daemon said no")
}

func TestASavedJobClosesTheCardAndJoinsTheListSelected(t *testing.T) {
	out := step(t, newJobCard(t), key("ctrl+o"))
	save, _ := savedJob(out.Cmds)
	created := guarded(save.UID, "Report", "EV9")
	done := step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Op: model.JobCreated, Job: &created}).State
	m := done.Model
	if m.JobForm.Open || m.Jobs.Selected != save.UID || len(m.Jobs.Jobs) != 2 || !strings.Contains(m.Notice, "Created job") {
		t.Fatalf("after the create: card open %v, selected %q, %d jobs, notice %q", m.JobForm.Open, m.Jobs.Selected, len(m.Jobs.Jobs), m.Notice)
	}
	wantText(t, "list", jobCardText(t, done, save.UID), "Report")
}

func TestJOnAnIssueOpensTheCardWithThatIssueAsParentFromEveryMainSurface(t *testing.T) {
	for name, keys := range map[string][]string{"list": nil, "Board": {"b"}, "detail": {"enter"}} {
		t.Run(name, func(t *testing.T) {
			card := boardCard("alpha")
			card.ProjectID = 7
			s := projectPane(t, 160, card)
			if keys != nil {
				s = feed(t, s, key(keys[0])).State
				if keys[0] == "b" {
					s = feed(t, s, model.LanesLoaded{ProjectUID: "p"}).State
				}
			}
			out := step(t, s, key("shift+j"))
			f := out.State.Model.JobForm
			if !f.Open || f.Change || f.Parent != "alpha" || f.ProjectID != 7 || out.State.Model.CurrentMode() != model.ModeJobForm {
				t.Fatalf("form %+v mode %s", f, out.State.Model.CurrentMode())
			}
			wantOps(t, out, tern.OpFocus(view.JobFieldID(model.JobFieldName)))
		})
	}
}

func TestEditOpensTheCardPrefilledAndSavesAGuardedReplaceKeepingTheParent(t *testing.T) {
	s := jobsPane(t, 160, guarded("J1", "Weekly", "EV1"))
	s = step(t, s, key("e")).State
	if !s.Model.JobForm.Open || !s.Model.JobForm.Change {
		t.Fatalf("the card did not open to change the job: %+v", s.Model.JobForm)
	}
	wantText(t, "prefilled card", cardText(t, s), "Weekly", "3600", "Tick", "Write the report", "Every missed run", "every hour")
	if drawn(t, s, model.JobFieldParent) {
		t.Error("a job's parent can be edited; it is fixed")
	}
	wantText(t, "card", cardText(t, s), "Parent: #pa11 Weekly report")
	s = set(t, s, model.JobFieldName, "Weekly digest")
	out := step(t, s, key("ctrl+o"))
	save, ok := savedJob(out.Cmds)
	if !ok || save.UID != "J1" || save.ExpectedEventUID != "EV1" || save.Name != "Weekly digest" || save.ParentRef != "" || save.Definition.Issue.ParentUID != "parent-a" {
		t.Fatalf("replace %+v", save)
	}
	if got := save.Definition.Trigger; got.Kind != cronjob.TriggerInterval || got.IntervalSeconds != 3600 {
		t.Errorf("trigger %+v", got)
	}
}

func TestEditRefusesAnArchivedJobAndAnArchivedProject(t *testing.T) {
	retired := guarded("J1", "Old", "EV1")
	at := now
	retired.DeletedAt = &at
	s := jobsPane(t, 160, retired)
	on := step(t, s, key("a"))
	load, _ := askedJobs(on.Cmds)
	s = step(t, on.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{retired}}).State
	if got := step(t, s, key("e")).State.Model; got.JobForm.Open || !strings.Contains(got.Notice, "archived") {
		t.Errorf("archived job: card open %v notice %q", got.JobForm.Open, got.Notice)
	}
}

func TestJobCardShowsTheFieldsOfItsTriggerOnly(t *testing.T) {
	s := step(t, jobsPane(t, 160), key("n")).State
	for _, field := range []model.Field{model.JobFieldCron, model.JobFieldZone} {
		if !drawn(t, s, field) {
			t.Errorf("a cron trigger draws no %s field", field)
		}
	}
	if drawn(t, s, model.JobFieldInterval) || drawn(t, s, model.JobFieldGrace) {
		t.Error("a cron trigger with a latest catch-up draws the interval or the grace")
	}
	s = choose(t, s, model.JobFieldTrigger, 1)
	if !drawn(t, s, model.JobFieldInterval) || drawn(t, s, model.JobFieldCron) || drawn(t, s, model.JobFieldZone) {
		t.Error("an interval trigger draws the cron fields or no interval")
	}
	// Tab skips the hidden fields: from the trigger it goes to the interval, then the parent.
	s = step(t, s, &tern.Focus{ID: view.JobFieldID(model.JobFieldTrigger)}).State
	s = step(t, s, key("tab")).State
	if got := s.Model.JobForm.Field; got != model.JobFieldInterval {
		t.Fatalf("tab from the trigger reached %q, want interval", got)
	}
	if got := step(t, s, key("tab")).State.Model.JobForm.Field; got != model.JobFieldParent {
		t.Fatalf("tab from the interval reached %q, want parent", got)
	}
	// The grace belongs to the skip catch-up alone.
	s = choose(t, s, model.JobFieldCatchup, 2)
	if !drawn(t, s, model.JobFieldGrace) {
		t.Error("a skip catch-up draws no grace")
	}
	// The kind shows native only for now.
	wantText(t, "kind field", cardText(t, s), "Recurring child")
}

func TestJobCardPreviewsTheScheduleInWordsAndTheNextThreeRuns(t *testing.T) {
	s := step(t, jobsPane(t, 160), key("n")).State
	// The Pane's clock is Thursday 2026-10-08 12:00 UTC.
	wantText(t, "cron preview", cardText(t, s), "every day at 09:00 UTC", "Fri 2026-10-09 09:00 UTC", "Sat 2026-10-10 09:00 UTC", "Sun 2026-10-11 09:00 UTC")

	s = set(t, s, model.JobFieldCron, "*/20 * * * *")
	s = set(t, s, model.JobFieldZone, "Europe/Berlin")
	wantText(t, "preview follows the fields", cardText(t, s), "every 20 minutes Europe/Berlin", "Thu 2026-10-08 14:20 CEST", "Thu 2026-10-08 14:40 CEST", "Thu 2026-10-08 15:00 CEST")

	s = set(t, s, model.JobFieldCron, "*/40 * * * *")
	wantText(t, "a step that does not divide an hour reads as its expression", cardText(t, s), "cron */40 * * * * Europe/Berlin")
	if strings.Contains(cardText(t, s), "every 40 minutes") {
		t.Errorf("a */40 step is worded as every 40 minutes, but it fires at :00 and :40: %s", cardText(t, s))
	}

	s = choose(t, s, model.JobFieldTrigger, 1)
	s = set(t, s, model.JobFieldInterval, "15m")
	wantText(t, "interval preview", cardText(t, s), "every 15 minutes", "Thu 2026-10-08 12:15 UTC", "Thu 2026-10-08 12:30 UTC", "Thu 2026-10-08 12:45 UTC")

	s = set(t, s, model.JobFieldInterval, "30")
	wantText(t, "interval below the minimum", cardText(t, s), "not yet", "Interval", "at least 60")
	if strings.Contains(cardText(t, s), "Next runs") {
		t.Error("an interval below the minimum still lists runs")
	}
}

func TestJobCardRefusesInvalidInputNamingTheFieldAndSendsNothing(t *testing.T) {
	for _, tt := range []struct {
		name   string
		break_ func(*testing.T, driver.State) driver.State
		field  model.Field
		want   string
	}{
		{"cron expression", func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldCron, "nonsense") }, model.JobFieldCron, "Cron expression"},
		{"time zone", func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldZone, "Mars/Olympus") }, model.JobFieldZone, "Time zone"},
		{"interval under a minute", func(t *testing.T, s driver.State) driver.State {
			return set(t, choose(t, s, model.JobFieldTrigger, 1), model.JobFieldInterval, "30")
		}, model.JobFieldInterval, "Interval"},
		{"skip without grace", func(t *testing.T, s driver.State) driver.State { return choose(t, s, model.JobFieldCatchup, 2) }, model.JobFieldGrace, "Grace"},
		{"blank child title", func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldTitle, "") }, model.JobFieldTitle, "Child title"},
		{"blank name", func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldName, "") }, model.JobFieldName, "Name"},
		{"blank parent", func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldParent, "") }, model.JobFieldParent, "Parent Issue"},
		{"recipient with a space", func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldRecipient, "two words")
		}, model.JobFieldRecipient, "Inbox recipient"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.break_(t, newJobCard(t))
			out := step(t, s, key("ctrl+o"))
			if _, sent := savedJob(out.Cmds); sent || len(out.Cmds) != 0 {
				t.Fatalf("sent %#v for invalid input", out.Cmds)
			}
			f := out.State.Model.JobForm
			if f.Saving || f.Err == nil || !strings.HasPrefix(f.Err.Error(), tt.want+":") || f.Field != tt.field {
				t.Fatalf("saving %v error %v focus %q, want %q naming %q", f.Saving, f.Err, f.Field, tt.field, tt.want)
			}
			wantText(t, "card", cardText(t, out.State), tt.want+":")
		})
	}
}

func TestAnEveryMinuteIntervalAndAnAddressedRecipientSaveAsTyped(t *testing.T) {
	s := choose(t, newJobCard(t), model.JobFieldTrigger, 1)
	s = set(t, s, model.JobFieldInterval, "60")
	s = set(t, s, model.JobFieldRecipient, "alice/reviewer")
	s = set(t, s, model.JobFieldMessage, "Time for the report")
	s = choose(t, s, model.JobFieldCatchup, 2)
	s = set(t, s, model.JobFieldGrace, "300")
	s = choose(t, s, model.JobFieldEnabled, 1)
	save, ok := savedJob(step(t, s, key("ctrl+o")).Cmds)
	if !ok {
		t.Fatal("a valid card sent nothing")
	}
	d := save.Definition
	if d.Trigger != (cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 60}) || d.Action.Recipient != "alice/reviewer" || d.Action.Message != "Time for the report" ||
		d.Catchup != model.CatchupSkip || d.GraceSeconds != 300 || d.Enabled {
		t.Fatalf("definition %+v", d)
	}
}

func TestASaveAgainstAJobChangedMeanwhileWritesNothingShowsTheNewerJobAndKeepsTheDraft(t *testing.T) {
	s := step(t, jobsPane(t, 160, guarded("J1", "Weekly", "EV1")), key("e")).State
	s = set(t, s, model.JobFieldName, "My draft")
	out := step(t, s, key("ctrl+o"))
	newer := guarded("J1", "Their name", "EV2")
	newer.Definition.Issue.Title = "Their tick"
	s = step(t, out.State, model.JobWritten{ProjectID: 7, UID: "J1", Changed: &newer}).State
	m := s.Model
	if m.CurrentMode() != model.ModeJobConflict || m.JobForm.Name != "My draft" || m.JobForm.Conflict == nil {
		t.Fatalf("mode %s name %q conflict %v", m.CurrentMode(), m.JobForm.Name, m.JobForm.Conflict)
	}
	wantText(t, "conflict card", cardText(t, s), "changed meanwhile", "nothing was written", "Their name", "Their tick", "every hour")
	// The list already shows the newer job.
	if got := m.SelectedJob().Name; got != "Their name" {
		t.Errorf("the list holds %q, want the newer job", got)
	}
	// A key that is no choice does not save: saving again needs an explicit choice.
	if again := step(t, s, key("ctrl+o")); len(again.Cmds) != 0 || again.State.Model.CurrentMode() != model.ModeJobConflict {
		t.Fatalf("ctrl+o in the conflict sent %#v", again.Cmds)
	}

	overwrite := step(t, s, key("o"))
	save, ok := savedJob(overwrite.Cmds)
	if !ok || save.ExpectedEventUID != "EV2" || save.Name != "My draft" {
		t.Fatalf("overwrite sent %#v, want my draft guarded on the newer version", overwrite.Cmds)
	}

	taken := step(t, s, key("u")).State.Model
	if taken.CurrentMode() != model.ModeJobForm || taken.JobForm.Name != "Their name" || taken.JobForm.Title != "Their tick" || taken.JobForm.Opened.DefinitionEventUID != "EV2" {
		t.Fatalf("taking the newer job gave %+v", taken.JobForm)
	}

	discarded := step(t, s, key("d")).State.Model
	if discarded.JobForm.Open {
		t.Error("discard left the card open")
	}
}

func TestPauseResumeArchiveAndRestoreAreGuardedWritesThatWaitForTheirAnswer(t *testing.T) {
	live := guarded("J1", "Weekly", "EV1")
	s := jobsPane(t, 160, live)

	paused := step(t, s, key("p"))
	save, ok := savedJob(paused.Cmds)
	if !ok || save.UID != "J1" || save.ExpectedEventUID != "EV1" || save.Definition.Enabled || save.Name != "Weekly" || save.Definition.Issue.ParentUID != "parent-a" {
		t.Fatalf("pause sent %#v", paused.Cmds)
	}
	if again := step(t, paused.State, key("p")); len(again.Cmds) != 0 {
		t.Fatalf("p while the write is in flight sent %#v", again.Cmds)
	}
	off := live
	off.Definition.Enabled, off.DefinitionEventUID = false, "EV2"
	s = step(t, paused.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobReplaced, Job: &off}).State
	wantText(t, "paused row", jobCardText(t, s, "J1"), "disabled")
	if !strings.Contains(s.Model.Notice, "Paused") {
		t.Errorf("notice %q", s.Model.Notice)
	}

	resumed := step(t, s, key("p"))
	save, ok = savedJob(resumed.Cmds)
	if !ok || save.ExpectedEventUID != "EV2" || !save.Definition.Enabled {
		t.Fatalf("resume sent %#v", resumed.Cmds)
	}
	on := off
	on.Definition.Enabled, on.DefinitionEventUID = true, "EV3"
	s = step(t, resumed.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobReplaced, Job: &on}).State
	wantText(t, "resumed row", jobCardText(t, s, "J1"), "active")

	archiving := step(t, s, key("x"))
	var archive model.ArchiveJobCmd
	for _, cmd := range archiving.Cmds {
		archive, _ = cmd.(model.ArchiveJobCmd)
	}
	if archive.UID != "J1" || archive.ExpectedEventUID != "EV3" || archive.ProjectID != 7 {
		t.Fatalf("x sent %#v", archiving.Cmds)
	}
	gone := on
	at := now
	gone.DeletedAt, gone.DefinitionEventUID = &at, "EV4"
	s = step(t, archiving.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobArchived, Job: &gone}).State
	if drawnIDs(t, view.Render(s.Model, now))[view.JobID("J1")] {
		t.Error("an archived job is still listed with the archived hidden")
	}

	// Restore comes from the archived view.
	listed := step(t, s, key("a"))
	load, _ := askedJobs(listed.Cmds)
	s = step(t, listed.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{gone}}).State
	wantText(t, "archived footer", footerText(t, s), "restore")
	restoring := step(t, s, key("x"))
	var restore model.RestoreJobCmd
	for _, cmd := range restoring.Cmds {
		restore, _ = cmd.(model.RestoreJobCmd)
	}
	if restore.UID != "J1" || restore.ExpectedEventUID != "EV4" {
		t.Fatalf("x on an archived job sent %#v", restoring.Cmds)
	}
	back := on
	back.DefinitionEventUID = "EV5"
	s = step(t, restoring.State, model.JobWritten{ProjectID: 7, UID: "J1", Op: model.JobRestored, Job: &back}).State
	wantText(t, "restored row", jobCardText(t, s, "J1"), "active")
}

func TestJobKeysThatDoNotApplyExplainInsteadOfWriting(t *testing.T) {
	s := jobsPane(t, 160, guarded("J1", "Weekly", "EV1"))
	retired := guarded("J2", "Old", "EV1")
	at := now
	retired.DeletedAt = &at
	on := step(t, s, key("a"))
	load, _ := askedJobs(on.Cmds)
	s = step(t, on.State, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{retired}}).State
	for _, c := range []string{"p"} {
		if out := step(t, s, key(c)); len(out.Cmds) != 0 || out.State.Model.Notice == "" {
			t.Errorf("%s on an archived job: cmds %#v notice %q", c, out.Cmds, out.State.Model.Notice)
		}
	}
}

func TestAPauseAgainstAJobChangedMeanwhileWritesNothingAndShowsTheNewerJob(t *testing.T) {
	s := jobsPane(t, 160, guarded("J1", "Weekly", "EV1"))
	paused := step(t, s, key("p"))
	newer := guarded("J1", "Renamed", "EV2")
	s = step(t, paused.State, model.JobWritten{ProjectID: 7, UID: "J1", Changed: &newer}).State
	m := s.Model
	if m.Jobs.Writing != "" || m.SelectedJob().Name != "Renamed" || !strings.Contains(m.Notice, "changed meanwhile") || !strings.Contains(m.Notice, "nothing was written") {
		t.Fatalf("writing %q selected %+v notice %q", m.Jobs.Writing, m.SelectedJob(), m.Notice)
	}
	// The next press is aimed at the newer version.
	if save, ok := savedJob(step(t, s, key("p")).Cmds); !ok || save.ExpectedEventUID != "EV2" {
		t.Fatalf("next pause sent %+v", save)
	}
}

func TestJobKeysAreOnTheFooterAndTheHelpSheet(t *testing.T) {
	s := jobsPane(t, 160, guarded("J1", "Weekly", "EV1"))
	wantText(t, "Jobs Page footer", footerText(t, s), "new job", "edit job", "pause or resume", "archive")
	help := wireTextOf(t, feed(t, s, key("?")).State)
	wantText(t, "help sheet", help, "new job (Job card)", "edit the selected job", "pause or resume the selected job", "archive the selected job, or restore it", "new native job on the selected Issue")
	card := step(t, s, key("n")).State
	wantText(t, "card footer", footerText(t, card), "save job", "cancel", "next field")
}
