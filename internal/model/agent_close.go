package model

import (
	"fmt"
	"maps"
	"slices"
)

// CloseAgent closes a finished background agent's block with `tern close` and then removes its entry and prompt file.
type CloseAgent struct{ Block string }

func (CloseAgent) cmd() {}

// AgentClosed answers CloseAgent. Agents is agents.json after the entry went; Err carries Tern's words for a close that failed,
// and then the entry stays.
type AgentClosed struct {
	Block  string
	Agents []AgentEntry
	Err    error
}

func (AgentClosed) msg() {}

// ReadAgentIssue reads the status of an Issue the Collection does not list, through the Daemon, by its Project and UID.
type ReadAgentIssue struct {
	Workspace string
	ProjectID int64
	UID       string
}

func (ReadAgentIssue) cmd() {}

// AgentIssueRead answers ReadAgentIssue with the Issue's Status, "" when the read failed.
type AgentIssueRead struct {
	UID    string
	Status string
	Err    error
}

func (AgentIssueRead) msg() {}

// agentCloseState is what the rule for ending a finished Background agent remembers.
type agentCloseState struct {
	// tried are the blocks whose close is in flight or has failed; none is closed twice.
	tried []string
	// reading are the Issues (by UID) a status read is in flight for.
	reading []string
	// known is the status the last read gave an Issue the Collection does not list.
	known map[string]string
}

type agentCloseTrigger int

const (
	// closeOnRelay is a relayed agent list.
	closeOnRelay agentCloseTrigger = iota
	// closeOnIssues is the Issues changing: a snapshot or a stream event.
	closeOnIssues
	// closeOnRead is a status read coming back.
	closeOnRead
)

func agentFinished(states []AgentState, block string) bool {
	for _, s := range states {
		if s.Block == block {
			return s.State == AgentIdle || s.State == AgentExited
		}
	}
	return false
}

// closeFinishedAgents is the rule for ending a Background agent: the Pane that started it closes its block once the agent is in the
// background and not open, its Issue is closed and the relay shows it idle or exited. An Issue the Collection does not list is
// read through the Daemon by its Project and UID, and only for an agent that just finished, when the Issues change or while its
// status is not known; never while a read of it is in flight.
func closeFinishedAgents(m Model, trigger agentCloseTrigger) (Model, []Cmd) {
	if m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if !e.Background || e.Open || e.PaneBlock != m.PaneBlock ||
			!agentFinished(m.Agents.States, e.Block) || slices.Contains(m.AgentClose.tried, e.Block) {
			continue
		}
		status, listed := "", false
		if issue := m.issueByUID(e.IssueUID); issue != nil {
			status, listed = issue.Status, true
		} else {
			status = m.AgentClose.known[e.IssueUID]
		}
		switch {
		case status == "closed":
			m.AgentClose.tried = append(slices.Clone(m.AgentClose.tried), e.Block)
			cmds = append(cmds, CloseAgent{Block: e.Block})
		case listed || e.ProjectID == 0 || slices.Contains(m.AgentClose.reading, e.IssueUID):
		case status == "" || trigger == closeOnIssues || (trigger == closeOnRelay && !agentFinished(m.Agents.Previous, e.Block)):
			m.AgentClose.reading = append(slices.Clone(m.AgentClose.reading), e.IssueUID)
			cmds = append(cmds, ReadAgentIssue{Workspace: m.Workspace, ProjectID: e.ProjectID, UID: e.IssueUID})
		}
	}
	return m, cmds
}

func agentIssueRead(m Model, msg AgentIssueRead) (Model, []Cmd) {
	m.AgentClose.reading = slices.DeleteFunc(slices.Clone(m.AgentClose.reading), func(uid string) bool { return uid == msg.UID })
	if msg.Err != nil {
		return m, nil
	}
	m.AgentClose.known = maps.Clone(m.AgentClose.known)
	if m.AgentClose.known == nil {
		m.AgentClose.known = map[string]string{}
	}
	m.AgentClose.known[msg.UID] = msg.Status
	return closeFinishedAgents(m, closeOnRead)
}

func agentClosed(m Model, msg AgentClosed) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot close the agent: %s", msg.Err)
		return m
	}
	m.AgentMapping = msg.Agents
	return m
}
