package worker

import (
	"context"
	"os/exec"
	"slices"
	"sync"
	"testing"
)

func newSessionTestWorker(t *testing.T, hooks Hooks) (*Worker, *Journal) {
	t.Helper()
	w := &Worker{clock: SystemClock{}, store: NewStore(Dirs{Config: t.TempDir(), State: t.TempDir()}, hooks), cancels: map[string]*cancelSignal{}}
	j := &Journal{Version: 1, RunUID: "run", Phase: PhaseAdmitted, TimeoutSeconds: 30}
	if err := w.store.Create(j, "admitted"); err != nil {
		t.Fatal(err)
	}
	return w, j
}

// omp may print its session line before the start record is saved. The line's own save finds no launch record and is
// skipped; supervise saves the held id right after the start record, so the "session" save point follows it. One named
// after the record is saved by its own update.
func TestTheStartRecordTakesASessionNamedBeforeItWasSaved(t *testing.T) {
	var mu sync.Mutex
	var points []string
	w, j := newSessionTestWorker(t, Hooks{AfterSave: func(point string, _ *Journal) error {
		mu.Lock()
		defer mu.Unlock()
		points = append(points, point)
		return nil
	}})
	session := &promptSession{dir: t.TempDir(), resumeID: "asked-for"}
	w.recordSession(session, j.RunUID, "named-early")
	mu.Lock()
	if slices.Contains(points, "session") {
		t.Fatalf("a session named before the start record was saved a journal: %v", points)
	}
	mu.Unlock()
	cmd := exec.Command("sh", "-c", "exit 0")
	isolate(cmd)
	if _, err := w.supervise(context.Background(), j, cmd, session); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	order := slices.Clone(points)
	mu.Unlock()
	if started, session := slices.Index(order, "started"), slices.Index(order, "session"); started < 0 || session < started {
		t.Fatalf("save points %v, want session after started", order)
	}
	saved, err := w.store.Load(j.RunUID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Launch == nil || saved.Launch.SessionID != "named-early" || saved.Launch.SessionDir != session.dir {
		t.Fatalf("start record %+v, want session named-early in %s", saved.Launch, session.dir)
	}
	w.recordSession(session, j.RunUID, "named-late")
	if saved, err := w.store.Load(j.RunUID); err != nil || saved.Launch == nil || saved.Launch.SessionID != "named-late" {
		t.Fatalf("start record %+v, err %v, want the session named after it", saved.Launch, err)
	}
}

// A start save that fails leaves the journal `launching` with no launch record. A session line that arrives after it
// (the agent's output is still being copied while the group is killed) must neither crash nor write.
func TestASessionLineAfterAFailedStartSaveSavesNothing(t *testing.T) {
	var mu sync.Mutex
	var points []string
	var w *Worker
	w, j := newSessionTestWorker(t, Hooks{AfterSave: func(point string, j *Journal) error {
		mu.Lock()
		points = append(points, point)
		mu.Unlock()
		if point == "launching" { // the journal leaves `launching` under the start save, which then fails
			j.Phase = PhaseFinished
			return w.store.write(j, "moved-on")
		}
		return nil
	}})
	session := &promptSession{dir: t.TempDir()}
	cmd := exec.Command("sh", "-c", "sleep 30")
	isolate(cmd)
	if _, err := w.supervise(context.Background(), j, cmd, session); err == nil {
		t.Fatal("a start record over a journal that is not launching was saved")
	}
	w.recordSession(session, j.RunUID, "too-late")
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(points, "session") {
		t.Fatalf("a session line with no start record saved a journal: %v", points)
	}
	if saved, err := w.store.Load(j.RunUID); err != nil || saved.Launch != nil {
		t.Fatalf("journal launch %+v, err %v, want no launch record", saved.Launch, err)
	}
}

func TestAStartRecordKeepsTheSessionAResumeAsksForWhenNoneIsNamed(t *testing.T) {
	w, j := newSessionTestWorker(t, Hooks{})
	cmd := exec.Command("sh", "-c", "exit 0")
	isolate(cmd)
	if _, err := w.supervise(context.Background(), j, cmd, &promptSession{resumeID: "asked-for"}); err != nil {
		t.Fatal(err)
	}
	if saved, err := w.store.Load(j.RunUID); err != nil || saved.Launch == nil || saved.Launch.SessionID != "asked-for" {
		t.Fatalf("start record %+v, err %v", saved.Launch, err)
	}
}
