package worker

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// SupervisorTern is the Supervisor of a worker the plugin host half in Tern's session daemon started; the host half
// sets it in the worker's environment as TERN_KATA_SUPERVISOR.
const SupervisorTern = "tern"

// Status is what `worker status` shows: what runs, what is served and what is missing.
type Status struct {
	Executor    string
	Actor       string
	Version     string
	Supervisor  string
	PID         int
	Started     time.Time
	Now         time.Time
	Unsupported string
	Projects    []ProjectStatus
	Runs        []RunStatus
	Quarantined []string
}

// ProjectStatus is one served Project.
type ProjectStatus struct {
	Name        string
	Capacity    int
	InUse       int
	Admitting   bool
	Why         string // set when it admits nothing
	AdapterJobs int
	NativeJobs  int
	Following   bool
	// Scheduled counts adapter jobs with a schedule, Queued the occurrences waiting for a slot or an overlap gate, and
	// NextRun is the earliest next occurrence (zero when none).
	Scheduled int
	Queued    int
	NextRun   time.Time
}

// RunStatus is one run of this machine.
type RunStatus struct {
	UID     string
	Project string
	Job     string
	Issue   string
	State   string // running, resuming, held: <reason>, succeeded, failed, cancelled, ...
	Pending string // what is still unreported
	// Attempt is the current attempt of the run's prompt step, 1 for a run never resumed.
	Attempt int
	// Held is true while the run waits for the user; Hold is its reason (a Hold* constant) and Detail what the
	// worker knew when it held. Resumable and Abandonable say which verbs would be accepted now, and ResumeRefusal
	// why a resume would not.
	Held          bool
	Hold          string
	Detail        string
	Resumable     bool
	ResumeRefusal string
	Abandonable   bool
	StartedAt     time.Time
	EndedAt       time.Time
}

// Status reports the worker as it is now.
func (w *Worker) Status() Status {
	s := Status{Executor: w.opt.Config.Executor, Actor: cmpOr(w.opt.Config.Actor, w.opt.Actor), Version: w.opt.Version, Supervisor: w.opt.Supervisor, PID: w.pid, Started: w.started, Now: w.clock.Now(), Unsupported: unsupported}
	w.mu.Lock()
	for _, ps := range w.projects {
		p := ProjectStatus{Name: ps.cfg.Name, Capacity: ps.cfg.Capacity, InUse: len(ps.held) + ps.pending, Following: ps.streamOpen}
		switch {
		case ps.blocked != "":
			p.Why = ps.blocked
		case !ps.resolved:
			p.Why = "not resolved yet"
		case !ps.lease.Held:
			p.Why = ps.lease.Why()
		default:
			p.Admitting = true
		}
		p.Scheduled, p.Queued, p.NextRun = ps.scheduled, ps.queued, ps.nextRun
		for _, j := range ps.jobs {
			if j.IsNative() {
				p.NativeJobs++
			} else {
				p.AdapterJobs++
			}
		}
		s.Projects = append(s.Projects, p)
	}
	w.mu.Unlock()
	journals, quarantined, _ := w.store.Scan()
	s.Quarantined = append(quarantined, w.store.Quarantine()...)
	slices.Sort(s.Quarantined)
	s.Quarantined = slices.Compact(s.Quarantined)
	slices.SortFunc(journals, func(a, b *Journal) int { return b.StartedAt.Compare(a.StartedAt) })
	settled := 0
	for _, j := range journals {
		if j.Executor != w.opt.Config.Executor {
			continue
		}
		if j.Phase == PhaseSettled && !j.Held() {
			if settled++; settled > 5 {
				continue
			}
		}
		s.Runs = append(s.Runs, w.runStatus(j))
	}
	return s
}

// runStatus is runStatus plus what depends on this worker: whether a resume or an abandon would be accepted.
func (w *Worker) runStatus(j *Journal) RunStatus {
	r := runStatus(j)
	if j.Held() {
		r.ResumeRefusal = w.resumeRefusal(j)
		r.Resumable = r.ResumeRefusal == ""
		r.Abandonable = abandonRefusal(j) == ""
	}
	return r
}

func runStatus(j *Journal) RunStatus {
	r := RunStatus{UID: j.RunUID, Project: j.ProjectName, Job: j.JobName, Attempt: j.attempt(), StartedAt: j.StartedAt}
	if j.Issue != nil {
		r.Issue = j.Issue.Ref
	}
	switch {
	case j.Held():
		r.State = "held: " + j.Outcome.Hold
		r.Held, r.Hold, r.Detail = true, j.Outcome.Hold, j.Outcome.Summary
	case j.Outcome != nil:
		r.State = j.Outcome.Status
	case j.Phase == PhaseStarted, j.Phase == PhaseLaunching:
		r.State = "running"
	default:
		r.State = string(j.Phase)
	}
	if j.Outcome != nil {
		r.EndedAt = j.Outcome.At
	}
	var pending []string
	for _, it := range j.Intents {
		if !it.Done {
			pending = append(pending, string(it.Kind))
		}
	}
	r.Pending = strings.Join(pending, ", ")
	return r
}

// String is the text of `worker status`.
func (s Status) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Worker %q running as %s (process %d, up %s", s.Executor, s.Actor, s.PID, s.Now.Sub(s.Started).Round(time.Second))
	if s.Version != "" {
		fmt.Fprintf(&b, ", tern-kata %s", s.Version)
	}
	b.WriteString(")\n")
	b.WriteString(s.supervision())
	if s.Unsupported != "" {
		fmt.Fprintf(&b, "It runs no agents: %s.\n", s.Unsupported)
	}
	if len(s.Projects) == 0 {
		b.WriteString("It serves no Projects: list them under [[projects]] in worker.toml.\n")
	}
	for _, p := range s.Projects {
		if p.Admitting {
			follow := "events not followed yet"
			if p.Following {
				follow = "following events"
			}
			fmt.Fprintf(&b, "Project %s: admitting, %d of %d slots in use, %d adapter %s", p.Name, p.InUse, p.Capacity, p.AdapterJobs, plural(p.AdapterJobs, "job", "jobs"))
			if p.NativeJobs > 0 {
				fmt.Fprintf(&b, " (%d native, left to kata)", p.NativeJobs)
			}
			fmt.Fprintf(&b, ", %s\n", follow)
			if p.Scheduled > 0 {
				fmt.Fprintf(&b, "  %d scheduled %s", p.Scheduled, plural(p.Scheduled, "job", "jobs"))
				if !p.NextRun.IsZero() {
					fmt.Fprintf(&b, ", next run %s", p.NextRun.Local().Format("2006-01-02 15:04"))
				}
				if p.Queued > 0 {
					fmt.Fprintf(&b, ", %d queued", p.Queued)
				}
				b.WriteString("\n")
			}
		} else {
			fmt.Fprintf(&b, "Project %s: admits nothing: %s\n", p.Name, p.Why)
		}
	}
	if len(s.Runs) == 0 {
		b.WriteString("No runs on this machine yet.\n")
	}
	for _, r := range s.Runs {
		fmt.Fprintf(&b, "Run %s  %s  %q  %s", r.UID, r.Project, r.Job, r.State)
		if r.Issue != "" {
			fmt.Fprintf(&b, "  %s", r.Issue)
		}
		if r.Pending != "" {
			fmt.Fprintf(&b, "  (not yet reported: %s)", r.Pending)
		}
		b.WriteString("\n")
		if r.Held {
			fmt.Fprintf(&b, "    %s\n", r.Detail)
			switch {
			case r.Resumable:
				fmt.Fprintf(&b, "    Resume it with `tern-kata worker resume %s` or abandon it with `tern-kata worker abandon %s`.\n", r.UID, r.UID)
			case r.Abandonable:
				fmt.Fprintf(&b, "    It cannot be resumed: %s. Abandon it with `tern-kata worker abandon %s`.\n", r.ResumeRefusal, r.UID)
			default:
				fmt.Fprintf(&b, "    It cannot be resumed or abandoned yet: %s.\n", r.ResumeRefusal)
			}
		}
	}
	for _, q := range s.Quarantined {
		fmt.Fprintf(&b, "Quarantined run %s: its journal could not be read; run `tern-kata worker doctor`.\n", q)
	}
	return b.String()
}

// supervision says what keeps the worker running and how to turn that off.
func (s Status) supervision() string {
	switch s.Supervisor {
	case "":
		return "Not supervised: it was started by hand, so nothing starts it again when it exits.\n"
	case SupervisorTern:
		return "Supervised by Tern's session daemon (the tern-kata plugin's host half): it starts the worker again when it exits, waiting longer after each quick crash. To turn that off, create worker.manual beside worker.toml.\n"
	}
	return fmt.Sprintf("Supervised by %s.\n", s.Supervisor)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
