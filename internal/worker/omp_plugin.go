package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// OmpEnv is where omp keeps what the doctor looks at. The zero value means "do not look", so a test never reads the
// real ~/.omp or the real environment; the command sets DefaultOmpEnv.
type OmpEnv struct {
	// Home is the user's home folder: omp's config root lives in <Home>/.omp.
	Home string
	// AgentDir is omp's agent folder when PI_CODING_AGENT_DIR moves it; blank means the config root's own agent folder.
	AgentDir string
	// Checkouts are folders whose own .omp/plugins omp reads for a run started there.
	Checkouts []string
	// Lookup reads the environment variables omp's folders depend on (OMP_PROFILE, PI_PROFILE, PI_CONFIG_DIR,
	// XDG_DATA_HOME); nil reads none.
	Lookup func(key string) (string, bool)
}

// DefaultOmpEnv is the running user's omp layout, honouring PI_CODING_AGENT_DIR as omp does.
func DefaultOmpEnv() OmpEnv {
	home, err := os.UserHomeDir()
	if err != nil {
		return OmpEnv{}
	}
	return OmpEnv{Home: home, AgentDir: os.Getenv("PI_CODING_AGENT_DIR"), Lookup: os.LookupEnv}
}

func (e OmpEnv) env(key string) (string, bool) {
	if e.Lookup == nil {
		return "", false
	}
	return e.Lookup(key)
}

// profileName is omp's rule for a profile name (utils/src/dirs.ts normalizeProfileName), minus the Windows reserved
// device names: ok is false for a name omp refuses, "" is the default profile.
var profileName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func normalizeProfile(name string) (profile string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" || name == "default" {
		return "", true
	}
	return name, profileName.MatchString(name) && !strings.HasSuffix(name, ".")
}

// layout is where omp, started with the profile named by a --profile argument (argProfile, when hasArg) or else by
// OMP_PROFILE / PI_PROFILE, keeps its plugins and its agent folder. ok is false when omp would refuse the profile name.
func (e OmpEnv) layout(argProfile string, hasArg bool) (plugins, agent string, ok bool) {
	name := argProfile
	if !hasArg {
		var set bool
		if name, set = e.env("OMP_PROFILE"); !set {
			name, _ = e.env("PI_PROFILE")
		}
	}
	profile, ok := normalizeProfile(name)
	if !ok {
		return "", "", false
	}
	dir, _ := e.env("PI_CONFIG_DIR")
	if dir == "" {
		dir = ".omp"
	}
	root := filepath.Join(e.Home, dir)
	if profile != "" {
		root = filepath.Join(root, "profiles", profile)
	}
	agent = filepath.Join(root, "agent")
	plugins = filepath.Join(root, "plugins")
	if profile == "" && e.AgentDir != "" {
		agent = filepath.Clean(e.AgentDir)
	}
	// On Linux and macOS omp keeps the plugins under $XDG_DATA_HOME/omp once that folder exists, unless the agent
	// folder was moved; a profile follows only its own folder under it.
	if xdg, _ := e.env("XDG_DATA_HOME"); xdg != "" && (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && agent == filepath.Join(root, "agent") {
		app := filepath.Join(xdg, "omp")
		if profile != "" {
			app = filepath.Join(app, "profiles", profile)
		}
		if info, err := os.Stat(app); err == nil && info.IsDir() {
			plugins = filepath.Join(app, "plugins")
		}
	}
	return plugins, agent, true
}

type pluginScan struct {
	env   OmpEnv
	locks map[string]lockState
}

type lockState struct{ found, disabled bool }

func (e OmpEnv) scan() *pluginScan { return &pluginScan{env: e, locks: map[string]lockState{}} }

// kataPlugin says where kata-plugin's omp extension (npm package kata-omp) is installed for an omp run with the given
// --profile argument (argProfile, when hasArg), "" when none of the places omp loads from names it. It looks at the
// plugins' node_modules and lock file, the ambient extensions folder and the settings' extension list. The
// marketplace registry (installed_plugins.json) alone loads nothing, so it is not read.
func (s *pluginScan) kataPlugin(argProfile string, hasArg bool) string {
	e := s.env
	if e.Home == "" {
		return ""
	}
	plugins, agent, ok := e.layout(argProfile, hasArg)
	if !ok {
		return ""
	}
	roots := []string{plugins}
	for _, dir := range e.Checkouts {
		roots = append(roots, filepath.Join(dir, ".omp", "plugins"))
	}
	for _, root := range slices.Compact(roots) {
		lock := s.lock(root)
		if lock.disabled {
			continue
		}
		if path := filepath.Join(root, "node_modules", "kata-omp", "package.json"); fileExists(path) {
			return path
		}
		if lock.found {
			return filepath.Join(root, "omp-plugins.lock.json")
		}
	}
	if entries, err := os.ReadDir(filepath.Join(agent, "extensions")); err == nil {
		for _, entry := range entries {
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if name == "kata" || name == "kata-omp" {
				return filepath.Join(agent, "extensions", entry.Name())
			}
		}
	}
	for _, name := range []string{"config.yml", "config.yaml", "settings.json"} {
		path := filepath.Join(agent, name)
		if fileMentions(path, "kata-omp") || fileMentions(path, "kata-plugin") {
			return path
		}
	}
	return ""
}

func (s *pluginScan) lock(root string) lockState {
	if st, ok := s.locks[root]; ok {
		return st
	}
	var st lockState
	if data, err := os.ReadFile(filepath.Join(root, "omp-plugins.lock.json")); err == nil {
		var lock struct {
			Plugins map[string]struct {
				Enabled *bool `json:"enabled"`
			} `json:"plugins"`
		}
		if json.Unmarshal(data, &lock) == nil {
			entry, found := lock.Plugins["kata-omp"]
			st = lockState{found, found && entry.Enabled != nil && !*entry.Enabled}
		}
	}
	s.locks[root] = st
	return st
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileMentions(path, text string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), text)
}
