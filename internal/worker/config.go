package worker

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/data/ompext"
)

// Dirs are the two folders the worker uses: Config holds worker.toml beside lanes.toml, State holds the lock, the
// control socket, the pins, cursors and run journals (ADR-0006, ADR-0010). Tests point both at temporary folders.
type Dirs struct {
	Config, State string
}

// ConfigDir is os.UserConfigDir()/tern-kata: the one rule for tern-kata's config folder, shared by worker.toml and the Pane's
// lanes.toml and filters.toml. It needs no state folder. The Luau host half (host.luau) does not repeat it: it asks
// `tern-kata worker supervised`.
func ConfigDir() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "tern-kata"), nil
}

// DefaultDirs is ConfigDir and $XDG_STATE_HOME/tern-kata, else ~/.local/state/tern-kata. When only the state folder cannot
// be found, the error comes back with Config still set, so a caller that needs only the config folder can go on.
func DefaultDirs() (Dirs, error) {
	config, err := ConfigDir()
	if err != nil {
		return Dirs{}, err
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Dirs{Config: config}, err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return Dirs{Config: config, State: filepath.Join(state, "tern-kata")}, nil
}

func (d Dirs) ConfigPath() string { return filepath.Join(d.Config, ConfigFileName) }

// Lock is worker.lock, the one-worker-per-machine lock (ADR-0006).
func (d Dirs) Lock() string { return filepath.Join(d.State, "worker.lock") }

// Socket is the control socket (ADR-0004).
func (d Dirs) Socket() string { return filepath.Join(d.State, "worker.sock") }

func (d Dirs) Runs() string { return filepath.Join(d.State, "runs") }

// Quarantine holds journals that could not be read, whole, under the same uid; a quarantined uid is never prepared again.
func (d Dirs) Quarantine() string { return filepath.Join(d.State, "quarantine", "runs") }

func (d Dirs) ProjectDir(projectUID string) string {
	return filepath.Join(d.State, "projects", projectUID)
}

// Pin is the Project's pin file (see pin); pinFiles lists every Project's.
func (d Dirs) Pin(projectUID string) string {
	return filepath.Join(d.ProjectDir(projectUID), "pin.json")
}

// pinFiles lists the pin files that exist. It reads the folder rather than globbing, since the state path may hold
// glob characters.
func (d Dirs) pinFiles() []string {
	entries, _ := os.ReadDir(filepath.Join(d.State, "projects"))
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			if path := d.Pin(e.Name()); fileExists(path) {
				files = append(files, path)
			}
		}
	}
	return files
}

// Skips is the Project's skip record file: the occurrences its conditions skipped (ADR-0016).
func (d Dirs) Skips(projectUID string) string {
	return filepath.Join(d.ProjectDir(projectUID), "skips.json")
}

// Config is worker.toml, strictly decoded (ADR-0010). It names what the worker may serve and holds no secret value.
type Config struct {
	// Executor labels this worker's runs and its Project leases; unique and stable, at most 256 bytes, no whitespace.
	Executor  string                  `toml:"executor"`
	Actor     string                  `toml:"actor"`
	Limits    Limits                  `toml:"limits"`
	Projects  []ProjectConfig         `toml:"projects"`
	Checkouts map[string]string       `toml:"checkouts"`
	Profiles  map[string]Profile      `toml:"profiles"`
	Secrets   map[string]SecretSource `toml:"secrets"`
}

// Limits are the worker-wide limits; zero values mean the defaults of ADR-0007 and ADR-0010.
type Limits struct {
	MaxActiveRuns  int    `toml:"max_active_runs"`
	DefaultTimeout string `toml:"default_timeout"`
	MaxTimeout     string `toml:"max_timeout"`
	MaxRunPrompts  int    `toml:"max_run_prompts"`
	// PauseAfterFailures is how many runs of one job recorded failed in a row pause it; nil means the default, 0 turns pausing off.
	PauseAfterFailures *int `toml:"pause_after_failures"`
}

// ProjectConfig lists one served Project; it is trusted, including its job and workflow authors.
type ProjectConfig struct {
	Name      string   `toml:"name"`
	Capacity  int      `toml:"capacity"`
	Actor     string   `toml:"actor"`
	Checkouts []string `toml:"checkouts"`
	Profiles  []string `toml:"profiles"`
	// Secrets names the [secrets] entries this Project's jobs may use. Unlike the other lists an empty one allows
	// nothing: a job written for one Project must not reach a secret meant for another.
	Secrets  []string `toml:"secrets"`
	Commands bool     `toml:"commands"`
}

// DefaultCommand is the agent program a Profile runs when its Command is blank, and the one `worker init` writes.
const DefaultCommand = ompext.Program

type Profile struct {
	Command  string   `toml:"command"`
	Model    string   `toml:"model"`
	Thinking string   `toml:"thinking"`
	Args     []string `toml:"args"`
	// Extensions picks which omp extensions the profile's runs load: "none" (the default when the key is missing),
	// "ambient" (omp's own discovery) or a list of absolute paths (ADR-0010).
	Extensions Extensions `toml:"extensions"`
}

const (
	ExtensionsAmbient = "ambient"
	ExtensionsNone    = "none"
)

// Extensions is a profile's `extensions` key: the string "ambient" or "none", or a list of absolute extension paths.
// The zero value is none: a missing key runs omp with --no-extensions. A form that does not fit is kept in bad and
// refused when the config is validated, where the profile's name is known.
type Extensions struct {
	// Mode is "" (a missing key, which means none) or ExtensionsAmbient, ExtensionsNone, or "list" when Paths is set.
	Mode  string
	Paths []string
	bad   string
}

const extensionsList = "list"

func (e *Extensions) UnmarshalTOML(data any) error {
	*e = Extensions{}
	switch v := data.(type) {
	case string:
		if v != ExtensionsAmbient && v != ExtensionsNone {
			e.bad = fmt.Sprintf("%q is not \"ambient\", \"none\" or a list of absolute paths", v)
			return nil
		}
		e.Mode = v
	case []any:
		e.Mode = extensionsList
		for _, item := range v {
			path, ok := item.(string)
			if !ok {
				e.bad = "a list must hold only paths (strings)"
				return nil
			}
			e.Paths = append(e.Paths, path)
		}
	default:
		e.bad = "must be \"ambient\", \"none\" or a list of absolute paths"
	}
	return nil
}

func (e Extensions) validate() string {
	if e.bad != "" {
		return e.bad
	}
	if e.Mode != extensionsList {
		return ""
	}
	if len(e.Paths) == 0 {
		return "an empty list loads nothing; use \"none\" for that"
	}
	for _, p := range e.Paths {
		if !filepath.IsAbs(p) {
			return fmt.Sprintf("path %q must be absolute (omp refuses relative --trusted-extension paths)", p)
		}
	}
	return ""
}

// Ambient is true only when the profile opts in to omp's own discovery, so kata-plugin's takeover may load.
func (e Extensions) Ambient() bool { return e.Mode == ExtensionsAmbient }

// args are the omp flags the key stands for. A missing key is "none"; omp keeps explicit -e, --hook and
// --trusted-extension paths under --no-extensions, so a profile's args can still name what it loads.
func (e Extensions) args() []string {
	switch e.Mode {
	case "", ExtensionsNone:
		return []string{"--no-extensions"}
	case extensionsList:
		var out []string
		for _, p := range e.Paths {
			out = append(out, "--trusted-extension", p)
		}
		return out
	}
	return nil
}

func (p Profile) hasArg(flags ...string) bool {
	return slices.ContainsFunc(p.Args, func(a string) bool {
		flag, _, _ := strings.Cut(a, "=")
		return slices.Contains(flags, flag)
	})
}

// LoadsAmbientExtensions is true when a run of the profile lets omp discover its extensions: neither the `extensions`
// key nor the profile's own args (--no-extensions, --trusted-extension) switch discovery off.
func (p Profile) LoadsAmbientExtensions() bool {
	return p.Extensions.Ambient() && !p.hasArg("--no-extensions", "--trusted-extension")
}

// ompProfile is the omp profile the profile's args select with --profile <name> or --profile=<name> (the last wins, as
// in omp); ok is false when they select none.
func (p Profile) ompProfile() (name string, ok bool) {
	for i, a := range p.Args {
		if value, found := strings.CutPrefix(a, "--profile="); found {
			name, ok = value, true
		} else if a == "--profile" && i+1 < len(p.Args) {
			name, ok = p.Args[i+1], true
		}
	}
	return name, ok
}

type SecretSource struct {
	Env  string `toml:"env"`
	File string `toml:"file"`
}

const (
	// DefaultTimeout and MaxTimeout bound a run when a job names none or too much.
	DefaultTimeout = 2 * time.Hour
	MaxTimeout     = 8 * time.Hour
	// DefaultRunPrompts is max_run_prompts' default (ADR-0010).
	DefaultRunPrompts = 12
)

var secretName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)

// LoadConfig reads and validates path. A missing file is an error that says where worker.toml belongs.
func LoadConfig(path string) (Config, error) {
	var c Config
	meta, err := toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("no worker config at %s; see docs/guide/worker.md", path)
	}
	if err := c.checkDecoded(meta, err, path); err != nil {
		return Config{}, err
	}
	return c, nil
}

// checkDecoded is what both loaders do after decoding: the decode error, keys the schema does not know, and validation,
// each named with the given file name.
func (c *Config) checkDecoded(meta toml.MetaData, err error, name string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return fmt.Errorf("%s: unknown key %s", name, strings.Join(keys, ", "))
	}
	if err := c.refuseExplicitZeros(meta); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := c.validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// refuseExplicitZeros fails a zero that was written out: an unset max_active_runs, max_run_prompts or capacity takes its
// default, but a written 0 is not a positive number and would otherwise turn into the default unseen. A project's capacity
// is in an array of tables, where the decoder keeps no per-project key, so a written capacity that left a project at 0 is
// counted instead.
func (c *Config) refuseExplicitZeros(meta toml.MetaData) error {
	if meta.IsDefined("limits", "max_active_runs") && c.Limits.MaxActiveRuns == 0 {
		return errors.New("limits: max_active_runs must be positive; remove the line for no limit")
	}
	if meta.IsDefined("limits", "max_run_prompts") && c.Limits.MaxRunPrompts == 0 {
		return errors.New("limits: max_run_prompts must be 1 to 100; remove the line for the default")
	}
	written := 0
	for _, k := range meta.Keys() {
		if len(k) == 2 && k[0] == "projects" && k[1] == "capacity" {
			written++
		}
	}
	for _, p := range c.Projects {
		if p.Capacity != 0 {
			written--
		}
	}
	if written > 0 {
		return errors.New("projects: capacity must be positive; remove the line for the default")
	}
	return nil
}

const (
	FieldExecutor = "executor"
	FieldCommand  = "command"
)

// FieldError is a validation failure about one worker.toml field: Field is FieldExecutor or FieldCommand, so a caller
// finds the input at fault without reading the message.
type FieldError struct {
	Field string
	Err   error
}

func (e *FieldError) Error() string { return e.Err.Error() }
func (e *FieldError) Unwrap() error { return e.Err }

func (c *Config) validate() error {
	if c.Executor == "" {
		return &FieldError{FieldExecutor, errors.New("executor is required: a unique, stable label for this machine's worker")}
	}
	if len(c.Executor) > 256 || strings.ContainsAny(c.Executor, " \t\r\n") {
		return &FieldError{FieldExecutor, errors.New("executor must be at most 256 bytes with no whitespace")}
	}
	if c.Limits.MaxActiveRuns < 0 || c.Limits.MaxRunPrompts < 0 || c.Limits.MaxRunPrompts > 100 {
		return errors.New("limits: max_active_runs must be positive and max_run_prompts 1 to 100")
	}
	if n := c.Limits.PauseAfterFailures; n != nil && *n < 0 {
		return errors.New("limits: pause_after_failures must be 0 (never pause) or more")
	}
	if _, err := c.Limits.parse(c.Limits.DefaultTimeout, DefaultTimeout, "default_timeout"); err != nil {
		return err
	}
	if _, err := c.Limits.parse(c.Limits.MaxTimeout, MaxTimeout, "max_timeout"); err != nil {
		return err
	}
	if c.maxTimeout() < c.defaultTimeout() {
		return errors.New("limits: max_timeout must be at least default_timeout")
	}
	if _, ok := c.Profiles["default"]; !ok {
		return errors.New("profiles.default is required")
	}
	for name, p := range c.Profiles {
		if why := p.Extensions.validate(); why != "" {
			return fmt.Errorf("profiles.%s: extensions: %s", name, why)
		}
		if p.Extensions.Mode == extensionsList && p.hasArg("-e", "--extension", "--hook") {
			return fmt.Errorf("profiles.%s: extensions: a list cannot be combined with -e, --extension or --hook in args (omp refuses that combination)", name)
		}
		if p.Command != "" && strings.ContainsAny(p.Command, `/\`) && !filepath.IsAbs(p.Command) {
			return &FieldError{FieldCommand, fmt.Errorf("profiles.%s: command %q must be absolute or a name on PATH", name, p.Command)}
		}
	}
	for key, dir := range c.Checkouts {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("checkouts.%s: %q is not an absolute directory", key, dir)
		}
	}
	for name, s := range c.Secrets {
		if !secretName.MatchString(name) {
			return fmt.Errorf("secrets.%s: names are 1 to 256 of A-Z a-z 0-9 . _ -", name)
		}
		if (s.Env == "") == (s.File == "") {
			return fmt.Errorf("secrets.%s: give exactly one of env and file", name)
		}
		if _, home := homeRest(s.File); s.File != "" && !home && !filepath.IsAbs(s.File) {
			return fmt.Errorf("secrets.%s: file %q must be absolute or start with ~/", name, s.File)
		}
		if why := credentialAlias(s); why != "" {
			return fmt.Errorf("secrets.%s: %s", name, why)
		}
	}
	seen := map[string]bool{}
	for i, p := range c.Projects {
		if p.Name == "" {
			return fmt.Errorf("projects[%d]: name is required", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("projects: %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.Capacity < 0 {
			return fmt.Errorf("projects.%s: capacity must be positive", p.Name)
		}
		for _, key := range p.Checkouts {
			if _, ok := c.Checkouts[key]; !ok {
				return fmt.Errorf("projects.%s: checkout %q is not in [checkouts]", p.Name, key)
			}
		}
		for _, name := range p.Profiles {
			if _, ok := c.Profiles[name]; !ok {
				return fmt.Errorf("projects.%s: profile %q is not in [profiles]", p.Name, name)
			}
		}
		for _, name := range p.Secrets {
			if _, ok := c.Secrets[name]; !ok {
				return fmt.Errorf("projects.%s: secret %q is not in [secrets]", p.Name, name)
			}
		}
	}
	return nil
}

func (Limits) parse(text string, fallback time.Duration, name string) (time.Duration, error) {
	if text == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(text)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("limits.%s: %q is not a positive duration like 2h", name, text)
	}
	return d, nil
}

// defaultTimeout is default_timeout; unset, it is DefaultTimeout or max_timeout when that is shorter, so a lone
// max_timeout under 2h is valid.
func (c Config) defaultTimeout() time.Duration {
	d, _ := c.Limits.parse(c.Limits.DefaultTimeout, DefaultTimeout, "")
	if c.Limits.DefaultTimeout == "" {
		d = min(d, c.maxTimeout())
	}
	return d
}

func (c Config) maxTimeout() time.Duration {
	d, _ := c.Limits.parse(c.Limits.MaxTimeout, MaxTimeout, "")
	return d
}

const DefaultPauseAfterFailures = 3

func (c Config) PauseAfterFailures() int {
	if c.Limits.PauseAfterFailures == nil {
		return DefaultPauseAfterFailures
	}
	return *c.Limits.PauseAfterFailures
}

func (c Config) MaxRunPrompts() int {
	if c.Limits.MaxRunPrompts == 0 {
		return DefaultRunPrompts
	}
	return c.Limits.MaxRunPrompts
}

const defaultCapacity = 3

func (c Config) project(name string) (ProjectConfig, bool) {
	i := slices.IndexFunc(c.Projects, func(p ProjectConfig) bool { return p.Name == name })
	if i < 0 {
		return ProjectConfig{}, false
	}
	p := c.Projects[i]
	if p.Capacity == 0 {
		p.Capacity = defaultCapacity
	}
	return p, true
}

// allowsCheckout and allowsProfile apply a Project's optional allowlists; an empty list allows all.
func (p ProjectConfig) allowsCheckout(key string) bool {
	return len(p.Checkouts) == 0 || slices.Contains(p.Checkouts, key)
}

// PickerChoices are the profile names and checkout keys the Job card offers for the named Project, sorted: the ones its
// allowlists let through (all, when it lists none or worker.toml does not serve it). A checkout key that is not a
// portable name is left out: the card could not save it.
func (c Config) PickerChoices(project string) (profiles, checkouts []string) {
	pc, _ := c.project(project)
	for _, name := range slices.Sorted(maps.Keys(c.Profiles)) {
		if pc.allowsProfile(name) {
			profiles = append(profiles, name)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(c.Checkouts)) {
		if cronjob.PortableName(key) && pc.allowsCheckout(key) {
			checkouts = append(checkouts, key)
		}
	}
	return profiles, checkouts
}

func (p ProjectConfig) allowsProfile(name string) bool {
	return len(p.Profiles) == 0 || slices.Contains(p.Profiles, name)
}

// allowsSecret is true only for a secret the Project lists; an empty list allows none.
func (p ProjectConfig) allowsSecret(name string) bool {
	return slices.Contains(p.Secrets, name)
}

// secretEnvNames are the environment variables the [secrets] env sources read. A child inherits none of them: a job
// gets only the secrets it names (ADR-0009), not every variable another job's or Project's secret came from. Names
// are upper-cased when caseInsensitive (Windows).
func (c Config) secretEnvNames(caseInsensitive bool) map[string]bool {
	hidden := map[string]bool{}
	for _, src := range c.Secrets {
		if src.Env == "" {
			continue
		}
		name := src.Env
		if caseInsensitive {
			name = strings.ToUpper(name)
		}
		hidden[name] = true
	}
	return hidden
}

func (c Config) readSecret(name string) (string, error) {
	src, ok := c.Secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q is not defined in worker.toml", name)
	}
	if src.Env != "" {
		value, ok := os.LookupEnv(src.Env)
		if !ok {
			return "", fmt.Errorf("secret %q: environment variable %s is not set", name, src.Env)
		}
		return value, nil
	}
	path, err := expandHome(src.File)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret %q: %s is not a regular file (links are refused)", name, path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret %q: %s must be private (mode 0600), it is %o", name, path, info.Mode().Perm())
	}
	if foreignOwned(info) {
		return "", fmt.Errorf("secret %q: %s must be owned by the user the worker runs as", name, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

func (p Profile) command() string {
	if p.Command == "" {
		return DefaultCommand
	}
	return p.Command
}

// credentialAlias says why a secret source is one of kata's own credential sources, empty when it is not. A job could
// otherwise be handed the token the worker reaches kata with (ADR-0010).
func credentialAlias(s SecretSource) string {
	switch s.Env {
	case "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE":
		return s.Env + " is kata's credential; a secret may not alias it"
	}
	if s.File == "" {
		return ""
	}
	path, err := expandHome(s.File)
	if err != nil {
		return ""
	}
	if tokenFile := strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN_FILE")); tokenFile != "" && samePath(path, tokenFile) {
		return s.File + " is the file KATA_AUTH_TOKEN_FILE names; a secret may not alias kata's credential"
	}
	home := os.Getenv("KATA_HOME")
	if home == "" {
		if user, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(user, ".kata")
		}
	}
	if home != "" && insidePath(home, path) {
		return s.File + " is inside kata's home, where its credentials live; a secret may not alias them"
	}
	return ""
}

// homeRest is path without its leading ~/ (or ~\ where that is the separator); ok is false when it has neither.
func homeRest(path string) (rest string, ok bool) {
	rest, ok = strings.CutPrefix(path, "~/")
	if !ok {
		rest, ok = strings.CutPrefix(path, "~"+string(filepath.Separator))
	}
	return rest, ok
}

// expandHome resolves a leading ~/ (and ~\ where that is the separator, as Windows users write it) to the home folder.
func expandHome(path string) (string, error) {
	rest, ok := homeRest(path)
	if !ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rest), nil
}

// resolved is path with its symlinks followed where it exists, so an alias through a link is still seen.
func resolved(path string) string {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

func samePath(a, b string) bool { return resolved(a) == resolved(b) }

func insidePath(dir, path string) bool {
	rel, err := filepath.Rel(resolved(dir), resolved(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
