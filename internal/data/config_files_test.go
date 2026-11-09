package data

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	want := []model.ConfigFileKey{model.ConfigLanes, model.ConfigFilters, model.ConfigViews, model.ConfigSeen, model.ConfigWorker, model.ConfigAgent, model.ConfigDocbank, model.ConfigKata}
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
		model.ConfigSeen:    "not created: no Runs marked seen",
		model.ConfigWorker:  "no worker set up",
		model.ConfigAgent:   "agents start with the defaults",
		model.ConfigDocbank: "not set up: `kata docbank setup` sets Docbank up",
		model.ConfigKata:    "empty daemon catalog",
	} {
		row := rows[key]
		if row.Status != model.ConfigMissing || row.Err != "" || row.Summary != want || !filepath.IsAbs(row.Path) {
			t.Errorf("%s: %+v, want a missing file reading %q at an absolute path", key, row, want)
		}
	}
	if rows[model.ConfigLanes].Path != filepath.Join(dirs.Config, "lanes.toml") || rows[model.ConfigWorker].Path != dirs.ConfigPath() ||
		rows[model.ConfigKata].Path != filepath.Join(os.Getenv("KATA_HOME"), "config.toml") || rows[model.ConfigDocbank].Path != rows[model.ConfigKata].Path {
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
		"seen.toml":    "[\"01A\"]\nseen = 2026-11-07T08:00:00Z\ntimeline = true\n\n[\"01B\"]\nseen = 2026-11-06T08:00:00Z\n\n[\"01C\"]\ntimeline = true\n",
		"worker.toml":  servingWork(repo),
		"agent.toml":   "actor = \"me-agent\"\nhost = \"laptop\"\nname = \"builder\"\ncommand = [\"omp\", \"--fast\"]\nmessage_from = [\"dana\", \"dana-laptop\"]\n\n[projects.zeta]\nname = \"z\"\n\n[projects.alpha]\nname = \"a\"\n",
	}, "[[daemon]]\nname = \"home\"\nlocal = true\n\n[tui]\nconfirm_quit = false\n")
	for key, want := range map[model.ConfigFileKey]string{
		model.ConfigLanes:   "1 Project with a Lane list",
		model.ConfigFilters: "1 Project with a default filter",
		model.ConfigViews:   "1 saved view, 1 skipped as invalid",
		model.ConfigSeen:    "2 Projects with a seen mark, 2 with since you last looked on",
		model.ConfigWorker:  "1 Project served · Supervision on",
		model.ConfigAgent:   "actor me-agent · host laptop · name builder · command omp --fast · message from dana, dana-laptop · own table for alpha, zeta",
		model.ConfigDocbank: "not set up: `kata docbank setup` sets Docbank up",
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
	rows, _, _ := configFiles(t, map[string]string{
		"lanes.toml":   "[\"01A\"\n",
		"filters.toml": "[\"01A\"]\nstatus = \n",
		"views.toml":   "[[view]\n",
		"seen.toml":    "[\"01A\"\n",
		"worker.toml":  "executor = \n",
		"agent.toml":   "nme = \"x\"\n",
	}, "[[daemon]]\nname = \"a\"\n")

	for _, key := range []model.ConfigFileKey{model.ConfigLanes, model.ConfigFilters, model.ConfigViews, model.ConfigSeen, model.ConfigWorker, model.ConfigAgent, model.ConfigKata} {
		if row := rows[key]; row.Status != model.ConfigBroken || row.Err == "" {
			t.Errorf("%s: %+v, want a file that does not load with the loader's error", key, row)
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

const docbankServerConfig = "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \"k\"\n"

func docbankRowWith(t *testing.T, kata, docbank string) model.ConfigFile {
	t.Helper()
	docbankHome := t.TempDir()
	t.Setenv("DOCBANK_HOME", docbankHome)
	if docbank != "" {
		if err := os.WriteFile(filepath.Join(docbankHome, "config.toml"), []byte(docbank), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, _ := configFiles(t, nil, kata)
	return rows[model.ConfigDocbank]
}

func TestConfigFilesDocbankRowSaysWhichSetupKatasSectionIs(t *testing.T) {
	for name, tc := range map[string]struct {
		kata, docbank string
		status        model.ConfigFileStatus
		summary       string
	}{
		"no config.toml":                        {"", "", model.ConfigMissing, "not set up: `kata docbank setup` sets Docbank up"},
		"no [docbank] section":                  {"[tui]\nconfirm_quit = false\n", docbankServerConfig, model.ConfigLoads, "not set up: `kata docbank setup` sets Docbank up"},
		"local = true":                          {"[docbank]\nlocal = true\n", "", model.ConfigLoads, "the Docbank on this machine: kata starts it when needed and follows its port and key"},
		"a url on another machine":              {"[docbank]\nurl = \"https://docbank.example\"\napi_key_file = \"/k\"\n", docbankServerConfig, model.ConfigLoads, "Docbank at https://docbank.example"},
		"a loopback url on another port":        {"[docbank]\nurl = \"http://127.0.0.1:9090\"\n", docbankServerConfig, model.ConfigLoads, "Docbank at http://127.0.0.1:9090"},
		"a loopback url with no Docbank config": {"[docbank]\nurl = \"http://127.0.0.1:8080\"\n", "", model.ConfigLoads, "Docbank at http://127.0.0.1:8080"},
		"tern-kata's old pinned setup":          {"[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key_file = \"/k\"\nfolder = \"/kata\"\n", docbankServerConfig, model.ConfigLoads, "pinned to http://127.0.0.1:8080 by tern-kata's old setup: kata does not start it"},
		"the old form on localhost":             {"[docbank]\nurl = \"http://localhost:8080\"\n", docbankServerConfig, model.ConfigLoads, "pinned to http://localhost:8080 by tern-kata's old setup: kata does not start it"},
		"the old form on ::1":                   {"[docbank]\nurl = \"http://[::1]:8080\"\n", docbankServerConfig, model.ConfigLoads, "pinned to http://[::1]:8080 by tern-kata's old setup: kata does not start it"},
		"a url beats local = true":              {"[docbank]\nlocal = true\nurl = \"https://docbank.example\"\n", "", model.ConfigLoads, "Docbank at https://docbank.example"},
		"keys kata may add":                     {"[docbank]\nlocal = true\nsome_future_key = 3\n", "", model.ConfigLoads, "the Docbank on this machine: kata starts it when needed and follows its port and key"},
	} {
		row := docbankRowWith(t, tc.kata, tc.docbank)
		if row.Status != tc.status || row.Err != "" || row.Summary != tc.summary {
			t.Errorf("%s: %+v, want status %v and summary %q", name, row, tc.status, tc.summary)
		}
	}
}

func TestConfigFilesDocbankRowReadsTheDefaultDaemonsFileWhateverDaemonThePaneUses(t *testing.T) {
	row := docbankRowWith(t, "[docbank]\nlocal = true\n", "")
	if want := filepath.Join(os.Getenv("KATA_HOME"), "config.toml"); row.Path != want || row.Title == "" {
		t.Errorf("row %+v, want the path %s", row, want)
	}
}

func TestConfigFilesDocbankRowShowsTheReadersErrorWhenKatasFileCannotBeRead(t *testing.T) {
	for name, kata := range map[string]string{"invalid TOML": "[docbank\nlocal = true\n", "a wrong type": "[docbank]\nlocal = \"yes\"\n"} {
		row := docbankRowWith(t, kata, "")
		if row.Status != model.ConfigBroken || row.Err == "" || !strings.Contains(row.Err, "config.toml") || row.Summary != "" {
			t.Errorf("%s: %+v, want a file that does not load with the reader's error", name, row)
		}
	}
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	if err := os.Mkdir(filepath.Join(home, "config.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	g, _ := setupGateway(t)
	got := as[model.ConfigFilesLoaded](t, run(t, g, model.LoadConfigFiles{Request: 1}))
	i := slices.IndexFunc(got.Files, func(f model.ConfigFile) bool { return f.Key == model.ConfigDocbank })
	if i < 0 || got.Files[i].Status != model.ConfigBroken || got.Files[i].Err == "" {
		t.Errorf("an unreadable config.toml gave %+v", got.Files)
	}
}
