package model

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// A Team is one coordinator agent on a parent Issue and a queue of Background agents on its ready children, at most Limit running
// at once. teams.json, beside agents.json, holds one record per team; any Pane on the host drives every record (DriveTeams).

type TeamState string

const (
	// TeamRunning starts children while fewer than the limit run.
	TeamRunning TeamState = "running"
	// TeamPaused starts nothing and never ends by itself.
	TeamPaused TeamState = "paused"
	// TeamStopping is a team whose coordinator exited: nothing new starts and the record goes once none of its children runs.
	TeamStopping TeamState = "stopping"
	// TeamEnding is a team with no child left to start and none running whose end message waits for the coordinator's unread
	// message to clear.
	TeamEnding TeamState = "ending"
)

// The children a team runs at once: the start dialog's range and default.
const (
	TeamMinLimit     = 1
	TeamMaxLimit     = 8
	TeamDefaultLimit = 2
)

// TeamChild is a child the team started. Block is "" while the start is recorded and the agent not yet started.
type TeamChild struct {
	UID       string    `json:"uid"`
	Ref       string    `json:"ref"`
	Block     string    `json:"block"`
	StartedAt time.Time `json:"started_at"`
}

// ClaimExpired says the child was recorded without a block for TeamPendingMaxAge or longer: a Pane died between the record and the
// start, so the child counts as neither running nor started.
func (c TeamChild) ClaimExpired(now time.Time) bool {
	return c.Block == "" && now.Sub(c.StartedAt) >= TeamPendingMaxAge
}

type TeamChildRef struct{ Ref, Title string }

// TeamRole is what an agent's prompt says of its team. A coordinator has Coordinating, the Project's open Children at the start
// and the Limit; a child has Coordinator (the coordinator's Agent address) and Parent (the parent's qualified ref). The zero value
// is an agent on no team.
type TeamRole struct {
	Coordinating bool
	Coordinator  string
	Parent       string
	Children     []TeamChildRef
	Limit        int
}

// Team is one record of teams.json. The coordinator runs on the parent Issue (ParentUID) in the block CoordinatorBlock, at the
// Agent address CoordinatorAddress. Project is the parent's Project: only its children are started.
type Team struct {
	ParentUID          string `json:"parent_uid"`
	ParentRef          string `json:"parent_ref"`
	ProjectID          int64  `json:"project_id"`
	Project            string `json:"project"`
	CoordinatorBlock   string `json:"coordinator_block"`
	CoordinatorAddress string `json:"coordinator_address"`
	// Actor is the Pane's actor that started the team; only a Pane of that actor drives it. "" in a record written before it was kept, which any Pane drives.
	Actor    string      `json:"actor,omitempty"`
	Limit    int         `json:"limit"`
	State    TeamState   `json:"state"`
	Children []TeamChild `json:"children"`
}

func (t Team) Started(uid string) bool {
	return slices.ContainsFunc(t.Children, func(c TeamChild) bool { return c.UID == uid })
}

func (m Model) TeamOn(uid string) (Team, bool) {
	for _, t := range m.Teams {
		if t.ParentUID == uid {
			return t, true
		}
	}
	return Team{}, false
}

// StartTeam asks the gateway to start a team on the Issue: the coordinator where Placement says, then the record in teams.json.
// The fields besides Limit are StartAgent's. The gateway also refuses what only agent.toml and the Issue's children can tell:
// a command that is not omp, an agent name too long for a teammate, no open child in the Issue's own Project.
type StartTeam struct {
	Placement                         AgentPlacement
	IssueUID, IssueRef, IssueTitle    string
	ProjectID                         int64
	ProjectName, Workspace, PaneBlock string
	Actor                             string
	Limit                             int
}

func (StartTeam) cmd() {}

type TeamStarted struct {
	Placement AgentPlacement
	IssueRef  string
	Entry     AgentEntry
	Note      string
	Err       error
}

func (TeamStarted) msg() {}

// DriveTeams asks the gateway to move every team in teams.json forward: end the record of a team whose coordinator exited,
// start the next ready child while fewer than the limit run, write the end message. Exited are the blocks the latest relay
// listed as exited; the entries come from agents.json as it stands. Workspace reaches the daemon, BoundProjectID is the Project
// of the Pane's Binding (0 when unbound), whose children start in the Pane's workspace.
type DriveTeams struct {
	Workspace      string
	PaneBlock      string
	Actor          string
	BoundProjectID int64
	Exited         []string
}

func (DriveTeams) cmd() {}

// TeamsDriven answers DriveTeams: the records as they stand now, the agents it started (Background children) and what went
// wrong. Err is teams.json or agents.json failing; Failures are children that could not start, each worded for a Notice.
type TeamsDriven struct {
	Teams    []Team
	Started  []AgentEntry
	Failures []string
	Err      error
}

func (TeamsDriven) msg() {}

// teamDriveState keeps one DriveTeams in flight at a time; a trigger that arrives meanwhile is answered by one more drive.
type teamDriveState struct {
	inFlight, again bool
}

// driveTeams asks for a DriveTeams once the Pane knows the agents (the mapping and the first relay), or marks that one more is
// owed when one is out. A relay (probe false) drives only a Pane that knows of a team; a stream event, the Agent card opening and
// a team starting (probe true) also look for teams another Pane wrote, so a Pane with no team adds no command to a relay.
func driveTeams(m Model, probe bool) (Model, []Cmd) {
	if !m.agentsKnown() || m.Actor == "" || !probe && len(m.Teams) == 0 {
		return m, nil
	}
	if m.teamDrive.inFlight {
		m.teamDrive.again = true
		return m, nil
	}
	m.teamDrive.inFlight = true
	cmd := DriveTeams{Workspace: m.Workspace, PaneBlock: m.PaneBlock, Actor: m.MessageActor()}
	if m.Bound {
		cmd.BoundProjectID = m.Project.ID
	}
	for _, s := range m.Agents.States {
		if s.State == AgentExited {
			cmd.Exited = append(cmd.Exited, s.Block)
		}
	}
	return m, []Cmd{cmd}
}

func teamsDriven(m Model, msg TeamsDriven) (Model, []Cmd) {
	m.teamDrive.inFlight = false
	if msg.Err == nil {
		m.Teams = msg.Teams
	}
	for _, e := range msg.Started {
		m = m.mapStarted(e)
	}
	switch {
	case len(msg.Failures) > 0:
		m.Notice = msg.Failures[0]
	case msg.Err != nil:
		m.Notice = fmt.Sprintf("Cannot drive the teams: %s", msg.Err)
	}
	var cmds []Cmd
	if m.teamDrive.again {
		m.teamDrive.again = false
		m, cmds = driveTeams(m, true)
	}
	return restartAgentSessions(m, cmds)
}

// mapStarted files a freshly started agent in the mapping, so a refresh asked before it started keeps it.
func (m Model) mapStarted(e AgentEntry) Model {
	if m.agentMapped(e.Block) {
		return m
	}
	m.AgentMapping = append(slices.Clone(m.AgentMapping), e)
	m.agentFresh = maps.Clone(m.agentFresh)
	if m.agentFresh == nil {
		m.agentFresh = map[string]int64{}
	}
	m.agentFresh[e.Block] = m.agentAsks
	return m
}

func teamStarted(m Model, msg TeamStarted) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot start a team on %s: %s", msg.IssueRef, msg.Err)
		return m, nil
	}
	m = m.mapStarted(msg.Entry)
	_, where := msg.Placement.words()
	m.Notice = fmt.Sprintf("Started a team on %s: its coordinator is in %s.", msg.IssueRef, where)
	if msg.Note != "" {
		m.Notice += " But " + msg.Note
	}
	m, cmds := driveTeams(m, true)
	return restartAgentSessions(m, cmds)
}
