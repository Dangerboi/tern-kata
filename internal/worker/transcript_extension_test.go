package worker_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data/ompext"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// A run of omp loads tern-kata's omp extension from the run's folder, so the agent's `kata close` names the run's session.
func TestAnOmpRunLoadsTernKatasExtensionFromItsRunFolder(t *testing.T) {
	h := newHarness(t)
	omp := filepath.Join(t.TempDir(), "omp")
	if runtime.GOOS == "windows" {
		omp += ".exe"
	}
	stub, err := os.ReadFile(h.stub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(omp, stub, 0o700); err != nil {
		t.Fatal(err)
	}
	h.config = strings.Replace(h.config, quote(h.stub), quote(omp), 1)
	h.dirs = workertest.WriteConfig(t, h.config)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Review it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the job to be known", func() bool { return r.w.Status().Projects[0].AdapterJobs == 1 })

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatalf("run: %q", resp.Error)
	}
	h.settled(resp.Run.RunUID)
	launches := h.launches()
	if len(launches) != 1 {
		t.Fatalf("launches %v, want one", launches)
	}
	var args []string
	for _, a := range launches[0]["args"].([]any) {
		args = append(args, a.(string))
	}
	i := slices.Index(args, "-e")
	if i < 0 || i+1 >= len(args) || filepath.Base(args[i+1]) != ompext.FileName || !filepath.IsAbs(args[i+1]) {
		t.Fatalf("args %v: want -e <absolute path to %s>", args, ompext.FileName)
	}
	if held, err := os.ReadFile(args[i+1]); err != nil || !bytes.Equal(held, ompext.Source) {
		t.Fatalf("the extension file %s holds %d bytes, %v; want the embedded %d", args[i+1], len(held), err, len(ompext.Source))
	}
}
