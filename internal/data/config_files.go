package data

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The Lane lists and default filters files in tern-kata's config folder.
const (
	lanesFileName   = "lanes.toml"
	filtersFileName = "filters.toml"
)

// loadConfigFiles is the one read of the Config Page: every row, each read by the loader its feature already uses, so a file that
// does not load shows that loader's error on its own row and the others still show. A config folder that cannot be found is the
// answer's error and no rows.
func (g *Gateway) loadConfigFiles(_ context.Context, cmd model.LoadConfigFiles) model.Msg {
	folder, err := g.configFolder()
	if err != nil {
		return model.ConfigFilesLoaded{Request: cmd.Request, Err: err}
	}
	dirs := worker.Dirs{Config: folder}
	enter := []model.ConfigAction{model.ConfigOpen}
	files := []model.ConfigFile{
		configRow(model.ConfigLanes, lanesFileName, filepath.Join(folder, lanesFileName), "not created: every Project uses the default Lanes", enter, func(path string) (string, error) {
			file, err := readLanes(path)
			if err != nil {
				return "", err
			}
			n := 0
			for _, p := range file {
				if len(cleanLanes(p.Labels)) > 0 {
					n++
				}
			}
			return countOf(n, "Project", "Projects") + " with a Lane list", nil
		}),
		configRow(model.ConfigFilters, filtersFileName, filepath.Join(folder, filtersFileName), "not created: no Project has a default filter", enter, func(path string) (string, error) {
			file, err := readTables[filtersFile](path)
			if err != nil {
				return "", err
			}
			n := 0
			for _, p := range file {
				if p != (projectFilters{}) {
					n++
				}
			}
			return countOf(n, "Project", "Projects") + " with a default filter", nil
		}),
		configRow(model.ConfigViews, "views.toml", NewViewsStore(folder).Path, "not created: no saved views", enter, func(string) (string, error) {
			read, err := NewViewsStore(folder).Read()
			if err != nil {
				return "", err
			}
			summary := countOf(len(read.Views), "saved view", "saved views")
			if len(read.Problems) > 0 {
				summary += ", " + strconv.Itoa(len(read.Problems)) + " skipped as invalid"
			}
			return summary, nil
		}),
		configRow(model.ConfigSeen, seenFileName, filepath.Join(folder, seenFileName), "not created: no Runs marked seen", enter, func(path string) (string, error) {
			file, err := readTables[seenFile](path)
			if err != nil {
				return "", err
			}
			marked, timeline := 0, 0
			for _, p := range file {
				if !p.Seen.IsZero() {
					marked++
				}
				if p.Timeline {
					timeline++
				}
			}
			return countOf(marked, "Project", "Projects") + " with a seen mark, " + strconv.Itoa(timeline) + " with since you last looked on", nil
		}),
		configRow(model.ConfigWorker, worker.ConfigFileName, dirs.ConfigPath(), "no worker set up", enter, func(path string) (string, error) {
			cfg, err := worker.LoadConfig(path)
			summary := ""
			if err == nil {
				summary = countOf(len(cfg.Projects), "Project", "Projects") + " served"
			}
			return joinSummary(summary, workerSupervision(dirs)), err
		}),
		agentRow(filepath.Join(folder, AgentConfigFile)),
	}
	if row, ok := g.docbankRow(); ok {
		files = append(files, row)
	}
	files = append(files, g.kataConfigRow())
	return model.ConfigFilesLoaded{Request: cmd.Request, Folder: folder, Files: files}
}

// configRow reads one file: missing says what a missing file implies, and read is the file's own loader, given the path. A summary
// returned with an error is kept, so a row that does not load can still say what it can (the worker row's Supervision).
func configRow(key model.ConfigFileKey, title, path, missing string, actions []model.ConfigAction, read func(path string) (string, error)) model.ConfigFile {
	row := model.ConfigFile{Key: key, Title: title, Path: path, Actions: actions}
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		row.Summary = missing
		return row
	}
	if err == nil {
		row.Summary, err = read(path)
	}
	if err != nil {
		row.Status, row.Err = model.ConfigBroken, err.Error()
		return row
	}
	row.Status = model.ConfigLoads
	return row
}

// agentRow is the agent.toml row: read by the agent start's own reader, carrying the text the Page's edit starts from and the base
// it must still find on disk.
func agentRow(path string) model.ConfigFile {
	row := configRow(model.ConfigAgent, AgentConfigFile, path, "agents start with the defaults", []model.ConfigAction{model.ConfigEdit}, func(path string) (string, error) {
		cfg, err := ReadAgentConfig(path)
		if err != nil {
			return "", err
		}
		return agentSummary(cfg), nil
	})
	row.Starter = AgentConfigStarter
	if token, err := guardedfile.Read(path); err == nil {
		row.Text = token.Text()
	}
	return row
}

// saveAgentConfig replaces agent.toml with text an agent start accepts, when the file is still the base the edit began from. The
// write takes the config folder's lock and replaces the file atomically, as the Lane, filter and view files are written. A text
// that does not load is refused and a changed file is model.ErrAgentConfigChanged; nothing is written in either case.
func (g *Gateway) saveAgentConfig(cmd model.SaveAgentConfig) model.AgentConfigSaved {
	done := model.AgentConfigSaved{Text: cmd.Text}
	folder, err := g.configFolder()
	if err != nil {
		done.Err = err
		return done
	}
	if done.Err = CheckAgentConfig(cmd.Text); done.Err != nil {
		return done
	}
	path := filepath.Join(folder, AgentConfigFile)
	token, err := guardedfile.Read(path)
	if err != nil {
		done.Err = err
		return done
	}
	if token.Text() != cmd.Base {
		done.Err = model.ErrAgentConfigChanged
		return done
	}
	switch err := guardedfile.Write(path, 0o700, token, []byte(cmd.Text)); {
	case errors.Is(err, guardedfile.ErrChanged):
		done.Err = model.ErrAgentConfigChanged
	case err != nil:
		done.Err = err
	}
	return done
}

// kataConfigRow is kata's own config.toml, read by the Daemon catalog reader; the Pane never writes it.
func (g *Gateway) kataConfigRow() model.ConfigFile {
	const title = "config.toml (kata's, read-only)"
	home, err := kataHome()
	if err != nil {
		return model.ConfigFile{Key: model.ConfigKata, Title: title, Status: model.ConfigBroken, Err: err.Error()}
	}
	return configRow(model.ConfigKata, title, filepath.Join(home, "config.toml"), "empty daemon catalog", []model.ConfigAction{model.ConfigOpen}, func(string) (string, error) {
		catalog, err := ReadCatalogAt(home)
		if err != nil {
			return "", err
		}
		quit := "q quits without asking"
		if catalog.ConfirmQuit {
			quit = "q asks before quitting"
		}
		return joinSummary(countOf(len(catalog.Daemons), "daemon", "daemons"), quit), nil
	})
}

// agentSummary is the agent.toml row's line: what an agent starts with at the top level, and the Projects that have their own table.
// A key of agent.toml that deserves a place in the row is one more entry in fields.
func agentSummary(c AgentConfig) string {
	fields := []struct{ label, value string }{
		{"actor", c.Actor},
		{"host", c.Host},
		{"name", c.Name},
		{"command", strings.Join(c.Command, " ")},
		{"message from", strings.Join(c.MessageFrom, ", ")},
	}
	var parts []string
	for _, f := range fields {
		if f.value != "" {
			parts = append(parts, f.label+" "+f.value)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "no top-level settings")
	}
	if projects := slices.Sorted(maps.Keys(c.Projects)); len(projects) > 0 {
		parts = append(parts, "own table for "+strings.Join(projects, ", "))
	}
	return strings.Join(parts, " · ")
}

// workerSupervision says whether the plugin supervises the worker, by the worker package's own rule, or that worker.manual turned it off.
func workerSupervision(d worker.Dirs) string {
	switch {
	case d.ManualPresent():
		return "Supervision off (" + worker.ManualFileName + ")"
	case d.SupervisionOn():
		return "Supervision on"
	}
	return ""
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func joinSummary(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), " · ")
}
