package model

import (
	"slices"
	"strings"
	"time"
)

// CronOccurrence says which Cron job made an Issue and for which run: kata's `cron.occurrence` metadata, which only its daemon
// writes. ScheduledAt is zero when kata's value is not a time.
type CronOccurrence struct {
	JobUID, DefinitionEventUID string
	ScheduledAt                time.Time
}

// cronProgressPrefix starts the metadata key a native job keeps its delivery state under on its parent.
const cronProgressPrefix = "cron.progress."

// CronMade says a Cron job made the Issue.
func (i Issue) CronMade() bool { return i.CronOccurrence != nil }

// cronParent says the Issue carries a native job's delivery state, so some Cron job makes children of it.
func (i Issue) cronParent() bool {
	for key := range i.Metadata {
		if strings.HasPrefix(key, cronProgressPrefix) {
			return true
		}
	}
	return false
}

// wantsCronJobs says the Issue's detail names Cron jobs: it was made by one or is the parent of some.
func (i Issue) wantsCronJobs() bool { return i.CronMade() || i.cronParent() }

// CronRow is a Cron job as the Issue detail names it: Property says which detail property the row belongs to, "Cron job" on
// a cron-made Issue and "Cron jobs" on a parent. Name is the job's name, its uid when the Jobs Page's list does not hold it.
type CronRow struct {
	Property string
	Name     string
	// Scheduled is the run's scheduled time in the Pane calendar, of a Cron job row.
	Scheduled string
	// State and Schedule are a Cron jobs row's job state ("active", "paused: parent closed") and its schedule in words.
	State, Schedule string
}

const (
	cronJobProperty  = "Cron job"
	cronJobsProperty = "Cron jobs"
)

// cronItems are the detail's cursor rows for its Cron job properties: the run's job on a cron-made Issue, the bound jobs on a
// parent. They come first, ahead of the children.
func (m Model) cronItems() []DetailItem {
	if m.authority == nil || m.authority.Issue == nil {
		return nil
	}
	issue := m.authority.Issue
	var items []DetailItem
	jobs, known := m.cronJobsOf(issue)
	if occ := issue.CronOccurrence; occ != nil {
		row := CronRow{Property: cronJobProperty, Name: occ.JobUID}
		if known {
			if i := slices.IndexFunc(jobs, func(j CronJob) bool { return j.UID == occ.JobUID }); i >= 0 {
				row.Name = jobs[i].Name
			}
		}
		if !occ.ScheduledAt.IsZero() {
			row.Scheduled = m.DetailTime(occ.ScheduledAt)
		}
		items = append(items, DetailItem{Kind: RowCron, Key: "cron-job", JobUID: occ.JobUID, Cron: row})
	}
	if !known {
		return items
	}
	for _, job := range jobs {
		if job.ParentUID() != issue.UID || job.Archived() {
			continue
		}
		pause := m.parentPause(job, issue)
		items = append(items, DetailItem{Kind: RowCron, Key: "cron-jobs:" + job.UID, JobUID: job.UID,
			Cron: CronRow{Property: cronJobsProperty, Name: job.Name, State: pause.StateText(), Schedule: job.Definition.Trigger.Words()}})
	}
	return items
}

// parentPause is why the job bound to the shown parent is not running, with the parent's status taken from the Issue the
// detail shows, which is newer than what the Jobs Page's read of the job held.
func (m Model) parentPause(job CronJob, parent *Issue) PauseReason {
	if job.Parent != nil {
		held := *job.Parent
		held.Status = parent.Status
		job.Parent = &held
	}
	return job.Pause(m.Jobs.ProjectArchived)
}

// CronJobsClosed says the detail's Issue is a closed parent with Cron jobs listed, whose jobs are therefore paused.
func (m Model) CronJobsClosed() bool {
	if m.authority == nil || m.authority.Issue == nil || m.authority.Issue.Status != "closed" {
		return false
	}
	return slices.ContainsFunc(m.rows.items, func(r DetailItem) bool { return r.Kind == RowCron && r.Cron.Property == cronJobsProperty })
}

// cronJobsOf are the Cron jobs the Pane holds for the Issue's Project; known is false until they have been read.
func (m Model) cronJobsOf(issue *Issue) (jobs []CronJob, known bool) {
	if m.Jobs.ProjectID != issue.ProjectID || m.Jobs.request == 0 || (m.Jobs.Loading && m.Jobs.Jobs == nil) {
		return nil, false
	}
	return m.Jobs.Jobs, true
}

// cronRead is the Issue and revision the Jobs Page's job list was last read for from an Issue detail.
type cronRead struct {
	uid      string
	revision int64
}

// settleCronJobs reads the Project's Cron jobs when the detail of an Issue names them and the list the Pane holds was not read
// for this Issue as it stands: the detail's names and states come from it. The Jobs Page keeps its own reads.
func (m Model) settleCronJobs() (Model, []Cmd) {
	if m.OnJobs() || m.authority == nil || m.authority.Issue == nil || m.hidden {
		return m, nil
	}
	issue := m.authority.Issue
	if !issue.wantsCronJobs() || issue.ProjectID == 0 || issue.UID != m.SelectedUID {
		return m, nil
	}
	read := cronRead{issue.UID, issue.Revision}
	if m.Jobs.cronRead == read && m.Jobs.ProjectID == issue.ProjectID {
		return m, nil
	}
	m.Jobs.cronRead = read
	if m.Jobs.ProjectID != issue.ProjectID {
		m.Jobs.Jobs, m.Jobs.ProjectArchived = nil, false
	}
	m.Jobs.ProjectID, m.Jobs.Err = issue.ProjectID, nil
	m.Jobs.Loading = true
	m.Jobs.request++
	return m, []Cmd{LoadJobs{Workspace: m.Workspace, ProjectID: issue.ProjectID, Archived: true, Request: m.Jobs.request}}
}

// openJob shows the Jobs Page with the job selected, the Pane's Project rule applying as for any Page. A job the Pane holds
// archived is listed for it.
func openJob(m Model, uid string) (Model, []Cmd) {
	if i := slices.IndexFunc(m.Jobs.Jobs, func(j CronJob) bool { return j.UID == uid }); i >= 0 && m.Jobs.Jobs[i].Archived() {
		m.Jobs.ShowArchived = true
	}
	m, cmds := showPage(PageJobs)(m)
	if m.OnJobs() {
		m.Jobs.pick = uid
	}
	return m, cmds
}

// OpenJobMsg is a click on a Cron job named in the Issue detail.
type OpenJobMsg struct{ UID string }

func (OpenJobMsg) msg() {}
