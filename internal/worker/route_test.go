package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

func isCapabilities(r *http.Request) bool { return strings.HasSuffix(r.URL.Path, "/cron/capabilities") }

// withoutCron makes the daemon answer the Project's capabilities as an older kata would.
func (h *harness) withoutCron() (restore func()) {
	return h.kata.Intercept(isCapabilities, func(w http.ResponseWriter, r *http.Request, _ http.Handler) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"event_features": []string{}, "project_uid": h.proj.UID})
	})
}

func TestADaemonThatStopsAdvertisingCronAdmitsNothingOnceTheStreamReconnectsUntilACheckPasses(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)

	restore := h.withoutCron()
	h.kata.Server.CloseClientConnections() // the event stream drops and reconnects
	h.eventually("the reconnect to find the route unconfirmed", func() bool {
		p := r.w.Status().Projects[0]
		return !p.Admitting && strings.Contains(p.Why, "cron_v1")
	})
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.OK || !strings.Contains(resp.Error, "cron_v1") {
		t.Fatalf("run while the route is unconfirmed: ok=%v error=%q", resp.OK, resp.Error)
	}

	restore()
	h.clock.Advance(6 * time.Minute)
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); !resp.OK {
		t.Fatalf("run after the route was confirmed again: %q", resp.Error)
	}
}

func TestAFinishedRunIsNotReportedWhileTheRouteIsUnconfirmedAndIsOnceItIs(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	var restore func()
	var once sync.Once
	r := h.start(worker.Hooks{AfterSave: func(point string, _ *worker.Journal) error {
		if point == "finished" {
			once.Do(func() {
				restore = h.withoutCron()
				h.clock.Advance(10 * time.Second) // a passing check is reused for five seconds
			})
		}
		return nil
	}})
	h.admitting(r)
	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	probed := h.kata.Requests("cron/capabilities")
	h.eventually("the report to ask whether the route holds", func() bool { return h.kata.Requests("cron/capabilities") > probed })
	if run, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, resp.Run.RunUID); err != nil || run.Status != "running" {
		t.Fatalf("kata holds the run as %q (%v) while the route is unconfirmed; the final observation must wait", run.Status, err)
	}

	restore()
	h.clock.Advance(6 * time.Minute)
	if run := h.settled(resp.Run.RunUID); run.Status != "succeeded" {
		t.Fatalf("after the route held again the run is %q", run.Status)
	}
}

func TestAnInstanceDifferentFromTheOneAProjectWasPinnedToAdmitsNothing(t *testing.T) {
	h := newHarness(t)
	first := h.start(worker.Hooks{})
	h.admitting(first)
	first.stop()

	path := filepath.Join(h.dirs.State, "projects", h.proj.UID, "pin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var pinned map[string]any
	if err := json.Unmarshal(raw, &pinned); err != nil {
		t.Fatal(err)
	}
	if pinned["instance_uid"] == "" || pinned["instance_uid"] == nil {
		t.Fatalf("the pin %s records no daemon instance", raw)
	}
	pinned["instance_uid"] = "another-daemon"
	raw, _ = json.Marshal(pinned)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	second := h.start(worker.Hooks{})
	h.eventually("the pin to be compared", func() bool { return strings.Contains(second.w.Status().Projects[0].Why, "daemon instance") })
	if p := second.w.Status().Projects[0]; p.Admitting || !strings.Contains(p.Why, "another-daemon") {
		t.Fatalf("status %+v: a Project pinned to another daemon admits nothing", p)
	}
}

func TestAProjectWhosePinCannotBeSavedIsNotServed(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dirs.State, "projects"), []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	h.eventually("the pin to be refused", func() bool { return strings.Contains(r.w.Status().Projects[0].Why, "pin cannot be saved") })
	if r.w.Status().Projects[0].Admitting {
		t.Fatal("a Project that cannot be pinned admitted")
	}
}

// holdingCalls makes every request match accepts take d to answer, then pass it on.
func holdingCalls(d *workertest.Daemon, match func(*http.Request) bool, hold time.Duration) (restore func()) {
	return d.Intercept(match, func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		select {
		case <-time.After(hold):
			next.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	})
}

func TestAtMostFourKataCallsRunAtOnceAndEachHasItsOwnDeadlineFromItsSlot(t *testing.T) {
	d := workertest.StartKata(t)
	isProjects := func(r *http.Request) bool { return r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects" }
	holdingCalls(d, isProjects, 300*time.Millisecond)
	// Eight calls of 300 ms behind four slots take 600 ms from the first; a deadline counted from the call's start
	// would end the second four at 500 ms.
	k := worker.NewKata(d.Client, worker.WithCallTimeout(500*time.Millisecond))
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := k.Projects(context.Background()); err != nil {
				failed.Add(1)
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if failed.Load() != 0 {
		t.Fatalf("%d calls failed", failed.Load())
	}
	if peak := d.PeakInFlight(); peak > 4 {
		t.Fatalf("%d kata calls were in flight at once, want at most four", peak)
	}
}

func TestAReadThatRunsOutOfTimeIsTriedAgainButAWriteNever(t *testing.T) {
	d := workertest.StartKata(t)
	k := worker.NewKata(d.Client, worker.WithCallTimeout(80*time.Millisecond), worker.WithReadRetryDelays(5*time.Millisecond, 10*time.Millisecond))
	var seen atomic.Int32
	d.Intercept(func(r *http.Request) bool { return r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects" },
		func(w http.ResponseWriter, r *http.Request, next http.Handler) {
			if seen.Add(1) <= 2 {
				<-r.Context().Done() // the first two attempts never answer
				return
			}
			next.ServeHTTP(w, r)
		})
	if _, err := k.Projects(context.Background()); err != nil {
		t.Fatalf("a read whose first two attempts timed out: %v", err)
	}
	if n := seen.Load(); n != 3 {
		t.Fatalf("the read was asked %d times, want three", n)
	}

	d.Intercept(func(r *http.Request) bool {
		return r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/comments")
	},
		func(w http.ResponseWriter, r *http.Request, _ http.Handler) {
			select { // the server notices a closed connection only after the body is read, so do not wait for that
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		})
	p := d.Project(t, "demo")
	if err := k.Comment(context.Background(), p.ID, "robot", "no-such-issue", "key", "text"); err == nil {
		t.Fatal("a comment that never got an answer reported success")
	}
	if n := d.Requests("/comments"); n != 1 {
		t.Fatalf("the comment was sent %d times, want once", n)
	}
}

func TestOnlyTwoProjectsStartAtATime(t *testing.T) {
	h := newHarness(t)
	h.kata.Project(t, "second")
	h.kata.Project(t, "third")
	h.config = strings.Replace(h.config, "[[projects]]\nname = \"demo\"\n", "[[projects]]\nname = \"demo\"\n[[projects]]\nname = \"second\"\n[[projects]]\nname = \"third\"\n", 1)
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		t.Fatal(err)
	}
	var waiting atomic.Int32
	release := make(chan struct{})
	h.kata.Intercept(isCapabilities, func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		waiting.Add(1)
		select {
		case <-release:
			next.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	})
	r := h.start(worker.Hooks{})
	deadline := time.Now().Add(10 * time.Second)
	for waiting.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // a third Project would have asked by now
	if n := waiting.Load(); n != 2 {
		t.Fatalf("%d Projects were resolving at once, want two", n)
	}
	close(release)
	h.eventually("all three Projects to admit", func() bool {
		projects := r.w.Status().Projects
		return len(projects) == 3 && projects[0].Admitting && projects[1].Admitting && projects[2].Admitting
	})
}

func TestAProjectRemovedFromTheConfigGetsItsSavedReportDeliveredAndItsLeaseOwnerCleared(t *testing.T) {
	h := newHarness(t)
	old := h.kata.Project(t, "old")
	job := h.kata.CreateJob(t, old, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	h.config = strings.Replace(h.config, "[[projects]]\nname = \"demo\"\n", "[[projects]]\nname = \"demo\"\n[[projects]]\nname = \"old\"\n", 1)
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newCrash("finished")
	r := h.start(c.hooks())
	h.eventually("both Projects to admit", func() bool {
		p := r.w.Status().Projects
		return len(p) == 2 && p[0].Admitting && p[1].Admitting
	})
	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	uid := c.wait(t)
	r.stop()
	if run, err := h.kata.Kata.ShowRun(context.Background(), old.ID, uid); err != nil || run.Status != "running" {
		t.Fatalf("before the restart kata holds the run as %q (%v)", run.Status, err)
	}

	h.config = strings.Replace(h.config, "[[projects]]\nname = \"old\"\n", "", 1)
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		t.Fatal(err)
	}
	r2 := h.start(worker.Hooks{})
	h.admitting(r2)
	var run worker.Run
	h.eventually("the retired Project's report to be delivered", func() bool {
		var err error
		run, err = h.kata.Kata.ShowRun(context.Background(), old.ID, uid)
		return err == nil && run.Status == "succeeded"
	})
	h.eventually("the retired Project's lease owner to be cleared", func() bool {
		p, err := h.kata.Kata.Project(context.Background(), old.ID)
		lease, _ := p.Metadata["tern_kata.lease"].(map[string]any)
		owner, hasOwner := lease["owner"].(string)
		return err == nil && lease != nil && (!hasOwner || owner == "") && lease["generation"] == float64(1)
	})
}
