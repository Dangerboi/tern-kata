package model

import (
	"fmt"
	"slices"
	"strconv"
)

// A Backfill is what kata's daemon creates at once when a native job with catch-up "all" runs again after a pause: one child
// Issue for every occurrence it missed. Reopening the parent, resuming the job and restoring the job each end a pause, so each
// first asks, in the Backfill card, whether the children should be made. The count is worked out here from the trigger, its
// zone and the parent's progress; nothing is asked when none were missed or the job catches up with "latest" or "skip".

const ModeBackfill Mode = "backfill"

const (
	// BackfillCreate lets the daemon create the missed children and goes on.
	BackfillCreate Action = "backfill-create"
	// BackfillLatest switches the jobs to catch-up "latest", guarded, and then goes on.
	BackfillLatest Action = "backfill-latest"
	BackfillCancel Action = "backfill-cancel"
)

type BackfillFor string

const (
	BackfillResume  BackfillFor = "resume"
	BackfillRestore BackfillFor = "restore"
	BackfillReopen  BackfillFor = "reopen"
	// BackfillSave is the Job card's save that turns a paused job on: the card's draft is what goes out once answered.
	BackfillSave BackfillFor = "save"
)

type BackfillJob struct {
	Job    CronJob
	Missed int
}

// BackfillState is the Backfill card. Issue is the closed parent a reopen is for. While reading is set the Pane reads the
// parent's jobs to find out whether the reopen owes a Backfill, and the answer to that read decides.
type BackfillState struct {
	Open   bool
	For    BackfillFor
	Jobs   []BackfillJob
	Issue  Issue
	Saving bool
	Err    error
	Save   *SaveJobCmd
	// reading says a reopen waits for a read of the jobs, which request numbers.
	reading bool
	request int64
}

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

func parentOpen(job CronJob) bool {
	return job.Parent != nil && !job.Parent.Deleted && job.Parent.Status == "open"
}

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

// Every resume of a job goes through here, so a native job that owes a Backfill asks first.
func resumeJob(m Model, job CronJob) (Model, []Cmd) {
	if parentOpen(job) {
		if asked, ok := m.askBackfill(BackfillResume, Issue{}, job); ok {
			return asked, nil
		}
	}
	return writeJobEnabled(m, job, true)
}

// A job that comes back enabled under an open parent owes a Backfill, so it asks first.
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

// A parent of native jobs first reads them afresh, because its progress stopped advancing when it closed, and asks if reopening owes a Backfill.
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
	request := m.Jobs.reads.Ask()
	m.Backfill = BackfillState{Issue: issue, reading: true, request: request}
	return m, []Cmd{LoadJobs{Workspace: m.Workspace, ProjectID: issue.ProjectID, Archived: true, Request: request}}
}

func reopenNow(m Model, issue Issue) (Model, []Cmd) {
	m.CloseForm = CloseFormState{UID: issue.UID, Saving: true}
	return m, []Cmd{ReopenIssueCmd{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Actor: m.Actor}}
}

func backfillJobsRead(m Model, msg JobsLoaded) (Model, []Cmd) {
	if m.Backfill.Open || !m.Backfill.reading || m.Backfill.request != msg.Request {
		return m, nil
	}
	issue := m.Backfill.Issue
	m.Backfill = BackfillState{}
	if msg.Err != nil {
		m = m.fail("Cannot read the Issue's jobs, so nothing was reopened: " + Reason(msg.Err) + ".")
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
		m.Backfill.Err = cardFailure("change", "the job", ErrDisconnected)
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
		m.Backfill.Err = cardFailure("change", "the job", msg.Err)
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
		m = m.say(fmt.Sprintf("Resumed %q with only its latest missed run.", msg.Job.Name))
	case BackfillRestore:
		m = m.say(fmt.Sprintf("Restored %q with only its latest missed run.", msg.Job.Name))
	}
	return m, cmds, true
}

func MissedWords(n int) string {
	return "missed " + RunWords(n)
}

// RunWords is a number of runs in words; at the cap the real count is unknown, so it reads "or more".
func RunWords(n int) string {
	if n >= maxMissedCount {
		return strconv.Itoa(maxMissedCount) + " or more runs"
	}
	return Count(n, "run", "runs")
}

// Verb is what the card's answer goes on to do, as the card's pending line and its sentence say it.
func (b BackfillState) Verb() string {
	switch b.For {
	case BackfillRestore:
		return "Restoring"
	case BackfillSave:
		return "Saving"
	case BackfillReopen:
		return "Reopening"
	}
	return "Resuming"
}

// Head is the card's question: how many child Issues answering "create" makes at once.
func (b BackfillState) Head() string {
	n := b.Total()
	if n >= maxMissedCount {
		return "Create " + strconv.Itoa(maxMissedCount) + " or more missed Issues?"
	}
	return "Create " + Count(n, "missed Issue", "missed Issues") + "?"
}

// BackfillSentence is the one sentence the card warns with. A single job is named in it; several jobs, which only a reopen
// has, are named in the rows under it.
func (m Model) BackfillSentence() string {
	b := m.Backfill
	verb := b.Verb()
	if b.For == BackfillReopen {
		ref := m.RefName(b.Issue.ProjectName, b.Issue.ShortID)
		if b.Total() == 1 {
			return verb + " " + ref + " creates the Issue its job missed."
		}
		return verb + " " + ref + " creates the Issues its jobs missed, all of them or only the latest of each."
	}
	if len(b.Jobs) == 1 {
		job := b.Jobs[0]
		if job.Missed == 1 {
			return fmt.Sprintf("%s missed 1 run. %s creates it now.", job.Job.Name, verb)
		}
		return fmt.Sprintf("%s missed %s. %s creates all of them now, or only the latest.", job.Job.Name, RunWords(job.Missed), verb)
	}
	return verb + " creates the Issues its jobs missed, all of them or only the latest of each."
}
