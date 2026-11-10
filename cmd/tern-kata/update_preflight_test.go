package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	err := runPreflight(context.Background(), preflightEnv{Locate: locate, Dirs: dirs, Version: "v0.9.0"}, false, &out)
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
	err := runPreflight(context.Background(), preflightEnv{Locate: daemonAt(t, "0.23.0"), Dirs: dirs, Version: "v0.9.0"}, false, &out)
	if err != nil || strings.TrimSpace(out.String()) != `{"refusals":[],"warnings":[]}` {
		t.Errorf("got %q, %v", out.String(), err)
	}
}

func TestPreflightRefusesADaemonOlderThanTheNewBinaryNeeds(t *testing.T) {
	answer, status := preflightOf(t, daemonAt(t, "0.19.0"), "")
	want := "tern-kata v0.9.0 needs kata " + worker.MinKataVersion + " or newer; this machine's daemon is v0.19.0: upgrade kata"
	if status != 1 || len(answer.Refusals) != 1 || answer.Refusals[0] != (preflightRefusal{Kind: "kata", Message: want}) || len(answer.Warnings) != 0 {
		t.Errorf("status %d, answer %+v, want a kata refusal %q", status, answer, want)
	}
}

func TestPreflightWarnsOfADaemonWithoutEveryFeatureAndStillExitsZero(t *testing.T) {
	answer, status := preflightOf(t, daemonAt(t, "0.22.1"), "")
	want := "this machine's kata daemon is v0.22.1: attachments and search by meaning need kata v0.23.0 or newer"
	if status != 0 || len(answer.Refusals) != 0 || len(answer.Warnings) != 1 || answer.Warnings[0] != want {
		t.Errorf("status %d, answer %+v, want the warning %q", status, answer, want)
	}
}

func TestPreflightTextPrintsEveryRefusalAndWarningAsAWarningLine(t *testing.T) {
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	if err := os.WriteFile(dirs.ConfigPath(), []byte("no_such_key = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runPreflight(context.Background(), preflightEnv{Locate: daemonAt(t, "0.19.0"), Dirs: dirs, Version: "v0.9.0"}, true, &out)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if exitStatus(err) != 1 || len(lines) != 2 ||
		lines[0] != "warning: tern-kata v0.9.0 needs kata "+worker.MinKataVersion+" or newer; this machine's daemon is v0.19.0: upgrade kata" ||
		!strings.HasPrefix(lines[1], "warning: ") || !strings.Contains(lines[1], "no_such_key") {
		t.Errorf("exit %d, output:\n%s", exitStatus(err), out.String())
	}

	out.Reset()
	err = runPreflight(context.Background(), preflightEnv{Locate: func(context.Context, string) (data.Target, error) {
		return data.Target{}, errors.New("no daemon\nstart the kata daemon")
	}, Dirs: worker.Dirs{Config: t.TempDir(), State: t.TempDir()}, Version: "v0.9.0"}, true, &out)
	want := "warning: cannot check this machine's kata daemon (tern-kata v0.9.0 needs kata " + worker.MinKataVersion + " or newer): no daemon\n"
	if err != nil || out.String() != want {
		t.Errorf("unreachable daemon: %v, output %q, want %q", err, out.String(), want)
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
	answer, status := preflightOf(t, daemonAt(t, "0.21.0"), "no_such_key = 1\n")
	if status != 1 || len(answer.Refusals) != 1 || answer.Refusals[0].Kind != "config" {
		t.Fatalf("status %d, answer %+v", status, answer)
	}
	msg := answer.Refusals[0].Message
	if !strings.Contains(msg, "unknown key no_such_key") || !strings.HasSuffix(msg, "tern-kata v0.9.0's `worker upgrade` or `worker edit` fixes it") {
		t.Errorf("message %q", msg)
	}
}

func TestPreflightAcceptsAWorkerConfigTheNewBinaryCanLoad(t *testing.T) {
	if answer, status := preflightOf(t, daemonAt(t, "0.21.0"), "executor = \"laptop\"\n[profiles.default]\n"); status != 0 || len(answer.Refusals) != 0 {
		t.Errorf("status %d, answer %+v", status, answer)
	}
}

const refusingKata = `{"refusals":[{"kind":"kata","message":"tern-kata v0.2.0 needs kata v0.21.0 or newer; this machine's daemon is v0.19.0: upgrade kata"}],"warnings":["a warning"]}`
const refusingConfig = `{"refusals":[{"kind":"config","message":"worker.toml: unknown key x; tern-kata v0.2.0's ` + "`worker upgrade`" + ` or ` + "`worker edit`" + ` fixes it"}],"warnings":[]}`

func TestUpdateStopsOnAPreflightRefusalWithTheBinaryUntouched(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingKata, 1, "")
	err := f.run(LaunchPlan{Subcommand: "update"})
	if err == nil || exitStatus(err) != 1 || !strings.Contains(err.Error(), "this machine's daemon is v0.19.0: upgrade kata") || !strings.Contains(err.Error(), "nothing was installed") {
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
	if err := f.run(LaunchPlan{Subcommand: "update", Force: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"warning: a warning\n", "warning: tern-kata v0.2.0 needs kata v0.21.0 or newer", "Updated tern-kata from v0.1.0 to v0.2.0"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, f.out.String())
		}
	}
	if got, _ := os.ReadFile(f.target); string(got) == oldBinaryText {
		t.Error("--force did not swap the binary")
	}
}

func TestUpdateStopsOnAConfigRefusal(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.preflight(t, refusingConfig, 1, "")
	if err := f.run(LaunchPlan{Subcommand: "update"}); err == nil {
		t.Fatal("a config refusal did not stop the update")
	}
	f.unchanged(t)
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
	t.Setenv("TERN_KATA_TEST_PREFLIGHT_HANG", filepath.Join(t.TempDir(), "started"))
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

func TestPreflightWarnsOfADaemonThatNeverAnswersWellBeforeTheTimeout(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() { // accepts and never answers
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	c, err := client.New("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	env := preflightEnv{
		Locate:      func(context.Context, string) (data.Target, error) { return data.Target{Client: c}, nil },
		Dirs:        worker.Dirs{Config: t.TempDir(), State: t.TempDir()},
		Version:     "v0.9.0",
		KataTimeout: 300 * time.Millisecond,
	}
	start := time.Now()
	err = runPreflight(context.Background(), env, false, &out)
	var answer preflightAnswer
	if jsonErr := json.Unmarshal(out.Bytes(), &answer); jsonErr != nil {
		t.Fatalf("stdout %q is not JSON: %v", out.String(), jsonErr)
	}
	if err != nil || len(answer.Refusals) != 0 || len(answer.Warnings) != 1 || !strings.Contains(answer.Warnings[0], "did not answer within 300ms") {
		t.Errorf("err %v, answer %+v", err, answer)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the check took %s", time.Since(start))
	}
}

func TestUpdateCancelledDuringPreflightIsInterruptedEvenWithForce(t *testing.T) {
	for _, force := range []bool{false, true} {
		f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
		marker := filepath.Join(t.TempDir(), "started")
		t.Setenv("TERN_KATA_TEST_PREFLIGHT_HANG", marker)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for {
				if _, err := os.Stat(marker); err == nil {
					cancel()
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
		err := updateRelease(ctx, f.env, force, &f.out)
		if err == nil || !strings.Contains(err.Error(), "interrupted") || !strings.Contains(err.Error(), "nothing was installed") || strings.Contains(err.Error(), "longer than") {
			t.Errorf("force %v: error %v", force, err)
		}
		f.unchanged(t)
	}
}

func TestUpdateDoesNotStopTheWorkerAfterForcingPastAPreflightThatCouldNotRun(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.env.Supervisor = worker.SupervisorTern
	w.serve(t)
	f := updateWithWorker(t, w)
	f.preflight(t, "not json", 0, "")
	if err := f.run(LaunchPlan{Subcommand: "update", Force: true}); err != nil {
		t.Fatal(err)
	}
	want := "The worker was not stopped: the new release could not check worker.toml. Check it with `tern-kata worker edit`, then run `tern-kata worker stop`."
	if !strings.Contains(f.out.String(), want) || strings.Contains(f.out.String(), "Stopping") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if !workerAnswers(f) {
		t.Error("the worker was stopped")
	}
}
