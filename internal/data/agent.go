package data

import (
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
		line = append(line, "--as="+actor)
	}
	if messaging.AddressPrefix != "" {
		line = append(line, "--address-prefix="+messaging.AddressPrefix)
		if messaging.ForwardComments {
			line = append(line, "--forward-comments")
		}
		for _, actor := range messaging.MessageFrom {
			line = append(line, "--message-from="+actor)
		}
		if messaging.Coordinator != "" {
			line = append(line, "--coordinator="+messaging.Coordinator)
		}
		if messaging.Coordinating {
			line = append(line, "--coordinating")
		}
	}
	line = append(line, route.Args()...)
	line = append(line, "--ref="+ref, "--")
	line = append(line, command...)
	line = append(line, messaging.ExtensionArgs...)
	return append(line, "@"+promptFile)
}

// Tern quotes each word after -- for the login shell, so the words go as they are.
func launchArgs(dir string, commandLine []string) []string {
	return append([]string{"--cwd", dir, "--json", "--"}, commandLine...)
}

// AgentSplitArgs names the focused block when the Pane cannot name its own (paneBlock is "").
func AgentSplitArgs(paneBlock, dir string, commandLine []string) []string {
	return append([]string{"split", cmp.Or(paneBlock, "@focused"), "right"}, launchArgs(dir, commandLine)...)
}

func AgentTabArgs(dir string, commandLine []string) []string {
	return append([]string{"new", "tab"}, launchArgs(dir, commandLine)...)
}

func agentNewArgs(sessionExists bool, rest ...string) []string {
	kind := "session"
	if sessionExists {
		kind = "tab"
	}
	return append([]string{"new", kind, model.AgentSession}, rest...)
}

// RenameAgentArgs names the tab of an agent's block; Tern keeps the name only while the tab lives, so it runs after every move.
func RenameAgentArgs(block, name string) []string { return []string{"rename", block, name} }

// MoveRightOfArgs moves a block right of another (target "" names the focused block, as w does).
func MoveRightOfArgs(block, target string) []string {
	return []string{"move", block, "right-of", cmp.Or(target, "@focused"), "--json"}
}

func MoveNewTabArgs(block string) []string { return []string{"move", block, "new-tab", "--json"} }

func CloseBlockArgs(block string) []string { return []string{"close", block, "--json"} }

func hiddenSession(l ternListing) (exists bool, blocks []string) {
	for _, s := range l.sessions {
		if s.name == model.AgentSession {
			exists = true
			blocks = append(blocks, s.blocks...)
		}
	}
	return exists, blocks
}

type agentsDoc struct {
	Agents []model.AgentEntry `json:"agents"`
}

func jsonDoc[D any](path string, token guardedfile.Token) (D, error) {
	var doc D
	if token.Text() == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(token.Text()), &doc); err != nil {
		var zero D
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

func readJSONDoc[D any](path string) (D, error) {
	token, err := guardedfile.Read(path)
	if err != nil {
		var zero D
		return zero, err
	}
	return jsonDoc[D](path, token)
}

// updateJSONDoc reads the JSON file at path again, lets change return the next document from what the read saw, and writes it
// indented under guardedfile.Update: only while the file is still what was read, change running again when it is not.
func updateJSONDoc[D any](path string, change func(guardedfile.Token) (D, error)) error {
	return guardedfile.Update(path, 0o700, func(token guardedfile.Token) ([]byte, error) {
		doc, err := change(token)
		if err != nil {
			return nil, err
		}
		raw, err := json.MarshalIndent(doc, "", "  ")
		return append(raw, '\n'), err
	})
}

func ReadAgents(path string) ([]model.AgentEntry, error) {
	doc, err := readJSONDoc[agentsDoc](path)
	return doc.Agents, err
}

// UpdateAgents reads agents.json again, lets change return the new list, and writes it only while the file is still what was
// read. When another Pane changed the file in between, the whole change runs again on the new list, so change must tolerate
// being called more than once and must start its own results afresh. A file that keeps changing is an error wrapping
// guardedfile.ErrChanged, with the other writer's file left as it is.
func UpdateAgents(path string, change func([]model.AgentEntry) []model.AgentEntry) ([]model.AgentEntry, error) {
	var next []model.AgentEntry
	err := updateJSONDoc(path, func(token guardedfile.Token) (agentsDoc, error) {
		current, err := jsonDoc[agentsDoc](path, token)
		if err != nil {
			return agentsDoc{}, err
		}
		next = change(slices.Clone(current.Agents))
		return agentsDoc{Agents: next}, nil
	})
	if err != nil {
		return nil, err
	}
	return next, nil
}

func (g *Gateway) agentsFile() (string, error) {
	state, err := g.stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(state, AgentsFile), nil
}

// reconcileAgents is what the Pane does to agents.json once Tern has said which blocks are alive. An entry goes, with its prompt
// file, when suspect names its block and Tern does not list it; an entry whose starting Pane is not a live block is adopted by
// paneBlock (never when it is ""). Only entries that were in the file before tern was asked can change: one another Pane wrote
// since was never checked, and the write adopts an entry only if it still names the Pane it named when read, so two Panes
// do not both take it. A decision that rests on a cached block set is confirmed with a fresh one first, for a Pane or an agent
// that started after the cache would look gone. live is the set of blocks Tern listed, nil when it was not asked (no entries, or
// nothing to decide). When tern cannot be asked nothing changes: current is the file as read and askErr says why.
func (g *Gateway) reconcileAgents(ctx context.Context, file, paneBlock string, fresh bool, suspect func(block string) bool) (current []model.AgentEntry, live map[string]bool, askErr, err error) {
	current, err = ReadAgents(file)
	if err != nil || len(current) == 0 {
		return current, nil, nil, err
	}
	if !fresh && paneBlock == "" && !slices.ContainsFunc(current, func(e model.AgentEntry) bool { return suspect(e.Block) }) {
		return current, nil, nil, nil
	}
	read := make(map[string]string, len(current))
	for _, e := range current {
		read[e.Block] = e.PaneBlock
	}
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
		return current, nil, err, nil
	}
	// A suspect block still in a cached answer may have closed since it: only a fresh answer can say.
	if cached && (changes() || slices.ContainsFunc(current, func(e model.AgentEntry) bool { return suspect(e.Block) })) {
		if live, _, err = g.liveBlocks(ctx, true); err != nil {
			return current, nil, err, nil
		}
	}
	if !changes() {
		return current, live, nil, nil
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
		return current, nil, nil, err
	}
	g.retireAgents(ctx, gone)
	return kept, live, nil, nil
}

func (g *Gateway) pruneAgents(ctx context.Context, cmd model.PruneAgents) model.AgentsPruned {
	file, err := g.agentsFile()
	if err != nil {
		return model.AgentsPruned{Seq: cmd.Seq, Err: err}
	}
	forks := g.forkFilesNow()
	agents, live, askErr, err := g.reconcileAgents(ctx, file, cmd.PaneBlock, true, func(string) bool { return true })
	if askErr == nil && err == nil {
		g.syncAgentRecordsAtStart(ctx, cmd.Workspace, file)
		g.retireForkFiles(ctx, forks, live)
	}
	return model.AgentsPruned{Seq: cmd.Seq, Agents: agents, Err: cmp.Or(askErr, err)}
}

// syncAgentRecordsAtStart reconciles the Pane's records with the agents that remain (syncAgentRecords). Only a read of the blocks
// that succeeded gets here: an entry whose block is gone for good must not be published. A failure waits for the next start.
func (g *Gateway) syncAgentRecordsAtStart(ctx context.Context, workspace, file string) {
	dirs, err := g.workerDirs()
	if err != nil || dirs.State == "" {
		return
	}
	_ = g.syncAgentRecords(ctx, workspace, agentDirs{config: dirs.Config, state: dirs.State}, file)
}

// forkFilesNow lists, by full path, the fork copies (forkFilePrefix) that exist now. A prune reads it before it asks Tern which
// blocks live, so a copy a fork makes after that listing is never in the list, and the fork's block missing from the answer cannot
// retire it.
func (g *Gateway) forkFilesNow() []string {
	dirs, err := g.workerDirs()
	if err != nil || dirs.State == "" {
		return nil
	}
	folder := filepath.Join(dirs.State, AgentPromptsFolder)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), forkFilePrefix) {
			files = append(files, filepath.Join(folder, e.Name()))
		}
	}
	return files
}

// retireForkFiles removes those of files, the copies forked chats made of the Pane's secret files (AgentRoute.Forked), whose block
// Tern no longer lists: a fork has no agents.json entry, so its copies go with its block. files were read before Tern was asked.
// live is the set of blocks the prune's reconcile asked for, nil when it asked none (no entries): Tern is then asked here, and
// only when a copy exists. Nothing is removed when Tern cannot be asked. Prompt and secret files that no entry owns stay: another
// Pane's start writes them before it records its entry (startAgent), with no lock held in between, so a file with no entry may
// be one that is about to have it.
func (g *Gateway) retireForkFiles(ctx context.Context, files []string, live map[string]bool) {
	if len(files) == 0 {
		return
	}
	if live == nil {
		var err error
		if live, _, err = g.liveBlocks(ctx, true); err != nil {
			return
		}
	}
	for _, file := range files {
		if block, _, ok := strings.Cut(strings.TrimPrefix(filepath.Base(file), forkFilePrefix), "-"); ok && blockID(block) && !live[block] {
			_ = os.Remove(file)
		}
	}
}

// removePrompt removes a prompt file and the secret files and session path file beside it, if the agent was started with them.
func removePrompt(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
	for _, s := range SecretFiles {
		_ = os.Remove(path + s.Suffix)
	}
	_ = os.Remove(path + AgentSessionSuffix)
}

func teamRole(cmd model.StartAgent) model.TeamRole {
	role := cmd.Role
	role.Coordinating, role.Coordinator = cmd.Coordinating, cmd.Coordinator
	return role
}

// agent.toml is read on every start.
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
	promptFile, err := writeAgentPrompt(filepath.Join(dirs.State, AgentPromptsFolder), starter, AgentPrompt(cmd.IssueRef, cmd.IssueTitle, sender, messaging.MessageFrom, teamRole(cmd)))
	if err != nil {
		return failed(err)
	}
	abort := func(err error) model.AgentStarted {
		removePrompt(promptFile)
		return failed(err)
	}
	if route, err = route.withSecretFiles(promptFile); err != nil {
		return abort(err)
	}
	block, err := g.launchAgent(ctx, cmd, dir, AgentCommandLine(settings.Actor, cmd.IssueRef, messaging, route, settings.Command, promptFile))
	if err != nil {
		return abort(err)
	}
	entry := model.AgentEntry{IssueUID: cmd.IssueUID, IssueRef: cmd.IssueRef, ProjectID: cmd.ProjectID, Block: block, StartedAt: g.now().UTC().Truncate(time.Second),
		Teammate: settings.Name, PromptFile: promptFile, PaneBlock: cmd.PaneBlock, Placement: cmd.Placement, Background: cmd.Placement == model.AgentBackground,
		NoAddress: noAddress, Actor: cmd.Actor, Coordinator: cmd.Coordinator}
	if messaging.AddressPrefix != "" {
		entry.Address = AgentAddress(messaging.AddressPrefix, block)
		entry.MessageFrom = strings.Join(messaging.MessageFrom, ",")
	}
	agentsFile := filepath.Join(dirs.State, AgentsFile)
	if _, err := UpdateAgents(agentsFile, func(list []model.AgentEntry) []model.AgentEntry {
		return append(slices.DeleteFunc(list, func(e model.AgentEntry) bool { return e.Block == block }), entry)
	}); err != nil {
		// An agent without a mapping entry cannot be messaged, focused or closed from the Pane.
		_, closeErr := g.ternOutput(ctx, ternTimeout, CloseBlockArgs(block))
		return abort(closeStartError("agent", block, "agents.json could not be written", err, closeErr))
	}
	started := model.AgentStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Entry: entry}
	if published, err := g.publishAgent(ctx, cmd.Workspace, agentDirs{config: dirs.Config, state: dirs.State}, agentsFile, entry); err != nil {
		started.Note = "cannot publish its agent record: " + err.Error()
	} else {
		started.Entry = published
	}
	if entry.Background {
		if _, err := g.ternOutput(ctx, ternTimeout, RenameAgentArgs(block, cmd.IssueRef)); err != nil {
			started.Note = strings.TrimSpace(started.Note + " cannot name its tab: " + err.Error())
		}
	}
	return started
}

func (g *Gateway) launchAgent(ctx context.Context, cmd model.StartAgent, dir string, line []string) (string, error) {
	var out []byte
	var err error
	switch cmd.Placement {
	case model.AgentTab:
		out, err = g.ternOutput(ctx, ternTimeout, AgentTabArgs(dir, line))
	case model.AgentBackground:
		out, err = g.startBackground(ctx, dir, line)
	default:
		out, err = g.ternOutput(ctx, ternTimeout, AgentSplitArgs(cmd.PaneBlock, dir, line))
	}
	if err != nil {
		return "", err
	}
	return ParseTernBlock(out)
}

// closeStartError is the error of a start whose agent could not be recorded: closeErr is the result of closing its block, which keeps
// an agent the Pane cannot reach from running, and a retry from starting a second one.
func closeStartError(who, block, failure string, cause, closeErr error) error {
	if closeErr != nil {
		return fmt.Errorf("the %s started in block %s but %s (%w), and the block could not be closed (%w): close block %s by hand", who, block, failure, cause, closeErr, block)
	}
	return fmt.Errorf("the %s was stopped because %s: %w", who, failure, cause)
}

// retireAgents removes the prompt files of entries that left agents.json, clears what is left at their Agent address (nobody reads
// it again) and removes their records. A failure is not the caller's: a record that stays (daemon unreachable)
// is removed by the next reconcile, which removes every record of this host with no entry.
func (g *Gateway) retireAgents(ctx context.Context, gone []model.AgentEntry) {
	for _, e := range gone {
		removePrompt(e.PromptFile)
	}
	g.removeAgentRecordsOf(ctx, gone)
}

// startBackground starts the agent in the hidden session: a new tab when `tern ls --json` lists the session, a new session when
// not, and a new tab after all when a failed new session leaves `tern ls --json` listing it (another Pane made it in between).
func (g *Gateway) startBackground(ctx context.Context, dir string, line []string) ([]byte, error) {
	listing, err := g.ternWindowLs(ctx, "")
	if err != nil {
		return nil, err
	}
	launch := func(exists bool) ([]byte, error) {
		return g.ternOutput(ctx, ternTimeout, agentNewArgs(exists, launchArgs(dir, line)...))
	}
	if exists, _ := hiddenSession(listing); exists {
		return launch(true)
	}
	out, err := launch(false)
	if err == nil {
		return out, nil
	}
	if again, lsErr := g.ternWindowLs(ctx, ""); lsErr == nil {
		if exists, _ := hiddenSession(again); exists {
			return launch(true)
		}
	}
	return out, err
}

// explainPlacement adds to a failed placement that the agent is in another Tern window, when it is: `tern move` places a block
// beside another of its own window only, so the Pane cannot do it from here, and Tern's words alone say no block is called so.
func (g *Gateway) explainPlacement(ctx context.Context, block string, err error) error {
	if g.inOtherWindow(ctx, block) {
		return fmt.Errorf("the agent is in another Tern window, so it cannot be placed from this one (focus it there, or use Move pane to window): %w", err)
	}
	return err
}

// The open flag is read from agents.json as it stands now.
func (g *Gateway) setAgentOpenFlag(cmd model.SetAgentOpen) model.AgentOpenSet {
	out := model.AgentOpenSet{Block: cmd.Block, Open: cmd.Open, Background: cmd.Background}
	file, err := g.agentsFile()
	if err != nil {
		out.Err = err
		return out
	}
	_, out.Err = UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		for i := range list {
			if list[i].Block == cmd.Block {
				list[i].Open = cmd.Open
				list[i].Background = list[i].Background || cmd.Background
			}
		}
		return list
	})
	return out
}

func (g *Gateway) openAgent(ctx context.Context, cmd model.OpenAgent) model.AgentPlaced {
	if _, err := g.ternOutput(ctx, ternTimeout, MoveRightOfArgs(cmd.Block, cmd.PaneBlock)); err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: g.explainPlacement(ctx, cmd.Block, err)}
	}
	if err := g.setAgentOpenFlag(model.SetAgentOpen{Block: cmd.Block, Open: true}).Err; err != nil {
		return model.AgentPlaced{Block: cmd.Block, Err: fmt.Errorf("the agent moved but agents.json could not be written: %w", err)}
	}
	return model.AgentPlaced{Block: cmd.Block, Moved: true, Open: true}
}

// sendAgentBack moves an agent in view (or an open background agent) into a tab of the hidden session and clears its open flag and marks it a background agent. It goes right of a block
// the session still holds and then into a tab of its own; with none left it goes beside a shell opened for the purpose, which is
// closed again. The tab is named last, for Tern drops a tab's name when its block moves.
func (g *Gateway) sendAgentBack(ctx context.Context, cmd model.SendAgentBack) model.AgentPlaced {
	failed := func(err error) model.AgentPlaced { return model.AgentPlaced{Block: cmd.Block, Back: true, Err: err} }
	listing, err := g.ternWindowLs(ctx, "")
	if err != nil {
		return failed(err)
	}
	exists, blocks := hiddenSession(listing)
	blocks = slices.DeleteFunc(blocks, func(b string) bool { return b == cmd.Block })
	step := func(args []string) error {
		_, err := g.ternOutput(ctx, ternTimeout, args)
		return err
	}
	// hidden answers for a block that has left the view and sits in the hidden session, whatever step failed after: the entry is
	// written with the move, so agents.json, the badge and the card say where the block is, and err still reaches the Pane.
	hidden := func(stepErr error) model.AgentPlaced {
		placed := model.AgentPlaced{Block: cmd.Block, Back: true, Moved: true, Err: stepErr}
		if err := g.setAgentOpenFlag(model.SetAgentOpen{Block: cmd.Block, Background: true}).Err; err != nil {
			placed.Err = errors.Join(stepErr, fmt.Errorf("the agent moved but agents.json could not be written: %w", err))
		}
		return placed
	}
	if len(blocks) > 0 {
		if err := step(MoveRightOfArgs(cmd.Block, blocks[0])); err != nil {
			return failed(g.explainPlacement(ctx, cmd.Block, err))
		}
		if err := step(MoveNewTabArgs(cmd.Block)); err != nil {
			return hidden(err)
		}
	} else {
		// A shell (no command) holds the place of an agent that goes back to a session with no block left.
		out, err := g.ternOutput(ctx, ternTimeout, agentNewArgs(exists, "--json"))
		if err != nil {
			return failed(err)
		}
		shell, err := ParseTernBlock(out)
		if err != nil {
			return failed(err)
		}
		if err := step(MoveRightOfArgs(cmd.Block, shell)); err != nil {
			_ = step(CloseBlockArgs(shell))
			return failed(g.explainPlacement(ctx, cmd.Block, err))
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
	file, err := g.agentsFile()
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

func (g *Gateway) closeAndForget(ctx context.Context, file, block string) ([]model.AgentEntry, error) {
	if _, err := g.ternOnBlock(ctx, block, CloseBlockArgs(block)); err != nil {
		return nil, err
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
	g.retireAgents(ctx, gone)
	return kept, nil
}

// stopAgent is the Agent card's Stop: it reads agents.json again, so an agent another Pane started or moved is stopped as it now
// stands, then closes the block and forgets the entry. Any Pane may stop any mapped agent; the rule that only the starting Pane
// closes a finished Background agent by itself is closeAgent's.
func (g *Gateway) stopAgent(ctx context.Context, cmd model.StopAgent) model.AgentStopped {
	file, err := g.agentsFile()
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

// interruptAgent types Escape into the agent's block, through the same tern the Pane closes blocks with: it ends a working omp
// turn and keeps the block and its conversation. Ctrl-C, which Tern's own agent interrupt sends, leaves an omp turn running.
func (g *Gateway) interruptAgent(ctx context.Context, cmd model.InterruptAgent) model.AgentInterrupted {
	_, err := g.ternOnBlock(ctx, cmd.Block, []string{"send", cmd.Block, "keys", "escape", "--json"})
	return model.AgentInterrupted{Block: cmd.Block, Err: err}
}

// AgentPromptName is the pattern of a prompt file's name for os.CreateTemp. A tab's or background agent carries the block id of the kata Pane
// that started it (`prompt-<block>-<random>.md`), which the plugin's window half reads from the new tab's command line to put
// the tab right of the Pane's, or to open a background agent beside it; a split agent, or a Pane that cannot name its block, gets `prompt-<random>.md`. A block that
// is not all digits is left out, so the name stays one the window half parses.
func AgentPromptName(starter string) string {
	if !blockID(starter) {
		return "prompt-*.md"
	}
	return "prompt-" + starter + "-*.md"
}

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
	file, err := g.agentsFile()
	if err != nil {
		return model.AgentsRefreshed{Seq: cmd.Seq, Err: err}
	}
	agents, _, _, err := g.reconcileAgents(ctx, file, cmd.PaneBlock, false, func(block string) bool { return slices.Contains(cmd.Suspects, block) })
	return model.AgentsRefreshed{Seq: cmd.Seq, Agents: agents, Err: err}
}

func (g *Gateway) focusAgent(ctx context.Context, cmd model.FocusAgent) model.AgentFocused {
	_, err := g.ternOnBlock(ctx, cmd.Block, []string{"focus", cmd.Block})
	return model.AgentFocused{Err: err}
}
