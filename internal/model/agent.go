package model

import (
	"fmt"
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
	// PaneBlock is the block id of the kata Pane that started the agent; "" when that Pane could not name itself.
	PaneBlock string `json:"pane_block"`
	// Background says the agent belongs in the hidden session; Open says it is out of it right now.
	Background bool `json:"background"`
	Open       bool `json:"open"`
	// Placement is where the agent was started, split or tab; "" in an entry written before it was kept, which reads as a split.
	Placement AgentPlacement `json:"placement,omitempty"`
}

// StartAgent asks the gateway to start an interactive agent on an Issue. Workspace is the Pane's workspace when the Issue's
// Project is the Binding and "" otherwise; the gateway picks the directory from it, agent.toml and worker.toml.
type StartAgent struct {
	Placement                         AgentPlacement
	IssueUID, IssueRef, IssueTitle    string
	ProjectID                         int64
	ProjectName, Workspace, PaneBlock string
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
type PruneAgents struct{ PaneBlock string }

func (PruneAgents) cmd() {}

// AgentsPruned answers PruneAgents with the entries that remain, or the reason they could not be read.
type AgentsPruned struct {
	Agents []AgentEntry
	Err    error
}

func (AgentsPruned) msg() {}

// ClosedIssueRefusal is the Notice `w` shows on a closed Issue.
const ClosedIssueRefusal = "This Issue is closed; reopen it first (r)."

// openAgent is `w`: the Agent card for the selected Issue, with or without agents; a closed Issue gets the refusal and no card.
// agents.json is read again as it opens, since another Pane may have sent an agent to the background since the last relay.
func openAgent(m Model) (Model, []Cmd) {
	issue := closeIssue(m)
	if issue == nil {
		return m, nil
	}
	if IsClosed(*issue) {
		m.Notice = ClosedIssueRefusal
		return m, nil
	}
	m = openAgentCard(m, *issue)
	if m.Agents.inFlight {
		m.Agents.again = true
		return m, nil
	}
	m.Agents.inFlight = true
	return m, []Cmd{RefreshAgents{PaneBlock: m.PaneBlock}}
}

// startAgent starts an agent on the Agent card's Issue where the card's key says; the card closes first.
func startAgent(m Model, placement AgentPlacement) (Model, []Cmd) {
	card := m.AgentCard
	m.AgentCard = AgentCardState{}
	issue := m.issueByUID(card.IssueUID)
	if issue == nil {
		return m, nil
	}
	if IsClosed(*issue) {
		m.Notice = ClosedIssueRefusal
		return m, nil
	}
	cmd := StartAgent{Placement: placement, IssueUID: issue.UID, IssueRef: issue.QualifiedID, IssueTitle: issue.Title, ProjectID: issue.ProjectID,
		ProjectName: issue.ProjectName, PaneBlock: m.PaneBlock}
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
	m.AgentMapping = append(m.AgentMapping, msg.Entry)
	_, where := msg.Placement.words()
	m.Notice = fmt.Sprintf("Started an agent on %s in %s.", msg.IssueRef, where)
	if msg.Note != "" {
		m.Notice += " But " + msg.Note
	}
	return m
}

// agentsPruned takes in the entries even when tern could not be asked: they are the file as read, as a later refresh keeps them.
func agentsPruned(m Model, msg AgentsPruned) (Model, []Cmd) {
	before := m.AgentMapping
	m.AgentMapping = msg.Agents
	m.Agents = m.Agents.trimmed(m.agentMapped)
	return raiseAgentAttention(m, before, false)
}
