package worker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Dirs are the two folders the worker uses: Config holds worker.toml beside lanes.toml, State holds the lock, the
// control socket, the pins, cursors and run journals (ADR-0006, ADR-0010). Tests point both at temporary folders.
type Dirs struct {
	Config, State string
}

// ConfigDir is os.UserConfigDir()/tern-kata: the one rule for tern-kata's config folder, shared by worker.toml and the Pane's
// lanes.toml and filters.toml. It needs no state folder. The Luau host half (host.luau) keeps the one other copy of the rule;
// the supervision smoke step (smoke/supervision.sh) pins the two to the same folder.
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

// ConfigPath is worker.toml.
func (d Dirs) ConfigPath() string { return filepath.Join(d.Config, ConfigFileName) }

// Lock is worker.lock, the one-worker-per-machine lock (ADR-0006).
func (d Dirs) Lock() string { return filepath.Join(d.State, "worker.lock") }

// Socket is the control socket (ADR-0004).
func (d Dirs) Socket() string { return filepath.Join(d.State, "worker.sock") }

// Runs is the folder of run directories, one per run uid, each holding run.json.
func (d Dirs) Runs() string { return filepath.Join(d.State, "runs") }

// Quarantine holds journals that could not be read, whole, under the same uid; a quarantined uid is never prepared again.
func (d Dirs) Quarantine() string { return filepath.Join(d.State, "quarantine", "runs") }

// ProjectDir is the Project's pin, event cursor and (later) per-job checkpoints.
func (d Dirs) ProjectDir(projectUID string) string {
	return filepath.Join(d.State, "projects", projectUID)
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
}

// ProjectConfig lists one served Project; it is trusted, including its job and workflow authors.
type ProjectConfig struct {
	Name      string   `toml:"name"`
	Capacity  int      `toml:"capacity"`
	Actor     string   `toml:"actor"`
	Checkouts []string `toml:"checkouts"`
	Profiles  []string `toml:"profiles"`
}

// DefaultCommand is the agent program a Profile runs when its Command is blank, and the one `worker init` writes.
const DefaultCommand = "omp"

// Profile is a named agent command line. Command defaults to DefaultCommand.
type Profile struct {
	Command  string   `toml:"command"`
	Model    string   `toml:"model"`
	Thinking string   `toml:"thinking"`
	Args     []string `toml:"args"`
}

// SecretSource is where a secret's value comes from: an environment variable or a private file.
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
		return Config{}, fmt.Errorf("no worker config at %s; see the README's worker setup", path)
	}
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return Config{}, fmt.Errorf("%s: unknown key %s", path, strings.Join(keys, ", "))
	}
	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Config fields a FieldError names.
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
	def, err := c.Limits.parse(c.Limits.DefaultTimeout, DefaultTimeout, "default_timeout")
	if err != nil {
		return err
	}
	max, err := c.Limits.parse(c.Limits.MaxTimeout, MaxTimeout, "max_timeout")
	if err != nil {
		return err
	}
	if max < def {
		return errors.New("limits: max_timeout must be at least default_timeout")
	}
	if _, ok := c.Profiles["default"]; !ok {
		return errors.New("profiles.default is required")
	}
	for name, p := range c.Profiles {
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
		if s.File != "" && !strings.HasPrefix(s.File, "~/") && !filepath.IsAbs(s.File) {
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

// defaultTimeout and the other limits' accessors apply the defaults; LoadConfig has already validated the text.
func (c Config) defaultTimeout() time.Duration {
	d, _ := c.Limits.parse(c.Limits.DefaultTimeout, DefaultTimeout, "")
	return d
}

func (c Config) maxTimeout() time.Duration {
	d, _ := c.Limits.parse(c.Limits.MaxTimeout, MaxTimeout, "")
	return d
}

// MaxRunPrompts is how many prompts one run may send: the configured max_run_prompts, or the default.
func (c Config) MaxRunPrompts() int {
	if c.Limits.MaxRunPrompts == 0 {
		return DefaultRunPrompts
	}
	return c.Limits.MaxRunPrompts
}

// project is the entry for name.
func (c Config) project(name string) (ProjectConfig, bool) {
	i := slices.IndexFunc(c.Projects, func(p ProjectConfig) bool { return p.Name == name })
	if i < 0 {
		return ProjectConfig{}, false
	}
	p := c.Projects[i]
	if p.Capacity == 0 {
		p.Capacity = 1
	}
	return p, true
}

// allowsCheckout and allowsProfile apply a Project's optional allowlists; an empty list allows all.
func (p ProjectConfig) allowsCheckout(key string) bool {
	return len(p.Checkouts) == 0 || slices.Contains(p.Checkouts, key)
}

func (p ProjectConfig) allowsProfile(name string) bool {
	return len(p.Profiles) == 0 || slices.Contains(p.Profiles, name)
}

// command is the profile's program, DefaultCommand when unset.
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

// expandHome resolves a leading ~/ to the user's home folder.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
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

// insidePath reports whether path is dir or lies under it.
func insidePath(dir, path string) bool {
	rel, err := filepath.Rel(resolved(dir), resolved(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
