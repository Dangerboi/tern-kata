package model

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RemoteAgent is an agent another host started, as the Agent record on its Issue names it (ADR-0018). It has no relayed state: the
// record is static, and the Pane can only message it or Forget its record.
type RemoteAgent struct {
	AgentRecord
	// Unread is the first line of the message the Issue's metadata holds at the agent's address, which it has not cleared; "".
	Unread string
}

// Key is the metadata key of the agent's record, which names its row on the Agent card.
func (r RemoteAgent) Key() string { return AgentRecordKey(r.HostID, r.Block) }

// HostLabel is the host as the Agent card and the badge name it: the host name, else the Host id.
func (r RemoteAgent) HostLabel() string { return cmp.Or(r.Host, r.HostID) }

// remoteAgentsIn reads the records in metadata that another host (a Host id other than hostID) published. With no Host id of
// its own the Pane cannot tell its own records from the others', so it lists none.
func remoteAgentsIn(metadata map[string]any, hostID string) []RemoteAgent {
	if hostID == "" {
		return nil
	}
	var agents []RemoteAgent
	for _, r := range AgentRecords(metadata) {
		if r.HostID != hostID {
			agents = append(agents, RemoteAgent{AgentRecord: r, Unread: unreadMessage(metadata, r.Address)})
		}
	}
	return agents
}

// RemoteHostOn is the label of the host of an agent another host id published on an Issue with this metadata, "" when there is none
// (or hostID is not known).
func RemoteHostOn(metadata map[string]any, hostID string) string {
	if agents := remoteAgentsIn(metadata, hostID); len(agents) > 0 {
		return agents[0].HostLabel()
	}
	return ""
}

// RemoteAgents lists the agents other hosts started on the Issue, oldest start first.
func (m Model) RemoteAgents(issueUID string) []RemoteAgent {
	issue := m.issueByUID(issueUID)
	if issue == nil {
		return nil
	}
	return remoteAgentsIn(issue.Metadata, m.HostID)
}

// RemoteAgentRefusal is why an action only a local agent takes (go to it, interrupt, stop, send to the background) is off for a
// remote one.
func RemoteAgentRefusal(host string) string {
	return fmt.Sprintf("That agent runs on %s; this Pane can only message it or forget it.", host)
}

// remoteMessageRefusal is why a message reaches no remote agent r, "" when it can: it is not omp, it has no address, or the sender
// rule refuses (the agent is on another host, so the daemon must verify actors).
func (m Model) remoteMessageRefusal(r RemoteAgent) string {
	switch {
	case r.NoAddress == AgentNotOmp:
		return AgentNotOmpRefusal
	case r.Address == "":
		return AgentWithoutAddressRefusal
	}
	return MessageSenderRefusal(m.Actor, cmp.Or(r.Actor, AgentAddressActor(r.Address)), r.MessageFrom, true, m.VerifiedSenders)
}

// newestRemoteAgentOn is the agent other hosts published on an Issue, among those match accepts, that started last.
func (m Model) newestRemoteAgentOn(issueUID string, match func(RemoteAgent) bool) (RemoteAgent, bool) {
	var target RemoteAgent
	found := false
	for _, r := range m.RemoteAgents(issueUID) {
		if match(r) && (!found || r.StartedAt.After(target.StartedAt)) {
			target, found = r, true
		}
	}
	return target, found
}

// AgentForgetLocalReason is why Forget is off on a row of this host's own agent.
const AgentForgetLocalReason = "Forget removes the record of an agent on another host; stop this agent with x."

// AddressUsedWords is what an agent's row says when another host's record held its address at start, so it published none.
func AddressUsedWords(host string) string {
	return "not published: " + host + " already uses its address"
}

// RemoteBadgeWords is what an Issue's badge says when only other hosts run agents on it: "agent on <host>", or "agents on N hosts".
func RemoteBadgeWords(remote []RemoteAgent) string {
	var hosts []string
	for _, r := range remote {
		if !slices.Contains(hosts, r.HostID) {
			hosts = append(hosts, r.HostID)
		}
	}
	if len(hosts) == 1 {
		return "agent on " + remote[0].HostLabel()
	}
	return fmt.Sprintf("agents on %d hosts", len(hosts))
}

// ForgetAgentRecord removes the Agent record of an agent on another host from its Issue, and clears any message left at its
// address. It never touches the other host. Actor is the Pane's, for kata's attribution.
type ForgetAgentRecord struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	// HostID and Block make the record's key; Address is the agent's address ("" when it had none).
	HostID  string
	Block   string
	Address string
	// Host names the host in the answer's words.
	Host string
}

func (ForgetAgentRecord) cmd() {}

// AgentRecordForgotten answers ForgetAgentRecord.
type AgentRecordForgotten struct {
	Host string
	Err  error
}

func (AgentRecordForgotten) msg() {}

func agentRecordForgotten(m Model, msg AgentRecordForgotten) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot forget the agent on %s: %s", msg.Host, msg.Err)
		return m
	}
	m.Notice = "Forgot the agent on " + msg.Host + "."
	return m
}

// ErrNoRemoteScope is why agents on other hosts cannot be listed without an Issue: no KATA_REF and no Project the workspace is
// bound to say where to look.
var ErrNoRemoteScope = errors.New("no Project to look in: give --issue, run it where KATA_REF is set, or run it in a workspace bound to a kata Project")

// LoadRemoteAgents asks for the agents other hosts published (Agent records) that `tern-kata agents` and `tern-kata message` list:
// with Issue, the records on that Issue (any status); without it, on the open Issues of the Project ScopeRef (KATA_REF) names, else
// the Project the workspace is bound to. Records of this host's own id are left out; its agents come from agents.json.
type LoadRemoteAgents struct {
	Workspace string
	Issue     string
	ScopeRef  string
}

func (LoadRemoteAgents) cmd() {}

// RemoteAgentsLoaded answers LoadRemoteAgents with the remote agents, oldest start first within each Issue. NotFound says Issue
// names no Issue, which `message` takes to mean its target was an address; Err is every other failure.
type RemoteAgentsLoaded struct {
	Agents   []IssueAgent
	NotFound bool
	Err      error
}

func (RemoteAgentsLoaded) msg() {}

// RemoteIssueAgent is the agent an Agent record publishes on the Issue with ref, uid and projectID. Its state is unknown: records
// carry none.
func RemoteIssueAgent(r AgentRecord, issueRef, issueUID string, projectID int64) IssueAgent {
	return IssueAgent{
		AgentEntry: AgentEntry{
			IssueRef: issueRef, IssueUID: issueUID, ProjectID: projectID, Block: r.Block, Teammate: r.Name, Address: r.Address,
			StartedAt: r.StartedAt, NoAddress: r.NoAddress, MessageFrom: strings.Join(r.MessageFrom, ","), Actor: r.Actor,
		},
		Host: r.Host, HostID: r.HostID,
	}
}

// Remote says the agent runs on another host.
func (a IssueAgent) Remote() bool { return a.HostID != "" }
