package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

const trustNote = "Anyone who can write jobs in a listed Project can run prompts and commands on this machine as you."

var (
	unsafeKeyChars = regexp.MustCompile(`[^a-z0-9_-]`)
	checkoutsTable = regexp.MustCompile(`^\s*\[\s*checkouts\s*\]\s*(#.*)?$`)
)

// setupWorkerConfig is `worker init` (add false) or `worker add` (add true): it writes worker.toml for the Project bound
// to env.Dir, or adds that Project and its checkout to the existing file. The result is checked with the worker's own
// LoadConfig before it replaces anything.
func setupWorkerConfig(add bool, plan LaunchPlan, env workerEnv, out io.Writer) error {
	path := env.Dirs.ConfigPath()
	existing, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return subcommandError{readErr}
	}
	switch {
	case add && readErr != nil:
		return subcommandError{fmt.Errorf("no worker config at %s; run `tern-kata worker init` first", path)}
	case !add && readErr == nil:
		return subcommandError{fmt.Errorf("%s already exists; use `tern-kata worker add` to add this Project to it", path)}
	}
	project, checkout, err := data.ResolveBinding(env.Dir)
	if err != nil {
		return subcommandError{err}
	}
	if project == "" {
		return subcommandError{fmt.Errorf("%s is not bound to a kata Project; run `kata init` there first", env.Dir)}
	}

	var (
		content string
		key     string
		added   bool
	)
	if add {
		cfg, err := worker.LoadConfig(path)
		if err != nil {
			return subcommandError{err}
		}
		for _, p := range cfg.Projects {
			if p.Name == project {
				return subcommandError{fmt.Errorf("Project %s is already listed in %s", project, path)}
			}
		}
		key, added = checkoutKey(project, checkout, cfg.Checkouts)
		content = addToConfig(string(existing), project, key, checkout, added)
	} else {
		key, added = checkoutKey(project, checkout, nil)
		content = starterConfig(env.Hostname, plan.Actor, project, key, checkout)
	}
	if err := writeChecked(path, content); err != nil {
		return subcommandError{err}
	}

	verb := "Wrote"
	if add {
		verb = "Updated"
	}
	fmt.Fprintf(out, "%s %s: serving Project %s from %s (checkout %s).\n%s\n", verb, path, project, checkout, key, trustNote)
	if add {
		fmt.Fprintln(out, "A running worker reads worker.toml only at start: end it (`tern-kata worker status` prints its process id) and the plugin starts a fresh one.")
	} else {
		fmt.Fprintln(out, "The plugin's host half starts the worker within 10 seconds (else run `tern-kata worker`); `tern-kata worker doctor` checks the setup.")
	}
	return nil
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
		if sameDir(other, dir) {
			return key, false
		}
		key = base + "-" + strconv.Itoa(n)
	}
}

func sameDir(a, b string) bool {
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

func executorName(host string) string {
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

// starterConfig is the worker.toml `worker init` writes.
func starterConfig(host, actor, project, key, checkout string) string {
	return `# tern-kata worker config. ` + trustNote + `
# Only list Projects whose job and workflow authors you trust.

# Labels this machine's runs and its Project leases; keep it unique and stable.
executor = ` + tomlString(executorName(host)) + `
# Identity of runs when a Project names none; the default is shown.
# actor = ` + tomlString(actor) + `

# Worker-wide limits; the values shown are the defaults.
# [limits]
# max_active_runs = 4
# default_timeout = "2h"
# max_timeout = "8h"
# max_run_prompts = 12

# A Project's jobs may use only the checkouts listed here.
` + projectBlock(project, key) + `
[checkouts]
` + key + ` = ` + tomlString(checkout) + `

# The agent command line jobs run through.
[profiles.default]
command = "omp"
# model = "anthropic/claude-sonnet-4-5"
# thinking = "high"
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

// writeChecked writes content next to path, checks it loads as a worker config, then renames it into place (mode 0600).
// A file that would not load is removed and never replaces anything.
func writeChecked(path, content string) error {
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
	if _, err := worker.LoadConfig(tmp.Name()); err != nil {
		return fmt.Errorf("the new worker.toml would not load, so nothing was written: %s", strings.ReplaceAll(err.Error(), tmp.Name(), path))
	}
	return os.Rename(tmp.Name(), path)
}
