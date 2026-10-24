package data

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

func TestALoadThatFindsNoProjectStartsNoPointerMoveAndReadsNoDefaultFilter(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	state := t.TempDir()
	gateway.workerDirs = func() (worker.Dirs, error) { return worker.Dirs{Config: t.TempDir(), State: state}, nil }
	workspace := t.TempDir()

	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace, Actor: "tester"}))
	if loaded.Project.UID != "" || loaded.DefaultErr != nil || loaded.Default != (model.Filters{}) {
		t.Fatalf("loaded %+v, want no Project and no Default filter", loaded)
	}
	gateway.Close() // waits for any pointer move that was started
	var markers []string
	_ = filepath.WalkDir(state, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			markers = append(markers, path)
		}
		return nil
	})
	if len(markers) != 0 {
		t.Fatalf("a folder with no Project left pointer-move records %v", markers)
	}
}
