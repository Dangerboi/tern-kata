package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWorkerCommand(ctx, LaunchPlan{Subcommand: "worker", Actor: "fallback"}, f.env, io.Discard) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the worker did not stop")
		}
	})
	eventually(t, "the worker to admit for demo", func() bool {
		var out bytes.Buffer
		return runWorkerCommand(ctx, LaunchPlan{Subcommand: "worker", Operands: []string{"status"}}, f.env, &out) == nil && strings.Contains(out.String(), "Project demo: admitting")
	})
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
		"ok    kata: daemon v0.19.0",
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
	for args, want := range map[string]string{
		"run":                    "usage: tern-kata run <job-uid>",
		"run a b":                "usage: tern-kata run <job-uid>",
		"worker stop":            "usage: tern-kata worker",
		"worker status --takeover": "--takeover applies to `tern-kata worker` only",
		"run x --takeover":       "run: unknown flag --takeover",
	} {
		_, err := PlanLaunch(strings.Fields(args), lookupFrom(nil), "v")
		if _, is := err.(subcommandError); !is || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want a subcommand error saying %q", args, err, want)
		}
	}
	for _, line := range []string{"tern-kata run <job-uid>", "tern-kata worker status", "tern-kata worker doctor", "tern-kata worker [--takeover]"} {
		if !strings.Contains(usage, line) {
			t.Errorf("--help lacks %q", line)
		}
	}
}
