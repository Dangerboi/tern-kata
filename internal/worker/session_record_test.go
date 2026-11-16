package worker

import (
	"context"
	"os/exec"
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

// omp may print its session line before the start record is saved. The line finds no launch record to update; the start
// record then takes the held id. One named after the record is saved updates it.
func TestTheStartRecordTakesASessionNamedBeforeItWasSaved(t *testing.T) {
	w, j := newSessionTestWorker(t, Hooks{})
	session := &promptSession{dir: t.TempDir(), resumeID: "asked-for"}
	w.recordSession(session, j.RunUID, "named-early")
	if saved, err := w.store.Load(j.RunUID); err != nil || saved.Launch != nil {
		t.Fatalf("a session named before the start record was saved a launch %+v, err %v", saved.Launch, err)
	}
	cmd := exec.Command("sh", "-c", "exit 0")
	isolate(cmd)
	if _, err := w.supervise(context.Background(), j, cmd, session); err != nil {
		t.Fatal(err)
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
	var w *Worker
	w, j := newSessionTestWorker(t, Hooks{AfterSave: func(point string, j *Journal) error {
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
