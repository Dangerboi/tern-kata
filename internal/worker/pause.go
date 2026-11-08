package worker

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// A job that keeps failing pauses itself in this worker's own state (ADR-0006: the worker's state is its journals and the
// files beside them). kata's job definition is never written, so other machines' workers keep their own counts and the Jobs
// Page reads the pause from this worker's status.

type streakFile struct {
	Version int `json:"version"`
	// Failed counts the runs recorded failed since the last succeeded run or resume.
	Failed int `json:"failed"`
	// Paused is set once Failed reached the limit; it holds until a resume, whatever the limit is later set to.
	Paused bool      `json:"paused,omitempty"`
	Since  time.Time `json:"since,omitzero"`
	// Name is the job's name when it paused, for `worker status` while kata cannot be asked.
	Name string `json:"name,omitempty"`
	// Seen lists the latest run uids counted, so a report replayed after a crash is not counted twice.
	Seen []string `json:"seen,omitempty"`
}

const maxSeenRuns = 32

const streakDir = "failures"

type PausedJob struct {
	Project string
	JobUID  string
	Name    string
	Failed  int
	Since   time.Time
}

// Reason is the pause as `worker status` words it.
func (p PausedJob) Reason() string {
	return fmt.Sprintf("paused after %d failed %s in a row; resume it with `tern-kata worker resume %s`", p.Failed, plural(p.Failed, "run", "runs"), p.JobUID)
}

func (w *Worker) streakPath(projectUID, jobUID string) string {
	return filepath.Join(w.opt.Dirs.ProjectDir(projectUID), streakDir, jobUID+".json")
}

func (w *Worker) streakLock(projectUID, jobUID string) *sync.Mutex {
	return w.streaks.get(projectUID + "/" + jobUID)
}

func (w *Worker) loadStreak(projectUID, jobUID string) (streakFile, error) {
	raw, err := readFile(w.streakPath(projectUID, jobUID))
	if errors.Is(err, fs.ErrNotExist) {
		return streakFile{}, nil
	}
	if err != nil {
		return streakFile{}, err
	}
	var s streakFile
	if err := json.Unmarshal(raw, &s); err != nil || s.Version != 1 {
		return streakFile{}, fmt.Errorf("the failure count of job %s is unreadable", jobUID)
	}
	return s, nil
}

func (w *Worker) saveStreak(projectUID, jobUID string, s streakFile) error {
	s.Version = 1
	return saveJSON(w.streakPath(projectUID, jobUID), s)
}

// noteOutcome counts a run whose outcome is recorded: a run recorded failed (a timeout included) adds one, a succeeded run
// resets the count unless the job is paused (only a resume lifts a pause), and every other run (a hold, a cancel) leaves it as it is. The count pauses the job once it reaches
// pause_after_failures. It is idempotent per run, so the report that replays after a crash counts nothing twice.
func (w *Worker) noteOutcome(j *Journal) {
	if j.Outcome == nil || j.held() {
		return
	}
	status := j.Outcome.Status
	if status != cronjob.RunFailed && status != cronjob.RunSucceeded {
		return
	}
	lock := w.streakLock(j.ProjectUID, j.JobUID)
	lock.Lock()
	defer lock.Unlock()
	s, err := w.loadStreak(j.ProjectUID, j.JobUID)
	if err != nil {
		w.opt.Log("%v; counting starts again", err)
		s = streakFile{}
	}
	if slices.Contains(s.Seen, j.RunUID) {
		return
	}
	s.Seen = append(s.Seen, j.RunUID)
	if len(s.Seen) > maxSeenRuns {
		s.Seen = s.Seen[len(s.Seen)-maxSeenRuns:]
	}
	if status == cronjob.RunSucceeded {
		if !s.Paused { // a run in flight when the job paused, or held and resumed, finishes without lifting the pause
			s.Failed = 0
		}
	} else {
		s.Failed++
		if limit := w.opt.Config.PauseAfterFailures(); limit > 0 && s.Failed >= limit && !s.Paused {
			s.Paused, s.Since, s.Name = true, w.clock.Now().UTC(), j.JobName
			w.opt.Log("job %q of Project %s is paused after %d failed runs in a row; resume it with `tern-kata worker resume %s`", j.JobName, j.ProjectName, s.Failed, j.JobUID)
		}
	}
	if err := w.saveStreak(j.ProjectUID, j.JobUID, s); err != nil {
		w.opt.Log("saving the failure count of job %s: %v", j.JobUID, err)
	}
}

// pauseRefusal is why a paused job admits no run, nil when it is not paused. A pause with pausing since turned off stays
// until resumed: only a resume lifts it.
func (w *Worker) pauseRefusal(ps *projectState, job cronjob.Job) error {
	s, err := w.loadStreak(ps.project.UID, job.UID)
	if err != nil {
		w.opt.Log("%v; counting starts again", err)
		return nil
	}
	if !s.Paused {
		return nil
	}
	p := PausedJob{JobUID: job.UID, Failed: s.Failed}
	return &busyError{fmt.Sprintf("job %q is paused: %s", job.Name, p.Reason()), false}
}

// ResumeJob lifts a job's pause and starts its count again from none. It refuses a job that is not paused.
func (w *Worker) ResumeJob(jobUID string) (PausedJob, error) {
	if err := w.stopRefusal(cmdResumeJob); err != nil {
		return PausedJob{}, err
	}
	for _, p := range w.PausedJobs() {
		if p.JobUID != jobUID {
			continue
		}
		ps := w.projectNamed(p.Project)
		lock := w.streakLock(ps.project.UID, jobUID)
		lock.Lock()
		defer lock.Unlock()
		s, err := w.loadStreak(ps.project.UID, jobUID)
		if err != nil {
			return PausedJob{}, err
		}
		s.Failed, s.Paused, s.Since, s.Name = 0, false, time.Time{}, ""
		if err := w.saveStreak(ps.project.UID, jobUID, s); err != nil {
			return PausedJob{}, err
		}
		w.opt.Log("job %q of Project %s was resumed", p.Name, p.Project)
		w.kick(ps) // its waiting occurrences are due another look
		return p, nil
	}
	return PausedJob{}, fmt.Errorf("job %s is not paused by this worker", jobUID)
}

func (w *Worker) PausedJobs() []PausedJob {
	var out []PausedJob
	for _, ps := range w.projects {
		w.mu.Lock()
		resolved, uid, name, loaded := ps.resolved, ps.project.UID, ps.cfg.Name, ps.jobs != nil
		names := map[string]string{}
		for id, job := range ps.jobs {
			names[id] = job.Name
		}
		w.mu.Unlock()
		if !resolved {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(w.opt.Dirs.ProjectDir(uid), streakDir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			jobUID, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok {
				continue
			}
			s, err := w.loadStreak(uid, jobUID)
			if err != nil || !s.Paused {
				continue
			}
			if _, live := names[jobUID]; !live && loaded {
				continue // the job is gone or archived: nothing to resume
			}
			out = append(out, PausedJob{Project: name, JobUID: jobUID, Name: cmp.Or(names[jobUID], s.Name), Failed: s.Failed, Since: s.Since})
		}
	}
	slices.SortFunc(out, func(a, b PausedJob) int {
		if c := strings.Compare(a.Project, b.Project); c != 0 {
			return c
		}
		return strings.Compare(a.Name+a.JobUID, b.Name+b.JobUID)
	})
	return out
}
