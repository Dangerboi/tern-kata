package data

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// AgentsFile is the Agent mapping in tern-kata's state folder; AgentPromptsFolder holds the prompt files in the same place.
const (
	AgentsFile         = "agents.json"
	AgentPromptsFolder = "agent-prompts"
)

// AgentWrapperProgram is the program that sets the actor around an agent's command. It is found on PATH, as the plugin finds
// tern-kata itself; the running executable's path would go stale (Linux appends " (deleted)") when the binary is replaced.
const AgentWrapperProgram = "tern-kata"

// AgentCommandLine is the command an agent block runs: command with @promptFile appended (omp reads it as the first message),
// run through `tern-kata agent --as actor --` when an actor is set and directly otherwise.
func AgentCommandLine(actor string, command []string, promptFile string) []string {
	line := append(slices.Clone(command), "@"+promptFile)
	if actor == "" {
		return line
	}
	return append([]string{AgentWrapperProgram, "agent", "--as", actor, "--"}, line...)
}

// AgentSplitArgs is the tern command line for `w`: a split right of the Pane's block. A Pane that cannot name its block (paneBlock
// is "") names the focused one. Tern quotes each word after -- for the login shell, so the words go as they are.
func AgentSplitArgs(paneBlock, dir string, commandLine []string) []string {
	return append([]string{"split", cmp.Or(paneBlock, "@focused"), "right", "--cwd", dir, "--json", "--"}, commandLine...)
}

// AgentTabArgs is the tern command line for `W`: a new tab.
func AgentTabArgs(dir string, commandLine []string) []string {
	return append([]string{"new", "tab", "--cwd", dir, "--json", "--"}, commandLine...)
}

// ParseTernBlock is the block id in what `tern split|new tab --json` printed: {"session": 1, "tab": 2, "block": 3}. Ids carry a
// host tag and can pass 2^53, so the digits are kept as written.
func ParseTernBlock(out []byte) (string, error) {
	start := bytes.IndexByte(out, '{')
	if start < 0 {
		return "", fmt.Errorf("tern printed no JSON: %q", strings.TrimSpace(string(out)))
	}
	var found struct {
		Block json.Number `json:"block"`
	}
	if err := json.NewDecoder(bytes.NewReader(out[start:])).Decode(&found); err != nil {
		return "", fmt.Errorf("tern's JSON: %w", err)
	}
	if found.Block == "" {
		return "", errors.New("tern's JSON names no block")
	}
	return found.Block.String(), nil
}

// agentsDoc is agents.json.
type agentsDoc struct {
	Agents []model.AgentEntry `json:"agents"`
}

// ReadAgents reads agents.json at path; a missing file is no agents.
func ReadAgents(path string) ([]model.AgentEntry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc agentsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Agents, nil
}

// UpdateAgents reads agents.json again, lets change return the new list, and writes it with atomicfile. Every change goes
// through it, so a write never starts from a list read earlier. Panes of the host share the file without a lock: two changes in
// the same instant can lose one, and the Pane's prune and the relay's cleanup remove entries, never add them back.
func UpdateAgents(path string, change func([]model.AgentEntry) []model.AgentEntry) ([]model.AgentEntry, error) {
	current, err := ReadAgents(path)
	if err != nil {
		return nil, err
	}
	next := change(slices.Clone(current))
	raw, err := json.MarshalIndent(agentsDoc{Agents: next}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := atomicfile.Write(path, append(raw, '\n'), atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return nil, err
	}
	return next, nil
}

// ternBlocks is the set of block ids `tern ls --json` lists, in every session.
func ternBlocks(out []byte) (map[string]bool, error) {
	var ls struct {
		Sessions []struct {
			Tabs []struct {
				Blocks []struct {
					ID json.Number `json:"id"`
				} `json:"blocks"`
			} `json:"tabs"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &ls); err != nil {
		return nil, fmt.Errorf("tern ls --json: %w", err)
	}
	blocks := map[string]bool{}
	for _, s := range ls.Sessions {
		for _, t := range s.Tabs {
			for _, b := range t.Blocks {
				blocks[b.ID.String()] = true
			}
		}
	}
	return blocks, nil
}

// agentStatePaths are agents.json and the prompt folder, with the state folder's error when there is none.
func (g *Gateway) agentStatePaths() (file, prompts string, err error) {
	dirs, err := g.workerDirs()
	if dirs.State == "" {
		if err == nil {
			err = errors.New("tern-kata's state folder is not known")
		}
		return "", "", err
	}
	return filepath.Join(dirs.State, AgentsFile), filepath.Join(dirs.State, AgentPromptsFolder), nil
}

// pruneAgents drops the entries whose block `tern ls --json` does not list, with their prompt files. When tern cannot be asked
// nothing is dropped.
func (g *Gateway) pruneAgents(ctx context.Context) model.AgentsPruned {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentsPruned{Err: err}
	}
	if current, err := ReadAgents(file); err != nil || len(current) == 0 {
		return model.AgentsPruned{Agents: current, Err: err}
	}
	out, err := g.runTern(ctx, []string{"ls", "--json"})
	if err != nil {
		return model.AgentsPruned{Err: ternFailure(err, out)}
	}
	alive, err := ternBlocks(out)
	if err != nil {
		return model.AgentsPruned{Err: err}
	}
	var gone []model.AgentEntry
	kept, err := UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		return slices.DeleteFunc(list, func(e model.AgentEntry) bool {
			if alive[e.Block] {
				return false
			}
			gone = append(gone, e)
			return true
		})
	})
	if err != nil {
		return model.AgentsPruned{Err: err}
	}
	for _, e := range gone {
		removePrompt(e.PromptFile)
	}
	return model.AgentsPruned{Agents: kept}
}

func removePrompt(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func ternFailure(err error, out []byte) error {
	if text := strings.TrimSpace(string(out)); text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	return err
}

// startAgent starts an agent on the Issue: it resolves the actor, command and directory (agent.toml is read on every start),
// writes the prompt file, asks tern for the block, and records the agent in agents.json.
func (g *Gateway) startAgent(ctx context.Context, cmd model.StartAgent) model.AgentStarted {
	failed := func(err error) model.AgentStarted {
		return model.AgentStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Err: err}
	}
	dirs, err := g.workerDirs()
	if dirs.Config == "" || dirs.State == "" {
		return failed(cmp.Or(err, errors.New("tern-kata's folders are not known")))
	}
	cfg, err := ReadAgentConfig(filepath.Join(dirs.Config, AgentConfigFile))
	if err != nil {
		return failed(err)
	}
	settings := cfg.For(cmd.ProjectName)
	dir, err := AgentDir(cmd.Workspace, cmd.ProjectName, settings, dirs.ConfigPath())
	if err != nil {
		return failed(err)
	}
	promptFile, err := writeAgentPrompt(filepath.Join(dirs.State, AgentPromptsFolder), AgentPrompt(cmd.IssueRef, cmd.IssueTitle))
	if err != nil {
		return failed(err)
	}
	line := AgentCommandLine(settings.Actor, settings.Command, promptFile)
	args := AgentSplitArgs(cmd.PaneBlock, dir, line)
	if cmd.Placement == model.AgentTab {
		args = AgentTabArgs(dir, line)
	}
	out, err := g.runTern(ctx, args)
	if err != nil {
		removePrompt(promptFile)
		return failed(ternFailure(err, out))
	}
	block, err := ParseTernBlock(out)
	if err != nil {
		removePrompt(promptFile)
		return failed(err)
	}
	entry := model.AgentEntry{IssueUID: cmd.IssueUID, IssueRef: cmd.IssueRef, Block: block, StartedAt: g.now().UTC().Truncate(time.Second),
		PromptFile: promptFile, PaneBlock: cmd.PaneBlock}
	if _, err := UpdateAgents(filepath.Join(dirs.State, AgentsFile), func(list []model.AgentEntry) []model.AgentEntry {
		return append(slices.DeleteFunc(list, func(e model.AgentEntry) bool { return e.Block == block }), entry)
	}); err != nil {
		return failed(fmt.Errorf("the agent started in block %s but agents.json could not be written: %w", block, err))
	}
	return model.AgentStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Entry: entry}
}

// writeAgentPrompt puts text in a new file of folder and returns its path.
func writeAgentPrompt(folder, text string) (string, error) {
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(folder, "prompt-*.md")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}
