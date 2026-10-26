package worker_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// A resume renews the lease first: after another machine took the Project over, the cached lease still reads held for
// minutes, and the agent must not start on it.
func TestAResumeAfterATakeoverIsRefusedAndStartsNoAgent(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.settled(uid)
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Resumable })
	launched := len(h.launches())

	h.foreignLease("desktop", h.clock.Now(), 99)
	resp := h.ask(worker.Request{Op: "resume", Run: uid})
	if resp.OK || !strings.Contains(resp.Error, "lease is not held") || !strings.Contains(resp.Error, "desktop") {
		t.Fatalf("resume after a takeover: ok=%v error=%q", resp.OK, resp.Error)
	}
	if got := len(h.launches()); got != launched {
		t.Fatalf("%d launches after the refused resume, want %d", got, launched)
	}
	if s, ok := h.runStatus(r, uid); !ok || !s.Resumable {
		t.Fatalf("the refused resume changed the run: %+v", s)
	}
}

// A Project whose first resolve failed gave its start-up place back; it takes one again before it takes the lease and
// recovers, so the limit on Projects starting at once holds. With two places (startingProjects), the Project that is
// already waiting at its lease claim and the one that fails first leave one free place for two retrying Projects.
func TestAProjectThatFailedToResolveHoldsAStartUpPlaceWhileItRecovers(t *testing.T) {
	h := newHarness(t)
	alpha, beta := h.kata.Project(t, "alpha"), h.kata.Project(t, "beta")
	h.config += "[[projects]]\nname = \"alpha\"\n[[projects]]\nname = \"beta\"\n"
	if err := writeConfig(h); err != nil {
		t.Fatal(err)
	}
	var alphaFails, betaFails atomic.Int32
	h.kata.Fail(func(req *http.Request) bool {
		if !isCapabilities(req) {
			return false
		}
		switch {
		case strings.Contains(req.URL.Path, fmt.Sprintf("/projects/%d/", alpha.ID)):
			alphaFails.Add(1)
		case strings.Contains(req.URL.Path, fmt.Sprintf("/projects/%d/", beta.ID)):
			betaFails.Add(1)
		default:
			return false
		}
		return true
	})
	// demo resolves and then waits at its lease claim, holding its place.
	gate := h.kata.Hold(func(req *http.Request) bool {
		return req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/metadata") && !strings.Contains(req.URL.Path, "/issues/")
	})
	arrivals := 0
	drain := func() {
		for {
			select {
			case <-gate.Arrived:
				arrivals++
			default:
				return
			}
		}
	}
	r := h.start(worker.Hooks{})
	h.within("demo to wait at its lease claim while alpha and beta fail to resolve", func() bool {
		drain()
		return arrivals == 1 && alphaFails.Load() > 0 && betaFails.Load() > 0
	})

	h.kata.Heal()
	h.eventually("one retrying Project to reach its lease claim", func() bool { drain(); return arrivals >= 2 })
	for range 20 {
		time.Sleep(20 * time.Millisecond)
		h.clock.Advance(time.Second)
		drain()
	}
	if arrivals != 2 {
		t.Fatalf("%d Projects were at their lease claim at once, want 2: a Project that failed to resolve took no start-up place", arrivals)
	}

	gate.Open()
	h.eventually("every Project to admit", func() bool {
		p := r.w.Status().Projects
		return len(p) == 3 && p[0].Admitting && p[1].Admitting && p[2].Admitting
	})
}
