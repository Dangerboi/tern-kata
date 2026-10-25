package worker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

var projectsHeader = regexp.MustCompile(`^\s*\[\[\s*projects\s*\]\]\s*(#.*)?$`)

// errInlineProjects is Upgrade's refusal for a worker.toml that lists its Projects some way other than [[projects]] tables.
var errInlineProjects = errors.New("worker.toml lists its Projects some way other than [[projects]] tables, so `worker upgrade` cannot edit it; add `secrets` and `commands` to each Project by hand")

// jobNeeds is what a Project's jobs ask of worker.toml: the secrets they name and whether a workflow they run has a
// command step.
type jobNeeds struct {
	secrets  map[string]bool
	commands bool
}

// projectNeeds reads the Project's jobs, and the workflows they run, for the secrets and command steps they use. A job
// the worker sets aside whatever worker.toml says (a reserved variable name, an archived or missing workflow) asks for
// nothing, since listing it would not make it run. Any read that fails fails the whole call.
func projectNeeds(ctx context.Context, k *Kata, p Project) (jobNeeds, error) {
	needs := jobNeeds{secrets: map[string]bool{}}
	jobs, err := k.Jobs(ctx, p.ID)
	if err != nil {
		return needs, fmt.Errorf("cannot read the jobs of Project %s: %w", p.Name, err)
	}
	workflows := map[string]*Workflow{}
	for _, job := range jobs {
		if job.Archived() || job.Native() {
			continue
		}
		d := job.Definition
		for env, name := range d.SecretRefs {
			if !strings.HasPrefix(env, "KATA_") && !strings.HasPrefix(env, "TERN_KATA_") {
				needs.secrets[name] = true
			}
		}
		uid := d.Action.WorkflowUID
		if d.Action.Kind != cronjob.ActionExecute || uid == "" {
			continue
		}
		wf, done := workflows[uid]
		if !done {
			read, err := k.Workflow(ctx, p.ID, uid)
			switch {
			case errors.Is(err, ErrNotFound):
			case err != nil:
				return needs, fmt.Errorf("cannot read workflow %s of Project %s: %w", uid, p.Name, err)
			default:
				wf = &read
			}
			workflows[uid] = wf
		}
		if wf == nil || wf.Deleted {
			continue
		}
		for _, s := range wf.Steps {
			if s.Kind == cronjob.StepCommand {
				needs.commands = true
			}
		}
	}
	return needs, nil
}

// projectTable is one [[projects]] table of worker.toml: last is the index of its final key line, hasSecrets and hasCommands
// say which of the keys the user wrote, and add holds the lines Upgrade puts after last.
type projectTable struct {
	name        string
	last        int
	hasSecrets  bool
	hasCommands bool
	add         []string
}

// projectTables finds each [[projects]] table (from its header to the next line starting with [, or the end) and
// decodes its own text to learn which Project it is and which keys it holds, so quoting and comments cannot mislead.
func projectTables(lines []string) ([]*projectTable, error) {
	var tables []*projectTable
	for i := 0; i < len(lines); i++ {
		if !projectsHeader.MatchString(strings.TrimRight(lines[i], "\r\n")) {
			continue
		}
		end, last := i+1, i
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			if t := strings.TrimSpace(lines[end]); t != "" && !strings.HasPrefix(t, "#") {
				last = end
			}
			end++
		}
		var block struct {
			Projects []struct {
				Name     string    `toml:"name"`
				Secrets  *[]string `toml:"secrets"`
				Commands *bool     `toml:"commands"`
			} `toml:"projects"`
		}
		if _, err := toml.Decode(strings.Join(lines[i:end], ""), &block); err != nil || len(block.Projects) != 1 {
			return nil, errors.New("worker.toml has a [[projects]] table `worker upgrade` cannot read; add `secrets` and `commands` to each Project by hand")
		}
		p := block.Projects[0]
		tables = append(tables, &projectTable{name: p.Name, last: last, hasSecrets: p.Secrets != nil, hasCommands: p.Commands != nil})
		i = end - 1
	}
	return tables, nil
}

// Upgrade gives the jobs that exist today what they had before a Project had to list its secrets and allow command
// steps: for each [[projects]] table of text with no `secrets` key it adds the [secrets] names the Project's jobs use,
// and with no `commands` key it adds commands = true when a workflow of the Project has a command step. A key the user
// wrote is left as it is. It returns the new text (text itself when nothing is needed) and one report line per Project
// changed and per need it could not meet. Every other line stays byte for byte, and the new text is checked with the
// config loader before the caller writes it. It reads kata only; when any Project or its jobs cannot be read it
// returns an error and no text.
func Upgrade(ctx context.Context, k *Kata, cfg Config, text string) (newText string, report []string, err error) {
	if len(cfg.Projects) == 0 {
		return text, nil, nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	tables, err := projectTables(lines)
	if err != nil {
		return "", nil, err
	}
	if len(tables) != len(cfg.Projects) {
		return "", nil, errInlineProjects
	}
	projects, err := k.Projects(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("cannot list the kata Projects: %w", err)
	}
	for _, pc := range cfg.Projects {
		i := slices.IndexFunc(projects, func(p Project) bool { return p.Name == pc.Name })
		if i < 0 {
			return "", nil, fmt.Errorf("kata has no Project %s", pc.Name)
		}
		needs, err := projectNeeds(ctx, k, projects[i])
		if err != nil {
			return "", nil, err
		}
		t := tables[slices.IndexFunc(tables, func(t *projectTable) bool { return t.name == pc.Name })]
		var unmet, secrets []string
		for _, name := range slices.Sorted(maps.Keys(needs.secrets)) {
			if _, defined := cfg.Secrets[name]; !defined {
				unmet = append(unmet, fmt.Sprintf("Project %s: a job uses secret %q, which [secrets] does not define, so nothing was added for it.", pc.Name, name))
				continue
			}
			switch {
			case !t.hasSecrets:
				secrets = append(secrets, tomlString(name))
			case !pc.allowsSecret(name):
				unmet = append(unmet, fmt.Sprintf("Project %s: a job uses secret %q, which the Project's secrets list does not allow; it was left as you wrote it.", pc.Name, name))
			}
		}
		if len(secrets) > 0 {
			t.add = append(t.add, "secrets = ["+strings.Join(secrets, ", ")+"]")
		}
		switch {
		case !needs.commands:
		case !t.hasCommands:
			t.add = append(t.add, "commands = true")
		case !pc.Commands:
			unmet = append(unmet, fmt.Sprintf("Project %s: a workflow has a command step, and the Project's commands = false was left as you wrote it.", pc.Name))
		}
		if len(t.add) > 0 {
			report = append(report, fmt.Sprintf("Project %s: added %s", pc.Name, strings.Join(t.add, ", ")))
		}
		report = append(report, unmet...)
	}
	for _, t := range slices.Backward(tables) {
		if len(t.add) == 0 {
			continue
		}
		nl := "\n"
		if strings.HasSuffix(lines[t.last], "\r\n") {
			nl = "\r\n"
		}
		if !strings.HasSuffix(lines[t.last], "\n") {
			lines[t.last] += nl
		}
		insert := make([]string, len(t.add))
		for i, l := range t.add {
			insert[i] = l + nl
		}
		lines = slices.Insert(lines, t.last+1, insert...)
	}
	newText = strings.Join(lines, "")
	if newText == text {
		return text, report, nil
	}
	if err := CheckConfig(newText); err != nil {
		return "", nil, fmt.Errorf("the upgraded worker.toml would not load, so nothing was written: %w", err)
	}
	return newText, report, nil
}
