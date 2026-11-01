package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.kenn.io/kata/pkg/client"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// daemonAt is a locator for a kata daemon that answers a ping with version.
func daemonAt(t *testing.T, version string) data.Locator {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": version})
	}))
	t.Cleanup(server.Close)
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string) (data.Target, error) { return data.Target{Client: c}, nil }
}

// preflightOf runs `update --preflight` of a binary at version against a machine and returns what it printed and its status.
func preflightOf(t *testing.T, locate data.Locator, config string) (preflightAnswer, int) {
	t.Helper()
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	if config != "" {
		if err := os.WriteFile(dirs.ConfigPath(), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	err := runPreflight(context.Background(), preflightEnv{Locate: locate, Dirs: dirs, Version: "v0.9.0"}, &out)
	status := 0
	if err != nil {
		status = exitStatus(err)
	}
	var answer preflightAnswer
	if jsonErr := json.Unmarshal(out.Bytes(), &answer); jsonErr != nil {
		t.Fatalf("stdout %q is not JSON: %v", out.String(), jsonErr)
	}
	return answer, status
}

func TestPreflightOfAHealthyMachineRefusesNothing(t *testing.T) {
	var out bytes.Buffer
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	err := runPreflight(context.Background(), preflightEnv{Locate: daemonAt(t, "0.20.0"), Dirs: dirs, Version: "v0.9.0"}, &out)
	if err != nil || strings.TrimSpace(out.String()) != `{"refusals":[],"warnings":[]}` {
		t.Errorf("got %q, %v", out.String(), err)
	}
}

func TestPreflightRefusesADaemonOlderThanTheNewBinaryNeeds(t *testing.T) {
	answer, status := preflightOf(t, daemonAt(t, "0.19.0"), "")
	want := "tern-kata v0.9.0 needs kata " + worker.MinKataVersion + " or newer; this machine's daemon is v0.19.0: upgrade kata, then run tern-kata update again"
	if status != 1 || len(answer.Refusals) != 1 || answer.Refusals[0] != (preflightRefusal{Kind: "kata", Message: want}) || len(answer.Warnings) != 0 {
		t.Errorf("status %d, answer %+v, want a kata refusal %q", status, answer, want)
	}
}

func TestPreflightWarnsOfAnUnreachableDaemonAndStillExitsZero(t *testing.T) {
	answer, status := preflightOf(t, func(context.Context, string) (data.Target, error) {
		return data.Target{}, errors.New("no daemon\nstart the kata daemon")
	}, "")
	if status != 0 || len(answer.Refusals) != 0 || len(answer.Warnings) != 1 || !strings.Contains(answer.Warnings[0], "no daemon") {
		t.Errorf("status %d, answer %+v", status, answer)
	}

	server := httptest.NewServer(http.NotFoundHandler())
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	server.Close() // a daemon that was found but does not answer
	answer, status = preflightOf(t, func(context.Context, string) (data.Target, error) { return data.Target{Client: c}, nil }, "")
	if status != 0 || len(answer.Refusals) != 0 || len(answer.Warnings) != 1 {
		t.Errorf("a failed ping: status %d, answer %+v", status, answer)
	}
}

func TestPreflightRefusesAWorkerConfigTheNewBinaryCannotLoad(t *testing.T) {
	answer, status := preflightOf(t, daemonAt(t, "0.20.0"), "no_such_key = 1\n")
	if status != 1 || len(answer.Refusals) != 1 || answer.Refusals[0].Kind != "config" {
		t.Fatalf("status %d, answer %+v", status, answer)
	}
	msg := answer.Refusals[0].Message
	if !strings.Contains(msg, "unknown key no_such_key") || !strings.HasSuffix(msg, "after updating, fix it with `tern-kata worker upgrade` or `tern-kata worker edit`") {
		t.Errorf("message %q", msg)
	}
}

func TestPreflightAcceptsAWorkerConfigTheNewBinaryCanLoad(t *testing.T) {
	if answer, status := preflightOf(t, daemonAt(t, "0.20.0"), "executor = \"laptop\"\n[profiles.default]\n"); status != 0 || len(answer.Refusals) != 0 {
		t.Errorf("status %d, answer %+v", status, answer)
	}
}

func TestPreflightCommandLine(t *testing.T) {
	plan, err := PlanLaunch([]string{"update", "--preflight"}, lookupFrom(nil), "v")
	if err != nil || plan.Subcommand != "update" || !plan.Preflight || plan.Force {
		t.Errorf("update --preflight: %+v, %v", plan, err)
	}
}

const refusingKata = `{"refusals":[{"kind":"kata","message":"tern-kata v0.2.0 needs kata v0.20.0 or newer; this machine's daemon is v0.19.0: upgrade kata, then run tern-kata update again"}],"warnings":["a warning"]}`
const refusingConfig = `{"refusals":[{"kind":"config","message":"worker.toml: unknown key x; after updating, fix it with ` + "`tern-kata worker upgrade`" + ` or ` + "`tern-kata worker edit`" + `"}],"warnings":[]}`

func TestUpdateStopsOnAPreflightRefusalWithTheBinaryUntouched(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingKata, 1, "")
	err := f.run(LaunchPlan{Subcommand: "update"})
	if err == nil || exitStatus(err) != 1 || !strings.Contains(err.Error(), "this machine's daemon is v0.19.0: upgrade kata, then run tern-kata update again") || !strings.Contains(err.Error(), "nothing was installed") {
		t.Errorf("error %v (exit %d)", err, exitStatus(err))
	}
	if !strings.Contains(f.out.String(), "warning: a warning\n") || strings.Contains(f.out.String(), "Updated") {
		t.Errorf("output:\n%s", f.out.String())
	}
	f.unchanged(t)
}

func TestUpdateForceSwapsPastARefusalAndPrintsItAsAWarning(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingKata, 1, "")
	outcome, err := updateRelease(context.Background(), f.env, true, &f.out)
	if err != nil || outcome.ForcedConfig {
		t.Fatalf("err %v, outcome %+v", err, outcome)
	}
	for _, want := range []string{"warning: a warning\n", "warning: tern-kata v0.2.0 needs kata v0.20.0 or newer", "Updated tern-kata from v0.1.0 to v0.2.0"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, f.out.String())
		}
	}
	if got, _ := os.ReadFile(f.target); string(got) == oldBinaryText {
		t.Error("--force did not swap the binary")
	}
}

func TestUpdateRemembersAForcedConfigRefusalForTheWorkerStep(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingConfig, 1, "")
	if _, err := updateRelease(context.Background(), f.env, false, &f.out); err == nil {
		t.Fatal("a config refusal did not stop the update")
	}
	f.unchanged(t)
	outcome, err := updateRelease(context.Background(), f.env, true, &f.out)
	if err != nil || !outcome.ForcedConfig {
		t.Errorf("forced: err %v, outcome %+v", err, outcome)
	}

	f = newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingKata, 1, "")
	if outcome, err := updateRelease(context.Background(), f.env, true, &f.out); err != nil || outcome.ForcedConfig {
		t.Errorf("a kata refusal forced past: err %v, outcome %+v", err, outcome)
	}
}

func TestUpdatePrintsPreflightWarningsAndSwaps(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, `{"refusals":[],"warnings":["cannot check this machine's kata daemon: no daemon"]}`, 0, "")
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), "warning: cannot check this machine's kata daemon: no daemon\n") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestUpdateRefusesWhenTheNewBinaryCannotCheckThisMachine(t *testing.T) {
	for name, tc := range map[string]struct {
		stdout, stderr string
		status         int
		want           string
	}{
		"exit status 2":      {`{"refusals":[],"warnings":[]}`, "boom\nfirst failure\nlast words\n", 2, "the new tern-kata could not check this machine: last words"},
		"bad JSON":           {"not json", "it printed junk", 0, "the new tern-kata could not check this machine: it printed junk"},
		"no output":          {"", "", 0, "the new tern-kata could not check this machine: its answer was not readable"},
		"unknown kind":       {`{"refusals":[{"kind":"disk","message":"full"}],"warnings":[]}`, "", 1, `the new tern-kata could not check this machine: its answer has a refusal of unknown kind "disk"`},
		"status without why": {`{"refusals":[],"warnings":[]}`, "", 1, "the new tern-kata could not check this machine: its exit status and its answer disagree"},
	} {
		f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
		f.preflight(t, tc.stdout, tc.status, tc.stderr)
		err := f.run(LaunchPlan{Subcommand: "update"})
		if err == nil || exitStatus(err) != 1 || !strings.HasPrefix(err.Error(), tc.want+"\n") {
			t.Errorf("%s: error %v (exit %d), want %q", name, err, exitStatus(err), tc.want)
		}
		f.unchanged(t)

		f = newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
		f.preflight(t, tc.stdout, tc.status, tc.stderr)
		if err := f.run(LaunchPlan{Subcommand: "update", Force: true}); err != nil || !strings.Contains(f.out.String(), "warning: "+tc.want+"\n") {
			t.Errorf("%s forced: error %v, output:\n%s", name, err, f.out.String())
		}
	}
}

func TestUpdateRefusesWhenTheNewBinaryTakesTooLong(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	t.Setenv("TERN_KATA_TEST_PREFLIGHT_HANG", "1")
	f.env.PreflightTimeout = 500 * time.Millisecond
	start := time.Now()
	err := f.run(LaunchPlan{Subcommand: "update"})
	if err == nil || !strings.HasPrefix(err.Error(), "the new tern-kata could not check this machine: it took longer than 500ms") {
		t.Errorf("error %v", err)
	}
	if time.Since(start) > 20*time.Second {
		t.Error("the update waited for the hung binary")
	}
	f.unchanged(t)
}
