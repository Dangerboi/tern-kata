package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// workerFixture is kata in process, a worker.toml serving Project "demo" with the stub agent, and the folders for it.
type workerFixture struct {
	kata *workertest.Daemon
	proj worker.Project
	env  workerEnv
	stub string
}

func newWorkerFixture(t *testing.T, extraConfig string) workerFixture {
	t.Helper()
	k := workertest.StartKata(t)
	f := workerFixture{kata: k, proj: k.Project(t, "demo"), stub: workertest.BuildStub(t)}
	repo := t.TempDir()
	command, _ := json.Marshal(f.stub)
	dirs := workertest.WriteConfig(t, `executor = "laptop"
actor = "robot"
[[projects]]
name = "demo"
[checkouts]
repo = `+quote(repo)+`
[profiles.default]
command = `+string(command)+"\n"+extraConfig)
	f.env = workerEnv{Dirs: dirs, Locate: func(context.Context, string) (data.Target, error) { return data.Target{Client: k.Client}, nil }, Log: io.Discard}
	return f
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

// serve runs the foreground worker until the test ends and waits until demo admits.
func (f workerFixture) serve(t *testing.T) {
	t.Helper()
	f.serveUntilStopped(t)
}

// serveUntilStopped runs the foreground worker, waits until demo admits and returns the function that stops it.
func (f workerFixture) serveUntilStopped(t *testing.T) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runWorkerCommand(ctx, LaunchPlan{Subcommand: "worker", Actor: "fallback"}, f.env, io.Discard)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("the worker did not stop")
			}
		})
	}
	t.Cleanup(stop)
	eventually(t, "the worker to admit for demo", func() bool {
		var out bytes.Buffer
		return runWorkerCommand(ctx, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}}, f.env, &out) == nil && strings.Contains(out.String(), "Project demo: admitting")
	})
	return stop
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (f workerFixture) run(t *testing.T, plan LaunchPlan) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runWorkerCommand(context.Background(), plan, f.env, &out)
	return out.String(), err
}

func TestRunAsksTheWorkerAndTheRunEndsRecordedInKata(t *testing.T) {
	f := newWorkerFixture(t, "")
	job := f.kata.CreateJob(t, f.proj, workertest.Job{Name: "review", Prompt: "Review it.", Checkout: "repo"})
	f.serve(t)

	out, err := f.run(t, LaunchPlan{Subcommand: "run", TargetRef: job.UID})
	if err != nil {
		t.Fatal(err)
	}
	var uid string
	if _, rest, ok := strings.Cut(out, "Run "); ok {
		uid, _, _ = strings.Cut(rest, " ")
	}
	if want := "Run " + uid + ` of job "review" in Project demo accepted; follow it with ` + "`tern-kata worker status`.\n"; uid == "" || out != want {
		t.Fatalf("stdout %q", out)
	}
	eventually(t, "kata to hold the run as succeeded", func() bool {
		run, err := f.kata.Kata.ShowRun(context.Background(), f.proj.ID, uid)
		return err == nil && run.Status == "succeeded" && run.Summary == "done"
	})
	eventually(t, "status to list the settled run", func() bool {
		status, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return err == nil && strings.Contains(status, "Run "+uid) && strings.Contains(status, "succeeded")
	})
}

func TestRunRefusalsSayWhy(t *testing.T) {
	f := newWorkerFixture(t, "")
	if _, err := f.run(t, LaunchPlan{Subcommand: "run", TargetRef: "01M5J9AAAAAAAAAAAAAAAAAAAA"}); err == nil || err.Error() != "no tern-kata worker is running on this machine; start one with `tern-kata worker`" {
		t.Fatalf("with no worker: %v", err)
	}
	other := f.kata.Project(t, "elsewhere")
	foreign := f.kata.CreateJob(t, other, workertest.Job{Name: "foreign", Prompt: "x"})
	f.serve(t)
	_, err := f.run(t, LaunchPlan{Subcommand: "run", TargetRef: foreign.UID})
	if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), "belongs to Project elsewhere, which this worker does not serve") {
		t.Fatalf("job of an unserved Project: %v", err)
	}
	if _, err := f.run(t, LaunchPlan{Subcommand: "run", TargetRef: "01M5J9AAAAAAAAAAAAAAAAAAAA"}); err == nil || !strings.Contains(err.Error(), "no job 01M5J9AAAAAAAAAAAAAAAAAAAA in a Project this worker serves") {
		t.Fatalf("unknown job: %v", err)
	}
}

func TestWorkerStatusWithNoWorkerSaysSo(t *testing.T) {
	f := newWorkerFixture(t, "")
	_, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
	if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), "no tern-kata worker is running on this machine") {
		t.Fatalf("error %v", err)
	}
}

func TestWorkerStatusSaysWhatItServes(t *testing.T) {
	f := newWorkerFixture(t, "")
	parent := f.kata.Issue(t, f.proj, "Weekly")
	f.kata.CreateNativeJob(t, f.proj, "child maker", parent.UID)
	f.kata.CreateJob(t, f.proj, workertest.Job{Name: "review", Prompt: "Review it."})
	f.serve(t)
	eventually(t, "the jobs to be counted", func() bool {
		out, _ := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return strings.Contains(out, "Project demo: admitting, 0 of 1 slots in use, 1 adapter job (1 native, left to kata)")
	})
	out, _ := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
	if !strings.HasPrefix(out, `Worker "laptop" running as robot (process `) || !strings.Contains(out, "No runs on this machine yet.") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestWorkerStatusAndDoctorListSetAsideJobsAndTheHighRateWarning(t *testing.T) {
	f := newWorkerFixture(t, "")
	f.kata.CreateJob(t, f.proj, workertest.Job{Name: "fine", Prompt: "Do it."})
	f.kata.CreateJob(t, f.proj, workertest.Job{Name: "mapped nowhere", Prompt: "Do it.", Profile: "nope"})
	f.kata.CreateJob(t, f.proj, workertest.Job{Name: "chatty", Prompt: "Do it.", Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 600}})
	f.kata.CreateJob(t, f.proj, workertest.Job{Name: "chatty but off", Prompt: "Do it.", Disabled: true, Trigger: cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 600}})
	f.serve(t)
	setAside := `Job "mapped nowhere" is set aside: it names profile "nope", which is not available to Project demo in worker.toml`
	chatty := `Job "chatty" creates about 144 Issues a day; consider an existing Issue`
	eventually(t, "status to list both", func() bool {
		out, _ := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return strings.Contains(out, setAside) && strings.Contains(out, chatty)
	})
	if out, _ := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}}); strings.Contains(out, "chatty but off") {
		t.Errorf("status warns of a disabled job:\n%s", out)
	}
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"doctor"}, Actor: "robot"})
	if err == nil {
		t.Errorf("doctor accepted a set-aside job:\n%s", out)
	}
	for _, want := range []string{strings.ToLower(setAside[:1]) + setAside[1:], strings.ToLower(chatty[:1]) + chatty[1:]} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "chatty but off") {
		t.Errorf("doctor warns of a disabled job:\n%s", out)
	}
}

func TestWorkerStatusSaysHowItIsSupervised(t *testing.T) {
	for _, tc := range []struct{ supervisor, want string }{
		{"", "Not supervised: it was started by hand"},
		{worker.SupervisorTern, "Supervised by Tern's session daemon"},
	} {
		f := newWorkerFixture(t, "")
		f.env.Supervisor = tc.supervisor
		f.serve(t)
		out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		if err != nil || !strings.Contains(out, tc.want) {
			t.Errorf("supervisor %q: want %q in:\n%s (err %v)", tc.supervisor, tc.want, out, err)
		}
		if tc.supervisor != "" && !strings.Contains(out, "create "+worker.SupervisionOffWords+".") {
			t.Errorf("supervised status does not say how to turn supervision off:\n%s", out)
		}
	}
}

func TestWorkerDoctorReportsAGoodSetupAndSuggestsUnservedProjects(t *testing.T) {
	f := newWorkerFixture(t, "")
	other := f.kata.Project(t, "elsewhere")
	f.kata.CreateJob(t, other, workertest.Job{Name: "foreign", Prompt: "x"})
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"doctor"}, Actor: "robot"})
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"ok    config: " + f.env.Dirs.ConfigPath(),
		"note  worker: not running on this machine",
		"ok    checkout repo: ",
		"ok    profile default: ",
		"ok    kata: daemon v0.20.0",
		"ok    Project demo: the lease is free",
		"note  Project elsewhere: has 1 adapter job and is not served",
		"[[projects]]\n        name = \"elsewhere\"",
		"Nothing is missing.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
}

func TestWorkerDoctorFailsOnWhatIsBroken(t *testing.T) {
	f := newWorkerFixture(t, "[profiles.broken]\ncommand = \"no-such-agent-program\"\n")
	f.kata.Project(t, "unused")
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"doctor"}, Actor: "robot"})
	if _, is := err.(subcommandError); !is || err.Error() != "worker doctor found 1 problem" {
		t.Fatalf("error %v\n%s", err, out)
	}
	if want := `FAIL  profile broken: command "no-such-agent-program" was not found`; !strings.Contains(out, want) {
		t.Fatalf("doctor output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, "Nothing is missing.") {
		t.Fatal("doctor claims nothing is missing")
	}
}

func TestWorkerDoctorWithoutAConfigSaysWhereItBelongs(t *testing.T) {
	f := newWorkerFixture(t, "")
	if err := os.Remove(f.env.Dirs.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"doctor"}})
	if err == nil || !strings.Contains(out, "FAIL  config: no worker config at "+filepath.Join(f.env.Dirs.Config, "worker.toml")) {
		t.Fatalf("error %v\n%s", err, out)
	}
}

func TestWorkerDoctorNamesAHeldLease(t *testing.T) {
	f := newWorkerFixture(t, "")
	p, _ := f.kata.Kata.Project(context.Background(), f.proj.ID)
	lease := map[string]any{"version": 1, "owner": "desktop", "generation": 3, "heartbeat": time.Now().UTC()}
	if _, err := f.kata.Kata.PatchProjectMetadata(context.Background(), f.proj.ID, p.Revision, "x", map[string]any{"tern_kata.lease": lease}); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"doctor"}, Actor: "robot"})
	if err == nil || !strings.Contains(out, "another worker (desktop) holds the lease (generation 3); this worker would admit nothing") {
		t.Fatalf("error %v\n%s", err, out)
	}
}

func TestPlanLaunchWorkerSubcommands(t *testing.T) {
	plan, err := PlanLaunch([]string{"run", "01M5J9AAAAAAAAAAAAAAAAAAAA", "--as", "ann"}, lookupFrom(nil), "v")
	if err != nil || plan.Subcommand != "run" || plan.TargetRef != "01M5J9AAAAAAAAAAAAAAAAAAAA" || plan.Actor != "ann" {
		t.Fatalf("run: %+v, %v", plan, err)
	}
	plan, err = PlanLaunch([]string{"worker", "--takeover"}, lookupFrom(map[string]string{"USER": "bo"}), "v")
	if err != nil || plan.Subcommand != "worker" || !plan.Takeover || len(plan.Operands) != 0 || plan.Actor != "bo" {
		t.Fatalf("worker: %+v, %v", plan, err)
	}
	for _, args := range [][]string{{"worker", "status"}, {"worker", "doctor"}} {
		if plan, err := PlanLaunch(args, lookupFrom(nil), "v"); err != nil || plan.Subcommand != "worker" || plan.Operands[0] != args[1] {
			t.Fatalf("%v: %+v, %v", args, plan, err)
		}
	}
	if plan, err := PlanLaunch([]string{"worker", "stop", "--now"}, lookupFrom(nil), "v"); err != nil || plan.Operands[0] != "stop" || !plan.Now {
		t.Fatalf("worker stop --now: %+v, %v", plan, err)
	}
	for _, verb := range []string{"resume", "abandon"} {
		plan, err := PlanLaunch([]string{"worker", verb, "01M5J9AAAAAAAAAAAAAAAAAAAA", "--as", "ann"}, lookupFrom(nil), "v")
		if err != nil || plan.Subcommand != "worker" || len(plan.Operands) != 2 || plan.Operands[0] != verb || plan.Operands[1] != "01M5J9AAAAAAAAAAAAAAAAAAAA" || plan.Actor != "ann" {
			t.Fatalf("worker %s: %+v, %v", verb, plan, err)
		}
	}
	for args, want := range map[string]string{
		"run":                            "usage: tern-kata run <job-uid>",
		"run a b":                        "usage: tern-kata run <job-uid>",
		"worker stop now":                "usage: tern-kata worker",
		"worker status --now":            "--now applies to `tern-kata worker stop` only",
		"worker status --takeover":       "--takeover applies to `tern-kata worker` only",
		"worker resume":                  "usage: tern-kata worker",
		"worker abandon a --actor alice": "worker init` only",
		"worker resume a --now":          "--now applies to `tern-kata worker stop` only",
		"worker abandon a b":             "usage: tern-kata worker",
		"worker resume a --takeover":     "usage: tern-kata worker",
		"run x --takeover":               "run: unknown flag --takeover",
	} {
		_, err := PlanLaunch(strings.Fields(args), lookupFrom(nil), "v")
		if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want a subcommand error saying %q", args, err, want)
		}
	}
	for _, line := range []string{"tern-kata run <job-uid>", "tern-kata worker status", "tern-kata worker stop [--now]", "tern-kata worker doctor", "tern-kata worker [--takeover]", "tern-kata worker resume <run-uid>", "tern-kata worker abandon <run-uid>"} {
		if !strings.Contains(usage, line) {
			t.Errorf("--help lacks %q", line)
		}
	}
}

// runJob asks the worker to run job now and returns the run's uid.
func (f workerFixture) runJob(t *testing.T, job cronjob.Job) string {
	t.Helper()
	out, err := f.run(t, LaunchPlan{Subcommand: "run", TargetRef: job.UID})
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(out, "Run ")
	uid, _, _ := strings.Cut(rest, " ")
	return uid
}

// startedRun waits until the run's journal says its agent was started.
func (f workerFixture) startedRun(t *testing.T, uid string) {
	t.Helper()
	store := worker.NewStore(f.env.Dirs, worker.Hooks{})
	eventually(t, "the agent of run "+uid+" to start", func() bool {
		j, err := store.Load(uid)
		return err == nil && j.Phase == worker.PhaseStarted && j.Launch != nil && j.Launch.SessionID != ""
	})
}

func TestWorkerResumeAndAbandonSettleRunsHeldByARestart(t *testing.T) {
	stub := workertest.BuildStub(t)
	f := newWorkerFixture(t, "[profiles.first]\ncommand = "+quote(stub)+"\nargs = [\"--stub-hang-first\", \"--stub-text\", \"Checked it.\"]\n"+
		"[profiles.stuck]\ncommand = "+quote(stub)+"\nargs = [\"--stub-hang\"]\n")
	resumable := f.kata.CreateJob(t, f.proj, workertest.Job{Name: "resumable", Prompt: "Check it.", Profile: "first"})
	doomed := f.kata.CreateJob(t, f.proj, workertest.Job{Name: "doomed", Prompt: "Check it.", Profile: "stuck"})

	// Each worker is stopped while its agent runs; the next one finds the run held.
	stop := f.serveUntilStopped(t)
	uidA := f.runJob(t, resumable)
	f.startedRun(t, uidA)
	stop()
	stop = f.serveUntilStopped(t)
	eventually(t, "the first run to be held and its slot free", func() bool {
		out, _ := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return strings.Contains(out, "Run "+uidA) && strings.Contains(out, "tern-kata worker resume "+uidA) && strings.Contains(out, "0 of 1 slots in use")
	})
	uidB := f.runJob(t, doomed)
	f.startedRun(t, uidB)
	stop()
	f.serve(t)

	var status string
	// The slot of a run found held at start is freed only once its hold is reported to kata, after status first offers it.
	eventually(t, "status to offer both held runs with their slots free", func() bool {
		status, _ = f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return strings.Contains(status, "tern-kata worker resume "+uidA) && strings.Contains(status, "tern-kata worker resume "+uidB) && strings.Contains(status, "0 of 1 slots in use")
	})
	for _, want := range []string{"Run " + uidA, "held: interrupted", "or abandon it with `tern-kata worker abandon " + uidA + "`"} {
		if !strings.Contains(status, want) {
			t.Errorf("status lacks %q:\n%s", want, status)
		}
	}

	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"resume", uidA}, Actor: "ann"})
	if want := "Run " + uidA + ` of job "resumable" in Project demo resumed: the worker continues its omp session; follow it with ` + "`tern-kata worker status`.\n"; err != nil || out != want {
		t.Fatalf("resume printed %q, %v", out, err)
	}
	eventually(t, "kata to hold the resumed run as succeeded", func() bool {
		run, err := f.kata.Kata.ShowRun(context.Background(), f.proj.ID, uidA)
		return err == nil && run.Status == "succeeded" && run.Summary == "Checked it."
	})

	out, err = f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"abandon", uidB}, Actor: "ann"})
	if want := "Run " + uidB + ` of job "doomed" in Project demo abandoned: kata will record it cancelled and its slot is free.` + "\n"; err != nil || out != want {
		t.Fatalf("abandon printed %q, %v", out, err)
	}
	eventually(t, "kata to hold the abandoned run as cancelled", func() bool {
		run, err := f.kata.Kata.ShowRun(context.Background(), f.proj.ID, uidB)
		return err == nil && run.Status == "cancelled" && strings.HasPrefix(run.Summary, "abandoned by ann (was held: interrupted: ")
	})

	for _, plan := range []LaunchPlan{
		{Subcommand: "worker", Operands: []string{"resume", uidB}},
		{Subcommand: "worker", Operands: []string{"abandon", uidA}},
		{Subcommand: "worker", Operands: []string{"resume", "01M5J9AAAAAAAAAAAAAAAAAAAA"}},
	} {
		_, err := f.run(t, plan)
		if _, is := err.(subcommandError); !is {
			t.Errorf("%v: %v, want a refusal", plan.Operands, err)
		}
	}
	if _, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"resume", uidB}}); err == nil || !strings.Contains(err.Error(), "not held") {
		t.Errorf("resuming a cancelled run: %v", err)
	}
}

func TestWorkerResumeWithNoWorkerSaysSo(t *testing.T) {
	f := newWorkerFixture(t, "")
	_, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"abandon", "01M5J9AAAAAAAAAAAAAAAAAAAA"}})
	if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), "no tern-kata worker is running on this machine") {
		t.Fatalf("error %v", err)
	}
}

func TestWorkerAbandonStopsARunThatIsRunning(t *testing.T) {
	stub := workertest.BuildStub(t)
	f := newWorkerFixture(t, "[profiles.stuck]\ncommand = "+quote(stub)+"\nargs = [\"--stub-hang\"]\n")
	job := f.kata.CreateJob(t, f.proj, workertest.Job{Name: "stuck", Prompt: "Check it.", Profile: "stuck"})
	f.serve(t)
	uid := f.runJob(t, job)
	f.startedRun(t, uid)

	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"abandon", uid}, Actor: "ann"})
	if want := "Run " + uid + ` of job "stuck" in Project demo is being stopped: the worker signals its process group (SIGTERM, then SIGKILL after 10 s) and kata records it cancelled once it is gone.` + "\n"; err != nil || out != want {
		t.Fatalf("abandon printed %q, %v", out, err)
	}
	eventually(t, "kata to hold the stopped run as cancelled", func() bool {
		run, err := f.kata.Kata.ShowRun(context.Background(), f.proj.ID, uid)
		return err == nil && run.Status == "cancelled" && strings.HasPrefix(run.Summary, "abandoned by ann (was running)")
	})
}

func TestWorkerStopSaysTheModeTheRunsInFlightAndWhetherItStartsAgain(t *testing.T) {
	stub := workertest.BuildStub(t)
	f := newWorkerFixture(t, "[profiles.stuck]\ncommand = "+quote(stub)+"\nargs = [\"--stub-hang\"]\n")
	job := f.kata.CreateJob(t, f.proj, workertest.Job{Name: "stuck", Prompt: "Check it.", Profile: "stuck"})
	f.env.Supervisor = worker.SupervisorTern
	stopped := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = runWorkerCommand(ctx, LaunchPlan{Subcommand: "worker", Actor: "fallback"}, f.env, io.Discard)
		close(stopped)
	}()
	eventually(t, "the worker to admit", func() bool {
		out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}})
		return err == nil && strings.Contains(out, "Project demo: admitting")
	})
	uid := f.runJob(t, job)
	f.startedRun(t, uid)

	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"stop"}})
	if want := "Stopping when idle: the worker admits nothing new and exits once its runs in flight have finished (1 run in flight now). `tern-kata worker stop --now` ends it at once. The plugin will start a fresh worker.\n"; err != nil || out != want {
		t.Fatalf("stop printed %q, %v", out, err)
	}
	out, err = f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"stop"}, Now: true})
	if want := "Stopping now: the worker ends at once; its 1 run in flight will be held interrupted and can be resumed once a worker runs again. The plugin will start a fresh worker.\n"; err != nil || out != want {
		t.Fatalf("stop --now printed %q, %v", out, err)
	}
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the worker did not exit")
	}
}

func TestWorkerStopSaysNothingWillStartItAgainInTheSupervisionRulesWords(t *testing.T) {
	f := newWorkerFixture(t, "")
	stop := f.serveUntilStopped(t)
	defer stop()

	out, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"stop"}})
	want := "Stopping when idle: 0 runs in flight, so the worker exits now. Nothing will start it again: start one with `tern-kata worker`, or let the plugin (" + worker.SupervisionOnWords + ").\n"
	if err != nil || out != want {
		t.Fatalf("stop printed %q, %v; want %q", out, err, want)
	}
	if !strings.Contains(worker.SupervisionOnWords, worker.ConfigFileName) || !strings.Contains(worker.SupervisionOnWords, worker.ManualFileName) {
		t.Errorf("the rule's words %q do not name %s and %s", worker.SupervisionOnWords, worker.ConfigFileName, worker.ManualFileName)
	}
}

func TestWorkerStopWithNoWorkerSaysSo(t *testing.T) {
	f := newWorkerFixture(t, "")
	_, err := f.run(t, LaunchPlan{Subcommand: "worker", Operands: []string{"stop"}})
	if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), "no tern-kata worker is running on this machine") {
		t.Fatalf("error %v", err)
	}
}

// Only init and add read the working directory: the other worker commands work from a folder that was removed.
func TestNewWorkerEnvReadsTheWorkingDirectoryOnlyForInitAndAdd(t *testing.T) {
	gone := t.TempDir()
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("cannot remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this system still reports a removed working directory")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, operands := range [][]string{nil, {"status"}, {"stop"}, {"edit"}, {"doctor"}, {"abandon", "x"}} {
		if _, err := newWorkerEnv(LaunchPlan{Subcommand: "worker", Operands: operands}); err != nil {
			t.Errorf("worker %v from a removed folder: %v", operands, err)
		}
	}
	if _, err := newWorkerEnv(LaunchPlan{Subcommand: "run", TargetRef: "x"}); err != nil {
		t.Errorf("run from a removed folder: %v", err)
	}
	for _, verb := range []string{"init", "add"} {
		if _, err := newWorkerEnv(LaunchPlan{Subcommand: "worker", Operands: []string{verb}}); err == nil {
			t.Errorf("worker %s from a removed folder succeeded", verb)
		}
	}
}
