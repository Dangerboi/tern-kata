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
// and then the entry stays. Kept says the entry was open in agents.json as it stands now, or no background agent there, so
// nothing was closed and Agents is the file as read.
type AgentClosed struct {
	Block  string
	Agents []AgentEntry
	Kept   bool
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
	// refused are the blocks the gateway kept because the file did not call them background agents; one is asked about again
	// as soon as the mapping shows it as one, by whatever path the mapping changed.
	refused []string
	// reading are the Issues (by UID) a status read is in flight for.
	reading []string
	// unreadable are the Issues (by UID) whose last status read failed; none is read again until the event stream reconnects or the Pane
	// switches Daemon.
	unreadable []string
	// known is the status the last read gave an Issue the Collection does not list.
	known map[string]string
}

// forgetUnreadable lets the Issues whose status read failed be read again: the Daemon that failed them may be back.
func (s agentCloseState) forgetUnreadable() agentCloseState {
	s.unreadable = nil
	return s
}

// forDaemon is the state after the Pane connects to a different Daemon: its Issues are not the old Daemon's, so no read in flight,
// known status or failed read carries over. Which blocks were closed or refused is about agents, not Issues, and stays.
func (s agentCloseState) forDaemon() agentCloseState {
	s.reading, s.known = nil, nil
	return s.forgetUnreadable()
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

// closeFinishedAgents is the rule for ending a Background agent: the Pane that started it closes its block once the agent is not
// open (by its entry and, for a background agent, by the relay, which shows an agent another Pane opened), its Issue is closed and
// the relay shows it idle or exited. A split or tab agent is asked about once too, since another Pane may have sent it to the
// background without any relay saying so: the gateway reads agents.json and closes only a background agent. An Issue the
// Collection does not list is read through the Daemon by its Project and UID, and only for an agent that just finished, when the
// Issues change or while its status is not known; never while a read of it is in flight.
func closeFinishedAgents(m Model, trigger agentCloseTrigger) (Model, []Cmd) {
	if m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if e.Open || (e.Background && m.Agents.seenShown(e.Block)) || e.PaneBlock != m.PaneBlock ||
			!agentFinished(m.Agents.States, e.Block) || slices.Contains(m.AgentClose.tried, e.Block) ||
			(!e.Background && slices.Contains(m.AgentClose.refused, e.Block)) {
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
		case listed || e.ProjectID == 0 || slices.Contains(m.AgentClose.reading, e.IssueUID) || slices.Contains(m.AgentClose.unreadable, e.IssueUID):
		case status == "" || trigger == closeOnIssues || (trigger == closeOnRelay && m.Agents.enteredFinished(e.Block)):
			m.AgentClose.reading = append(slices.Clone(m.AgentClose.reading), e.IssueUID)
			cmds = append(cmds, ReadAgentIssue{Workspace: m.Workspace, ProjectID: e.ProjectID, UID: e.IssueUID})
		}
	}
	return m, cmds
}

func agentIssueRead(m Model, msg AgentIssueRead) (Model, []Cmd) {
	m.AgentClose.reading = slices.DeleteFunc(slices.Clone(m.AgentClose.reading), func(uid string) bool { return uid == msg.UID })
	if msg.Err != nil {
		// An Issue that can't be read is not read again, and nothing is raised for an agent gone from it.
		m.AgentClose.unreadable = append(slices.Clone(m.AgentClose.unreadable), msg.UID)
		m.AgentGone = slices.DeleteFunc(slices.Clone(m.AgentGone), func(g AgentEntry) bool { return g.IssueUID == msg.UID })
		return m, nil
	}
	m.AgentClose.known = maps.Clone(m.AgentClose.known)
	if m.AgentClose.known == nil {
		m.AgentClose.known = map[string]string{}
	}
	m.AgentClose.known[msg.UID] = msg.Status
	m, raised := raiseAgentAttention(m, false)
	m, ended := closeFinishedAgents(m, closeOnRead)
	return m, append(raised, ended...)
}

func agentClosed(m Model, msg AgentClosed) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot close the agent: %s", msg.Err)
		return m
	}
	if msg.Kept {
		// An agent the file does not call background is refused until the mapping shows it as one.
		m.AgentClose.tried = slices.DeleteFunc(slices.Clone(m.AgentClose.tried), func(b string) bool { return b == msg.Block })
		if !slices.ContainsFunc(msg.Agents, func(e AgentEntry) bool { return e.Block == msg.Block && e.Background }) &&
			!slices.Contains(m.AgentClose.refused, msg.Block) {
			m.AgentClose.refused = append(slices.Clone(m.AgentClose.refused), msg.Block)
		}
	}
	m.AgentMapping = msg.Agents
	return m
}

// StopAgent is the Agent card's Stop: it closes the agent's block with `tern close` and removes its entry and prompt file, whatever
// the agent's placement and whichever Pane started it. The entry is read from agents.json again just before the close.
type StopAgent struct{ Block string }

func (StopAgent) cmd() {}

// AgentStopped answers StopAgent. Agents is agents.json after the entry went; Err carries Tern's words for a close that failed, and
// then the entry stays.
type AgentStopped struct {
	Block  string
	Agents []AgentEntry
	Err    error
}

func (AgentStopped) msg() {}

// InterruptAgent is the Agent card's Interrupt: the window half interrupts the agent's current turn and keeps its block.
type InterruptAgent struct {
	Block string
	// PromptFile is the agent's prompt file, which names the agent in a window that numbers panes differently from this host; "" when the entry has none.
	PromptFile string
}

func (InterruptAgent) cmd() {}

// AgentInterrupted answers InterruptAgent; Err says the request could not be sent to the window half.
type AgentInterrupted struct {
	Block string
	Err   error
}

func (AgentInterrupted) msg() {}

// agentStopped takes in the mapping after a stop. A stopped agent this Pane started counts as one whose block is gone, so its Issue
// gets what an exit gets: the request "agent exited without closing <ref>" while the Issue is open.
func agentStopped(m Model, msg AgentStopped) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot stop the agent: %s", msg.Err)
		return m, nil
	}
	stopped := AgentLabel(AgentEntry{Block: msg.Block})
	for _, e := range m.AgentMapping {
		if e.Block == msg.Block {
			stopped = AgentLabel(e)
		}
	}
	m = m.noteGoneAgents(msg.Agents)
	m.AgentMapping = msg.Agents
	m.Agents = m.Agents.trimmed(m.agentMapped)
	m.Notice = "Stopped " + stopped + "."
	return raiseAgentAttention(m, false)
}

func agentInterrupted(m Model, msg AgentInterrupted) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot interrupt the agent: %s", msg.Err)
	}
	return m
}
