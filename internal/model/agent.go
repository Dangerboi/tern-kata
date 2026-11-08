package model

import (
	"fmt"
	"strings"
	"time"
)

// AgentPlacement is where a started agent's block goes.
type AgentPlacement string

const (
	// AgentSplit is the Agent card's `s`: a split right of the kata Pane.
	AgentSplit AgentPlacement = "split"
	// AgentTab is the Agent card's `t`: a new tab.
	AgentTab AgentPlacement = "tab"
	// AgentBackground is the Agent card's `g`: a tab of the hidden session AgentSession, until the user opens it.
	AgentBackground AgentPlacement = "background"
)

// words is the placement's name, as the Agent card's where column writes it, and as a Notice says where the agent went. A
// placement not kept ("") reads as a split.
func (p AgentPlacement) words() (name, notice string) {
	switch p {
	case AgentTab:
		return string(AgentTab), "a new tab"
	case AgentBackground:
		return string(AgentBackground), "the background (open it from its badge)"
	}
	return string(AgentSplit), "a split beside this Pane"
}

// AgentSession is the name of the hidden Tern session background agents live in.
const AgentSession = "kata agents"

// OpenAgentCardAction is `w`: it opens the Agent card on the selected Issue.
const OpenAgentCardAction Action = "open-agent-card"

// AgentEntry is one line of agents.json, the Agent mapping: an agent this host's Panes started, found again by its block id.
type AgentEntry struct {
	IssueUID string `json:"issue_uid"`
	IssueRef string `json:"issue_ref"`
	// ProjectID is the Issue's Project, which the Attention request's write needs; 0 in an entry written before it was kept.
	ProjectID int64     `json:"project_id,omitempty"`
	Block     string    `json:"block"`
	StartedAt time.Time `json:"started_at"`
	// PromptFile is the file the agent was started with; it goes when the entry does.
	PromptFile string `json:"prompt_file"`
	// Teammate is the agent's name from agent.toml when it started, which its Attention requests carry; "" in an entry written before it was kept.
	Teammate string `json:"teammate,omitempty"`
	// PaneBlock is the block id of the kata Pane that started the agent; "" when that Pane could not name itself.
	PaneBlock string `json:"pane_block"`
	// Background says the agent belongs in the hidden session; Open says it is out of it right now.
	Background bool `json:"background"`
	Open       bool `json:"open"`
	// Placement is where the agent was started, split or tab; "" in an entry written before it was kept, which reads as a split.
	Placement AgentPlacement `json:"placement,omitempty"`
	// Address is the agent's own Agent address, <actor>/<name>-<block>, when it loads tern-kata's omp extension, which delivers what
	// is written to it; "" when it does not.
	Address string `json:"address,omitempty"`
	// NoAddress says why an agent has no Address; "" in an entry that has one or was written before it was kept.
	NoAddress AgentNoAddress `json:"no_address,omitempty"`
	// MessageFrom is the actors, besides the agent's own, whose messages it acts on (agent.toml's message_from when it started),
	// comma-separated as the wrapper passes them to its extension, so an entry stays comparable; MessageFromActors lists them. The
	// sender rule checks a message's sender against them.
	MessageFrom string `json:"message_from,omitempty"`
	// Actor is the Pane's actor when it started the agent, the one its Agent address and its record carry; "" in an entry written before it was kept.
	Actor string `json:"actor,omitempty"`
	// AddressUsedBy is the host name of another host whose Agent record already held this agent's Address when it started, so the agent
	// published no record of its own and stays local; "" when it published one.
	AddressUsedBy string `json:"address_used_by,omitempty"`
	// Coordinator is the Agent address of the coordinator of the team that started the agent as a child; "" for an agent no team started.
	Coordinator string `json:"coordinator,omitempty"`
}

// MessageFromActors is the actors in MessageFrom, none for ""; a zero or legacy entry takes messages from its own actor only.
func (e AgentEntry) MessageFromActors() []string {
	if e.MessageFrom == "" {
		return nil
	}
	return strings.Split(e.MessageFrom, ",")
}

// AgentNoAddress is why an agent started without tern-kata's omp extension, so has no Agent address and no message reaches it.
type AgentNoAddress string

const (
	// AgentNotOmp is an agent whose agent.toml command is not omp: the extension is omp's.
	AgentNotOmp AgentNoAddress = "not-omp"
)

// StartAgent asks the gateway to start an interactive agent on an Issue. Workspace is the Pane's workspace when the Issue's
// Project is the Binding and "" otherwise; the gateway picks the directory from it, agent.toml and worker.toml. Actor is the
// Pane's actor, the one the agent's Attention requests come from and the only sender whose messages it acts on.
type StartAgent struct {
	Placement                         AgentPlacement
	IssueUID, IssueRef, IssueTitle    string
	ProjectID                         int64
	ProjectName, Workspace, PaneBlock string
	Actor                             string
	// Coordinator is the Agent address of the coordinator of the team the agent is a child of; Coordinating marks the agent as a
	// team's coordinator (see AgentMessaging).
	Coordinator  string
	Coordinating bool
	// Role carries what the prompt says of the team: the parent's ref for a child, the children and the limit for the coordinator.
	Role TeamRole
}

func (StartAgent) cmd() {}

// AgentStarted answers StartAgent.
type AgentStarted struct {
	Placement AgentPlacement
	IssueRef  string
	Entry     AgentEntry
	// Note is a step after the start that failed (naming a background agent's tab); the agent runs.
	Note string
	Err  error
}

func (AgentStarted) msg() {}

// PruneAgents asks for agents.json to lose the entries whose block `tern ls --json` no longer lists; the Pane sends it once, at start.
// PaneBlock is this Pane's block: an entry whose starting Pane is no longer a live block is adopted by it ("" adopts nothing).
// Seq numbers the request in the order the Pane asked; the answer echoes it.
type PruneAgents struct {
	// Workspace is the Pane's: the records of this host id on the open Issues of its Project are reconciled with agents.json.
	Workspace string
	PaneBlock string
	Seq       int64
}

func (PruneAgents) cmd() {}

// AgentsPruned answers PruneAgents with the entries that remain, or the reason they could not be read.
type AgentsPruned struct {
	Agents []AgentEntry
	Err    error
	Seq    int64
}

func (AgentsPruned) msg() {}

// ClosedIssueRefusal is the Notice `w` shows on a closed Issue, and the reason the buttons that start an agent on one are off.
const ClosedIssueRefusal = "This Issue is closed; reopen it first (r)."

// ClosedAlreadyRefusal is the reason Close is off on a closed Issue.
const ClosedAlreadyRefusal = "This Issue is already closed."

// IssueRefusal is why action cannot run on issue, or "" when it can: the closed-Issue rules that the Notices of `w` and `x`, the
// Agent card's buttons and the context menu's rows all follow.
func IssueRefusal(issue Issue, action Action) string {
	if !IsClosed(issue) {
		return ""
	}
	switch action {
	case OpenAgentCardAction, AgentCardSplit, AgentCardTab, AgentCardNewBackground, AgentCardTeam:
		return ClosedIssueRefusal
	case OpenClose:
		return ClosedAlreadyRefusal
	}
	return ""
}

// openAgent is `w`: the Agent card for the selected Issue, with or without agents; a closed Issue gets the refusal and no card.
// agents.json is read again as it opens, since another Pane may have sent an agent to the background since the last relay.
func openAgent(m Model) (Model, []Cmd) {
	issue := closeIssue(m)
	if issue == nil {
		return m, nil
	}
	if reason := IssueRefusal(*issue, OpenAgentCardAction); reason != "" {
		m.Notice = reason
		return m, nil
	}
	m = openAgentCard(m, *issue)
	m, sessions := askAgentSessions(m)
	m, driven := driveTeams(m, true)
	sessions = append(sessions, driven...)
	if m.Agents.inFlight {
		m.Agents.again = true
		return m, sessions
	}
	m.Agents.inFlight = true
	m, ask := m.askRefresh(nil)
	return m, append([]Cmd{ask}, sessions...)
}

// startAgent starts an agent on the Agent card's Issue where the card's key (action) says; the card closes first, and a refusal
// of the card's rules is the Notice.
func startAgent(m Model, action Action, placement AgentPlacement) (Model, []Cmd) {
	reason := m.AgentCardUnavailable(action)
	issue := m.issueByUID(m.AgentCard.IssueUID)
	m.AgentCard = AgentCardState{}
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	cmd := StartAgent{Placement: placement, IssueUID: issue.UID, IssueRef: issue.QualifiedID, IssueTitle: issue.Title, ProjectID: issue.ProjectID,
		ProjectName: issue.ProjectName, PaneBlock: m.PaneBlock, Actor: m.Actor}
	if m.Bound && issue.ProjectID == m.Project.ID {
		cmd.Workspace = m.Workspace
	}
	m.Notice = fmt.Sprintf("Starting an agent on %s…", issue.QualifiedID)
	return m, []Cmd{cmd}
}

func agentStarted(m Model, msg AgentStarted) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot start an agent on %s: %s", msg.IssueRef, msg.Err)
		return m
	}
	m = m.mapStarted(msg.Entry)
	_, where := msg.Placement.words()
	m.Notice = fmt.Sprintf("Started an agent on %s in %s.", msg.IssueRef, where)
	if msg.Note != "" {
		m.Notice += " But " + msg.Note
	}
	return m
}

// agentsPruned takes in the entries even when tern could not be asked: they are the file as read, as a later refresh keeps them.
func agentsPruned(m Model, msg AgentsPruned) (Model, []Cmd) {
	next, m := m.keepFresh(msg.Agents, msg.Seq)
	m.AgentMapping = next
	m.agentsPruned = true
	m.Agents = m.Agents.trimmed(m.agentMapped)
	m, raised := raiseAgentAttention(m, false)
	m, swept := runPendingSweep(m)
	m, driven := driveTeams(m, false)
	return m, append(append(raised, swept...), driven...)
}
