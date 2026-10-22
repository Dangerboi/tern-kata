package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// A Backfill is what kata's daemon creates at once when a native job with catch-up "all" runs again after a pause: one child
// Issue for every occurrence it missed. Reopening the parent, resuming the job and restoring the job each end a pause, so each
// first asks, in the Backfill card, whether the children should be made. The count is worked out here from the trigger, its
// zone and the parent's progress; nothing is asked when none were missed or the job catches up with "latest" or "skip".

// ModeBackfill is the Backfill card's keymap mode.
const ModeBackfill Mode = "backfill"

const (
	// BackfillCreate lets the daemon create the missed children and goes on.
	BackfillCreate Action = "backfill-create"
	// BackfillLatest switches the jobs to catch-up "latest", guarded, and then goes on.
	BackfillLatest Action = "backfill-latest"
	// BackfillCancel changes nothing.
	BackfillCancel Action = "backfill-cancel"
)

// BackfillFor is what the Backfill card holds back until the user has answered.
type BackfillFor string

const (
	BackfillResume  BackfillFor = "resume"
	BackfillRestore BackfillFor = "restore"
	BackfillReopen  BackfillFor = "reopen"
	// BackfillSave is the Job card's save that turns a paused job on: the card's draft is what goes out once answered.
	BackfillSave BackfillFor = "save"
)

// BackfillJob is a job that owes a Backfill with the number of children it would create.
type BackfillJob struct {
	Job    CronJob
	Missed int
}

// BackfillState is the Backfill card. Issue is the closed parent a reopen is for. While request is not zero the Pane reads the
// parent's jobs to find out whether the reopen owes a Backfill, and the answer to that read decides.
type BackfillState struct {
	Open   bool
	For    BackfillFor
	Jobs   []BackfillJob
	Issue  Issue
	Saving bool
	Err    error
	// Save is the Job card's write a BackfillSave holds back.
	Save *SaveJobCmd
	// request is the number of the jobs read a reopen waits for.
	request int64
}

// Total is how many children the card's jobs would create in all.
func (b BackfillState) Total() int {
	total := 0
	for _, j := range b.Jobs {
		total += j.Missed
	}
	return total
}

// backfillOwed is how many children delivery creates at once when job runs again with its parent open: zero unless the job is
// native, catches up with every missed run and has progress to count from.
func (m Model) backfillOwed(job CronJob) int {
	if !job.Native() || job.Definition.Catchup != CatchupAll || job.Parent == nil || job.Parent.Deleted {
		return 0
	}
	return job.Missed(m.Now)
}

// parentOpen says the job's parent is there to deliver into.
func parentOpen(job CronJob) bool {
	return job.Parent != nil && !job.Parent.Deleted && job.Parent.Status == "open"
}

// askBackfill opens the card when ending a pause owes a Backfill; it reports false when nothing is owed.
func (m Model) askBackfill(kind BackfillFor, issue Issue, jobs ...CronJob) (Model, bool) {
	var owed []BackfillJob
	for _, job := range jobs {
		if n := m.backfillOwed(job); n > 0 {
			owed = append(owed, BackfillJob{Job: job, Missed: n})
		}
	}
	if len(owed) == 0 {
		return m, false
	}
	m.Backfill = BackfillState{Open: true, For: kind, Jobs: owed, Issue: issue}
	return m, true
}

// resumeJob writes the job enabled. Every resume of a job goes through here, so a native job that owes a Backfill asks first.
func resumeJob(m Model, job CronJob) (Model, []Cmd) {
	if parentOpen(job) {
		if asked, ok := m.askBackfill(BackfillResume, Issue{}, job); ok {
			return asked, nil
		}
	}
	return writeJobEnabled(m, job, true)
}

// restoreJob restores the archived job. A job that comes back enabled under an open parent owes a Backfill, so it asks first.
func restoreJob(m Model, job CronJob) (Model, []Cmd) {
	if job.Definition.Enabled && parentOpen(job) {
		if asked, ok := m.askBackfill(BackfillRestore, Issue{}, job); ok {
			return asked, nil
		}
	}
	return restoreJobNow(m, job)
}

func restoreJobNow(m Model, job CronJob) (Model, []Cmd) {
	m.Jobs.Writing = job.UID
	return m, []Cmd{RestoreJobCmd{Workspace: m.Workspace, ProjectID: m.Jobs.ProjectID, Actor: m.Actor, UID: job.UID, ExpectedEventUID: job.DefinitionEventUID}}
}

// reopenIssue is `r`: it reopens a closed Issue. A parent of native jobs first reads them afresh, because its progress stopped
// advancing when it closed, and asks if reopening owes a Backfill.
func reopenIssue(m Model) (Model, []Cmd) {
	found := closeIssue(m)
	if found == nil || m.Stale || m.CloseForm.Saving {
		return m, nil
	}
	issue := *found
	if !IsClosed(issue) || !issue.cronParent() || issue.ProjectID == 0 {
		return reopenNow(m, issue)
	}
	if m.Jobs.ProjectID != issue.ProjectID {
		m.Jobs.Jobs, m.Jobs.ProjectArchived = nil, false
	}
	m.Jobs.ProjectID, m.Jobs.Err, m.Jobs.Loading = issue.ProjectID, nil, true
	m.Jobs.request++
	m.Backfill = BackfillState{Issue: issue, request: m.Jobs.request}
	return m, []Cmd{LoadJobs{Workspace: m.Workspace, ProjectID: issue.ProjectID, Archived: true, Request: m.Jobs.request}}
}

func reopenNow(m Model, issue Issue) (Model, []Cmd) {
	m.CloseForm = CloseFormState{UID: issue.UID, Saving: true}
	return m, []Cmd{ReopenIssueCmd{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Actor: m.Actor}}
}

// backfillJobsRead goes on with a reopen once the read of the parent's jobs it waited for has answered: it opens the card when
// reopening would make the jobs catch up, else reopens.
func backfillJobsRead(m Model, msg JobsLoaded) (Model, []Cmd) {
	if m.Backfill.Open || m.Backfill.request == 0 || m.Backfill.request != msg.Request {
		return m, nil
	}
	issue := m.Backfill.Issue
	m.Backfill = BackfillState{}
	if msg.Err != nil {
		m.Notice = "Could not read the Issue's Cron jobs, so nothing was reopened: " + msg.Err.Error()
		return m, nil
	}
	var jobs []CronJob
	for _, job := range m.Jobs.Jobs {
		if !job.Archived() && job.Definition.Enabled && job.ParentUID() == issue.UID {
			jobs = append(jobs, job)
		}
	}
	if !msg.ProjectArchived {
		if asked, ok := m.askBackfill(BackfillReopen, issue, jobs...); ok {
			return asked, nil
		}
	}
	return reopenNow(m, issue)
}

// backfillKey is one key in the Backfill card.
func backfillKey(m Model, msg KeyMsg) (Model, []Cmd) {
	b := m.Backfill
	if b.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeBackfill)
	if !bound {
		return m, nil
	}
	if binding.Action == BackfillCancel {
		m.Backfill = BackfillState{}
		return m, nil
	}
	if m.Stale {
		m.Backfill.Err = errors.New("Daemon disconnected; reconnect before changing a job.")
		return m, nil
	}
	m.Backfill.Err = nil
	switch binding.Action {
	case BackfillCreate:
		m.Backfill = BackfillState{}
		switch b.For {
		case BackfillSave:
			return sendJobSave(m, *b.Save)
		case BackfillResume:
			return writeJobEnabled(m, b.Jobs[0].Job, true)
		case BackfillRestore:
			return restoreJobNow(m, b.Jobs[0].Job)
		}
		return reopenNow(m, b.Issue)
	case BackfillLatest:
		if b.For == BackfillSave {
			m.Backfill = BackfillState{}
			save := *b.Save
			save.Definition.Catchup = CatchupLatest
			return sendJobSave(m, save)
		}
		return writeNextLatest(m)
	}
	return m, nil
}

// sendJobSave sends the Job card's save, which the card waits on as it does any save.
func sendJobSave(m Model, save SaveJobCmd) (Model, []Cmd) {
	m.JobForm.Saving, m.JobForm.Err = true, nil
	return m, []Cmd{save}
}

// writeNextLatest replaces the catch-up of the first job still set to "all" with "latest", guarded on the version the Pane
// read. A resume and a restore ride on that same write, which is the one that ends the pause; a reopen waits for the last one.
func writeNextLatest(m Model) (Model, []Cmd) {
	i := slices.IndexFunc(m.Backfill.Jobs, func(j BackfillJob) bool { return j.Job.Definition.Catchup == CatchupAll })
	if i < 0 {
		return finishBackfill(m)
	}
	job := m.Backfill.Jobs[i].Job
	def := job.Definition
	def.Catchup = CatchupLatest
	if m.Backfill.For == BackfillResume {
		def.Enabled = true
	}
	m.Backfill.Saving = true
	m.Jobs.Writing = job.UID
	return m, []Cmd{SaveJobCmd{Workspace: m.Workspace, ProjectID: m.Jobs.ProjectID, Actor: m.Actor, UID: job.UID,
		ExpectedEventUID: job.DefinitionEventUID, Name: job.Name, Definition: def}}
}

// finishBackfill is what follows once every job catches up with "latest": a reopen, the only entry point that did not ride on
// the job's own write.
func finishBackfill(m Model) (Model, []Cmd) {
	b := m.Backfill
	m.Backfill = BackfillState{}
	if b.For == BackfillReopen {
		return reopenNow(m, b.Issue)
	}
	return m, nil
}

// backfillWritten applies the answer to a write the Backfill card is waiting on; it reports false for any other answer. A
// failure leaves the card open with the error, so the choice can be made again; a moved guard writes nothing, shows the newer
// job in the list and closes the card.
func backfillWritten(m Model, msg JobWritten) (Model, []Cmd, bool) {
	b := m.Backfill
	if !b.Open || !b.Saving || m.Jobs.Writing != msg.UID {
		return m, nil, false
	}
	m.Backfill.Saving = false
	switch {
	case msg.Err != nil:
		m.Jobs.Writing = ""
		m.Backfill.Err = msg.Err
		return m, nil, true
	case msg.Changed != nil:
		m.Backfill = BackfillState{}
		return jobWritten(m, msg), nil, true
	case msg.Job == nil:
		return m, nil, false
	}
	m.Jobs.Writing = ""
	m = m.putJob(msg.ProjectID, *msg.Job)
	for i := range m.Backfill.Jobs {
		if m.Backfill.Jobs[i].Job.UID == msg.UID {
			m.Backfill.Jobs[i].Job = *msg.Job
		}
	}
	if slices.ContainsFunc(m.Backfill.Jobs, func(j BackfillJob) bool { return j.Job.Definition.Catchup == CatchupAll }) {
		next, cmds := writeNextLatest(m)
		return next, cmds, true
	}
	kind := b.For
	m, cmds := finishBackfill(m)
	switch kind {
	case BackfillResume:
		m.Notice = fmt.Sprintf("Switched job %q to the latest missed run and resumed it.", msg.Job.Name)
	case BackfillRestore:
		m.Notice = fmt.Sprintf("Switched job %q to the latest missed run and restored it.", msg.Job.Name)
	}
	return m, cmds, true
}

// MissedWords reads a count of missed runs: "1 missed run", "3 missed runs"; a count at the counting limit reads "10000 or more".
func MissedWords(n int) string {
	switch {
	case n >= maxMissedCount:
		return strconv.Itoa(maxMissedCount) + " or more missed runs"
	case n == 1:
		return "1 missed run"
	}
	return strconv.Itoa(n) + " missed runs"
}
