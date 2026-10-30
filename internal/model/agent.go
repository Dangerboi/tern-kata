package model

import (
	"fmt"
	"time"
)

// AgentPlacement is where a started agent's block goes.
type AgentPlacement string

const (
	// AgentSplit is `w`: a split right of the kata Pane.
	AgentSplit AgentPlacement = "split"
	// AgentTab is `W`: a new tab.
	AgentTab AgentPlacement = "tab"
)

// The keys that start an agent on the selected Issue.
const (
	StartAgentSplitAction Action = "start-agent-split"
	StartAgentTabAction   Action = "start-agent-tab"
)

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
	Err       error
}

func (AgentStarted) msg() {}

// PruneAgents asks for agents.json to lose the entries whose block `tern ls --json` no longer lists; the Pane sends it once, at start.
type PruneAgents struct{}

func (PruneAgents) cmd() {}

// AgentsPruned answers PruneAgents with the entries that remain, or the reason they could not be read.
type AgentsPruned struct {
	Agents []AgentEntry
	Err    error
}

func (AgentsPruned) msg() {}

// ClosedIssueRefusal is the Notice a start key shows on a closed Issue.
const ClosedIssueRefusal = "This Issue is closed; reopen it first (r)."

// openAgent is `w` and `W`: the Agent card when the selected Issue has agents, else a new agent.
func openAgent(placement AgentPlacement) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		if issue := closeIssue(m); issue != nil && len(m.IssueAgents(issue.UID)) > 0 {
			return openAgentCard(m, *issue, placement), nil
		}
		return startAgent(placement)(m)
	}
}

// startAgent starts an agent on the selected Issue unless the Issue is closed.
func startAgent(placement AgentPlacement) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		issue := closeIssue(m)
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
}

func agentStarted(m Model, msg AgentStarted) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot start an agent on %s: %s", msg.IssueRef, msg.Err)
		return m
	}
	m.AgentMapping = append(m.AgentMapping, msg.Entry)
	where := "a split beside this Pane"
	if msg.Placement == AgentTab {
		where = "a new tab"
	}
	m.Notice = fmt.Sprintf("Started an agent on %s in %s.", msg.IssueRef, where)
	return m
}

func agentsPruned(m Model, msg AgentsPruned) Model {
	if msg.Err == nil {
		m.AgentMapping = msg.Agents
	}
	return m
}
