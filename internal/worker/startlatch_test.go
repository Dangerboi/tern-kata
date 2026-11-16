package worker

import (
	"context"
	"os/exec"
	"testing"
)

// omp may print its session line before the start record is saved. The id then waits for that record, which takes it
// (the line's own save would find no launch to put it in); one named after the record is saved by its own update.
func TestTheStartRecordTakesASessionNamedBeforeItWasSaved(t *testing.T) {
	w := &Worker{clock: SystemClock{}, store: NewStore(Dirs{Config: t.TempDir(), State: t.TempDir()}, Hooks{}), cancels: map[string]*cancelSignal{}}
	j := &Journal{Version: 1, RunUID: "run", Phase: PhaseAdmitted, TimeoutSeconds: 30}
	if err := w.store.Create(j, "admitted"); err != nil {
		t.Fatal(err)
	}
	session := &promptSession{dir: t.TempDir(), resumeID: "asked-for"}
	if session.latch.name("named-early") {
		t.Fatal("the start record was not saved yet, but the latch said it was")
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
	if !session.latch.name("named-late") {
		t.Fatal("a session named after the start record was saved was not told to save on its own")
	}
}

func TestAStartRecordKeepsTheSessionAResumeAsksForWhenNoneIsNamed(t *testing.T) {
	w := &Worker{clock: SystemClock{}, store: NewStore(Dirs{Config: t.TempDir(), State: t.TempDir()}, Hooks{}), cancels: map[string]*cancelSignal{}}
	j := &Journal{Version: 1, RunUID: "run", Phase: PhaseAdmitted, TimeoutSeconds: 30}
	if err := w.store.Create(j, "admitted"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", "exit 0")
	isolate(cmd)
	if _, err := w.supervise(context.Background(), j, cmd, &promptSession{resumeID: "asked-for"}); err != nil {
		t.Fatal(err)
	}
	if saved, err := w.store.Load(j.RunUID); err != nil || saved.Launch == nil || saved.Launch.SessionID != "asked-for" {
		t.Fatalf("start record %+v, err %v", saved.Launch, err)
	}
}
