package data

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// boundFolder is a folder holding a .kata.toml that binds the Project name, under its own git root.
func boundFolder(t *testing.T, name string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".kata.toml"), []byte("version = 1\n\n[project]\nname = \""+name+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// setupGateway is a gateway whose worker folders are temporary ones.
func setupGateway(t *testing.T) (*Gateway, worker.Dirs) {
	t.Helper()
	dirs := worker.Dirs{Config: filepath.Join(t.TempDir(), "config"), State: t.TempDir()}
	if err := os.MkdirAll(dirs.Config, 0o755); err != nil {
		t.Fatal(err)
	}
	return workerGateway(dirs), dirs
}

func writeWorkerFile(t *testing.T, dirs worker.Dirs, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dirs.Config, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerSetupReadSaysWhatWorkerTomlIsAndWhetherItListsTheProject(t *testing.T) {
	folder := boundFolder(t, "work")
	repo := t.TempDir()
	listed := `executor = "laptop"

[[projects]]
name = "work"
checkouts = ["main"]

[checkouts]
main = '` + repo + `'

[profiles.default]
`
	for _, tt := range []struct {
		name string
		file string // worker.toml, "" for none
		more map[string]string
		want model.WorkerSetup
	}{
		{"no file", "", nil, model.WorkerSetup{}},
		{"file that does not load", "executor = \n", nil, model.WorkerSetup{Exists: true}},
		{"file listing the Project", listed, nil, model.WorkerSetup{Exists: true, Executor: "laptop", Lists: true, Checkouts: []string{repo}}},
		{"file listing another Project", strings.ReplaceAll(listed, `"work"`, `"other"`), nil, model.WorkerSetup{Exists: true, Executor: "laptop"}},
		{"worker.manual beside the file", listed, map[string]string{"worker.manual": ""}, model.WorkerSetup{Exists: true, Executor: "laptop", Lists: true, Checkouts: []string{repo}, Manual: true}},
		{"worker.manual with no file", "", map[string]string{"worker.manual": ""}, model.WorkerSetup{Manual: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g, dirs := setupGateway(t)
			if tt.file != "" {
				writeWorkerFile(t, dirs, "worker.toml", tt.file)
			}
			for name, text := range tt.more {
				writeWorkerFile(t, dirs, name, text)
			}
			got := as[model.WorkerSetupLoaded](t, run(t, g, model.LoadWorkerSetup{Workspace: folder}))
			if got.Err != nil {
				t.Fatal(got.Err)
			}
			unloadable := got.Setup.LoadErr
			if tt.name == "file that does not load" && unloadable == "" {
				t.Fatal("an unloadable file gave no reason")
			}
			got.Setup.LoadErr = ""
			tt.want.Text = tt.file
			tt.want.Project, tt.want.Checkout, tt.want.Path = "work", folder, dirs.ConfigPath()
			if !reflect.DeepEqual(got.Setup, tt.want) {
				t.Fatalf("setup %+v, want %+v", got.Setup, tt.want)
			}
		})
	}
}

func TestWorkerSetupReadOfAFolderBoundToNoProjectNamesNone(t *testing.T) {
	g, dirs := setupGateway(t)
	got := as[model.WorkerSetupLoaded](t, run(t, g, model.LoadWorkerSetup{Workspace: t.TempDir()}))
	if got.Err != nil || got.Setup.Project != "" || got.Setup.Path != dirs.ConfigPath() {
		t.Fatalf("setup %+v err %v", got.Setup, got.Err)
	}
}

func TestSettingUpWritesAWorkerTomlTheWorkerLoadsAndServingAddsTheProject(t *testing.T) {
	g, dirs := setupGateway(t)
	first, second := boundFolder(t, "work"), boundFolder(t, "side")

	setup := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: first, Trusted: true, Executor: "laptop", Actor: "bot", DefaultActor: "dana", Command: "omp", Model: "haiku", Thinking: "low"}))
	if setup.Err != nil || setup.Project != "work" || setup.Key != "work" {
		t.Fatalf("set up: %+v", setup)
	}
	cfg, err := worker.LoadConfig(dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Executor != "laptop" || cfg.Actor != "bot" || cfg.Profiles["default"].Command != "omp" || cfg.Profiles["default"].Model != "haiku" || cfg.Profiles["default"].Thinking != "low" ||
		cfg.Checkouts["work"] != first || len(cfg.Projects) != 1 || cfg.Projects[0].Name != "work" {
		t.Fatalf("config %+v", cfg)
	}
	if info, err := os.Stat(dirs.ConfigPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v err %v, want 0600", info.Mode(), err)
	}

	served := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: second, Serve: true, Trusted: true}))
	if served.Err != nil || served.Project != "side" || !served.Serve {
		t.Fatalf("serve: %+v", served)
	}
	cfg, err = worker.LoadConfig(dirs.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range cfg.Projects {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"work", "side"}) || cfg.Checkouts[served.Key] != second {
		t.Fatalf("projects %v checkouts %v", names, cfg.Checkouts)
	}
}

func TestSavingWorkerSetupRefusesWithoutTrustAnExistingFileAnUnboundFolderAndABadExecutor(t *testing.T) {
	g, dirs := setupGateway(t)
	folder := boundFolder(t, "work")
	if got := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: folder, Executor: "laptop"})); got.Err == nil || !strings.Contains(got.Err.Error(), "Trust") {
		t.Errorf("without trust: %v", got.Err)
	}
	if _, err := os.Stat(dirs.ConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused save wrote worker.toml: %v", err)
	}
	if got := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: t.TempDir(), Trusted: true, Executor: "laptop"})); got.Err == nil || !strings.Contains(got.Err.Error(), "not bound") {
		t.Errorf("unbound folder: %v", got.Err)
	}
	if got := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: folder, Trusted: true, Executor: "two words"})); got.Err == nil || !strings.Contains(got.Err.Error(), "executor") {
		t.Errorf("bad executor: %v", got.Err)
	}
	if _, err := os.Stat(dirs.ConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused write left worker.toml behind: %v", err)
	}
	ok := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: folder, Trusted: true, Executor: "laptop"}))
	if ok.Err != nil {
		t.Fatal(ok.Err)
	}
	if again := as[model.WorkerSetupSaved](t, run(t, g, model.SaveWorkerSetup{Workspace: folder, Trusted: true, Executor: "laptop"})); !errors.Is(again.Err, worker.ErrConfigExists) {
		t.Errorf("set up over a file: %v", again.Err)
	}
}

func TestStopRequestCarriesTheModeAndTheWorkersAnswer(t *testing.T) {
	dirs, seen := fakeWorker(t, map[string]worker.Response{"stop": {OK: true, Stop: &worker.StopAnswer{Mode: "idle", InFlight: 2, Restarts: true}}})
	got := as[model.WorkerStopped](t, run(t, workerGateway(dirs), model.StopWorker{Mode: "idle"}))
	if got.Err != nil || got.Mode != "idle" || got.InFlight != 2 || !got.Restarts || got.Gone {
		t.Fatalf("stopped %+v", got)
	}
	if len(*seen) != 1 || (*seen)[0].Op != "stop" || (*seen)[0].Mode != "idle" {
		t.Fatalf("requests %+v", *seen)
	}
	gone := as[model.WorkerStopped](t, run(t, workerGateway(worker.Dirs{Config: "/nonexistent", State: "/nonexistent"}), model.StopWorker{Mode: "now"}))
	if !gone.Gone || gone.Err != nil {
		t.Fatalf("no worker: %+v", gone)
	}
	refused, _ := fakeWorker(t, map[string]worker.Response{"stop": {Error: "unknown stop mode"}})
	if got := as[model.WorkerStopped](t, run(t, workerGateway(refused), model.StopWorker{Mode: "soon"})); got.Err == nil {
		t.Fatalf("a refused stop reported none: %+v", got)
	}
}

func TestEditedWorkerTomlIsWrittenOnlyWhenItLoads(t *testing.T) {
	g, dirs := setupGateway(t)
	writeWorkerFile(t, dirs, "worker.toml", "executor = \"laptop\"\n\n[profiles.default]\n")
	before, _ := os.ReadFile(dirs.ConfigPath())

	refused := as[model.WorkerEdited](t, run(t, g, model.SaveWorkerEdit{Text: "executor = \"laptop\"\nbogus = 1\n"}))
	if refused.Err == nil || !strings.Contains(refused.Err.Error(), "unknown key bogus") || strings.Contains(refused.Err.Error(), "/tmp") {
		t.Fatalf("refusal %v, want the problem named with the file called worker.toml", refused.Err)
	}
	if after, _ := os.ReadFile(dirs.ConfigPath()); string(after) != string(before) {
		t.Errorf("a refused edit changed the file:\n%s", after)
	}

	text := "executor = \"renamed\"\n\n[profiles.default]\n"
	if saved := as[model.WorkerEdited](t, run(t, g, model.SaveWorkerEdit{Text: text})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if after, _ := os.ReadFile(dirs.ConfigPath()); string(after) != text {
		t.Errorf("file:\n%s", after)
	}
}
