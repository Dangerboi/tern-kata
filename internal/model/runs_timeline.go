package model

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Runs Page's since-you-last-looked mode lists the Runs that ended since the user last looked on this machine, newest first,
// above a "Now" line, then the Project's next runs. The mark of the last look is seen.toml (see data.seenFile); the mode is
// remembered in the same table.

// foldOver is how many new results the mode lists one by one; more than this many fold into one row per job.
const foldOver = 8

type LoadSeen struct{ ProjectUID string }

func (LoadSeen) cmd() {}

// SeenLoaded answers LoadSeen. A zero Seen is a Project never marked seen on this machine.
type SeenLoaded struct {
	ProjectUID string
	Seen       time.Time
	Timeline   bool
	Err        error
}

func (SeenLoaded) msg() {}

// SaveSeen writes the Project's table: Seen when SetSeen, Timeline when SetTimeline.
type SaveSeen struct {
	ProjectUID  string
	SetSeen     bool
	Seen        time.Time
	SetTimeline bool
	Timeline    bool
}

func (SaveSeen) cmd() {}

type SeenSaved struct {
	ProjectUID string
	Err        error
}

func (SeenSaved) msg() {}

// Ended is when the Run stopped: its end, else when kata last updated it. A Run still running has none. A held Run counts as
// ended, as it is no longer running and the user wants to see it.
func (r Run) Ended() (time.Time, bool) {
	if r.Status == cronjob.RunRunning {
		return time.Time{}, false
	}
	if r.EndedAt != nil {
		return *r.EndedAt, true
	}
	return r.UpdatedAt, true
}

// endedSince is the Runs that ended after seen, newest ended first. A zero seen (never looked) lists every ended Run. Runs still
// running are never listed.
func endedSince(runs []Run, seen time.Time) []Run {
	var out []Run
	for _, run := range runs {
		if at, ok := run.Ended(); ok && at.After(seen) {
			out = append(out, run)
		}
	}
	slices.SortStableFunc(out, func(a, b Run) int {
		x, _ := a.Ended()
		y, _ := b.Ended()
		return cmp.Compare(y.UnixNano(), x.UnixNano())
	})
	return out
}

func newestEnd(runs []Run, floor time.Time) time.Time {
	for _, run := range runs {
		if at, ok := run.Ended(); ok && at.After(floor) {
			floor = at
		}
	}
	return floor
}

type RunGroup struct{ Runs []Run }

func (g RunGroup) Folded() bool { return len(g.Runs) > 1 }

func (g RunGroup) Latest() Run { return g.Runs[0] }

// Counts words the folded Runs by status, "3 succeeded · 1 cancelled".
func (g RunGroup) Counts() string {
	var parts []string
	for _, status := range []cronjob.RunStatus{cronjob.RunSucceeded, cronjob.RunFailed, cronjob.RunCancelled, cronjob.RunUnknown} {
		n := 0
		for _, run := range g.Runs {
			if run.Status == status {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+string(status))
		}
	}
	return strings.Join(parts, " · ")
}

// foldKey is what a Run folds with: its job, else its workflow. A Run of neither never folds.
func foldKey(r Run) string {
	switch {
	case r.JobUID != "":
		return "job:" + r.JobUID
	case r.WorkflowUID != "":
		return "workflow:" + r.WorkflowUID
	}
	return ""
}

// foldRuns groups ended Runs, newest first. With at most foldOver Runs nothing folds. With more, the Runs of
// one job fold into one summary row, placed where the job's newest folded Run is; a failed Run is never folded and stays listed
// by itself, and so does a job's single Run that has none to fold with.
func foldRuns(runs []Run) []RunGroup {
	groups := make([]RunGroup, 0, len(runs))
	if len(runs) <= foldOver {
		for _, run := range runs {
			groups = append(groups, RunGroup{Runs: []Run{run}})
		}
		return groups
	}
	at := map[string]int{}
	for _, run := range runs {
		key := foldKey(run)
		if run.Status == cronjob.RunFailed || key == "" {
			groups = append(groups, RunGroup{Runs: []Run{run}})
			continue
		}
		if i, ok := at[key]; ok {
			groups[i].Runs = append(groups[i].Runs, run)
			continue
		}
		at[key] = len(groups)
		groups = append(groups, RunGroup{Runs: []Run{run}})
	}
	return groups
}

// TimelineEntry is a row above the Now line: Row for one Run, Group when it is a summary.
type TimelineEntry struct {
	Row   RunRow
	Group RunGroup
}

// UpcomingRun is a row below the Now line. Next is empty for an adapter job, which only the worker starts; at is when a native
// job fires next, what the rows are sorted on.
type UpcomingRun struct {
	Job  CronJob
	Next string
	at   time.Time
}

type Timeline struct {
	Rows     []TimelineEntry
	RunCount int
	Seen     time.Time
	Upcoming []UpcomingRun
}

func (m Model) timelineRuns() []Run {
	runs := m.Runs.Runs
	if m.searchingPage(PageRuns) {
		runs = nil
		for _, row := range m.RunRows(m.Now) {
			runs = append(runs, row.Run)
		}
	}
	return endedSince(runs, m.Runs.Seen)
}

func (m Model) timelineSelectable() []Run {
	var out []Run
	for _, group := range foldRuns(m.timelineRuns()) {
		if !group.Folded() {
			out = append(out, group.Latest())
		}
	}
	return out
}

// TimelineLoading says what the mode lists is not all read yet: the Runs, seen.toml or the Project's jobs.
func (m Model) TimelineLoading() (runs, upcoming bool) {
	return m.Runs.Loading || !m.Runs.seenRead, m.Jobs.Loading
}

// Timeline is the mode's rows. now is when the next runs are counted from.
func (m Model) Timeline(now time.Time) Timeline {
	runs := m.timelineRuns()
	t := Timeline{RunCount: len(runs), Seen: m.Runs.Seen}
	rows := map[string]RunRow{}
	for _, row := range m.allRunRows(now) {
		rows[row.Run.UID] = row
	}
	for _, group := range foldRuns(runs) {
		entry := TimelineEntry{Group: group}
		if !group.Folded() {
			entry.Row = rows[group.Latest().UID]
		}
		t.Rows = append(t.Rows, entry)
	}
	t.Upcoming = m.upcoming(now)
	return t
}

// upcoming are the Project's next runs: native jobs by their next time, then adapter jobs with none, as only the worker knows when
// it starts them. A job that is paused, archived or manual is left out, and so is a native job with no next time to work out.
func (m Model) upcoming(now time.Time) []UpcomingRun {
	var native, adapter []UpcomingRun
	for _, job := range m.Jobs.Jobs {
		if job.Archived() || m.JobPause(job) != NotPaused || job.Definition.Trigger.Kind == cronjob.TriggerManual {
			continue
		}
		if m.Runs.JobUID != "" && job.UID != m.Runs.JobUID {
			continue
		}
		if m.searchingPage(PageRuns) && !m.matchesSearch(job.Name) {
			continue
		}
		if !job.Native() {
			adapter = append(adapter, UpcomingRun{Job: job})
			continue
		}
		if next, ok := job.NextRun(now, m.Jobs.ProjectArchived); ok {
			native = append(native, UpcomingRun{Job: job, Next: m.Calendar.Read(next, now), at: next})
		}
	}
	return append(sortedByNext(native), adapter...)
}

func sortedByNext(rows []UpcomingRun) []UpcomingRun {
	slices.SortStableFunc(rows, func(a, b UpcomingRun) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return cmp.Compare(a.Job.Name, b.Job.Name)
	})
	return rows
}

// toggleRunsTimeline is n: the Runs Page switches between its list of every run and the since-you-last-looked mode, and the
// choice is saved.
func toggleRunsTimeline(m Model) (Model, []Cmd) {
	m.Runs.Timeline, m.Runs.timelineSet = !m.Runs.Timeline, true
	m.Runs.jobsStale = m.Runs.Timeline
	m = m.settleRunSelection()
	return m, []Cmd{SaveSeen{ProjectUID: m.Runs.ProjectUID, SetTimeline: true, Timeline: m.Runs.Timeline}}
}

// canMarkRunsSeen says s is on offer: the mode is shown and seen.toml was read, without an error.
func canMarkRunsSeen(m Model) bool {
	return m.Runs.Timeline && m.Runs.seenRead && m.Runs.SeenErr == nil
}

// markRunsSeen is s: every Run loaded that has ended is marked seen, so the mode lists nothing until more ends. The mark is
// per Project, so it is refused while a job filter or the Page search hides Runs: the mark would also cover a Run the user was
// never shown. The mark is the
// newest end among them, not the clock, so a Run that ended before the key but is not loaded yet, or whose executor's clock is
// behind, still shows as new. With no ended Run loaded the mark stays.
func markRunsSeen(m Model) (Model, []Cmd) {
	if m.Runs.JobUID != "" || m.searchingPage(PageRuns) {
		m.Notice = "Clear the job filter and the search before marking seen: it would also cover runs not shown."
		return m, nil
	}
	seen := newestEnd(m.Runs.Runs, m.Runs.Seen)
	if seen.Equal(m.Runs.Seen) {
		return m, nil
	}
	m.Runs.Seen = seen
	m = m.settleRunSelection()
	return m, []Cmd{SaveSeen{ProjectUID: m.Runs.ProjectUID, SetSeen: true, Seen: seen}}
}

// seenLoaded applies the answer to LoadSeen for the Project the Page holds. A file that cannot be read leaves the mode off and
// nothing seen, and its error is shown.
func seenLoaded(m Model, msg SeenLoaded) Model {
	if msg.ProjectUID != m.Runs.ProjectUID {
		return m
	}
	m.Runs.seenRead, m.Runs.SeenErr = true, msg.Err
	if msg.Err != nil {
		return m
	}
	m.Runs.Seen = msg.Seen
	if m.Runs.Timeline != msg.Timeline && !m.Runs.timelineSet {
		m.Runs.Timeline, m.Runs.jobsStale = msg.Timeline, msg.Timeline
	}
	return m.settleRunSelection()
}

func seenSaved(m Model, msg SeenSaved) Model {
	if msg.Err != nil {
		m.Notice = "Not saved to seen.toml: " + msg.Err.Error()
	}
	return m
}
