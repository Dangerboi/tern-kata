package worker

import (
	"cmp"
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
	Projects    []ProjectStatus
	Runs        []RunStatus
	Quarantined []string
	// Stopping is "idle" while the worker drains toward an exit, "now" once it was told to end at once; InFlight is the runs a drain waits for.
	Stopping string
	InFlight int
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
	// Aside lists the jobs the worker sets aside with the reason of each; Warnings the per-run Issue jobs that create
	// many Issues a day. Both are as of the last definition refresh and stop no other job.
	Aside, Warnings []JobNote
}

// RunStatus is one run of this machine.
type RunStatus struct {
	UID     string
	Project string
	Job     string
	Issue   string
	State   string // running, resuming, held: <reason>, succeeded, failed, cancelled, ...
	Pending string // what is still unreported
	// Inspect names each report kata holds something newer than, so it was not sent: the run needs a person to look.
	Inspect []string
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
	s := Status{Executor: w.opt.Config.Executor, Actor: cmp.Or(w.opt.Config.Actor, w.opt.Actor), Version: w.opt.Version, Supervisor: w.opt.Supervisor, PID: w.pid, Started: w.started, Now: w.clock.Now()}
	w.mu.Lock()
	s.Stopping, s.InFlight = w.stopping, w.inFlightLocked()
	for _, ps := range w.projects {
		p := ProjectStatus{Name: ps.cfg.Name, Capacity: ps.cfg.Capacity, InUse: len(ps.held) + ps.pending, Following: ps.streamOpen}
		switch {
		case w.stopping != "":
			p.Why = errStopping.Error()
		case ps.blocked != "":
			p.Why = ps.blocked
		case !ps.resolved:
			p.Why = "not resolved yet"
		case !ps.lease.Held:
			p.Why = ps.lease.Why()
		case ps.route.err != "":
			p.Why = "the kata route is not confirmed: " + ps.route.err
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
		p.Aside, p.Warnings = slices.Clone(ps.aside), slices.Clone(ps.warnings)
		s.Projects = append(s.Projects, p)
	}
	w.mu.Unlock()
	journals, quarantined, _ := w.store.Scan()
	s.Quarantined = append(quarantined, w.store.Quarantine()...)
	slices.Sort(s.Quarantined)
	s.Quarantined = slices.Compact(s.Quarantined)
	s.Quarantined = slices.DeleteFunc(s.Quarantined, func(uid string) bool { _, seen := w.store.Acknowledged(uid); return seen })
	slices.SortFunc(journals, func(a, b *Journal) int { return b.StartedAt.Compare(a.StartedAt) })
	settled := 0
	for _, j := range journals {
		if j.Executor != w.opt.Config.Executor {
			continue
		}
		if j.settled() && !j.held() && len(j.Inspections()) == 0 {
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
	if j.held() {
		r.ResumeRefusal = w.life().mayResume(j)
		r.Resumable = r.ResumeRefusal == ""
	}
	r.Abandonable = w.life().mayAbandon(j) == "" // a running run can be asked to stop
	return r
}

func runStatus(j *Journal) RunStatus {
	r := RunStatus{UID: j.RunUID, Project: j.ProjectName, Job: j.JobName, Attempt: j.attempt(), StartedAt: j.StartedAt}
	if j.Issue != nil {
		r.Issue = j.Issue.Ref
	}
	r.State = j.stateName()
	if j.held() {
		r.Held, r.Hold, r.Detail = true, j.Outcome.Hold, j.Outcome.Summary
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
	r.Inspect = j.Inspections()
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
	switch s.Stopping {
	case StopIdle:
		fmt.Fprintf(&b, "Stopping: it admits nothing new and exits once its runs in flight have finished, waiting for %d %s. `tern-kata worker stop --now` ends it at once.\n", s.InFlight, plural(s.InFlight, "run", "runs"))
	case StopNow:
		b.WriteString("Stopping now: it is ending its runs in flight, which are held interrupted and resumable.\n")
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
		for _, n := range p.Aside {
			fmt.Fprintf(&b, "  Job %q is set aside: %s\n", n.Name, n.Text)
		}
		for _, n := range p.Warnings {
			fmt.Fprintf(&b, "  Job %q %s\n", n.Name, n.Text)
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
		for _, why := range r.Inspect {
			b.WriteString("\n    needs inspection: " + why)
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
		return "Supervised by Tern's session daemon (the tern-kata plugin's host half): it starts the worker again when it exits, waiting longer after each quick crash. To turn that off, create " + ManualFileName + " beside " + ConfigFileName + ".\n"
	}
	return fmt.Sprintf("Supervised by %s.\n", s.Supervisor)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
