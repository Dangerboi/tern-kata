package model

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
)

// The writes of a Cron job: the Job card's save, and the keys on the Jobs Page that pause, resume, archive and restore. Each is
// a Guarded write on the job's definition_event_uid, which every write to the job changes.

// JobOp is what a job write did, in kata's words.
type JobOp string

const (
	JobCreated  JobOp = "created"
	JobReplaced JobOp = "replaced"
	JobArchived JobOp = "archived"
	JobRestored JobOp = "restored"
)

// SaveJobCmd creates or replaces one Cron job. An empty ExpectedEventUID is a create at UID, which the card minted so a resend
// of a create that landed is no second job; otherwise it is a replace that writes nothing unless the job still has that
// definition_event_uid.
type SaveJobCmd struct {
	Workspace string
	ProjectID int64
	Actor     string
	UID       string
	// ExpectedEventUID is the version of the job the write replaces.
	ExpectedEventUID string
	Name             string
	Definition       CronDefinition
	// ParentRef names the Issue a new native job hangs from, by short id or uid; the Daemon gateway resolves it to the full uid
	// that Definition.Issue.ParentUID takes. Empty on a replace, which keeps the parent.
	ParentRef string
}

func (SaveJobCmd) cmd() {}

// ArchiveJobCmd archives the job at UID, guarded on ExpectedEventUID.
type ArchiveJobCmd struct {
	Workspace        string
	ProjectID        int64
	Actor            string
	UID              string
	ExpectedEventUID string
}

func (ArchiveJobCmd) cmd() {}

// RestoreJobCmd restores the archived job at UID, guarded on ExpectedEventUID.
type RestoreJobCmd struct {
	Workspace        string
	ProjectID        int64
	Actor            string
	UID              string
	ExpectedEventUID string
}

func (RestoreJobCmd) cmd() {}

// JobWritten answers a job write. Job is the job as it stands after the write. Changed is the current job when the guard had
// moved: nothing was written and Err is nil. Op is what the write did.
type JobWritten struct {
	ProjectID int64
	UID       string
	Op        JobOp
	Job       *CronJob
	Changed   *CronJob
	Err       error
}

func (JobWritten) msg() {}

// jobUID is a new job's uid, minted when its card opens so every send of the card's create is the same create. It is a ULID in
// kata's strict reading: 26 characters of Crockford base32 whose first stays within 7, here from a hash of the run's nonce and
// a counter.
func (m Model) jobUID() (Model, string) {
	m.keys++
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-job-%d", m.nonce, m.keys)))
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var out [26]byte
	out[0] = alphabet[sum[0]>>5]
	// The other 25 characters take the remaining 125 of the 128 bits, 5 at a time.
	for i := 1; i < 26; i++ {
		at := 3 + 5*(i-1)
		window := uint16(sum[at/8]) << 8
		if at/8+1 < len(sum[:16]) {
			window |= uint16(sum[at/8+1])
		}
		out[i] = alphabet[(window>>(11-at%8))&0x1f]
	}
	return m, string(out[:])
}

// writableJob is the selected job when a key may write to it: the Jobs Page shows it, the daemon is connected, no other write
// of the Jobs Page is in flight, and the Project is not archived. Otherwise it says why in the Notice, or says nothing when
// there is nothing to say.
func writableJob(m Model) (Model, CronJob, bool) {
	job := m.SelectedJob()
	switch {
	case !m.OnJobs() || job == nil:
	case m.Stale:
		m.Notice = "Daemon disconnected; reconnect before changing a job."
	case m.Jobs.Writing != "":
	case m.Jobs.ProjectArchived:
		m.Notice = "This Project is archived; its jobs can't be changed."
	default:
		return m, *job, true
	}
	return m, CronJob{}, false
}

// refuse is a job key that has nothing to do; the Notice says why.
func refuse(m Model, why string) (Model, []Cmd) {
	m.Notice = why
	return m, nil
}

// toggleJob is `p`: pause a job that runs, resume one that does not.
func toggleJob(m Model) (Model, []Cmd) {
	m, job, ok := writableJob(m)
	if !ok {
		return m, nil
	}
	switch {
	case job.Archived():
		return refuse(m, "The job is archived; restore it first.")
	case job.Definition.Enabled:
		return pauseJob(m, job)
	}
	return resumeJob(m, job)
}

// pauseJob writes the job disabled.
func pauseJob(m Model, job CronJob) (Model, []Cmd) { return writeJobEnabled(m, job, false) }

// resumeJob writes the job enabled. Every resume of a job goes through here, so what must happen before one (a native job's
// backfill, which delivery owes for the time it was off) wraps this function and calls it once the user has agreed.
func resumeJob(m Model, job CronJob) (Model, []Cmd) { return writeJobEnabled(m, job, true) }

func writeJobEnabled(m Model, job CronJob, enabled bool) (Model, []Cmd) {
	def := job.Definition
	def.Enabled = enabled
	m.Jobs.Writing = job.UID
	return m, []Cmd{SaveJobCmd{Workspace: m.Workspace, ProjectID: m.Jobs.ProjectID, Actor: m.Actor, UID: job.UID,
		ExpectedEventUID: job.DefinitionEventUID, Name: job.Name, Definition: def}}
}

// archiveJob is `x` on the Jobs Page.
func archiveJob(m Model) (Model, []Cmd) {
	m, job, ok := writableJob(m)
	if !ok {
		return m, nil
	}
	if job.Archived() {
		return refuse(m, "The job is already archived; R restores it.")
	}
	m.Jobs.Writing = job.UID
	return m, []Cmd{ArchiveJobCmd{Workspace: m.Workspace, ProjectID: m.Jobs.ProjectID, Actor: m.Actor, UID: job.UID, ExpectedEventUID: job.DefinitionEventUID}}
}

// restoreJob is `r` on the Jobs Page, on an archived job, which the list shows only when asked.
func restoreJob(m Model) (Model, []Cmd) {
	m, job, ok := writableJob(m)
	if !ok {
		return m, nil
	}
	if !job.Archived() {
		return refuse(m, "The job is not archived.")
	}
	m.Jobs.Writing = job.UID
	return m, []Cmd{RestoreJobCmd{Workspace: m.Workspace, ProjectID: m.Jobs.ProjectID, Actor: m.Actor, UID: job.UID, ExpectedEventUID: job.DefinitionEventUID}}
}

// putJob puts job into the Jobs Page's list in kata's order, name then uid, in place of the one it replaces.
func (m Model) putJob(projectID int64, job CronJob) Model {
	if projectID != m.Jobs.ProjectID {
		return m
	}
	jobs := slices.DeleteFunc(slices.Clone(m.Jobs.Jobs), func(j CronJob) bool { return j.UID == job.UID })
	jobs = append(jobs, job)
	slices.SortFunc(jobs, func(a, b CronJob) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.UID, b.UID)
	})
	m.Jobs.Jobs = jobs
	return m.settleJobSelection()
}

// jobWritten applies the answer to a job write: to the Job card when it is the write the card is waiting on, else to the
// Jobs Page's list and its Notice.
func jobWritten(m Model, msg JobWritten) Model {
	fromCard := m.JobForm.Open && m.JobForm.Saving && m.JobForm.UID == msg.UID
	if fromCard {
		m.JobForm.Saving = false
	} else if m.Jobs.Writing == msg.UID {
		m.Jobs.Writing = ""
	}
	switch {
	case msg.Err != nil && fromCard:
		m.JobForm.Err = msg.Err
	case msg.Err != nil:
		m.Notice = msg.Err.Error()
	case msg.Changed != nil:
		m = m.putJob(msg.ProjectID, *msg.Changed)
		if fromCard {
			newer := *msg.Changed
			m.JobForm.Conflict = &newer
			return m
		}
		m.Notice = fmt.Sprintf("Job %q changed meanwhile; nothing was written. The list shows it now: %s.", msg.Changed.Name, jobState(m, *msg.Changed))
	case msg.Job != nil:
		m = m.putJob(msg.ProjectID, *msg.Job)
		if fromCard {
			m.JobForm.Open = false
		}
		if msg.ProjectID == m.Jobs.ProjectID && (msg.Op == JobCreated) && m.OnJobs() {
			m.Jobs.Selected = msg.Job.UID
			m = m.settleJobSelection()
		}
		m.Notice = jobWrittenNotice(m, msg, fromCard)
	}
	return m
}

// jobState is the state of a job as the list words it.
func jobState(m Model, job CronJob) string {
	return job.Pause(m.Jobs.ProjectArchived).StateText()
}

// jobWrittenNotice words what a landed write did.
func jobWrittenNotice(m Model, msg JobWritten, fromCard bool) string {
	job := *msg.Job
	switch {
	case msg.Op == JobCreated:
		return fmt.Sprintf("Created job %q.", job.Name)
	case msg.Op == JobArchived:
		return fmt.Sprintf("Archived job %q.", job.Name)
	case msg.Op == JobRestored:
		return fmt.Sprintf("Restored job %q.", job.Name)
	case fromCard:
		return fmt.Sprintf("Saved job %q: %s.", job.Name, jobState(m, job))
	case !job.Definition.Enabled:
		return fmt.Sprintf("Paused job %q.", job.Name)
	}
	return fmt.Sprintf("Resumed job %q: %s.", job.Name, jobState(m, job))
}
