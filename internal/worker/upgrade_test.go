package worker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

const upgradeTail = `# keep me
[checkouts]
repo = "/tmp/repo" # and me
[profiles.default]
command = "omp"
[secrets]
token = { env = "TK_UPGRADE_TOKEN" }
`

// upgraded runs Upgrade on text against the daemon, the config being text itself as the loader reads it.
func upgraded(t *testing.T, d *workertest.Daemon, text string) (string, []string, error) {
	t.Helper()
	dirs := workertest.WriteConfig(t, text)
	cfg, err := worker.LoadConfig(dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	return worker.Upgrade(context.Background(), d.Kata, cfg, text)
}

func TestUpgradeAddsWhatTheJobsUseToThatProjectsTableOnly(t *testing.T) {
	d := workertest.StartKata(t)
	demo, other := d.Project(t, "demo"), d.Project(t, "other")
	wf := d.CreateWorkflow(t, demo, "build", workertest.Step{Key: "make", Command: "make"})
	d.CreateJob(t, demo, workertest.Job{Name: "uses secret", Prompt: "Do it.", Secrets: map[string]string{"TOKEN": "token"}})
	d.CreateJob(t, demo, workertest.Job{Name: "builds", Workflow: wf.UID})
	d.CreateJob(t, other, workertest.Job{Name: "plain", Prompt: "Do it."})

	text := "executor = \"laptop\"\n[[projects]]\nname = \"demo\" # the one\ncapacity = 2\n\n# a comment\n[[projects]]\nname = 'other'\n" + upgradeTail
	got, report, err := upgraded(t, d, text)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(text, "capacity = 2\n", "capacity = 2\nsecrets = [\"token\"]\ncommands = true\n", 1)
	if got != want {
		t.Fatalf("upgraded text\n%s\nwant\n%s", got, want)
	}
	if len(report) != 1 || report[0] != `Project demo: added secrets = ["token"], commands = true` {
		t.Fatalf("report %q", report)
	}

	again, report, err := upgraded(t, d, got)
	if err != nil || again != got || len(report) != 0 {
		t.Fatalf("second upgrade: %v, changed %v, report %q; want unchanged", err, again != got, report)
	}
}

func TestUpgradeLeavesAKeyTheUserWroteAndReportsTheNeed(t *testing.T) {
	d := workertest.StartKata(t)
	demo := d.Project(t, "demo")
	wf := d.CreateWorkflow(t, demo, "build", workertest.Step{Key: "make", Command: "make"})
	d.CreateJob(t, demo, workertest.Job{Name: "uses secret", Prompt: "Do it.", Secrets: map[string]string{"TOKEN": "token"}})
	d.CreateJob(t, demo, workertest.Job{Name: "builds", Workflow: wf.UID})

	text := "executor = \"laptop\"\n[[projects]]\nname = \"demo\"\nsecrets = []\ncommands = false\n" + upgradeTail
	got, report, err := upgraded(t, d, text)
	if err != nil {
		t.Fatal(err)
	}
	if got != text {
		t.Fatalf("explicit keys were changed:\n%s", got)
	}
	if len(report) != 2 || !strings.Contains(report[0], `secret "token"`) || !strings.Contains(report[0], "does not allow") || !strings.Contains(report[1], "command step") {
		t.Fatalf("report %q, want the unmet secret and command needs", report)
	}
}

func TestUpgradeReportsASecretWorkerTomlDoesNotDefineAndAddsNothingForIt(t *testing.T) {
	d := workertest.StartKata(t)
	demo := d.Project(t, "demo")
	d.CreateJob(t, demo, workertest.Job{Name: "uses secret", Prompt: "Do it.", Secrets: map[string]string{"TOKEN": "undefined"}})

	text := "executor = \"laptop\"\n[[projects]]\nname = \"demo\"\n" + upgradeTail
	got, report, err := upgraded(t, d, text)
	if err != nil {
		t.Fatal(err)
	}
	if got != text || len(report) != 1 || !strings.Contains(report[0], `"undefined"`) || !strings.Contains(report[0], "does not define") {
		t.Fatalf("got changed %v, report %q", got != text, report)
	}
}

func TestUpgradeWritesNothingWhenAProjectCannotBeResolvedOrWhenProjectsAreInline(t *testing.T) {
	d := workertest.StartKata(t)
	d.Project(t, "demo")

	_, _, err := upgraded(t, d, "executor = \"laptop\"\n[[projects]]\nname = \"demo\"\n[[projects]]\nname = \"ghost\"\n"+upgradeTail)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %v, want one naming the Project kata does not have", err)
	}
	_, _, err = upgraded(t, d, "executor = \"laptop\"\nprojects = [{ name = \"demo\" }]\n"+upgradeTail)
	if err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Fatalf("error %v, want a refusal to edit by hand", err)
	}
}

func TestDoctorSuggestsUpgradeOnlyWhenJobsAreSetAsideForUnlistedSecretsOrCommands(t *testing.T) {
	d := workertest.StartKata(t)
	demo := d.Project(t, "demo")
	dirs := workertest.WriteConfig(t, "executor = \"laptop\"\n[[projects]]\nname = \"demo\"\n"+upgradeTail)

	doctor := func() string {
		return worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: dirs, Kata: d.Kata}).String()
	}
	d.CreateJob(t, demo, workertest.Job{Name: "plain", Prompt: "Do it."})
	if out := doctor(); strings.Contains(out, "worker upgrade") {
		t.Fatalf("doctor suggests an upgrade with nothing set aside:\n%s", out)
	}
	d.CreateJob(t, demo, workertest.Job{Name: "uses secret", Prompt: "Do it.", Secrets: map[string]string{"TOKEN": "token"}})
	if out := doctor(); !strings.Contains(out, "tern-kata worker upgrade") {
		t.Fatalf("doctor does not suggest the upgrade for a job set aside for an unlisted secret:\n%s", out)
	}
}

func TestDoctorSuggestsUpgradeForACommandStepInAProjectWithoutCommands(t *testing.T) {
	d := workertest.StartKata(t)
	demo := d.Project(t, "demo")
	dirs := workertest.WriteConfig(t, "executor = \"laptop\"\n[[projects]]\nname = \"demo\"\n"+upgradeTail)
	wf := d.CreateWorkflow(t, demo, "build", workertest.Step{Key: "make", Command: "make"})
	d.CreateJob(t, demo, workertest.Job{Name: "builds", Workflow: wf.UID})

	out := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: dirs, Kata: d.Kata}).String()
	if !strings.Contains(out, "tern-kata worker upgrade") {
		t.Fatalf("doctor does not suggest the upgrade for a job set aside for a command step:\n%s", out)
	}
}
