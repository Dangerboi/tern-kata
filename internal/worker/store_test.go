package worker_test

import (
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
