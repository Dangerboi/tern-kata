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
	created  int
	afterNew func()
}

func (f *fakeJournals) admit(_ context.Context, jobUID string, at time.Time) (string, bool, error) {
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
	start := func() *projectSchedule {
		return &projectSchedule{
			store:     store,
			admit:     journals.admit,
			issueDate: func(cronjob.Job) (time.Time, bool) { return time.Time{}, false },
			stopped:   func() bool { return false },
			log:       func(string, ...any) {},
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
	if second.anyPending() || len(store.saved[job.UID].Pending) != 0 {
		t.Fatalf("occurrence still pending after the journal covered it: %v", store.saved[job.UID].Pending)
	}
}

// pausedHourlyJob is an hourly execute job with the given catchup policy and no grace.
func pausedHourlyJob(created time.Time, catchup string) cronjob.Job {
	return cronjob.Job{
		UID: "01JOB", Name: "hourly", DefinitionEventUID: "ev1", CreatedAt: created,
		Definition: cronjob.Definition{
			Enabled: true,
			Catchup: catchup,
			Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600},
			Action:  cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "Do it."},
		},
	}
}

func newMemSchedule(store checkpointStore) *projectSchedule {
	return &projectSchedule{
		store:     store,
		admit:     (&fakeJournals{journals: map[string]bool{}}).admit,
		issueDate: func(cronjob.Job) (time.Time, bool) { return time.Time{}, false },
		stopped:   func() bool { return false },
		log:       func(string, ...any) {},
		jobs:      map[string]*jobSchedule{},
		paused:    map[string]bool{},
	}
}

// Enabling a job again is a definition write like any edit: work waiting before the disable is dropped, and the time
// the job was off is not caught up.
func TestReEnablingAJobDropsTheOccurrencesWaitingBeforeTheDisableAndDoesNotCatchUpTheTimeOff(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := pausedHourlyJob(created, "all")
	store := &memCheckpoints{saved: map[string]checkpoint{}}
	s := newMemSchedule(store)

	s.planAll([]cronjob.Job{job}, nil, created)
	s.planAll([]cronjob.Job{job}, nil, due)
	if got := s.jobs[job.UID].pending; len(got) != 1 || !got[0].Equal(due) {
		t.Fatalf("pending before the disable is %v, want the occurrence at %s", got, due)
	}
	s.planAll(nil, map[string]bool{job.UID: true}, due.Add(time.Minute)) // disabled before the occurrence was admitted
	if !store.saved[job.UID].Paused {
		t.Fatal("the disabled job's checkpoint carries no pause mark")
	}

	job.DefinitionEventUID = "ev2" // kata gives the enabling write a new definition event
	s.planAll([]cronjob.Job{job}, nil, due.Add(5*time.Hour+30*time.Minute))
	if got := s.jobs[job.UID].pending; len(got) != 0 {
		t.Fatalf("pending after the re-enable is %v, want the work waiting before the disable dropped and the time off not caught up", got)
	}
}

func TestReEnablingAJobDropsAParkedOccurrenceAndAnUnadmittedOne(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := pausedHourlyJob(created, "skip")
	job.Definition.GraceSeconds = 0
	checkpointOf := func(parked []time.Time) checkpoint {
		return checkpoint{Version: 1, DefinitionEventUID: "ev1", Through: due, Pending: []time.Time{due}, Parked: parked, Paused: true}
	}
	reEnabled := due.Add(5 * time.Hour)
	job.DefinitionEventUID = "ev2"

	for name, parked := range map[string][]time.Time{"parked": {due}, "pending only": nil} {
		s := newMemSchedule(&memCheckpoints{saved: map[string]checkpoint{job.UID: checkpointOf(parked)}})
		s.planAll([]cronjob.Job{job}, nil, reEnabled)
		if got := s.jobs[job.UID].pending; len(got) != 0 {
			t.Fatalf("%s occurrence is %v after the re-enable, want it dropped with the old definition", name, got)
		}
		if got := s.jobs[job.UID].parked; len(got) != 0 {
			t.Fatalf("%s: parked is %v after the re-enable, want none", name, got)
		}
	}
}
