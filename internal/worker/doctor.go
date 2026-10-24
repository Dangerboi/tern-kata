package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Check is one line of `worker doctor`.
type Check struct {
	// Level is "ok", "FAIL" or "note" (information that needs no fix).
	Level   string
	Subject string
	Detail  string
	Fix     string
}

// Report is the doctor's findings.
type Report struct{ Checks []Check }

// Problems counts the failed checks.
func (r Report) Problems() int {
	n := 0
	for _, c := range r.Checks {
		if c.Level == "FAIL" {
			n++
		}
	}
	return n
}

func (r Report) String() string {
	var b strings.Builder
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "%-4s  %s: %s\n", c.Level, c.Subject, c.Detail)
		if c.Fix != "" {
			for _, line := range strings.Split(c.Fix, "\n") {
				fmt.Fprintf(&b, "        %s\n", line)
			}
		}
	}
	if r.Problems() == 0 {
		b.WriteString("Nothing is missing.\n")
	}
	return b.String()
}

// DoctorOptions is what the doctor looks at. Kata is nil, with KataErr saying why, when no daemon could be reached.
type DoctorOptions struct {
	Dirs    Dirs
	Kata    *Kata
	KataErr error
	Clock   Clock
	Actor   string
}

// Doctor checks the config, the machine and kata, and says what is missing and how to fix it. It writes nothing.
func Doctor(ctx context.Context, o DoctorOptions) Report {
	var r Report
	add := func(level, subject, detail, fix string) {
		r.Checks = append(r.Checks, Check{level, subject, detail, fix})
	}
	if o.Clock == nil {
		o.Clock = SystemClock{}
	}
	cfg, err := LoadConfig(o.Dirs.ConfigPath())
	if err != nil {
		add("FAIL", "config", err.Error(), "Create "+o.Dirs.ConfigPath()+" (the README's worker setup shows one).")
		return r
	}
	add("ok", "config", fmt.Sprintf("%s: executor %q, %d %s", o.Dirs.ConfigPath(), cfg.Executor, len(cfg.Projects), plural(len(cfg.Projects), "Project", "Projects")), "")
	if len(cfg.Projects) == 0 {
		add("FAIL", "projects", "no Project is served", "Add a [[projects]] table with name = \"<kata Project>\" to worker.toml.")
	}
	if resp, err := Ask(ctx, o.Dirs, Request{Op: "status"}); err == nil && resp.OK {
		add("ok", "worker", fmt.Sprintf("running as process %d", resp.Status.PID), "")
	} else {
		add("note", "worker", "not running on this machine", "Start it with `tern-kata worker`.")
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Checkouts)) {
		if info, err := os.Stat(cfg.Checkouts[name]); err != nil || !info.IsDir() {
			add("FAIL", "checkout "+name, cfg.Checkouts[name]+" is not a directory", "Fix the path under [checkouts] in worker.toml.")
		} else {
			add("ok", "checkout "+name, cfg.Checkouts[name], "")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Profiles)) {
		command := cfg.Profiles[name].command()
		if path, err := exec.LookPath(command); err != nil {
			add("FAIL", "profile "+name, fmt.Sprintf("command %q was not found", command), "Install it, or set the profile's command to an absolute path.")
		} else {
			add("ok", "profile "+name, path, "")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Secrets)) {
		if _, err := cfg.readSecret(name); err != nil {
			add("FAIL", "secret "+name, strings.TrimPrefix(err.Error(), "secret "+fmt.Sprintf("%q", name)+": "), "Fix the source under [secrets] in worker.toml.")
		} else {
			add("ok", "secret "+name, "readable (its value is never shown)", "")
		}
	}
	if o.Kata == nil {
		add("FAIL", "kata", cmp.Or(errString(o.KataErr), "no daemon"), "Start the kata daemon (`kata daemon start`) or run `kata doctor`.")
		return r
	}
	version, err := o.Kata.Ping(ctx)
	if err != nil {
		add("FAIL", "kata", err.Error(), "Upgrade kata to "+MinKataVersion+" or newer, or start its daemon.")
		return r
	}
	add("ok", "kata", "daemon "+version, "")
	projects, err := o.Kata.Projects(ctx)
	if err != nil {
		add("FAIL", "kata Projects", err.Error(), "")
		return r
	}
	served := map[string]bool{}
	for _, pc := range cfg.Projects {
		served[pc.Name] = true
		i := slices.IndexFunc(projects, func(p Project) bool { return p.Name == pc.Name })
		if i < 0 {
			add("FAIL", "Project "+pc.Name, "kata has no such Project", "Create it, or fix the name in worker.toml.")
			continue
		}
		o.checkProject(ctx, cfg, pc, projects[i], add)
	}
	for _, p := range projects {
		if served[p.Name] {
			continue
		}
		jobs, err := o.Kata.Jobs(ctx, p.ID)
		if err != nil {
			continue
		}
		n := 0
		for _, j := range jobs {
			if !j.Native() {
				n++
			}
		}
		if n > 0 {
			add("note", "Project "+p.Name, fmt.Sprintf("has %d adapter %s and is not served", n, plural(n, "job", "jobs")), "To serve it (anyone who can write its jobs can then run prompts and commands here), add to worker.toml:\n[[projects]]\nname = "+fmt.Sprintf("%q", p.Name))
		}
	}
	return r
}

func (o DoctorOptions) checkProject(ctx context.Context, cfg Config, pc ProjectConfig, p Project, add func(level, subject, detail, fix string)) {
	subject := "Project " + pc.Name
	caps, err := o.Kata.Capabilities(ctx, p.ID)
	switch {
	case err != nil:
		add("FAIL", subject, "cannot read the cron capabilities: "+err.Error(), "Upgrade kata to "+MinKataVersion+" or newer.")
		return
	case !caps.Has("cron_v1"):
		add("FAIL", subject, "the daemon does not advertise cron_v1", "Upgrade kata to "+MinKataVersion+" or newer.")
		return
	case caps.ProjectUID != p.UID:
		add("FAIL", subject, fmt.Sprintf("the daemon answers for Project %s where %q resolves to %s", caps.ProjectUID, p.Name, p.UID), "")
		return
	}
	instance, err := o.Kata.Instance(ctx)
	if err != nil {
		add("FAIL", subject, "cannot read the daemon's instance: "+err.Error(), "")
		return
	}
	actor := cmp.Or(pc.Actor, cfg.Actor, o.Actor)
	dir := filepath.Join(o.Dirs.State, "projects", p.UID)
	now := pin{ProjectUID: p.UID, InstanceUID: instance.UID, Name: p.Name, Actor: actor, AuthActor: instance.Actor, Executor: cfg.Executor}
	if prev, ok := readPin(filepath.Join(dir, "pin.json")); ok {
		if diff := pinDifferences(prev, now); len(diff) > 0 {
			add("FAIL", subject, "its pinned route differs: "+joinWords(diff), "There is no repin command: restore the old values, or use a new state folder and executor.")
			return
		}
	}
	owner, generation, live, err := readLease(ctx, o.Kata, o.Clock, p.ID)
	switch {
	case err != nil:
		add("FAIL", subject, "the lease could not be read: "+err.Error(), "")
	case owner != "" && live && owner != cfg.Executor:
		add("FAIL", subject, fmt.Sprintf("another worker (%s) holds the lease (generation %d); this worker would admit nothing", owner, generation), "Stop that worker, wait 10 minutes for its lease to expire, or run `tern-kata worker --takeover`.")
	case owner == cfg.Executor:
		add("ok", subject, fmt.Sprintf("this executor holds the lease (generation %d)", generation), "")
	default:
		add("ok", subject, "the lease is free", "")
	}
	if jobs, err := o.Kata.Jobs(ctx, p.ID); err == nil {
		aside, warnings := definitionNotes(ctx, o.Kata, cfg, pc, p.ID, jobs, o.Clock.Now())
		for _, n := range aside {
			add("FAIL", subject, fmt.Sprintf("job %q is set aside: %s", n.Name, n.Text), "Edit the job, or worker.toml, so the worker can run it.")
		}
		for _, n := range warnings {
			add("note", subject, fmt.Sprintf("job %q %s", n.Name, n.Text), "")
		}
	}
	// Runs kata says are running for this executor with no local journal are never rerun.
	store := NewStore(o.Dirs, Hooks{})
	runs, err := o.Kata.Runs(ctx, p.ID)
	if err != nil {
		return
	}
	for _, run := range runs {
		if run.Status != StatusRunning || run.ExecutorLabel != cfg.Executor {
			continue
		}
		if _, err := store.Load(run.UID); errors.Is(err, ErrNoJournal) && !store.IsQuarantined(run.UID) {
			add("FAIL", subject, fmt.Sprintf("run %s is running in kata for this executor but this machine has no journal of it; it is never rerun", run.UID), "Record it as `unknown` (\"local state lost\") with `tern-kata worker abandon "+run.UID+"`; do not delete anything to unblock the job.")
		}
	}
	for _, uid := range store.Quarantine() {
		detail := fmt.Sprintf("run %s: its journal could not be read and is kept whole under %s; the run is never repeated", uid, store.Quarantined(uid))
		if by, seen := store.Acknowledged(uid); seen {
			add("note", "quarantine", detail+" (acknowledged by "+by+")", "")
			continue
		}
		add("FAIL", "quarantine", detail, "Acknowledge it with `tern-kata worker abandon "+uid+"` (it records the run unknown in kata when kata still shows it running); the folder stays and keeps the veto.")
	}
	runDirs, _ := os.ReadDir(o.Dirs.Runs())
	for _, e := range runDirs {
		j, err := store.Load(e.Name())
		if err != nil {
			continue
		}
		if j.ProjectUID != p.UID || j.Executor != cfg.Executor {
			continue
		}
		for _, why := range j.Inspections() {
			add("FAIL", subject, fmt.Sprintf("run %s needs inspection: %s", j.RunUID, why), "Read the run in kata (`kata cron run show`) and decide what it should say; the worker never overwrites it.")
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
