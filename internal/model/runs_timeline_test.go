package model_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

var timelineEpoch = time.Date(2026, 11, 7, 6, 0, 0, 0, time.UTC)

// endedRun is a Run of job that ended minute minutes after timelineEpoch.
func endedRun(uid, job string, status cronjob.RunStatus, minute int) model.Run {
	end := timelineEpoch.Add(time.Duration(minute) * time.Minute)
	return model.Run{Run: cronjob.Run{UID: uid, JobUID: job, Status: status, EndedAt: &end, UpdatedAt: end}}
}

// timelineOf is the since-you-last-looked rows of a Page holding runs, last marked seen at seen.
func timelineOf(runs []model.Run, seen time.Time) model.Timeline {
	return model.Model{Runs: model.RunsState{Runs: runs, Seen: seen}}.Timeline(timelineEpoch)
}

func runUIDs(groups []model.RunGroup) [][]string {
	var out [][]string
	for _, g := range groups {
		var uids []string
		for _, r := range g.Runs {
			uids = append(uids, r.UID)
		}
		out = append(out, uids)
	}
	return out
}

func TestFoldRuns(t *testing.T) {
	many := func(n int, job string, status cronjob.RunStatus) []model.Run {
		var runs []model.Run
		for i := range n {
			runs = append(runs, endedRun(fmt.Sprintf("%s%d", job, i), job, status, 100-i))
		}
		return runs
	}
	for name, tc := range map[string]struct {
		runs []model.Run
		want [][]string
	}{
		"eight are not folded": {runs: many(8, "a", cronjob.RunSucceeded), want: [][]string{{"a0"}, {"a1"}, {"a2"}, {"a3"}, {"a4"}, {"a5"}, {"a6"}, {"a7"}}},
		"nine fold per job": {
			runs: append(many(5, "a", cronjob.RunSucceeded), many(4, "b", cronjob.RunSucceeded)...),
			want: [][]string{{"a0", "a1", "a2", "a3", "a4"}, {"b0", "b1", "b2", "b3"}},
		},
		"failures stay listed one by one": {
			runs: []model.Run{
				endedRun("s1", "a", cronjob.RunSucceeded, 90), endedRun("f1", "a", cronjob.RunFailed, 80), endedRun("s2", "a", cronjob.RunSucceeded, 70),
				endedRun("f2", "a", cronjob.RunFailed, 60), endedRun("s3", "a", cronjob.RunSucceeded, 50), endedRun("s4", "a", cronjob.RunCancelled, 40),
				endedRun("s5", "a", cronjob.RunSucceeded, 30), endedRun("s6", "a", cronjob.RunSucceeded, 20), endedRun("s7", "a", cronjob.RunSucceeded, 10),
			},
			want: [][]string{{"s1", "s2", "s3", "s4", "s5", "s6", "s7"}, {"f1"}, {"f2"}},
		},
		"a job's lone run is not a summary": {
			runs: append(many(8, "a", cronjob.RunSucceeded), endedRun("b0", "b", cronjob.RunSucceeded, 1)),
			want: [][]string{{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"}, {"b0"}},
		},
		"a run of no job or workflow never folds": {
			runs: append(many(8, "a", cronjob.RunSucceeded), endedRun("x", "", cronjob.RunSucceeded, 2), endedRun("y", "", cronjob.RunSucceeded, 1)),
			want: [][]string{{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"}, {"x"}, {"y"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var groups []model.RunGroup
			for _, entry := range timelineOf(tc.runs, time.Time{}).Rows {
				groups = append(groups, entry.Group)
			}
			got := runUIDs(groups)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("groups %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFoldedRunsCountByStatus(t *testing.T) {
	g := model.RunGroup{Runs: []model.Run{
		endedRun("1", "a", cronjob.RunSucceeded, 3), endedRun("2", "a", cronjob.RunCancelled, 2), endedRun("3", "a", cronjob.RunSucceeded, 1),
	}}
	if got, want := g.Counts(), "2 succeeded · 1 cancelled"; got != want {
		t.Errorf("counts %q, want %q", got, want)
	}
}

func TestTimelineListsRunsEndedSince(t *testing.T) {
	seen := timelineEpoch.Add(30 * time.Minute)
	running := model.Run{Run: cronjob.Run{UID: "run", JobUID: "a", Status: cronjob.RunRunning, UpdatedAt: timelineEpoch.Add(90 * time.Minute)}}
	noEnd := model.Run{Run: cronjob.Run{UID: "noend", Status: cronjob.RunUnknown, UpdatedAt: timelineEpoch.Add(50 * time.Minute)}}
	runs := []model.Run{
		running,
		endedRun("old", "a", cronjob.RunSucceeded, 10),
		endedRun("at", "a", cronjob.RunSucceeded, 30),
		endedRun("early", "a", cronjob.RunFailed, 40),
		noEnd,
		endedRun("late", "a", cronjob.RunSucceeded, 60),
	}
	for name, tc := range map[string]struct {
		seen time.Time
		want []string
	}{
		"after the last look, newest ended first": {seen: seen, want: []string{"late", "noend", "early"}},
		"never seen lists every ended run":        {want: []string{"late", "noend", "early", "at", "old"}},
	} {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, entry := range timelineOf(runs, tc.seen).Rows {
				got = append(got, entry.Group.Latest().UID)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("ended %v, want %v", got, tc.want)
			}
		})
	}
}

// runsPageInTimeline is a Pane bound to Project "p" on the Runs Page in the since-you-last-looked mode, with seen.toml read
// (empty, or failing with seenErr) and runs loaded.
func runsPageInTimeline(t *testing.T, seenErr error, runs ...model.Run) model.Model {
	t.Helper()
	m := model.WithSnapshotRead(model.Model{
		Bound: true, Project: model.Project{UID: "p", ID: 7, Name: "Work"}, Collection: model.ProjectCollection("p", "Work"),
	})
	m, _ = model.Update(m, model.KeyMsg{Chord: "3"})
	m, _ = model.Update(m, model.SeenLoaded{ProjectUID: m.Runs.ProjectUID, Timeline: true, Err: seenErr})
	m.Runs.Timeline = true
	m.Runs.Runs = runs
	return m
}

func TestSMarksTheNewestEndAmongTheLoadedRunsAsSeen(t *testing.T) {
	m := runsPageInTimeline(t, nil, endedRun("a", "j", cronjob.RunSucceeded, 10), endedRun("b", "j", cronjob.RunFailed, 40), endedRun("c", "j", cronjob.RunSucceeded, 20))
	// The clock is far ahead of every run; the mark must still be the newest end, so a run that ends a minute later is new.
	m.Now = timelineEpoch.Add(24 * time.Hour)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "s"})
	want := timelineEpoch.Add(40 * time.Minute)
	if !m.Runs.Seen.Equal(want) {
		t.Fatalf("seen %v, want %v", m.Runs.Seen, want)
	}
	if len(cmds) != 1 || cmds[0] != (model.SaveSeen{ProjectUID: m.Runs.ProjectUID, SetSeen: true, Seen: want}) {
		t.Errorf("commands %#v", cmds)
	}
	if got := timelineOf(m.Runs.Runs, m.Runs.Seen); got.RunCount != 0 {
		t.Errorf("%d runs still new", got.RunCount)
	}
}

func TestSNeverMovesTheMarkBackAndKeepsItWithNoEndedRun(t *testing.T) {
	running := model.Run{Run: cronjob.Run{UID: "r", Status: cronjob.RunRunning, UpdatedAt: timelineEpoch}}
	m := runsPageInTimeline(t, nil, running, endedRun("old", "j", cronjob.RunSucceeded, 10))
	m.Runs.Seen = timelineEpoch.Add(30 * time.Minute)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "s"})
	if !m.Runs.Seen.Equal(timelineEpoch.Add(30*time.Minute)) || savesSeen(cmds) {
		t.Errorf("seen %v, commands %#v", m.Runs.Seen, cmds)
	}
}

func savesSeen(cmds []model.Cmd) bool {
	return slices.ContainsFunc(cmds, func(c model.Cmd) bool { _, ok := c.(model.SaveSeen); return ok })
}

func TestSIsNotOfferedWithoutTheModeOrWithAnUnreadableSeenFile(t *testing.T) {
	run := endedRun("a", "j", cronjob.RunSucceeded, 10)
	unreadable := runsPageInTimeline(t, errors.New("seen.toml: bad"), run)
	if m, cmds := model.Update(unreadable, model.KeyMsg{Chord: "s"}); savesSeen(cmds) || !m.Runs.Seen.IsZero() {
		t.Errorf("an unreadable seen.toml still marked seen: %v, %#v", m.Runs.Seen, cmds)
	}
	off := runsPageInTimeline(t, nil, run)
	off.Runs.Timeline = false
	if m, cmds := model.Update(off, model.KeyMsg{Chord: "s"}); savesSeen(cmds) || !m.Runs.Seen.IsZero() {
		t.Errorf("s marked seen outside the mode: %v, %#v", m.Runs.Seen, cmds)
	}
}

func TestEnteringTheModeForAnotherProjectDropsTheJobsOfTheFirst(t *testing.T) {
	native := cronjob.Job{UID: "jb", Name: "from B", Definition: cronjob.Definition{Enabled: true, Trigger: cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "UTC"}}}
	m := model.WithSnapshotRead(model.Model{
		Bound: true, Project: model.Project{UID: "p", ID: 7, Name: "Work"}, Collection: model.ProjectCollection("p", "Work"),
	})
	m, _ = model.Update(m, model.KeyMsg{Chord: "3"})
	m, _ = model.Update(m, model.SeenLoaded{ProjectUID: m.Runs.ProjectUID, Timeline: true})
	m.Jobs = model.JobsState{ProjectID: 8, Jobs: []model.CronJob{{Job: native, Parent: &model.CronJobParent{}}}}
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+r"})
	asked := false
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadJobs); ok && load.ProjectID == 7 {
			asked = true
		}
	}
	if !asked {
		t.Fatalf("the Project's jobs were not asked for: %#v", cmds)
	}
	if got := m.Timeline(timelineEpoch).Upcoming; len(got) != 0 {
		t.Errorf("another Project's jobs listed before the answer: %v", got)
	}
	if _, loadingNext := m.TimelineLoading(); !loadingNext {
		t.Error("the next runs are not marked as loading")
	}
}

func TestSRefusesWhileAJobFilterHidesRuns(t *testing.T) {
	m := runsPageInTimeline(t, nil, endedRun("a", "j1", cronjob.RunSucceeded, 10))
	m.Runs.JobUID = "j1"
	m, cmds := model.Update(m, model.KeyMsg{Chord: "s"})
	if !m.Runs.Seen.IsZero() || len(cmds) != 0 || m.Notice == "" {
		t.Fatalf("seen %v, commands %#v, notice %q", m.Runs.Seen, cmds, m.Notice)
	}
}

func TestSRefusesWhileTheSearchHidesRuns(t *testing.T) {
	a := endedRun("a", "j1", cronjob.RunSucceeded, 10)
	a.JobName = "backup"
	b := endedRun("b", "j2", cronjob.RunFailed, 20)
	b.JobName = "deploy"
	m := runsPageInTimeline(t, nil, a, b)
	m.PageSearch.Text = "backup"
	m, cmds := model.Update(m, model.KeyMsg{Chord: "s"})
	if !m.Runs.Seen.IsZero() || len(cmds) != 0 || m.Notice == "" {
		t.Fatalf("seen %v, commands %#v, notice %q", m.Runs.Seen, cmds, m.Notice)
	}
	m.PageSearch.Text = ""
	if got := m.Timeline(timelineEpoch).RunCount; got != 2 {
		t.Errorf("%d runs new after the search ended, want 2", got)
	}
}
