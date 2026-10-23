package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// configFixture is a Project on in-process kata and a gateway whose config folder is a temporary one. again is a fresh gateway
// of a later Pane: a new process on the same kata and the same config folder.
type configFixture struct {
	gateway   *Gateway
	again     func() *Gateway
	workspace string
	project   model.Project
	folder    string
}

func newConfigFixture(t *testing.T) configFixture {
	t.Helper()
	loc := newLocator(startKata(t, "0.19.0").target())
	gateway := newGateway(t, loc)
	dirs, err := gateway.workerDirs()
	if err != nil {
		t.Fatal(err)
	}
	workspace, bound := project(t, gateway, "work")
	return configFixture{gateway: gateway, workspace: workspace, project: bound, folder: dirs.Config,
		again: func() *Gateway {
			fresh := newGateway(t, loc)
			fresh.workerDirs = gateway.workerDirs
			return fresh
		}}
}

// write puts content in the config folder's file called name; no content leaves no file.
func (f configFixture) write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(f.folder, name)
	if content != "" {
		if err := os.MkdirAll(f.folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func (f configFixture) saveFilters(t *testing.T, uid, name string, filters model.Filters) model.FiltersSaved {
	t.Helper()
	return as[model.FiltersSaved](t, run(t, f.gateway, model.SaveFilters{ProjectUID: uid, ProjectName: name, Default: filters}))
}

func (f configFixture) loadDefault(t *testing.T, gateway *Gateway) model.ProjectLoaded {
	t.Helper()
	return as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: f.workspace}))
}

func TestSavedFiltersComeBackOnAFreshGatewayAndKeepOtherTablesAndUnknownKeys(t *testing.T) {
	f := newConfigFixture(t)
	uid := f.project.UID
	path := f.write(t, "filters.toml", "[\"other\"]\nname = \"elsewhere\"\nstatus = \"closed\"\n\n[\""+uid+"\"]\nname = \"old\"\nowner = \"me\"\nnote = \"mine\"\n")
	want := model.Filters{Status: "open", Label: "bug", Relationship: "child"}
	if saved := f.saveFilters(t, uid, "work", want); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	raw, _ := os.ReadFile(path)
	for _, line := range []string{"[other]", `status = "closed"`, `note = "mine"`} {
		if !strings.Contains(string(raw), line) {
			t.Errorf("file lacks %q:\n%s", line, raw)
		}
	}
	if strings.Contains(string(raw), "owner") {
		t.Errorf("the emptied owner stayed:\n%s", raw)
	}
	loaded := f.loadDefault(t, f.again())
	if loaded.DefaultErr != nil || loaded.Default != want {
		t.Errorf("fresh gateway loaded %#v, %v; want %#v", loaded.Default, loaded.DefaultErr, want)
	}
	if saved := f.saveFilters(t, "other", "elsewhere", model.Filters{Status: "closed"}); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if again := f.loadDefault(t, f.again()); again.Default != want {
		t.Errorf("another Project's save changed this default: %#v", again.Default)
	}
}

func TestAnEmptyDefaultRemovesTheTableUnlessItHoldsUnknownKeys(t *testing.T) {
	f := newConfigFixture(t)
	uid := f.project.UID
	path := f.write(t, "filters.toml", "[\""+uid+"\"]\nname = \"work\"\nstatus = \"open\"\n\n[\"q\"]\nname = \"w\"\nstatus = \"open\"\nnote = \"keep\"\n")
	for _, saveUID := range []string{uid, "q"} {
		if saved := f.saveFilters(t, saveUID, "w", model.Filters{}); saved.Err != nil {
			t.Fatal(saved.Err)
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), uid) || strings.Contains(string(raw), "status") || !strings.Contains(string(raw), "[q]") || !strings.Contains(string(raw), `note = "keep"`) {
		t.Errorf("file:\n%s", raw)
	}
	if loaded := f.loadDefault(t, f.again()); loaded.Default != (model.Filters{}) || loaded.DefaultErr != nil {
		t.Errorf("loaded %#v, %v after the default was emptied", loaded.Default, loaded.DefaultErr)
	}
}

func TestAnUnreadableFiltersFileIsNotOverwrittenAndItsErrorComesBackOnSaveAndLoad(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "filters.toml", "this is [not toml")
	saved := f.saveFilters(t, f.project.UID, "work", model.Filters{Status: "open"})
	if saved.Err == nil || !strings.Contains(saved.Err.Error(), "filters.toml") {
		t.Fatalf("error %v, want one naming the file", saved.Err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "this is [not toml" {
		t.Errorf("file overwritten: %q", raw)
	}
	if loaded := f.loadDefault(t, f.again()); loaded.DefaultErr == nil || !strings.Contains(loaded.DefaultErr.Error(), "filters.toml") {
		t.Errorf("loading an unreadable file gave error %v", loaded.DefaultErr)
	}
}

func TestNoFiltersFileMeansNoDefault(t *testing.T) {
	f := newConfigFixture(t)
	if loaded := f.loadDefault(t, f.gateway); loaded.Default != (model.Filters{}) || loaded.DefaultErr != nil {
		t.Errorf("loaded %#v, %v", loaded.Default, loaded.DefaultErr)
	}
}
