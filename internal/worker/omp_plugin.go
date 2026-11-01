package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// OmpEnv is where omp keeps what the doctor looks at. The zero value means "do not look", so a test never reads the
// real ~/.omp; the command sets DefaultOmpEnv.
type OmpEnv struct {
	// Home is the user's home folder: omp's plugins live in <Home>/.omp/plugins.
	Home string
	// AgentDir is omp's agent folder, <Home>/.omp/agent unless PI_CODING_AGENT_DIR moves it.
	AgentDir string
	// Checkouts are folders whose own .omp/plugins/installed_plugins.json omp reads for a run started there.
	Checkouts []string
}

// DefaultOmpEnv is the running user's omp layout, honouring PI_CODING_AGENT_DIR as omp does.
func DefaultOmpEnv() OmpEnv {
	home, err := os.UserHomeDir()
	if err != nil {
		return OmpEnv{}
	}
	agent := os.Getenv("PI_CODING_AGENT_DIR")
	if agent == "" {
		agent = filepath.Join(home, ".omp", "agent")
	}
	return OmpEnv{Home: home, AgentDir: agent}
}

// kataPlugin says where kata-plugin's omp extension (npm package kata-omp, plugin id kata@kata-plugin) is installed
// for omp runs, "" when none of the places omp loads from names it. It looks at omp's plugin registries (the user's
// and each checkout's installed_plugins.json), the plugins' package.json, the ambient extensions folder and the
// settings' extension list.
func (e OmpEnv) kataPlugin() string {
	if e.Home == "" {
		return ""
	}
	roots := []string{filepath.Join(e.Home, ".omp", "plugins")}
	for _, dir := range e.Checkouts {
		roots = append(roots, filepath.Join(dir, ".omp", "plugins"))
	}
	for _, root := range roots {
		if path := filepath.Join(root, "node_modules", "kata-omp", "package.json"); fileExists(path) && !lockDisables(root) {
			return path
		}
		if lockEnables(root) {
			return filepath.Join(root, "omp-plugins.lock.json")
		}
		if path := filepath.Join(root, "installed_plugins.json"); registryHasKata(path) {
			return path
		}
	}
	if e.AgentDir == "" {
		return ""
	}
	if entries, err := os.ReadDir(filepath.Join(e.AgentDir, "extensions")); err == nil {
		for _, entry := range entries {
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if name == "kata" || name == "kata-omp" {
				return filepath.Join(e.AgentDir, "extensions", entry.Name())
			}
		}
	}
	for _, name := range []string{"config.yml", "config.yaml", "settings.json"} {
		path := filepath.Join(e.AgentDir, name)
		if fileMentions(path, "kata-omp") || fileMentions(path, "kata-plugin") {
			return path
		}
	}
	return ""
}

// lockEntry reads root's omp-plugins.lock.json entry for kata-omp: found, and whether it is disabled.
func lockEntry(root string) (found, disabled bool) {
	data, err := os.ReadFile(filepath.Join(root, "omp-plugins.lock.json"))
	if err != nil {
		return false, false
	}
	var lock struct {
		Plugins map[string]struct {
			Enabled *bool `json:"enabled"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &lock) != nil {
		return false, false
	}
	entry, ok := lock.Plugins["kata-omp"]
	return ok, ok && entry.Enabled != nil && !*entry.Enabled
}

// lockEnables is true when the lock lists kata-omp and does not disable it; lockDisables when it disables it.
func lockEnables(root string) bool {
	found, disabled := lockEntry(root)
	return found && !disabled
}

func lockDisables(root string) bool {
	_, disabled := lockEntry(root)
	return disabled
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// registryHasKata is true when an installed_plugins.json has an enabled entry for the plugin named kata (id kata@<marketplace>).
func registryHasKata(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var reg struct {
		Plugins map[string][]struct {
			Enabled *bool `json:"enabled"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &reg) != nil {
		return false
	}
	for id, entries := range reg.Plugins {
		if id != "kata" && !strings.HasPrefix(id, "kata@") {
			continue
		}
		for _, entry := range entries {
			if entry.Enabled == nil || *entry.Enabled {
				return true
			}
		}
	}
	return false
}

func fileMentions(path, text string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), text)
}
