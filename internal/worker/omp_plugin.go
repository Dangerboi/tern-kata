package worker

import (
	"encoding/json"
	"maps"
	"os"
	"path"
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
	// Checkouts are folders a run starts in; omp reads their project overrides and the plugins of the project holding them.
	Checkouts []string
	// Lookup reads the environment variables omp's folders depend on (OMP_PROFILE, PI_PROFILE, PI_CONFIG_DIR,
	// PI_CODING_AGENT_DIR, XDG_DATA_HOME); nil reads none.
	Lookup func(key string) (string, bool)
}

// DefaultOmpEnv is the running user's omp layout, honouring PI_CODING_AGENT_DIR as omp does.
func DefaultOmpEnv() OmpEnv {
	home, err := os.UserHomeDir()
	if err != nil {
		return OmpEnv{}
	}
	return OmpEnv{Home: home, Lookup: os.LookupEnv}
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
	root := filepath.Join(e.Home, e.configDirName())
	if profile != "" {
		root = filepath.Join(root, "profiles", profile)
	}
	agent = filepath.Join(root, "agent")
	plugins = filepath.Join(root, "plugins")
	if dir, _ := e.env("PI_CODING_AGENT_DIR"); profile == "" && dir != "" {
		agent = filepath.Clean(dir)
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

// kataOmpPackages are the npm names of kata-plugin's omp extension. omp registers npm and marketplace installs alike
// under the package name, which became @dangerboi/kata-omp; an install from before the rename keeps kata-omp.
var kataOmpPackages = []string{"@dangerboi/kata-omp", "kata-omp"}

const (
	// kataExtensionFile is the extension file kata-plugin ships (kata.ts), which an ambient extensions folder or the
	// settings' extension list can name; kataRepo is the folder it ships in, which a settings path names.
	kataExtensionFile = "kata"
	kataRepo          = "kata-plugin"
)

// kataExtensionNames are the names omp gives kata-plugin's extension when it loads it as a file or folder: the basename
// without extension. kataDisabled matches the settings' disabledExtensions entry for any of them.
var kataExtensionNames = append([]string{kataExtensionFile}, kataPackageBases()...)

var kataDisabled = regexp.MustCompile(`extension-module:(` + strings.Join(quoteAll(kataExtensionNames), "|") + `)([^A-Za-z0-9._-]|$)`)

func kataPackageBases() []string {
	bases := make([]string, len(kataOmpPackages))
	for i, pkg := range kataOmpPackages {
		bases[i] = path.Base(pkg)
	}
	return slices.Compact(bases)
}

func quoteAll(names []string) []string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return quoted
}

// kataPlugin says where kata-plugin's omp extension is installed for an omp run with the given --profile argument
// (argProfile, when hasArg), "" when none of the places omp loads from names it. A plugin counts when omp would load it
// (loadedPackage) and the settings do not disable the extension (omp's addPaths skips a path whose extension-module name
// is in disabledExtensions, installed plugins included); an extension file or settings mention counts likewise. The
// marketplace registry (installed_plugins.json) alone loads nothing, so it is not read.
func (e OmpEnv) kataPlugin(argProfile string, hasArg bool) string {
	if e.Home == "" {
		return ""
	}
	plugins, agent, ok := e.layout(argProfile, hasArg)
	if !ok {
		return ""
	}
	settings := map[string]string{}
	for _, name := range []string{"config.yml", "config.yaml", "settings.json"} {
		if data, err := os.ReadFile(filepath.Join(agent, name)); err == nil {
			if kataDisabled.Match(data) {
				return ""
			}
			settings[filepath.Join(agent, name)] = string(data)
		}
	}
	// A job with no checkout runs in the run folder, so that case is always there beside the configured checkouts. A run
	// in a checkout reads its project overrides, which apply to the user's plugins too, and its project's plugins.
	for _, checkout := range slices.Concat([]string{""}, e.Checkouts) {
		roots := []string{plugins}
		if checkout != "" {
			if root, ok := e.projectRoot(checkout, plugins); ok {
				roots = append(roots, root)
			}
		}
		disabled := e.projectDisabled(checkout)
		for _, root := range roots {
			for _, pkg := range kataOmpPackages {
				if !slices.Contains(disabled, pkg) {
					if found := loadedPackage(root, pkg); found != "" {
						return found
					}
				}
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(agent, "extensions")); err == nil {
		for _, entry := range entries {
			if slices.Contains(kataExtensionNames, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))) {
				return filepath.Join(agent, "extensions", entry.Name())
			}
		}
	}
	mentions := append([]string{kataRepo}, kataOmpPackages...)
	for _, file := range slices.Sorted(maps.Keys(settings)) {
		if slices.ContainsFunc(mentions, func(m string) bool { return strings.Contains(settings[file], m) }) {
			return file
		}
	}
	return ""
}

// loadedPackage is the package.json of pkg under the plugins root when omp's plugin loader (extensibility/plugins/loader.ts
// collectPluginsAtRoot) would load it, "" otherwise: a node_modules folder exists; the name is in the root's package.json
// dependencies or the lock file, and a name only in the lock file is a link when package.json exists; the package has an
// omp or pi manifest; and a lock entry says enabled, which a missing value does not.
func loadedPackage(root, pkg string) string {
	modules := filepath.Join(root, "node_modules")
	var manifest struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	hasManifest := err == nil
	if hasManifest && json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	var lock struct {
		Plugins map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"plugins"`
	}
	if data, err := os.ReadFile(filepath.Join(root, "omp-plugins.lock.json")); err == nil && json.Unmarshal(data, &lock) != nil {
		return ""
	}
	_, inDeps := manifest.Dependencies[pkg]
	entry, inLock := lock.Plugins[pkg]
	if !inDeps && !inLock {
		return ""
	}
	dir := filepath.Join(modules, filepath.FromSlash(pkg))
	if info, err := os.Lstat(dir); hasManifest && !inDeps && (err != nil || info.Mode()&os.ModeSymlink == 0) {
		return ""
	}
	data, err = os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var plugin struct {
		Omp json.RawMessage `json:"omp"`
		Pi  json.RawMessage `json:"pi"`
	}
	if json.Unmarshal(data, &plugin) != nil || (len(plugin.Omp) == 0 || string(plugin.Omp) == "null") && (len(plugin.Pi) == 0 || string(plugin.Pi) == "null") {
		return ""
	}
	if inLock && !entry.Enabled {
		return ""
	}
	return filepath.Join(dir, "package.json")
}

// ompConfigBases are the folders omp reads project config from, in its priority order (config.ts priorityList): its own
// config folder name, then the three it reads for other tools.
func (e OmpEnv) ompConfigBases() []string {
	return []string{e.configDirName(), ".claude", ".codex", ".gemini"}
}

// configDirName is omp's config folder name (getConfigDirName): PI_CONFIG_DIR, else .omp. It names both the user's root
// under Home and a project's.
func (e OmpEnv) configDirName() string {
	if name, _ := e.env("PI_CONFIG_DIR"); name != "" {
		return name
	}
	return ".omp"
}

// projectRoot is the plugins folder omp reads for a run in checkout (resolveActiveProjectRegistryPath): the nearest
// ancestor with a config folder, else the nearest with a .git, below Home and never the user's own config root. ok is
// false when the checkout is in no project or its root is the user's plugins folder.
func (e OmpEnv) projectRoot(checkout, userPlugins string) (root string, ok bool) {
	name := e.configDirName()
	userConfig := []string{filepath.Join(e.Home, name), filepath.Dir(userPlugins)}
	for _, marker := range []string{name, ".git"} {
		for dir := filepath.Clean(checkout); dir != filepath.Clean(e.Home); dir = filepath.Dir(dir) {
			info, err := os.Stat(filepath.Join(dir, marker))
			if err == nil && (marker == ".git" || info.IsDir()) && !slices.Contains(userConfig, filepath.Join(dir, name)) {
				root = filepath.Join(dir, name, "plugins")
				return root, root != userPlugins
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", false
}

// projectDisabled are the plugin names the checkout's plugin-overrides.json disables: omp reads the first readable one of
// its config folders (getConfigDirPaths with user: false, so folders of the checkout itself, not of its ancestors).
func (e OmpEnv) projectDisabled(checkout string) []string {
	if checkout == "" {
		return nil
	}
	for _, dir := range e.ompConfigBases() {
		data, err := os.ReadFile(filepath.Join(checkout, dir, "plugin-overrides.json"))
		if err != nil {
			continue
		}
		var overrides struct {
			Disabled []string `json:"disabled"`
		}
		if json.Unmarshal(data, &overrides) == nil {
			return overrides.Disabled
		}
	}
	return nil
}
