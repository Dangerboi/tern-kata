package worker_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// A Project whose recovery could not scan the journals admits nothing, because a run it cannot see may hold a slot,
// and recovers on its next pass, without waiting for a lease renewal.
func TestAProjectWhoseFirstRecoveryCouldNotScanAdmitsNothingUntilALaterPassRecovers(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)

	runs := h.dirs.Runs()
	aside := runs + ".aside"
	if err := os.Rename(runs, aside); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runs, nil, 0o600); err != nil { // a file where the folder belongs: listing it fails, it is not absent
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	h.eventually("the Project to refuse for its unrecovered runs", func() bool {
		s := r.w.Status()
		return len(s.Projects) == 1 && !s.Projects[0].Admitting && strings.Contains(s.Projects[0].Why, "not all recovered yet")
	})
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.OK || !strings.Contains(resp.Error, "not all recovered yet") {
		t.Fatalf("a manual run was not refused for the recovery: %+v", resp)
	}
	if s, ok := h.runStatus(r, uid); ok {
		t.Fatalf("the run is visible while the journals cannot be listed: %+v", s)
	}

	if err := os.Remove(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(aside, runs); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(15 * time.Second) // well short of a lease renewal
	h.eventually("the orphaned run to be recovered into a hold", func() bool {
		s, ok := h.runStatus(r, uid)
		return ok && s.Held
	})
	h.admitting(r)
}

func TestStatusTextNamesTheJournalsThatCouldNotBeRead(t *testing.T) {
	text := worker.Status{JournalError: "run abc: bad json"}.String()
	if !strings.Contains(text, "run abc: bad json") {
		t.Fatalf("worker status does not print the journal error:\n%s", text)
	}
	if strings.Contains(worker.Status{}.String(), "could not be read") {
		t.Fatal("worker status names unreadable journals when there are none")
	}
}

// A run whose journal stays unreadable is not quarantined, so the Project admits nothing; both the worker's status and
// doctor must name the run and say what to do about it.
func TestAnUnreadableJournalIsNamedWithTheActionByStatusAndDoctor(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)

	journal := filepath.Join(h.dirs.Runs(), uid, "run.json")
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0o700); err != nil { // a folder where the file belongs: reading it fails, it is not absent
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	var text string
	h.eventually("status to name the unreadable journal", func() bool {
		text = r.w.Status().String()
		return strings.Contains(text, uid)
	})
	for _, want := range []string{"move it out of the runs folder", "recovers on its next pass"} {
		if !strings.Contains(text, want) {
			t.Errorf("worker status lacks %q:\n%s", want, text)
		}
	}

	report := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock, Actor: "x"}).String()
	for _, want := range []string{"run " + uid, "cannot be read", "move the folder out of " + h.dirs.Runs(), "recovers on its next pass"} {
		if !strings.Contains(report, want) {
			t.Errorf("worker doctor lacks %q:\n%s", want, report)
		}
	}
}

// A transient kata failure while a resume renews the lease says nothing about the lease: the cached one stays held, so
// the Project does not pause until its next renewal.
func TestAResumeThatCouldNotReadTheLeaseLeavesTheCachedLeaseHeld(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.settled(uid)
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Resumable })

	project := fmt.Sprintf("/projects/%d", h.proj.ID)
	h.kata.Fail(func(req *http.Request) bool {
		return req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, project)
	})
	resp := h.ask(worker.Request{Op: "resume", Run: uid})
	h.kata.Heal()
	if resp.OK {
		t.Fatalf("resume succeeded although the lease could not be read")
	}
	if s := r.w.Status(); len(s.Projects) != 1 || !s.Projects[0].Admitting {
		t.Fatalf("the Project stopped admitting after a transient lease read failure: %+v", s.Projects)
	}
}
