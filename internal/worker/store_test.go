package worker_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// A journal read while another writer replaces it is read whole, so a scan never takes a healthy journal for a damaged
// one and quarantines it. On Windows a plain read and a replace refuse each other.
func TestAJournalReadWhileItIsReplacedIsNeverQuarantined(t *testing.T) {
	store := worker.NewStore(worker.Dirs{Config: t.TempDir(), State: t.TempDir()}, worker.Hooks{})
	const uid = "01M5JESXAK1NBYS6N2ZD9RTM9J"
	if err := store.Create(&worker.Journal{Version: 1, RunUID: uid}, "prepared"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 300 {
			if _, err := store.Update(uid, "replaced", func(j *worker.Journal) error { j.Attempt = i; return nil }); err != nil {
				t.Errorf("replace %d: %v", i, err)
				return
			}
		}
	}()
	for {
		select {
		case <-done:
			return
		default:
		}
		journals, quarantined, err := store.Scan()
		if err != nil || len(quarantined) > 0 || len(journals) != 1 {
			t.Fatalf("scan read %d journals, quarantined %v: %v", len(journals), quarantined, err)
		}
	}
}

// A journal that cannot be read (here run.json is a folder) is not damaged: Scan fails and leaves the run in place,
// while one that reads but is not valid goes to quarantine.
func TestScanQuarantinesOnlyDamagedJournals(t *testing.T) {
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	store := worker.NewStore(dirs, worker.Hooks{})
	const unreadable, badJSON, badVersion = "01M5JESXAK1NBYS6N2ZD9RTM9A", "01M5JESXAK1NBYS6N2ZD9RTM9B", "01M5JESXAK1NBYS6N2ZD9RTM9C"
	write := func(uid, body string) {
		if err := os.MkdirAll(store.RunDir(uid), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store.RunDir(uid), "run.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(store.RunDir(unreadable), "run.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	const healthy = "01M5JESXAK1NBYS6N2ZD9RTM9D"
	if err := store.Create(&worker.Journal{Version: 1, RunUID: healthy}, "prepared"); err != nil {
		t.Fatal(err)
	}
	journals, _, err := store.Scan()
	if err == nil || !strings.Contains(err.Error(), unreadable) {
		t.Fatalf("scan of an unreadable journal must name the run in its error, got %v", err)
	}
	if len(journals) != 1 || journals[0].RunUID != healthy {
		t.Fatalf("scan must still return the readable journals, got %d", len(journals))
	}
	if _, err := os.Stat(store.RunDir(unreadable)); err != nil {
		t.Fatalf("an unreadable journal must stay in place: %v", err)
	}
	if q := store.Quarantine(); len(q) != 0 {
		t.Fatalf("quarantined %v", q)
	}

	if err := os.RemoveAll(store.RunDir(unreadable)); err != nil {
		t.Fatal(err)
	}
	write(badJSON, "{not json")
	write(badVersion, `{"version":2,"run_uid":"`+badVersion+`"}`)
	_, quarantined, err := store.Scan()
	slices.Sort(quarantined)
	if err != nil || !slices.Equal(quarantined, []string{badJSON, badVersion}) {
		t.Fatalf("quarantined %v, err %v", quarantined, err)
	}
}

// The overlap index answers from memory: a run that is not settled closes its job's gate, settling it opens the gate,
// and a new store learns the open runs from the journals it seeds. The runs folder is gone when the index is asked,
// so no journal can have been read.
func TestTheOverlapIndexFollowsSavedJournalsWithoutReadingThem(t *testing.T) {
	dirs := worker.Dirs{Config: t.TempDir(), State: t.TempDir()}
	store := worker.NewStore(dirs, worker.Hooks{})
	const uid = "01M5JESXAK1NBYS6N2ZD9RTM9J"
	if err := store.Create(&worker.Journal{Version: 1, RunUID: uid, ProjectUID: "p", JobUID: "j", Phase: worker.PhasePrepared}, "prepared"); err != nil {
		t.Fatal(err)
	}
	journals, _, err := store.Scan()
	if err != nil || len(journals) != 1 {
		t.Fatalf("scan: %d journals, %v", len(journals), err)
	}
	restarted := worker.NewStore(dirs, worker.Hooks{})
	restarted.SeedGates(journals)
	if err := os.RemoveAll(dirs.Runs()); err != nil {
		t.Fatal(err)
	}
	if got := store.ClosingRun("p", "j"); got != uid {
		t.Fatalf("saved unsettled run closes the gate: got %q", got)
	}
	if got := restarted.ClosingRun("p", "j"); got != uid {
		t.Fatalf("seeded unsettled run closes the gate: got %q", got)
	}
	if got := store.ClosingRun("p", "other"); got != "" {
		t.Fatalf("another job's gate is closed by %q", got)
	}
	if err := store.Create(&worker.Journal{Version: 1, RunUID: uid, ProjectUID: "p", JobUID: "j", Phase: worker.PhaseSettled}, "settled"); err != nil {
		t.Fatal(err)
	}
	if got := store.ClosingRun("p", "j"); got != "" {
		t.Fatalf("settled run closes the gate: %q", got)
	}
}
