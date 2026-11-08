package data

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
	gen "go.kenn.io/kata/pkg/client/generated"
)

// TeamsFile is the Team record in tern-kata's state folder, beside agents.json: one record per team (model.Team).
const TeamsFile = "teams.json"

// teamsDoc is teams.json.
type teamsDoc struct {
	Teams []model.Team `json:"teams"`
}

// ReadTeams reads teams.json at path; a missing file is no teams.
func ReadTeams(path string) ([]model.Team, error) {
	doc, err := readJSONDoc[teamsDoc](path)
	return doc.Teams, err
}

// UpdateTeams reads teams.json again, lets change return the new list, and writes it only while the file is still what was read,
// as UpdateAgents does for agents.json: when another Pane changed the file in between, the whole change runs again on the new
// list, so change must tolerate being called more than once and must start its own results afresh. A file that keeps changing is
// an error wrapping guardedfile.ErrChanged.
func UpdateTeams(path string, change func([]model.Team) []model.Team) ([]model.Team, error) {
	var next []model.Team
	err := updateJSONDoc(path, func(token guardedfile.Token) (teamsDoc, error) {
		current, err := jsonDoc[teamsDoc](path, token)
		if err != nil {
			return teamsDoc{}, err
		}
		next = change(cloneTeams(current.Teams))
		return teamsDoc{Teams: next}, nil
	})
	if err != nil {
		return nil, err
	}
	return next, nil
}

func cloneTeams(list []model.Team) []model.Team {
	out := slices.Clone(list)
	for i := range out {
		out[i].Children = slices.Clone(out[i].Children)
	}
	return out
}

// teamPaths are teams.json and agents.json, with the state folder's error when there is none.
func (g *Gateway) teamPaths() (teams, agents string, err error) {
	dirs, err := g.workerDirs()
	if dirs.State == "" {
		return "", "", cmp.Or(err, errors.New("tern-kata's state folder is not known"))
	}
	return filepath.Join(dirs.State, TeamsFile), filepath.Join(dirs.State, AgentsFile), nil
}

// teamRefusal is why the agent settings cannot make a team, "" when they can: a coordinator directs children by message, so it
// needs an omp agent with an Agent address, and a child needs a name for which <name>-<block id> is a teammate: without one it
// cannot tell its coordinator from the person.
func teamRefusal(settings AgentSettings, actor string) string {
	if reason := agentNoAddress(settings.Command); reason != "" {
		return "A team needs an omp agent: the agent command is not omp, so no extension loads."
	}
	if actor == "" {
		return "A team needs an actor: agents have no address without one."
	}
	// A block id has up to 20 digits.
	if settings.Name == "" || !model.ValidTeammate(settings.Name+"-"+strings.Repeat("9", 20)) {
		return fmt.Sprintf("The agent name %q cannot be a teammate: <name>-<block id> must be at most 64 bytes.", settings.Name)
	}
	return ""
}

// startTeam starts the coordinator on the Issue, then records the team. The coordinator runs with TERN_KATA_COORDINATING=1; the
// children start from DriveTeams.
func (g *Gateway) startTeam(ctx context.Context, cmd model.StartTeam) model.TeamStarted {
	failed := func(err error) model.TeamStarted {
		return model.TeamStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Err: err}
	}
	if cmd.Limit < model.TeamMinLimit || cmd.Limit > model.TeamMaxLimit {
		return failed(fmt.Errorf("children at once must be %d to %d", model.TeamMinLimit, model.TeamMaxLimit))
	}
	dirs, err := g.workerDirs()
	if dirs.Config == "" || dirs.State == "" {
		return failed(cmp.Or(err, errors.New("tern-kata's folders are not known")))
	}
	cfg, err := ReadAgentConfig(filepath.Join(dirs.Config, AgentConfigFile))
	if err != nil {
		return failed(err)
	}
	if reason := teamRefusal(cfg.For(cmd.ProjectName), cmd.Actor); reason != "" {
		return failed(errors.New(reason))
	}
	teamsFile, agentsFile, err := g.teamPaths()
	if err != nil {
		return failed(err)
	}
	teams, err := ReadTeams(teamsFile)
	if err != nil {
		return failed(err)
	}
	if slices.ContainsFunc(teams, func(t model.Team) bool { return t.ParentUID == cmd.IssueUID }) {
		return failed(errors.New(model.TeamExistsReason))
	}
	var children []model.TeamChildRef
	if _, err := g.call(ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.IssueUID)
		if err != nil {
			return nil, daemonError(err)
		}
		for _, child := range shown.Children {
			if child.ProjectID == cmd.ProjectID && child.Status == "open" {
				children = append(children, model.TeamChildRef{Ref: child.QualifiedID, Title: child.Title})
			}
		}
		return nil, nil
	}); err != nil {
		return failed(err)
	}
	if len(children) == 0 {
		return failed(fmt.Errorf("%s has no open child in the Project %s", cmd.IssueRef, cmd.ProjectName))
	}
	started := g.startAgent(ctx, model.StartAgent{Placement: cmd.Placement, IssueUID: cmd.IssueUID, IssueRef: cmd.IssueRef, IssueTitle: cmd.IssueTitle,
		ProjectID: cmd.ProjectID, ProjectName: cmd.ProjectName, Workspace: cmd.Workspace, PaneBlock: cmd.PaneBlock, Actor: cmd.Actor, Coordinating: true,
		Role: model.TeamRole{Children: children, Limit: cmd.Limit}})
	if started.Err != nil {
		return failed(started.Err)
	}
	entry := started.Entry
	team := model.Team{ParentUID: cmd.IssueUID, ParentRef: cmd.IssueRef, ProjectID: cmd.ProjectID, Project: cmd.ProjectName,
		CoordinatorBlock: entry.Block, CoordinatorAddress: entry.Address, Actor: cmd.Actor, Limit: cmd.Limit, State: model.TeamRunning, Children: []model.TeamChild{}}
	recorded := false
	_, err = UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		recorded = !slices.ContainsFunc(list, func(t model.Team) bool { return t.ParentUID == cmd.IssueUID })
		if recorded {
			list = append(list, team)
		}
		return list
	})
	if err == nil && !recorded {
		err = errors.New(model.TeamExistsReason)
	}
	if err != nil {
		if _, closeErr := g.closeAndForget(ctx, agentsFile, entry.Block); closeErr != nil {
			return failed(fmt.Errorf("the coordinator started in block %s but the team could not be recorded (%w), and the block could not be closed (%w): close block %s by hand",
				entry.Block, err, closeErr, entry.Block))
		}
		return failed(fmt.Errorf("the coordinator was stopped because the team could not be recorded: %w", err))
	}
	return model.TeamStarted{Placement: cmd.Placement, IssueRef: cmd.IssueRef, Entry: entry, Note: started.Note}
}

// setTeamPaused writes paused or running for the team through UpdateTeams. A team that is gone, stopping or ending is refused
// and left as it is; a team already in the asked state is not an error.
func (g *Gateway) setTeamPaused(cmd model.SetTeamPaused) model.TeamPausedSet {
	out := model.TeamPausedSet{ParentRef: cmd.ParentRef, Paused: cmd.Paused}
	teamsFile, _, err := g.teamPaths()
	if err != nil {
		out.Err = err
		return out
	}
	var refusal string
	out.Teams, err = UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		refusal = ""
		i := teamIndex(list, cmd.ParentUID)
		switch {
		case i < 0:
			refusal = model.TeamNoneReason
		case list[i].State == model.TeamStopping:
			refusal = model.TeamStoppingReason
		case list[i].State == model.TeamEnding:
			refusal = model.TeamEndingReason
		case cmd.Paused:
			list[i].State = model.TeamPaused
		default:
			list[i].State = model.TeamRunning
		}
		return list
	})
	if err == nil && refusal != "" {
		err = errors.New(refusal)
	}
	out.Err = err
	return out
}

// agentAlive says an agent is mapped in block and has not exited, as far as exited (the blocks a relay listed as exited) says.
func agentAlive(entries []model.AgentEntry, exited []string, block string) bool {
	return block != "" && slices.ContainsFunc(entries, func(e model.AgentEntry) bool { return e.Block == block }) && !slices.Contains(exited, block)
}

// issueHasAgent says an agent that has not exited is mapped on the Issue.
func issueHasAgent(entries []model.AgentEntry, exited []string, uid string) bool {
	return slices.ContainsFunc(entries, func(e model.AgentEntry) bool { return e.IssueUID == uid && !slices.Contains(exited, e.Block) })
}

// countRunning counts the children of t that hold a place of the team now: the agent is mapped and has not exited, or the start is
// recorded without a block and recent. A child whose agent exited or whose block closed has freed its place.
func countRunning(t model.Team, entries []model.AgentEntry, exited []string, now time.Time) int {
	n := 0
	for _, c := range t.Children {
		if c.Block == "" && now.Sub(c.StartedAt) < model.TeamPendingMaxAge || c.Block != "" && agentAlive(entries, exited, c.Block) {
			n++
		}
	}
	return n
}

// runningChildren counts the team's children that hold a place, reading agents.json as it stands.
func runningChildren(agentsFile string, exited []string, t model.Team, now time.Time) (int, error) {
	entries, err := ReadAgents(agentsFile)
	return countRunning(t, entries, exited, now), err
}

func teamIndex(list []model.Team, uid string) int {
	return slices.IndexFunc(list, func(t model.Team) bool { return t.ParentUID == uid })
}

// driveTeams moves every team of teams.json forward (see model.DriveTeams). Pane-wide failures are Err; a team whose parent the
// daemon cannot show (another daemon's, or kata unreachable) is left as it is.
func (g *Gateway) driveTeams(ctx context.Context, cmd model.DriveTeams) model.TeamsDriven {
	teamsFile, agentsFile, err := g.teamPaths()
	if err != nil {
		return model.TeamsDriven{Err: err}
	}
	teams, err := ReadTeams(teamsFile)
	if err != nil {
		return model.TeamsDriven{Err: err}
	}
	var out model.TeamsDriven
	for _, t := range teams {
		if err := g.driveTeam(ctx, cmd, teamsFile, agentsFile, t.ParentUID, &out); err != nil {
			out.Err = err
			return out
		}
	}
	teams, err = ReadTeams(teamsFile)
	out.Teams, out.Err = teams, cmp.Or(out.Err, err)
	return out
}

// driveTeam is one team's step: a coordinator that exited makes it stopping; paused does nothing; stopping goes once no child
// runs; ending writes the end message; running starts children and, with none to start and none running, ends. The last two
// steps belong to the Pane of the actor that started the team.
func (g *Gateway) driveTeam(ctx context.Context, cmd model.DriveTeams, teamsFile, agentsFile, uid string, out *model.TeamsDriven) error {
	teams, err := ReadTeams(teamsFile)
	if err != nil {
		return err
	}
	i := teamIndex(teams, uid)
	if i < 0 {
		return nil
	}
	t := teams[i]
	// Any Pane retires a team (coordinator exited, stopping, gone); only the Pane of the actor that started it starts its children
	// (under another actor they would carry the wrong Agent address) and writes its end message.
	own := t.Actor == "" || t.Actor == cmd.Actor
	entries, err := ReadAgents(agentsFile)
	if err != nil {
		return err
	}
	now := g.now()
	if t.State != model.TeamStopping && !agentAlive(entries, cmd.Exited, t.CoordinatorBlock) {
		after, err := UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
			if i := teamIndex(list, uid); i >= 0 {
				list[i].State = model.TeamStopping
			}
			return list
		})
		if err != nil {
			return err
		}
		if i := teamIndex(after, uid); i >= 0 {
			t = after[i]
		}
	}
	switch t.State {
	case model.TeamStopping:
		_, err := UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
			i := teamIndex(list, uid)
			if i < 0 || list[i].State != model.TeamStopping {
				return list
			}
			if n, err := runningChildren(agentsFile, cmd.Exited, list[i], now); err != nil || n > 0 {
				return list
			}
			return slices.Delete(list, i, i+1)
		})
		return err
	case model.TeamEnding, model.TeamRunning:
		if !own {
			return nil
		}
		if t.State == model.TeamEnding {
			return g.writeEndMessage(ctx, cmd, teamsFile, uid)
		}
		return g.queueTeam(ctx, cmd, teamsFile, agentsFile, t, entries, out)
	}
	return nil
}

// queueTeam starts the next ready children while fewer than the limit run, and ends the team when none is left and none runs.
func (g *Gateway) queueTeam(ctx context.Context, cmd model.DriveTeams, teamsFile, agentsFile string, t model.Team, entries []model.AgentEntry, out *model.TeamsDriven) error {
	settled, err := UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		if i := teamIndex(list, t.ParentUID); i >= 0 {
			settleClaims(&list[i], entries, g.now())
		}
		return list
	})
	if err != nil {
		return err
	}
	if i := teamIndex(settled, t.ParentUID); i >= 0 {
		t = settled[i]
	}
	state, err := g.stateDir()
	if err != nil {
		return err
	}
	hostID, err := ReadHostID(state)
	if err != nil {
		return err
	}
	var parent *gen.ShowIssueResponseBody
	var ready []gen.IssueOut
	if _, err := g.call(ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		shown, err := c.showIssue(ctx, t.ProjectID, t.ParentUID)
		if err != nil {
			return nil, daemonError(err)
		}
		unowned := true
		r, err := c.ReadyIssuesWithResponse(ctx, &gen.ReadyIssuesRequestOptions{
			PathParams: &gen.ReadyIssuesPath{ProjectID: t.ProjectID}, Query: &gen.ReadyIssuesQuery{Unowned: &unowned}})
		if err != nil {
			return nil, daemonError(err)
		}
		if r.JSON200 == nil {
			return nil, errors.New("kata returned no ready Issues")
		}
		parent, ready = shown, r.JSON200.Issues
		return nil, nil
	}); err != nil {
		return nil //nolint:nilerr // the parent is another daemon's, or kata is not reachable now: the team waits for the next trigger
	}
	childOf := make(map[string]bool, len(parent.Children))
	// A child another host's record holds is neither startable nor finished: the team waits for that agent (Forget on the card
	// clears a dead one) instead of ending with the child never run.
	remoteHeld := false
	for _, c := range parent.Children {
		childOf[c.UID] = true
		if c.Status != "closed" && c.ProjectID == t.ProjectID && !t.Started(c.UID) && model.RemoteHostOn(c.Metadata, hostID) != "" {
			remoteHeld = true
		}
	}
	var candidates []gen.IssueOut
	for _, c := range ready {
		if childOf[c.UID] && c.ProjectID == t.ProjectID && !t.Started(c.UID) && !issueHasAgent(entries, cmd.Exited, c.UID) &&
			model.RemoteHostOn(c.Metadata, hostID) == "" {
			candidates = append(candidates, c)
		}
	}
	held := false // the limit is reached, or the team is no longer running
	failed := false
	for _, c := range candidates {
		claimed, full, err := claimChild(teamsFile, agentsFile, cmd.Exited, t.ParentUID, c, g.now())
		if err != nil {
			return err
		}
		if full {
			held = true
			break
		}
		if !claimed {
			continue
		}
		start := model.StartAgent{Placement: model.AgentBackground, IssueUID: c.UID, IssueRef: c.QualifiedID, IssueTitle: c.Title,
			ProjectID: c.ProjectID, ProjectName: t.Project, PaneBlock: cmd.PaneBlock, Actor: cmd.Actor, Coordinator: t.CoordinatorAddress,
			Role: model.TeamRole{Parent: t.ParentRef}}
		if cmd.BoundProjectID == t.ProjectID {
			start.Workspace = cmd.Workspace
		}
		started := g.startAgent(ctx, start)
		if started.Err != nil {
			failed = true
			out.Failures = append(out.Failures, fmt.Sprintf("Cannot start an agent on %s for the team on %s: %s", c.QualifiedID, t.ParentRef, started.Err))
			// The claim stays: it expires after model.TeamPendingMaxAge, so a failing start is not retried on every event.
			continue
		}
		if err := setChildBlock(teamsFile, t.ParentUID, c.UID, started.Entry.Block); err != nil {
			return err
		}
		out.Started = append(out.Started, started.Entry)
	}
	if held || failed || remoteHeld {
		return nil
	}
	// Read the record again: children another Pane started are in it now.
	after, err := ReadTeams(teamsFile)
	if err != nil {
		return err
	}
	i := teamIndex(after, t.ParentUID)
	if i < 0 {
		return nil
	}
	if n, err := runningChildren(agentsFile, cmd.Exited, after[i], g.now()); err != nil || n > 0 {
		return err
	}
	return g.endTeam(ctx, cmd, teamsFile, t.ParentUID)
}

// claimChild records c as started by the team, before its agent starts, so two Panes never start the same child. claimed is false
// when the team already has the child or the record is gone; full says the limit is reached or the team no longer runs, so no
// other candidate can start either. A recorded start with no block is completed by forgetChild.
func claimChild(teamsFile, agentsFile string, exited []string, uid string, c gen.IssueOut, now time.Time) (claimed, full bool, err error) {
	_, err = UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		claimed, full = false, false
		i := teamIndex(list, uid)
		if i < 0 || list[i].State != model.TeamRunning {
			full = true
			return list
		}
		t := &list[i]
		entries, readErr := ReadAgents(agentsFile)
		if readErr != nil {
			full = true
			return list
		}
		settleClaims(t, entries, now)
		if t.Started(c.UID) {
			return list
		}
		running := countRunning(*t, entries, exited, now)
		if running >= t.Limit {
			full = true
			return list
		}
		t.Children = append(t.Children, model.TeamChild{UID: c.UID, Ref: c.QualifiedID, StartedAt: now.UTC().Truncate(time.Second)})
		claimed = true
		return list
	})
	return claimed && err == nil, full && err == nil, err
}

// setChildBlock finishes a recorded start: the child's agent runs in block.
func setChildBlock(teamsFile, parentUID, childUID, block string) error {
	_, err := UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		i := teamIndex(list, parentUID)
		if i < 0 {
			return list
		}
		t := &list[i]
		for j := range t.Children {
			if t.Children[j].UID == childUID {
				t.Children[j].Block = block
			}
		}
		return list
	})
	return err
}

// endTeam turns a running team into an ending one, claiming the end message so two Panes never write it, then writes it.
func (g *Gateway) endTeam(ctx context.Context, cmd model.DriveTeams, teamsFile, uid string) error {
	claimed := false
	if _, err := UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		claimed = false
		if i := teamIndex(list, uid); i >= 0 && list[i].State == model.TeamRunning {
			list[i].State = model.TeamEnding
			claimed = true
		}
		return list
	}); err != nil || !claimed {
		return err
	}
	return g.writeEndMessage(ctx, cmd, teamsFile, uid)
}

// writeEndMessage writes the end message of an ending team to its coordinator's address, from the Pane's actor with no teammate, and
// removes the record once it is written. A message the coordinator has not cleared is never replaced: the record stays ending
// and a later drive writes the message once the address is clear.
func (g *Gateway) writeEndMessage(ctx context.Context, cmd model.DriveTeams, teamsFile, uid string) error {
	teams, err := ReadTeams(teamsFile)
	if err != nil {
		return err
	}
	i := teamIndex(teams, uid)
	if i < 0 || teams[i].State != model.TeamEnding {
		return nil
	}
	t := teams[i]
	var sent model.AgentMessageSent
	_, err = g.call(ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		shown, err := c.showIssue(ctx, t.ProjectID, t.ParentUID)
		if err != nil {
			return nil, daemonError(err)
		}
		sent, err = c.SendAgentMessage(ctx, model.SendAgentMessage{ProjectID: t.ProjectID, UID: t.ParentUID, Actor: cmd.Actor,
			Address: t.CoordinatorAddress, Message: TeamEndMessage(t, shown.Children)})
		return nil, err
	})
	if err != nil || sent.Held != "" {
		return nil //nolint:nilerr // the next drive tries again
	}
	_, err = UpdateTeams(teamsFile, func(list []model.Team) []model.Team {
		if i := teamIndex(list, uid); i >= 0 && list[i].State == model.TeamEnding {
			return slices.Delete(list, i, i+1)
		}
		return list
	})
	return err
}

// TeamEndMessage is the one message the coordinator gets when its team has no child left to start and none running: the children
// closed and those still open, as the parent's children read now.
func TeamEndMessage(t model.Team, children []gen.IssueOut) string {
	var closed, open []string
	for _, c := range children {
		label := c.QualifiedID + " " + c.Title
		if c.Status == "closed" {
			closed = append(closed, label)
		} else {
			open = append(open, label)
		}
	}
	list := func(items []string) string {
		if len(items) == 0 {
			return "none"
		}
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("The team on %s has no child left to start and none is running. Children closed (%d): %s. Children still open (%d): %s.",
		t.ParentRef, len(closed), list(closed), len(open), list(open))
}

// ReadTeamFiles reads what `tern-kata team` prints besides the parent's children: teams.json, agents.json and the latest relay.
func (g *Gateway) ReadTeamFiles() ([]model.Team, []model.AgentEntry, []model.AgentState, error) {
	teamsFile, agentsFile, err := g.teamPaths()
	if err != nil {
		return nil, nil, nil, err
	}
	teams, err := ReadTeams(teamsFile)
	if err != nil {
		return nil, nil, nil, err
	}
	entries, err := ReadAgents(agentsFile)
	if err != nil {
		return nil, nil, nil, err
	}
	states, err := readAgentStates(filepath.Join(filepath.Dir(agentsFile), AgentStatesFile))
	return teams, entries, states, err
}

// ReadTeamKids reads the children of the team's parent and which of them are ready, as the queue reads them.
func (c *Client) ReadTeamKids(ctx context.Context, hostID string, cmd model.LoadTeamKids) (model.TeamKidsLoaded, error) {
	shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.ParentUID)
	if err != nil {
		return model.TeamKidsLoaded{}, daemonError(err)
	}
	unowned := true
	r, err := c.ReadyIssuesWithResponse(ctx, &gen.ReadyIssuesRequestOptions{
		PathParams: &gen.ReadyIssuesPath{ProjectID: cmd.ProjectID}, Query: &gen.ReadyIssuesQuery{Unowned: &unowned}})
	if err != nil {
		return model.TeamKidsLoaded{}, daemonError(err)
	}
	if r.JSON200 == nil {
		return model.TeamKidsLoaded{}, errors.New("kata returned no ready Issues")
	}
	ready := make(map[string]bool, len(r.JSON200.Issues))
	for _, i := range r.JSON200.Issues {
		ready[i.UID] = true
	}
	kids := make([]model.TeamKid, 0, len(shown.Children))
	for _, i := range shown.Children {
		kids = append(kids, model.TeamKid{UID: i.UID, Ref: i.QualifiedID, ProjectID: i.ProjectID, Closed: i.Status == "closed", Ready: ready[i.UID],
			RemoteHost: model.RemoteHostOn(i.Metadata, hostID)})
	}
	return model.TeamKidsLoaded{Kids: kids}, nil
}

// ResolveComment finds the comment on the Issue whose handle is cmd.Handle.
func (c *Client) ResolveComment(ctx context.Context, cmd model.ResolveComment) (model.CommentResolved, error) {
	shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
	if err != nil {
		return model.CommentResolved{}, daemonError(err)
	}
	want := model.CommentHandle(cmd.Handle)
	for _, comment := range shown.Comments {
		if model.CommentHandle(comment.UID) == want {
			return model.CommentResolved{CommentUID: comment.UID}, nil
		}
	}
	return model.CommentResolved{}, model.ErrNoSuchComment
}

// settleClaims resolves the children recorded without a block for longer than model.TeamPendingMaxAge: a Pane that died after starting
// the agent but before recording its block left one whose agent runs, which takes that agent's block (an entry on the child's Issue
// that this team's coordinator started after the claim; an agent a person started by hand is not the team's); a claim with no such
// agent goes, so the child can be started.
func settleClaims(t *model.Team, entries []model.AgentEntry, now time.Time) {
	for j := range t.Children {
		child := &t.Children[j]
		if child.Block != "" || now.Sub(child.StartedAt) < model.TeamPendingMaxAge {
			continue
		}
		if k := slices.IndexFunc(entries, func(e model.AgentEntry) bool {
			return e.IssueUID == child.UID && t.CoordinatorAddress != "" && e.Coordinator == t.CoordinatorAddress && !e.StartedAt.Before(child.StartedAt)
		}); k >= 0 {
			child.Block = entries[k].Block
		}
	}
	t.Children = slices.DeleteFunc(t.Children, func(child model.TeamChild) bool {
		return child.Block == "" && now.Sub(child.StartedAt) >= model.TeamPendingMaxAge
	})
}
