package data

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data/ompext"
	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// AgentsFile is the Agent mapping in tern-kata's state folder; AgentPromptsFolder holds the prompt files in the same place.
const (
	AgentsFile         = "agents.json"
	AgentPromptsFolder = "agent-prompts"
)

// AgentWrapperProgram is the program that sets the actor and Issue ref around an agent's command. It is found on PATH, as the plugin finds
// tern-kata itself; the running executable's path would go stale (Linux appends " (deleted)") when the binary is replaced.
const AgentWrapperProgram = "tern-kata"

// AgentCommandLine is the command an agent block runs: command with @promptFile appended (omp reads it as the first message),
// run through `tern-kata agent [--as actor] [--address-prefix prefix [--forward-comments] [--message-from actor]... [--coordinator address | --coordinating]] [route flags] --ref ref --`. ref is the Issue's qualified
// ref, which the wrapper sets as the agent's KATA_REF; --as is left out when no actor is set, so KATA_AUTHOR stays as inherited.
// Messaging adds --address-prefix and the flag that loads the extension (ompext.Argv) before the prompt, so omp loads it even under
// --no-extensions.
// The route flags are what AgentRoute.Args writes: none for a Pane on the default daemon, so that line is the one it always was.
func AgentCommandLine(actor, ref string, messaging AgentMessaging, route AgentRoute, command []string, promptFile string) []string {
	line := []string{AgentWrapperProgram, "agent"}
	if actor != "" {
		line = append(line, "--as", actor)
	}
	if messaging.AddressPrefix != "" {
		line = append(line, "--address-prefix", messaging.AddressPrefix)
		if messaging.ForwardComments {
			line = append(line, "--forward-comments")
		}
		for _, actor := range messaging.MessageFrom {
			line = append(line, "--message-from", actor)
		}
		if messaging.Coordinator != "" {
			line = append(line, "--coordinator", messaging.Coordinator)
		}
		if messaging.Coordinating {
			line = append(line, "--coordinating")
		}
	}
	line = append(line, route.Args()...)
	line = append(line, "--ref", ref, "--")
	line = append(line, command...)
	line = append(line, messaging.ExtensionArgs...)
	return append(line, "@"+promptFile)
}

// AgentSplitArgs is the tern command line for the Agent card's `s`: a split right of the Pane's block. A Pane that cannot name its block (paneBlock
// is "") names the focused one. Tern quotes each word after -- for the login shell, so the words go as they are.
func AgentSplitArgs(paneBlock, dir string, commandLine []string) []string {
	return append([]string{"split", cmp.Or(paneBlock, "@focused"), "right", "--cwd", dir, "--json", "--"}, commandLine...)
}

// AgentTabArgs is the tern command line for the Agent card's `t`: a new tab.
func AgentTabArgs(dir string, commandLine []string) []string {
	return append([]string{"new", "tab", "--cwd", dir, "--json", "--"}, commandLine...)
}

// agentNewArgs starts a tern command that makes a tab in the hidden session, or the session itself when it does not exist.
func agentNewArgs(sessionExists bool) []string {
	if sessionExists {
		return []string{"new", "tab", model.AgentSession}
	}
	return []string{"new", "session", model.AgentSession}
}

// AgentBackgroundSessionArgs is the tern command line for the Agent card's `g` when the hidden session does not exist: a new session with the agent.
func AgentBackgroundSessionArgs(dir string, commandLine []string) []string {
	return append(agentNewArgs(false), append([]string{"--cwd", dir, "--json", "--"}, commandLine...)...)
}

// AgentBackgroundTabArgs is the tern command line for the Agent card's `g` when the hidden session exists: a new tab in it with the agent.
func AgentBackgroundTabArgs(dir string, commandLine []string) []string {
	return append(agentNewArgs(true), append([]string{"--cwd", dir, "--json", "--"}, commandLine...)...)
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
	return append(agentNewArgs(sessionExists), "--json")
}

// CloseBlockArgs closes a block.
func CloseBlockArgs(block string) []string { return []string{"close", block, "--json"} }

// InterruptBlockArgs is the Agent card's Interrupt: Escape typed into the agent's block, which ends a working omp turn and keeps
// the block and its conversation. Ctrl-C, which Tern's own agent interrupt sends, leaves an omp turn running.
func InterruptBlockArgs(block string) []string {
	return []string{"send", block, "keys", "escape", "--json"}
}

// ternSession is a session of `tern ls --json`: its name and the ids of its blocks in every tab.
type ternSession struct {
	name   string
	blocks []string
}

// parseTernLs reads `tern ls --json`. Block ids are kept as written, for they carry a host tag and can pass 2^53.
func parseTernLs(out []byte) ([]ternSession, error) {
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
		return nil, fmt.Errorf("tern ls --json: %w", err)
	}
	sessions := make([]ternSession, 0, len(ls.Sessions))
	for _, s := range ls.Sessions {
		session := ternSession{name: s.Name}
		for _, t := range s.Tabs {
			for _, b := range t.Blocks {
				session.blocks = append(session.blocks, b.ID.String())
			}
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// agentSessionBlocks reads `tern ls --json` for the hidden session: whether it exists and the ids of its blocks.
func agentSessionBlocks(out []byte) (exists bool, blocks []string, err error) {
	sessions, err := parseTernLs(out)
	if err != nil {
		return false, nil, err
	}
	for _, s := range sessions {
		if s.name == model.AgentSession {
			exists = true
			blocks = append(blocks, s.blocks...)
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
	token, err := guardedfile.Read(path)
	if err != nil {
		return nil, err
	}
	return parseAgents(path, token)
}

// parseAgents decodes what a read of agents.json saw; an empty file is no agents.
func parseAgents(path string, token guardedfile.Token) ([]model.AgentEntry, error) {
	if token.Text() == "" {
		return nil, nil
	}
	var doc agentsDoc
	if err := json.Unmarshal([]byte(token.Text()), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Agents, nil
}

// UpdateAgents reads agents.json again, lets change return the new list, and writes it only while the file is still what was
// read. When another Pane changed the file in between, the whole change runs again on the new list, so change must tolerate
// being called more than once and must start its own results afresh. A file that keeps changing is an error wrapping
// guardedfile.ErrChanged, with the other writer's file left as it is.
func UpdateAgents(path string, change func([]model.AgentEntry) []model.AgentEntry) ([]model.AgentEntry, error) {
	var next []model.AgentEntry
	err := guardedfile.Update(path, 0o700, func(token guardedfile.Token) ([]byte, error) {
		current, err := parseAgents(path, token)
		if err != nil {
			return nil, err
		}
		next = change(slices.Clone(current))
		raw, err := json.MarshalIndent(agentsDoc{Agents: next}, "", "  ")
		return append(raw, '\n'), err
	})
	if err != nil {
		return nil, err
	}
	return next, nil
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

// liveBlocksMaxAge is how long a refresh trusts the block set of an earlier `tern ls --json`.
const liveBlocksMaxAge = 10 * time.Second

// liveBlocksTimeout bounds one shared `tern ls --json`, which runs apart from any single caller's deadline.
const liveBlocksTimeout = 30 * time.Second

// lsFlight is one running `tern ls --json` that callers wait on; live and err are set before done closes. begun is when it
// started, the age of its answer.
type lsFlight struct {
	done  chan struct{}
	begun time.Time
	live  map[string]bool
	err   error
}

// liveBlocks is the set of blocks `tern ls --json` lists. It is cached for liveBlocksMaxAge, read by g.now; fresh asks tern
// again and replaces the cache. cached says the answer may be older than the call: it came from the cache, or from a `tern ls`
// another caller started before this one asked. Callers that need an answer while one `tern ls` runs share it, each waiting
// only until its own ctx ends; a fresh caller never joins a run that began before it asked.
func (g *Gateway) liveBlocks(ctx context.Context, fresh bool) (live map[string]bool, cached bool, err error) {
	g.lsMu.Lock()
	if !fresh && g.lsLive != nil && g.now().Sub(g.lsAt) < liveBlocksMaxAge {
		live = g.lsLive
		g.lsMu.Unlock()
		return live, true, nil
	}
	f := g.lsFlight
	joined := f != nil && !fresh
	if !joined {
		f = &lsFlight{done: make(chan struct{}), begun: g.now()}
		g.lsFlight = f
		go g.runLiveBlocks(context.WithoutCancel(ctx), f)
	}
	g.lsMu.Unlock()
	select {
	case <-f.done:
		return f.live, joined, f.err
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

// runLiveBlocks asks tern for the blocks, caches a good answer, stamped with when the run began, unless a later run's answer
// is cached already, and wakes the callers waiting on f.
func (g *Gateway) runLiveBlocks(ctx context.Context, f *lsFlight) {
	ctx, cancel := context.WithTimeout(ctx, liveBlocksTimeout)
	defer cancel()
	f.live, f.err = g.askLiveBlocks(ctx)
	g.lsMu.Lock()
	if f.err == nil && !f.begun.Before(g.lsAt) {
		g.lsLive, g.lsAt = f.live, f.begun
	}
	if g.lsFlight == f {
		g.lsFlight = nil
	}
	g.lsMu.Unlock()
	close(f.done)
}

func (g *Gateway) askLiveBlocks(ctx context.Context) (map[string]bool, error) {
	out, err := g.runTern(ctx, []string{"ls", "--json"})
	if err != nil {
		return nil, ternFailure(err, out)
	}
	listing, err := parseTernLs(out)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, s := range listing {
		for _, b := range s.blocks {
			live[b] = true
		}
	}
	return live, nil
}

// reconcileAgents is what the Pane does to agents.json once Tern has said which blocks are alive. An entry goes, with its prompt
// file, when suspect names its block and Tern does not list it; an entry whose starting Pane is not a live block is adopted by
// paneBlock (never when it is ""). Only entries that were in the file before tern was asked can change: one another Pane wrote
// since was never checked, and the write adopts an entry only if it still names the Pane it named when read, so two Panes
// do not both take it. A decision that rests on a cached block set is confirmed with a fresh one first, for a Pane or an agent
// that started after the cache would look gone. When tern cannot be asked nothing changes: current is the file as read and askErr
// says why.
func (g *Gateway) reconcileAgents(ctx context.Context, file, paneBlock string, fresh bool, suspect func(block string) bool) (current []model.AgentEntry, askErr, err error) {
	current, err = ReadAgents(file)
	if err != nil || len(current) == 0 {
		return current, nil, err
	}
	if !fresh && paneBlock == "" && !slices.ContainsFunc(current, func(e model.AgentEntry) bool { return suspect(e.Block) }) {
		return current, nil, nil
	}
	read := make(map[string]string, len(current))
	for _, e := range current {
		read[e.Block] = e.PaneBlock
	}
	var live map[string]bool
	var cached bool
	verdict := func(e model.AgentEntry) (drop, adopt bool) {
		if starter, ok := read[e.Block]; !ok || starter != e.PaneBlock {
			return false, false
		}
		if suspect(e.Block) && !live[e.Block] {
			return true, false
		}
		return false, paneBlock != "" && e.PaneBlock != paneBlock && !live[e.PaneBlock]
	}
	changes := func() bool {
		return slices.ContainsFunc(current, func(e model.AgentEntry) bool { drop, adopt := verdict(e); return drop || adopt })
	}
	if live, cached, err = g.liveBlocks(ctx, fresh); err != nil {
		return current, err, nil
	}
	// A suspect block still in a cached answer may have closed since it: only a fresh answer can say.
	if cached && (changes() || slices.ContainsFunc(current, func(e model.AgentEntry) bool { return suspect(e.Block) })) {
		if live, _, err = g.liveBlocks(ctx, true); err != nil {
			return current, err, nil
		}
	}
	if !changes() {
		return current, nil, nil
	}
	var gone []model.AgentEntry
	kept, err := UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		gone = gone[:0]
		list = slices.DeleteFunc(list, func(e model.AgentEntry) bool {
			if drop, _ := verdict(e); drop {
				gone = append(gone, e)
				return true
			}
			return false
		})
		for i, e := range list {
			if _, adopt := verdict(e); adopt {
				list[i].PaneBlock = paneBlock
			}
		}
		return list
	})
	if err != nil {
		return current, nil, err
	}
	for _, e := range gone {
		removePrompt(e.PromptFile)
	}
	g.forgetAgentRecords(ctx, gone)
	return kept, nil, nil
}

// pruneAgents is the Pane's start: it drops the entries whose block `tern ls --json` does not list, with their prompt files, and
// adopts the entries of a Pane that is gone (reconcileAgents).
func (g *Gateway) pruneAgents(ctx context.Context, cmd model.PruneAgents) model.AgentsPruned {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentsPruned{Seq: cmd.Seq, Err: err}
	}
	agents, askErr, err := g.reconcileAgents(ctx, file, cmd.PaneBlock, true, func(string) bool { return true })
	if askErr == nil && err == nil {
		g.syncAgentRecordsAtStart(ctx, file, agents)
	}
	return model.AgentsPruned{Seq: cmd.Seq, Agents: agents, Err: cmp.Or(askErr, err)}
}

// syncAgentRecordsAtStart reconciles the Pane's records with the agents that remain (syncAgentRecords). Only a read of the blocks
// that succeeded gets here: an entry whose block is gone for good must not be published. A failure waits for the next start.
func (g *Gateway) syncAgentRecordsAtStart(ctx context.Context, file string, agents []model.AgentEntry) {
	dirs, err := g.workerDirs()
	if err != nil || dirs.State == "" {
		return
	}
	_ = g.syncAgentRecords(ctx, "", agentDirs{config: dirs.Config, state: dirs.State}, file, agents)
}

// removePrompt removes a prompt file and the token, DSN, env and session path files beside it, if the agent was started with them.
func removePrompt(path string) {
	if path != "" {
		_ = os.Remove(path)
		_ = os.Remove(path + agentTokenSuffix)
		_ = os.Remove(path + agentDSNSuffix)
		_ = os.Remove(path + agentEnvSuffix)
		_ = os.Remove(path + AgentSessionSuffix)
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
	route, err := g.CurrentAgentRoute()
	if err != nil {
		return failed(err)
	}
	if err := route.Preflight(dir); err != nil {
		return failed(err)
	}
	// A background agent names its starter too: the window half finds the kata Pane to open it beside from the prompt file's name.
	var starter string
	if cmd.Placement == model.AgentTab || cmd.Placement == model.AgentBackground {
		starter = cmd.PaneBlock
	}
	noAddress := agentNoAddress(settings.Command)
	var messaging AgentMessaging
	sender := ""
	if noAddress == "" && cmd.Actor != "" && settings.Name != "" {
		extension, err := ompext.Install(filepath.Join(dirs.State, OmpExtensionFolder))
		if err != nil {
			return failed(fmt.Errorf("write the agent's omp extension: %w", err))
		}
		messaging = AgentMessaging{AddressPrefix: cmd.Actor + "/" + settings.Name, ExtensionArgs: ompext.Argv(extension, settings.Command[1:], false), ForwardComments: settings.ForwardComments,
			MessageFrom: settings.MessageFrom, Coordinator: cmd.Coordinator, Coordinating: cmd.Coordinating}
		sender = cmd.Actor
	}
	promptFile, err := writeAgentPrompt(filepath.Join(dirs.State, AgentPromptsFolder), starter, AgentPrompt(cmd.IssueRef, cmd.IssueTitle, sender, messaging.MessageFrom))
	if err != nil {
		return failed(err)
	}
	if route, err = route.withSecretFiles(promptFile); err != nil {
		removePrompt(promptFile)
		return failed(err)
	}
	line := AgentCommandLine(settings.Actor, cmd.IssueRef, messaging, route, settings.Command, promptFile)
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
		Teammate: settings.Name, PromptFile: promptFile, PaneBlock: cmd.PaneBlock, Placement: cmd.Placement, Background: cmd.Placement == model.AgentBackground,
		NoAddress: noAddress, Actor: cmd.Actor}
	if messaging.AddressPrefix != "" {
		entry.Address = AgentAddress(messaging.AddressPrefix, block)
		entry.MessageFrom = strings.Join(messaging.MessageFrom, ",")
	}
	if _, err := UpdateAgents(filepath.Join(dirs.State, AgentsFile), func(list []model.AgentEntry) []model.AgentEntry {
		return append(slices.DeleteFunc(list, func(e model.AgentEntry) bool { return e.Block == block }), entry)
	}); err != nil {
		// An agent without a mapping entry cannot be messaged, focused or closed from the Pane, and a retry would start a second one.
		removePrompt(promptFile)
		if out, closeErr := g.runTern(ctx, CloseBlockArgs(block)); closeErr != nil {
			return failed(fmt.Errorf("the agent started in block %s but agents.json could not be written (%w), and the block could not be closed (%w): close block %s by hand",
				block, err, ternFailure(closeErr, out), block))
		}
		return failed(fmt.Errorf("the agent was stopped because agents.json could not be written: %w", err))
	}
	started := model.AgentStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Entry: entry}
	if published, err := g.publishAgent(ctx, cmd.Workspace, agentDirs{config: dirs.Config, state: dirs.State}, filepath.Join(dirs.State, AgentsFile), entry); err != nil {
		started.Note = "cannot publish its agent record: " + err.Error()
	} else {
		started.Entry = published
	}
	if entry.Background {
		if out, err := g.runTern(ctx, RenameAgentArgs(block, cmd.IssueRef)); err != nil {
			started.Note = strings.TrimSpace(started.Note + " cannot name its tab: " + ternFailure(err, out).Error())
		}
	}
	return started
}

// forgetAgentRecords removes the records of entries that left agents.json. A record that stays (daemon unreachable) is removed by the
// next reconcile, which removes every record of this host with no entry, so a failure here is not the caller's.
func (g *Gateway) forgetAgentRecords(ctx context.Context, gone []model.AgentEntry) {
	_ = g.removeAgentRecordsOf(ctx, gone)
}

// startBackground starts the agent in the hidden session: a new tab when `tern ls --json` lists the session, a new session when
// not, and a new tab after all when a failed new session leaves `tern ls --json` listing it (another Pane made it in between).
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
	if err == nil {
		return out, nil
	}
	failure := ternFailure(err, out)
	if again, lsErr := g.runTern(ctx, []string{"ls", "--json"}); lsErr == nil {
		if exists, _, parseErr := agentSessionBlocks(again); parseErr == nil && exists {
			out, err = g.runTern(ctx, AgentBackgroundTabArgs(dir, line))
			return out, ternFailure(err, out)
		}
	}
	return out, failure
}

// setAgentOpen writes the open flag of the entry with block; background also marks it a background agent.
func (g *Gateway) setAgentOpen(block string, open, background bool) error {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return err
	}
	_, err = UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		for i := range list {
			if list[i].Block == block {
				list[i].Open = open
				list[i].Background = list[i].Background || background
			}
		}
		return list
	})
	return err
}

// setAgentOpenFlag records where the relay saw an agent: the open flag changes (and the background mark when cmd asks), read from agents.json as it stands now.
func (g *Gateway) setAgentOpenFlag(cmd model.SetAgentOpen) model.AgentOpenSet {
	return model.AgentOpenSet{Block: cmd.Block, Open: cmd.Open, Background: cmd.Background, Err: g.setAgentOpen(cmd.Block, cmd.Open, cmd.Background)}
}

// openAgent moves a background agent's block beside the Pane and marks it open.
func (g *Gateway) openAgent(ctx context.Context, cmd model.OpenAgent) model.AgentPlaced {
	if out, err := g.runTern(ctx, MoveRightOfArgs(cmd.Block, cmd.PaneBlock)); err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: ternFailure(err, out)}
	}
	if err := g.setAgentOpen(cmd.Block, true, false); err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: fmt.Errorf("the agent moved but agents.json could not be written: %w", err)}
	}
	return model.AgentPlaced{Block: cmd.Block, Moved: true, Open: true}
}

// sendAgentBack moves an agent in view (or an open background agent) into a tab of the hidden session and clears its open flag and marks it a background agent. It goes right of a block
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
	// hidden answers for a block that has left the view and sits in the hidden session, whatever step failed after: the entry is
	// written with the move, so agents.json, the badge and the card say where the block is, and err still reaches the Pane.
	hidden := func(stepErr error) model.AgentPlaced {
		placed := model.AgentPlaced{Block: cmd.Block, Back: true, Moved: true, Err: stepErr}
		if err := g.setAgentOpen(cmd.Block, false, true); err != nil {
			placed.Err = errors.Join(stepErr, fmt.Errorf("the agent moved but agents.json could not be written: %w", err))
		}
		return placed
	}
	if len(blocks) > 0 {
		if err := step(MoveRightOfArgs(cmd.Block, blocks[0])); err != nil {
			return failed(err)
		}
		if err := step(MoveNewTabArgs(cmd.Block)); err != nil {
			return hidden(err)
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
			return hidden(err)
		}
	}
	placed := hidden(nil)
	if placed.Err != nil {
		return placed
	}
	if err := step(RenameAgentArgs(cmd.Block, cmd.IssueRef)); err != nil {
		placed.Err = fmt.Errorf("cannot name its tab: %w", err)
	}
	return placed
}

// closeAgent closes the agent's block with `tern close`, then drops its entry and prompt file from agents.json as it stands now.
// When tern fails the entry stays, and so does an agent whose entry reads open now (another Pane opened it since this one decided)
// or is no background agent (this Pane asks about a split or tab agent, which another Pane may have sent to the background).
func (g *Gateway) closeAgent(ctx context.Context, cmd model.CloseAgent) model.AgentClosed {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentClosed{Block: cmd.Block, Err: fmt.Errorf("cannot tell whether the agent is a background agent: %w", err)}
	}
	list, err := ReadAgents(file)
	if err != nil {
		return model.AgentClosed{Block: cmd.Block, Err: fmt.Errorf("cannot tell whether the agent is a background agent: %w", err)}
	}
	if slices.ContainsFunc(list, func(e model.AgentEntry) bool { return e.Block == cmd.Block && (e.Open || !e.Background) }) {
		return model.AgentClosed{Block: cmd.Block, Agents: list, Kept: true}
	}
	agents, err := g.closeAndForget(ctx, file, cmd.Block)
	if err != nil {
		return model.AgentClosed{Block: cmd.Block, Err: err}
	}
	return model.AgentClosed{Block: cmd.Block, Agents: agents}
}

// closeAndForget closes the block with `tern close`, then drops its entry and prompt file from agents.json as it stands now, and
// returns the file after. When tern fails the entry stays.
func (g *Gateway) closeAndForget(ctx context.Context, file, block string) ([]model.AgentEntry, error) {
	if out, err := g.runTern(ctx, CloseBlockArgs(block)); err != nil {
		return nil, ternFailure(err, out)
	}
	var gone []model.AgentEntry
	kept, err := UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		gone = gone[:0]
		return slices.DeleteFunc(list, func(e model.AgentEntry) bool {
			if e.Block != block {
				return false
			}
			gone = append(gone, e)
			return true
		})
	})
	if err != nil {
		return nil, fmt.Errorf("the block closed but agents.json could not be written: %w", err)
	}
	for _, e := range gone {
		removePrompt(e.PromptFile)
	}
	g.forgetAgentRecords(ctx, gone)
	return kept, nil
}

// stopAgent is the Agent card's Stop: it reads agents.json again, so an agent another Pane started or moved is stopped as it now
// stands, then closes the block and forgets the entry. Any Pane may stop any mapped agent; the rule that only the starting Pane
// closes a finished Background agent by itself is closeAgent's.
func (g *Gateway) stopAgent(ctx context.Context, cmd model.StopAgent) model.AgentStopped {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentStopped{Block: cmd.Block, Err: err}
	}
	list, err := ReadAgents(file)
	if err != nil {
		return model.AgentStopped{Block: cmd.Block, Err: err}
	}
	if !slices.ContainsFunc(list, func(e model.AgentEntry) bool { return e.Block == cmd.Block }) {
		return model.AgentStopped{Block: cmd.Block, Agents: list, Gone: true}
	}
	agents, err := g.closeAndForget(ctx, file, cmd.Block)
	if err != nil {
		return model.AgentStopped{Block: cmd.Block, Err: err}
	}
	return model.AgentStopped{Block: cmd.Block, Agents: agents}
}

// PaneTitle is the title that identifies a kata Pane to the window half (window.luau); the Pane writes it at startup.
const PaneTitle = "tern-kata"

// interruptAgent types Escape into the agent's block, through the same tern the Pane closes blocks with.
func (g *Gateway) interruptAgent(ctx context.Context, cmd model.InterruptAgent) model.AgentInterrupted {
	if out, err := g.runTern(ctx, InterruptBlockArgs(cmd.Block)); err != nil {
		return model.AgentInterrupted{Block: cmd.Block, Err: ternFailure(err, out)}
	}
	return model.AgentInterrupted{Block: cmd.Block}
}

// AgentPromptName is the pattern of a prompt file's name for os.CreateTemp. A tab's or background agent carries the block id of the kata Pane
// that started it (`prompt-<block>-<random>.md`), which the plugin's window half reads from the new tab's command line to put
// the tab right of the Pane's, or to open a background agent beside it; a split agent, or a Pane that cannot name its block, gets `prompt-<random>.md`. A block that
// is not all digits is left out, so the name stays one the window half parses.
func AgentPromptName(starter string) string {
	if starter == "" || strings.Trim(starter, "0123456789") != "" {
		return "prompt-*.md"
	}
	return "prompt-" + starter + "-*.md"
}

// writeAgentPrompt puts text in a new file of folder, named for starter by AgentPromptName, and returns its path.
func writeAgentPrompt(folder, starter, text string) (string, error) {
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(folder, AgentPromptName(starter))
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

// refreshAgents reads agents.json again, so an agent another kata Pane started is in the answer. The blocks in cmd.Suspects were
// listed by some relay and are missing from the latest: an entry of theirs goes, with its prompt file, only when `tern ls --json`
// does not list the block, and an entry whose starting Pane is not a live block is adopted by cmd.PaneBlock (reconcileAgents).
// Tern's answer is cached for liveBlocksMaxAge, so a stream of relays costs few `tern ls`. When tern cannot be asked the file is
// returned as it is.
func (g *Gateway) refreshAgents(ctx context.Context, cmd model.RefreshAgents) model.AgentsRefreshed {
	file, _, err := g.agentStatePaths()
	if err != nil {
		return model.AgentsRefreshed{Seq: cmd.Seq, Err: err}
	}
	agents, _, err := g.reconcileAgents(ctx, file, cmd.PaneBlock, false, func(block string) bool { return slices.Contains(cmd.Suspects, block) })
	return model.AgentsRefreshed{Seq: cmd.Seq, Agents: agents, Err: err}
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
