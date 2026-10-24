package data

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

func (f configFixture) loadLanes(t *testing.T, gateway *Gateway, uid string) model.LanesLoaded {
	t.Helper()
	return as[model.LanesLoaded](t, run(t, gateway, model.LoadLanes{ProjectUID: uid}))
}

func TestSavedLanesAndDoneLimitsComeBackOnAFreshGateway(t *testing.T) {
	f := newConfigFixture(t)
	saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"needs-triage", "in-review"}, DoneDays: 14, DoneCards: 20}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	got := f.loadLanes(t, f.again(), "p")
	if got.Err != nil || !slices.Equal(got.Lanes, []string{"needs-triage", "in-review"}) || got.DoneDays != 14 || got.DoneCards != 20 {
		t.Errorf("loaded %+v", got)
	}
	if none := f.loadLanes(t, f.again(), "unsaved"); none.Err != nil || len(none.Lanes) != 0 {
		t.Errorf("a Project with no list loaded %+v", none)
	}
}

func TestLoadedLaneLabelsAreNormalisedAndRepeatedOrEmptyOnesDropped(t *testing.T) {
	f := newConfigFixture(t)
	f.write(t, "lanes.toml", "[\"p\"]\nname = \"work\"\nlabels = [\"  Needs-Triage \", \"needs-triage\", \"\", \"in-review\"]\n")
	got := f.loadLanes(t, f.gateway, "p")
	if got.Err != nil || !slices.Equal(got.Lanes, []string{"needs-triage", "in-review"}) {
		t.Errorf("loaded %+v", got)
	}
}

func TestSavingLanesKeepsOtherTablesAndUnknownKeys(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "[\"other\"]\nname = \"elsewhere\"\nlabels = [\"a\"]\n\n[\"p\"]\nname = \"old\"\nnote = \"mine\"\n")
	if saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"b"}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"[other]", `name = "elsewhere"`, `note = "mine"`, `name = "work"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file lacks %q:\n%s", want, raw)
		}
	}
	if other := f.loadLanes(t, f.again(), "other"); !slices.Equal(other.Lanes, []string{"a"}) {
		t.Errorf("the other Project's list changed: %+v", other)
	}
}

func TestAnAllDefaultLanesSaveRemovesAnOtherwiseEmptyTable(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "[\"p\"]\nname = \"work\"\nlabels = [\"a\"]\ndone_days = 3\n\n[\"q\"]\nname = \"w\"\nlabels = [\"a\"]\nnote = \"keep\"\n")
	for _, uid := range []string{"p", "q"} {
		if saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: uid, ProjectName: "w"})); saved.Err != nil {
			t.Fatal(saved.Err)
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "[p]") || strings.Contains(string(raw), "labels") || !strings.Contains(string(raw), "[q]") || !strings.Contains(string(raw), `note = "keep"`) {
		t.Errorf("file:\n%s", raw)
	}
}

func TestAnUnreadableLanesFileIsRefusedAndNotOverwritten(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "this is [not toml")
	saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"a"}}))
	if saved.Err == nil || !strings.Contains(saved.Err.Error(), "lanes.toml") {
		t.Fatalf("error %v, want one naming the file", saved.Err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "this is [not toml" {
		t.Errorf("file overwritten: %q", raw)
	}
	if got := f.loadLanes(t, f.again(), "p"); got.Err == nil {
		t.Error("loading an unreadable file gave no error")
	}
}

func TestTheConfigFolderIsFoundWithoutTheStateFolder(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	folder, err := worker.ConfigDir()
	if err != nil || !strings.HasPrefix(folder, config) {
		t.Fatalf("config folder %q, %v", folder, err)
	}
	dirs, err := worker.DefaultDirs()
	if err == nil || dirs.Config != folder {
		t.Fatalf("DefaultDirs gave %+v, %v; want the config folder and the state folder's error", dirs, err)
	}
	g := NewGateway(newLocator(Target{}).locate)
	t.Cleanup(g.Close)
	if saved := as[model.LanesSaved](t, run(t, g, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"a"}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if got := f2(t, g); !slices.Equal(got.Lanes, []string{"a"}) {
		t.Errorf("loaded %+v", got)
	}
}

func f2(t *testing.T, g *Gateway) model.LanesLoaded {
	t.Helper()
	return as[model.LanesLoaded](t, run(t, g, model.LoadLanes{ProjectUID: "p"}))
}

func TestASaveThatCannotCompleteLeavesTheLaneListAndDefaultFilterFilesAsTheyWere(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a folder this user cannot write to")
	}
	f := newConfigFixture(t)
	lanes := f.write(t, "lanes.toml", "[\"p\"]\nname = \"old\"\nlabels = [\"a\"]\n")
	filters := f.write(t, "filters.toml", "[\"p\"]\nname = \"old\"\nstatus = \"open\"\n")
	if err := os.Chmod(f.folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.folder, 0o755) })
	savedLanes := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "new", Lanes: []string{"b"}}))
	savedFilters := f.saveFilters(t, "p", "new", model.Filters{Status: "closed"})
	if savedLanes.Err == nil || savedFilters.Err == nil {
		t.Fatalf("errors: lanes %v, filters %v", savedLanes.Err, savedFilters.Err)
	}
	for path, want := range map[string]string{lanes: "[\"p\"]\nname = \"old\"\nlabels = [\"a\"]\n", filters: "[\"p\"]\nname = \"old\"\nstatus = \"open\"\n"} {
		if raw, _ := os.ReadFile(path); string(raw) != want {
			t.Errorf("%s holds %q", path, raw)
		}
	}
	if left, _ := os.ReadDir(f.folder); len(left) != 2 {
		t.Errorf("stray files %v", left)
	}
}
