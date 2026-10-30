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

// AgentBackgroundSessionArgs is the tern command line for `S` when the hidden session does not exist: a new session with the agent.
func AgentBackgroundSessionArgs(dir string, commandLine []string) []string {
	return append([]string{"new", "session", model.AgentSession, "--cwd", dir, "--json", "--"}, commandLine...)
}

// AgentBackgroundTabArgs is the tern command line for `S` when the hidden session exists: a new tab in it with the agent.
func AgentBackgroundTabArgs(dir string, commandLine []string) []string {
	return append([]string{"new", "tab", model.AgentSession, "--cwd", dir, "--json", "--"}, commandLine...)
}

// RenameAgentArgs names the tab of an agent's block; Tern keeps the name only while the tab lives, so it runs after every move.
func RenameAgentArgs(block, name string) []string { return []string{"rename", block, name} }

// MoveRightOfArgs moves a block right of another (target "" names the focused block, as w does).
func MoveRightOfArgs(block, target string) []string {
	return []string{"move", block, "right-of", cmp.Or(target, "@focused"), "--json"}
}

// MoveNewTabArgs moves a block into a tab of its own in its session.
func MoveNewTabArgs(block string) []string { return []string{"move", block, "new-tab", "--json"} }

// AgentShellTabArgs opens a shell (no command) in the hidden session, a new session when it does not exist; it holds the place of
// an agent that goes back to a session with no block left.
func AgentShellTabArgs(sessionExists bool) []string {
	if sessionExists {
		return []string{"new", "tab", model.AgentSession, "--json"}
	}
	return []string{"new", "session", model.AgentSession, "--json"}
}

// CloseBlockArgs closes a block.
func CloseBlockArgs(block string) []string { return []string{"close", block, "--json"} }

// agentSessionBlocks reads `tern ls --json` for the hidden session: whether it exists and the ids of its blocks.
func agentSessionBlocks(out []byte) (exists bool, blocks []string, err error) {
	var ls struct {
		Sessions []struct {
			Name string `json:"name"`
			Tabs []struct {
				Blocks []struct {
					ID json.Number `json:"id"`
				} `json:"blocks"`
			} `json:"tabs"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &ls); err != nil {
		return false, nil, fmt.Errorf("tern ls --json: %w", err)
	}
	for _, s := range ls.Sessions {
		if s.Name != model.AgentSession {
			continue
		}
		exists = true
		for _, t := range s.Tabs {
			for _, b := range t.Blocks {
				blocks = append(blocks, b.ID.String())
			}
		}
	}
	return exists, blocks, nil
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
	if err == nil {
		return nil
	}
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
	var out []byte
	switch cmd.Placement {
	case model.AgentTab:
		out, err = g.runTern(ctx, AgentTabArgs(dir, line))
		err = ternFailure(err, out)
	case model.AgentBackground:
		out, err = g.startBackground(ctx, dir, line)
	default:
		out, err = g.runTern(ctx, AgentSplitArgs(cmd.PaneBlock, dir, line))
		err = ternFailure(err, out)
	}
	if err != nil {
		removePrompt(promptFile)
		return failed(err)
	}
	block, err := ParseTernBlock(out)
	if err != nil {
		removePrompt(promptFile)
		return failed(err)
	}
	entry := model.AgentEntry{IssueUID: cmd.IssueUID, IssueRef: cmd.IssueRef, ProjectID: cmd.ProjectID, Block: block, StartedAt: g.now().UTC().Truncate(time.Second),
		PromptFile: promptFile, PaneBlock: cmd.PaneBlock, Background: cmd.Placement == model.AgentBackground}
	if _, err := UpdateAgents(filepath.Join(dirs.State, AgentsFile), func(list []model.AgentEntry) []model.AgentEntry {
		return append(slices.DeleteFunc(list, func(e model.AgentEntry) bool { return e.Block == block }), entry)
	}); err != nil {
		return failed(fmt.Errorf("the agent started in block %s but agents.json could not be written: %w", block, err))
	}
	started := model.AgentStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Entry: entry}
	if entry.Background {
		if out, err := g.runTern(ctx, RenameAgentArgs(block, cmd.IssueRef)); err != nil {
			started.Note = "cannot name its tab: " + ternFailure(err, out).Error()
		}
	}
	return started
}

// startBackground starts the agent in the hidden session: a new tab when `tern ls --json` lists the session, a new session when
// not, and a new tab after all when another Pane made the session in between.
func (g *Gateway) startBackground(ctx context.Context, dir string, line []string) ([]byte, error) {
	ls, err := g.runTern(ctx, []string{"ls", "--json"})
	if err != nil {
		return nil, ternFailure(err, ls)
	}
	exists, _, err := agentSessionBlocks(ls)
	if err != nil {
		return nil, err
	}
	if exists {
		out, err := g.runTern(ctx, AgentBackgroundTabArgs(dir, line))
		return out, ternFailure(err, out)
	}
	out, err := g.runTern(ctx, AgentBackgroundSessionArgs(dir, line))
	if err != nil && strings.Contains(string(out), "is already called") {
		out, err = g.runTern(ctx, AgentBackgroundTabArgs(dir, line))
	}
	return out, ternFailure(err, out)
}

// setAgentOpen writes the open flag of the entry with block.
func (g *Gateway) setAgentOpen(block string, open bool) error {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return err
	}
	_, err = UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		for i := range list {
			if list[i].Block == block {
				list[i].Open = open
			}
		}
		return list
	})
	return err
}

// openAgent moves a background agent's block beside the Pane and marks it open.
func (g *Gateway) openAgent(ctx context.Context, cmd model.OpenAgent) model.AgentPlaced {
	if out, err := g.runTern(ctx, MoveRightOfArgs(cmd.Block, cmd.PaneBlock)); err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: ternFailure(err, out)}
	}
	if err := g.setAgentOpen(cmd.Block, true); err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: fmt.Errorf("the agent moved but agents.json could not be written: %w", err)}
	}
	return model.AgentPlaced{Block: cmd.Block, Moved: true, Open: true}
}

// sendAgentBack moves an open background agent into a tab of the hidden session and clears its open flag. It goes right of a block
// the session still holds and then into a tab of its own; with none left it goes beside a shell opened for the purpose, which is
// closed again. The tab is named last, for Tern drops a tab's name when its block moves.
func (g *Gateway) sendAgentBack(ctx context.Context, cmd model.SendAgentBack) model.AgentPlaced {
	failed := func(err error) model.AgentPlaced { return model.AgentPlaced{Block: cmd.Block, Back: true, Err: err} }
	ls, err := g.runTern(ctx, []string{"ls", "--json"})
	if err != nil {
		return failed(ternFailure(err, ls))
	}
	exists, blocks, err := agentSessionBlocks(ls)
	if err != nil {
		return failed(err)
	}
	blocks = slices.DeleteFunc(blocks, func(b string) bool { return b == cmd.Block })
	step := func(args []string) error {
		out, err := g.runTern(ctx, args)
		return ternFailure(err, out)
	}
	if len(blocks) > 0 {
		if err := step(MoveRightOfArgs(cmd.Block, blocks[0])); err != nil {
			return failed(err)
		}
		if err := step(MoveNewTabArgs(cmd.Block)); err != nil {
			return failed(err)
		}
	} else {
		out, err := g.runTern(ctx, AgentShellTabArgs(exists))
		if err != nil {
			return failed(ternFailure(err, out))
		}
		shell, err := ParseTernBlock(out)
		if err != nil {
			return failed(err)
		}
		if err := step(MoveRightOfArgs(cmd.Block, shell)); err != nil {
			_ = step(CloseBlockArgs(shell))
			return failed(err)
		}
		if err := step(CloseBlockArgs(shell)); err != nil {
			return failed(err)
		}
	}
	placed := model.AgentPlaced{Block: cmd.Block, Back: true, Moved: true}
	if err := g.setAgentOpen(cmd.Block, false); err != nil {
		placed.Err = fmt.Errorf("the agent moved but agents.json could not be written: %w", err)
		return placed
	}
	if err := step(RenameAgentArgs(cmd.Block, cmd.IssueRef)); err != nil {
		placed.Err = fmt.Errorf("cannot name its tab: %w", err)
	}
	return placed
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

// refreshAgents reads agents.json again and, first, drops the entries of the gone blocks with their prompt files. The file is
// read fresh, so an agent another kata Pane started is in the answer.
func (g *Gateway) refreshAgents(cmd model.RefreshAgents) model.AgentsRefreshed {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentsRefreshed{Err: err}
	}
	current, err := ReadAgents(file)
	if err != nil || len(cmd.Gone) == 0 || !slices.ContainsFunc(current, func(e model.AgentEntry) bool { return slices.Contains(cmd.Gone, e.Block) }) {
		return model.AgentsRefreshed{Agents: current, Err: err}
	}
	var gone []model.AgentEntry
	kept, err := UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		gone = gone[:0]
		return slices.DeleteFunc(list, func(e model.AgentEntry) bool {
			if !slices.Contains(cmd.Gone, e.Block) {
				return false
			}
			gone = append(gone, e)
			return true
		})
	})
	if err != nil {
		return model.AgentsRefreshed{Err: err}
	}
	for _, e := range gone {
		removePrompt(e.PromptFile)
	}
	return model.AgentsRefreshed{Agents: kept}
}

// focusAgent has Tern show the agent's block.
func (g *Gateway) focusAgent(ctx context.Context, cmd model.FocusAgent) model.AgentFocused {
	if out, err := g.runTern(ctx, FocusAgentArgs(cmd.Block)); err != nil {
		return model.AgentFocused{Err: ternFailure(err, out)}
	}
	return model.AgentFocused{}
}

// FocusAgentArgs is the tern command line that shows a block.
func FocusAgentArgs(block string) []string { return []string{"focus", block} }
