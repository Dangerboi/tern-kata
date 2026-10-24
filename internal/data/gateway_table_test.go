package data

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestALoadThatFindsNoProjectStartsNoPointerMoveAndReadsNoDefaultFilter(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()

	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace, Actor: "tester"}))
	if loaded.Project.UID != "" || loaded.DefaultErr != nil || loaded.Default != (model.Filters{}) {
		t.Fatalf("loaded %+v, want no Project and no Default filter", loaded)
	}
	gateway.background.Wait()
	gateway.startMu.Lock()
	started := len(gateway.convertStarted)
	gateway.startMu.Unlock()
	if started != 0 {
		t.Fatalf("%d pointer moves started for a folder with no Project", started)
	}
}
