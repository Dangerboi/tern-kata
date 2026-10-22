package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// TrustNote is the warning printed and written wherever a config lists a Project.
const TrustNote = "Anyone who can write jobs in a listed Project can run prompts and commands on this machine as you."

var (
	// ErrConfigExists is InitConfig refusing to overwrite a worker.toml.
	ErrConfigExists = errors.New("worker config already exists")
	// ErrNoConfig is AddProject finding no worker.toml to extend.
	ErrNoConfig = errors.New("no worker config")

	unsafeKeyChars = regexp.MustCompile(`[^a-z0-9_-]`)
	checkoutsTable = regexp.MustCompile(`^\s*\[\s*checkouts\s*\]\s*(#.*)?$`)
)

// Binding is a kata Project and the primary checkout its directory is bound to.
type Binding struct {
	Project  string
	Checkout string
}

// Setup is what a starter worker.toml is built from. Executor is required; every other empty field keeps today's value:
// the actor stays a comment showing DefaultActor, the command is "omp", model and thinking stay commented examples.
type Setup struct {
	Binding
	Executor     string
	Actor        string
	DefaultActor string
	Command      string
	Model        string
	Thinking     string
}

// InitConfig writes a new worker.toml at path serving s's Project from its checkout and returns the checkout key it used.
// It refuses with ErrConfigExists when path exists, and never writes a file LoadConfig refuses.
func InitConfig(path string, s Setup) (key string, err error) {
	if _, err := os.Stat(path); err == nil {
		return "", ErrConfigExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	key, _ = checkoutKey(s.Project, s.Checkout, nil)
	return key, WriteConfig(path, starterConfig(s, key))
}

// AddProject adds b's Project and its checkout to the worker.toml at path, leaving every other line as it is, and returns
// the checkout key. It returns ErrNoConfig when path is missing and refuses a Project already listed; the existing file
// must load. The result is checked before it replaces anything.
func AddProject(path string, b Binding) (key string, err error) {
	existing, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoConfig
	}
	if err != nil {
		return "", err
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return "", err
	}
	for _, p := range cfg.Projects {
		if p.Name == b.Project {
			return "", fmt.Errorf("Project %s is already listed in %s", b.Project, path)
		}
	}
	key, added := checkoutKey(b.Project, b.Checkout, cfg.Checkouts)
	return key, WriteConfig(path, addToConfig(string(existing), b.Project, key, b.Checkout, added))
}

// checkoutKey is the Project name lowercased with every character outside a-z 0-9 _ - replaced by -, suffixed -2, -3, ...
// while the key names another directory. added is false when the key already names dir, so no line is needed.
func checkoutKey(project, dir string, existing map[string]string) (key string, added bool) {
	base := unsafeKeyChars.ReplaceAllString(strings.ToLower(project), "-")
	key = base
	for n := 2; ; n++ {
		other, taken := existing[key]
		if !taken {
			return key, true
		}
		if sameCheckout(other, dir) {
			return key, false
		}
		key = base + "-" + strconv.Itoa(n)
	}
}

func sameCheckout(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// ExecutorName is the host name as an executor label: lowercased, characters outside a-z 0-9 _ - replaced by -, at most 64 long.
func ExecutorName(host string) string {
	name := unsafeKeyChars.ReplaceAllString(strings.ToLower(host), "-")
	if len(name) > 64 {
		name = name[:64]
	}
	if name == "" {
		name = "worker"
	}
	return name
}

func projectBlock(project, key string) string {
	return "[[projects]]\nname = " + tomlString(project) + "\ncapacity = 1\ncheckouts = [" + tomlString(key) + "]\n"
}

// starterConfig is the worker.toml `worker init` writes; a value the Setup leaves empty keeps its commented default.
func starterConfig(s Setup, key string) string {
	actor := "# actor = " + tomlString(s.DefaultActor)
	if s.Actor != "" {
		actor = "actor = " + tomlString(s.Actor)
	}
	command := s.Command
	if command == "" {
		command = "omp"
	}
	model := "# model = \"anthropic/claude-sonnet-4-5\""
	if s.Model != "" {
		model = "model = " + tomlString(s.Model)
	}
	thinking := "# thinking = \"high\""
	if s.Thinking != "" {
		thinking = "thinking = " + tomlString(s.Thinking)
	}
	return `# tern-kata worker config. ` + TrustNote + `
# Only list Projects whose job and workflow authors you trust.

# Labels this machine's runs and its Project leases; keep it unique and stable.
executor = ` + tomlString(s.Executor) + `
# Identity of runs when a Project names none; the default is shown.
` + actor + `

# Worker-wide limits.
# [limits]
# max_active_runs = 4      # example; unset = each Project's capacity only
# default_timeout = "2h"   # the default
# max_timeout = "8h"       # the default
# max_run_prompts = 12     # the default

# A Project's jobs may use only the checkouts listed here.
` + projectBlock(s.Project, key) + `
[checkouts]
` + key + ` = ` + tomlString(s.Checkout) + `

# The agent command line jobs run through.
[profiles.default]
command = ` + tomlString(command) + `
` + model + `
` + thinking + `
# args = []

# Named secrets a job may ask for; this file holds no value, only where to read it.
# [secrets]
# GITHUB_TOKEN = { env = "GITHUB_TOKEN" }
`
}

// addToConfig appends the Project's block and puts the checkout line inside [checkouts], creating the table at the end when
// the file has none. Every other line stays byte for byte.
func addToConfig(existing, project, key, checkout string, addCheckout bool) string {
	line := key + " = " + tomlString(checkout)
	lines := strings.SplitAfter(existing, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	insertAt := -1
	if addCheckout {
		for i, l := range lines {
			if checkoutsTable.MatchString(strings.TrimRight(l, "\r\n")) {
				insertAt = i + 1
				for j := i + 1; j < len(lines); j++ {
					t := strings.TrimSpace(lines[j])
					if strings.HasPrefix(t, "[") {
						break
					}
					if t != "" && !strings.HasPrefix(t, "#") {
						insertAt = j + 1
					}
				}
				break
			}
		}
	}
	nl := "\n"
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r\n") {
		nl = "\r\n"
	}
	if insertAt >= 0 {
		if !strings.HasSuffix(lines[insertAt-1], "\n") {
			lines[insertAt-1] += nl
		}
		lines = append(lines[:insertAt], append([]string{line + nl}, lines[insertAt:]...)...)
	}
	text := strings.Join(lines, "")
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += nl
	}
	if text != "" {
		text += nl
	}
	text += projectBlock(project, key)
	if addCheckout && insertAt < 0 {
		text += nl + "[checkouts]" + nl + line + nl
	}
	return text
}

// WriteConfig writes a whole worker.toml: content goes next to path, is checked with LoadConfig, then renamed into place (mode 0600).
// A file that would not load is removed and never replaces anything.
func WriteConfig(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".worker-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && filepath.Separator == '/' {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := LoadConfig(tmp.Name()); err != nil {
		return fmt.Errorf("the new worker.toml would not load, so nothing was written: %s", strings.ReplaceAll(err.Error(), tmp.Name(), path))
	}
	return os.Rename(tmp.Name(), path)
}
