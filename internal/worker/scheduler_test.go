package worker

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// memCheckpoints is a checkpointStore in memory. Once crashed, saves are lost, as when the process dies first.
type memCheckpoints struct {
	saved   map[string]checkpoint
	crashed bool
}

func (m *memCheckpoints) Load(jobUID string) (checkpoint, bool) {
	c, ok := m.saved[jobUID]
	return c, ok
}

func (m *memCheckpoints) Save(jobUID string, c checkpoint) {
	if !m.crashed {
		c.Version = 1
		c.Pending, c.Parked = slices.Clone(c.Pending), slices.Clone(c.Parked) // a file holds a copy, not the caller's slice
		m.saved[jobUID] = c
	}
}

// fakeJournals answers admission the way run preparation does: an occurrence is always the same run, and a journal
// that already exists is returned with created false.
type fakeJournals struct {
	journals map[string]bool
	admitted int
	created  int
	afterNew func()
}

func (f *fakeJournals) admit(_ context.Context, jobUID string, at time.Time) (string, bool, error) {
	f.admitted++
	uid := RunUID(jobUID, OccurrenceKey(at), at)
	if f.journals[uid] {
		return uid, false, nil
	}
	f.journals[uid] = true
	f.created++
	if f.afterNew != nil {
		f.afterNew()
	}
	return uid, true, nil
}

func TestACrashBetweenAdmittingAnOccurrenceAndSavingTheCheckpointDoesNotRunItTwice(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := cronjob.Job{
		UID: "01JOB", Name: "hourly", DefinitionEventUID: "ev1", CreatedAt: created,
		Definition: cronjob.Definition{
			Enabled: true,
			Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600},
			Action:  cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "Do it."},
		},
	}
	store := &memCheckpoints{saved: map[string]checkpoint{}}
	journals := &fakeJournals{journals: map[string]bool{}}
	start := func() *scheduler {
		return &scheduler{
			project:   "demo",
			store:     store,
			admit:     journals.admit,
			issueDate: func(cronjob.Job) (time.Time, bool) { return time.Time{}, false },
			stopped:   func() bool { return false },
			logf:      func(string, ...any) {},
			jobs:      map[string]*jobSchedule{},
			paused:    map[string]bool{},
		}
	}
	jobs := []cronjob.Job{job}
	due := created.Add(time.Hour)

	first := start()
	first.planAll(jobs, nil, created)
	first.planAll(jobs, nil, due)
	if !first.anyPending() {
		t.Fatal("the occurrence an hour after creation was not planned")
	}
	journals.afterNew = func() { store.crashed = true } // the process dies once the journal is saved
	first.admitDue(context.Background())
	if journals.created != 1 {
		t.Fatalf("%d journals created by the first admission, want 1", journals.created)
	}
	if got := store.saved[job.UID].Pending; len(got) != 1 || !got[0].Equal(due) {
		t.Fatalf("saved pending %v, want the occurrence still waiting after the crash", got)
	}

	store.crashed = false
	second := start()
	second.planAll(jobs, nil, due)
	second.admitDue(context.Background())
	if journals.created != 1 {
		t.Fatalf("%d journals created after the restart, want the occurrence run once", journals.created)
	}
	if journals.admitted != 2 {
		t.Fatalf("%d admissions, want the restart to ask again and be answered by the journal", journals.admitted)
	}
	if second.anyPending() || len(store.saved[job.UID].Pending) != 0 {
		t.Fatalf("occurrence still pending after the journal covered it: %v", store.saved[job.UID].Pending)
	}
}
