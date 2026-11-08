package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

var errTestNoConfigDir = errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")

// configFiles is the Config Page's read on a temporary config folder and a temporary KATA_HOME, with the files given by name
// ("config.toml" is kata's, the others tern-kata's). It returns the rows by key.
func configFiles(t *testing.T, tern map[string]string, kata string) (map[model.ConfigFileKey]model.ConfigFile, model.ConfigFilesLoaded, worker.Dirs) {
	t.Helper()
	g, dirs := setupGateway(t)
	for name, text := range tern {
		writeWorkerFile(t, dirs, name, text)
	}
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	if kata != "" {
		writeCatalog(t, kata)
	}
	got := as[model.ConfigFilesLoaded](t, run(t, g, model.LoadConfigFiles{Request: 3}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	rows := map[model.ConfigFileKey]model.ConfigFile{}
	var order []model.ConfigFileKey
	for _, f := range got.Files {
		rows[f.Key] = f
		order = append(order, f.Key)
	}
	want := []model.ConfigFileKey{model.ConfigLanes, model.ConfigFilters, model.ConfigViews, model.ConfigWorker, model.ConfigAgent, model.ConfigKata}
	if strings.Join(keyStrings(order), ",") != strings.Join(keyStrings(want), ",") || got.Request != 3 || got.Folder != dirs.Config {
		t.Fatalf("rows %v, request %d, folder %q; want %v in order and folder %q", order, got.Request, got.Folder, want, dirs.Config)
	}
	return rows, got, dirs
}

func keyStrings(keys []model.ConfigFileKey) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = string(k)
	}
	return out
}

func TestConfigFilesReadsEveryMissingFileWithWhatItsAbsenceImplies(t *testing.T) {
	rows, _, dirs := configFiles(t, nil, "")
	for key, want := range map[model.ConfigFileKey]string{
		model.ConfigLanes:   "not created: every Project uses the default Lanes",
		model.ConfigFilters: "not created: no Project has a default filter",
		model.ConfigViews:   "not created: no saved views",
		model.ConfigWorker:  "no worker set up",
		model.ConfigAgent:   "agents start with the defaults",
		model.ConfigKata:    "empty daemon catalog",
	} {
		row := rows[key]
		if row.Status != model.ConfigMissing || row.Err != "" || row.Summary != want || !filepath.IsAbs(row.Path) {
			t.Errorf("%s: %+v, want a missing file reading %q at an absolute path", key, row, want)
		}
	}
	if rows[model.ConfigLanes].Path != filepath.Join(dirs.Config, "lanes.toml") || rows[model.ConfigWorker].Path != dirs.ConfigPath() ||
		rows[model.ConfigKata].Path != filepath.Join(os.Getenv("KATA_HOME"), "config.toml") {
		t.Errorf("paths: %+v", rows)
	}
}

// servingWork is a worker.toml serving one Project with one checkout.
func servingWork(repo string) string {
	return `executor = "laptop"

[[projects]]
name = "work"
checkouts = ["main"]

[checkouts]
main = '` + repo + `'

[profiles.default]
`
}

func TestConfigFilesSummarisesEveryFileThatLoads(t *testing.T) {
	repo := t.TempDir()
	rows, _, _ := configFiles(t, map[string]string{
		"lanes.toml":   "[\"01A\"]\nname = \"work\"\nlabels = [\"needs-triage\"]\n\n[\"01B\"]\nname = \"empty\"\n",
		"filters.toml": "[\"01A\"]\nstatus = \"open\"\n\n[\"01B\"]\nname = \"none\"\n",
		"views.toml":   "[[view]]\nname = \"Mine\"\nbase = \"today\"\n\n[[view]]\nname = \"Bad\"\nbase = \"nowhere\"\n",
		"worker.toml":  servingWork(repo),
		"agent.toml":   "actor = \"me-agent\"\nname = \"builder\"\ncommand = [\"omp\", \"--fast\"]\n\n[projects.zeta]\nname = \"z\"\n\n[projects.alpha]\nname = \"a\"\n",
	}, "[[daemon]]\nname = \"home\"\nlocal = true\n\n[tui]\nconfirm_quit = false\n")
	for key, want := range map[model.ConfigFileKey]string{
		model.ConfigLanes:   "1 Project with a Lane list",
		model.ConfigFilters: "1 Project with a default filter",
		model.ConfigViews:   "1 saved view, 1 skipped as invalid",
		model.ConfigWorker:  "1 Project served · Supervision on",
		model.ConfigAgent:   "actor me-agent · name builder · command omp --fast · own table for alpha, zeta",
		model.ConfigKata:    "1 daemon · q quits without asking",
	} {
		if row := rows[key]; row.Status != model.ConfigLoads || row.Err != "" || row.Summary != want {
			t.Errorf("%s: %+v, want a file that loads reading %q", key, row, want)
		}
	}
}

func TestConfigFilesSaysWhenWorkerManualTurnedSupervisionOff(t *testing.T) {
	rows, _, _ := configFiles(t, map[string]string{"worker.toml": servingWork(t.TempDir()), worker.ManualFileName: ""}, "")
	if row := rows[model.ConfigWorker]; row.Status != model.ConfigLoads || row.Summary != "1 Project served · Supervision off (worker.manual)" {
		t.Errorf("worker row %+v", row)
	}
}

func TestConfigFilesShowsEachBrokenFilesLoaderErrorOnItsOwnRowAndTheOthersStillRead(t *testing.T) {
	rows, _, dirs := configFiles(t, map[string]string{
		"lanes.toml":   "[\"01A\"\n",
		"filters.toml": "[\"01A\"]\nstatus = \n",
		"views.toml":   "[[view]\n",
		"worker.toml":  "executor = \n",
		"agent.toml":   "nme = \"x\"\n",
	}, "[[daemon]]\nname = \"a\"\n")

	wantErr := map[model.ConfigFileKey]error{}
	_, wantErr[model.ConfigLanes] = readLanes(filepath.Join(dirs.Config, "lanes.toml"))
	_, wantErr[model.ConfigFilters] = readTables[filtersFile](filepath.Join(dirs.Config, "filters.toml"))
	_, wantErr[model.ConfigViews] = NewViewsStore(dirs.Config).Read()
	_, wantErr[model.ConfigWorker] = worker.LoadConfig(dirs.ConfigPath())
	_, wantErr[model.ConfigAgent] = ReadAgentConfig(filepath.Join(dirs.Config, AgentConfigFile))
	_, wantErr[model.ConfigKata] = ReadCatalogAt(filepath.Dir(rows[model.ConfigKata].Path))
	for key, err := range wantErr {
		if err == nil {
			t.Fatalf("%s: the loader accepts the broken file", key)
		}
		if row := rows[key]; row.Status != model.ConfigBroken || row.Err != err.Error() {
			t.Errorf("%s: %+v, want a file that does not load with %q", key, row, err)
		}
	}
	if rows[model.ConfigWorker].Summary != "Supervision on" {
		t.Errorf("a worker.toml that does not load still reports supervision: %q", rows[model.ConfigWorker].Summary)
	}
}

func TestConfigFilesOneBrokenFileLeavesTheOthersLoading(t *testing.T) {
	rows, _, _ := configFiles(t, map[string]string{"lanes.toml": "[\"01A\"\n", "agent.toml": "name = \"builder\"\n"}, "")
	if rows[model.ConfigLanes].Status != model.ConfigBroken || rows[model.ConfigAgent].Status != model.ConfigLoads || rows[model.ConfigAgent].Summary != "name builder" {
		t.Errorf("lanes %+v, agent %+v", rows[model.ConfigLanes], rows[model.ConfigAgent])
	}
}

func TestConfigFilesAnswersAnErrorAndNoRowsWhenTheConfigFolderCannotBeFound(t *testing.T) {
	g := NewGateway(nil)
	g.workerDirs = func() (worker.Dirs, error) { return worker.Dirs{}, errTestNoConfigDir }
	got := as[model.ConfigFilesLoaded](t, run(t, g, model.LoadConfigFiles{Request: 5}))
	if got.Err == nil || len(got.Files) != 0 || got.Request != 5 {
		t.Errorf("answer %+v, want the folder error, no rows and the request number", got)
	}
}
