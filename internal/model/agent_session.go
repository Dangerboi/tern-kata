package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// What the Agent card shows of each omp agent besides its relayed state comes from the agent's session file: its model, the tool
// calls so far, the tool it is running now and the cost so far. The Pane reads a bounded tail of the file while the card is open.

// AgentSessionEvery is how often the open Agent card reads the sessions again.
const AgentSessionEvery = 2 * time.Second

// AgentSessionAsk names one agent whose session is to be read: its block and the prompt file the session path file sits beside.
// Only omp agents are asked about; an agent the Pane did not start as omp has no session to read.
type AgentSessionAsk struct {
	Block      string
	PromptFile string
}

// ReadAgentSessions asks for the sessions of Agents, After a pause when After is set (the refresh while the card stays open).
// Request numbers the asks.
type ReadAgentSessions struct {
	Request int64
	After   time.Duration
	Agents  []AgentSessionAsk
}

func (ReadAgentSessions) cmd() {}

// AgentSessionFacts is what one agent's session file says. Reason is why it says nothing ("" when it does); a muted line the card
// shows in place of the facts.
type AgentSessionFacts struct {
	Block  string
	Reason string
	// Model is the model of the session's last assistant message.
	Model string
	// ToolCalls is the number of tool calls in the tail read; CurrentTool the tool of the last call with no result after it.
	ToolCalls   int
	CurrentTool string
	// Cost is the sum of the assistant messages' usage.cost.total in the tail read; HasCost is false when none reported it.
	Cost    float64
	HasCost bool
	// Partial says the file was longer than the tail read, so ToolCalls and Cost count only what the tail held.
	Partial bool
}

// AgentSessionsRead answers ReadAgentSessions.
type AgentSessionsRead struct {
	Request  int64
	Sessions []AgentSessionFacts
}

func (AgentSessionsRead) msg() {}

// agentSessionState is what the Agent card knows of the agents' sessions. It outlives the card so that an answer to a read the
// card asked for before it closed is told from the current one.
type agentSessionState struct {
	reads Latest
	// looping says a read is out or scheduled, so its answer will ask the next; a refresh of the mapping starts the loop only
	// when it is false.
	looping  bool
	Sessions []AgentSessionFacts
}

// AgentSessionFor is the session facts last read for the agent in block, false when none were read yet.
func (m Model) AgentSessionFor(block string) (AgentSessionFacts, bool) {
	i := slices.IndexFunc(m.AgentSessions.Sessions, func(s AgentSessionFacts) bool { return s.Block == block })
	if i < 0 {
		return AgentSessionFacts{}, false
	}
	return m.AgentSessions.Sessions[i], true
}

// askAgentSessions reads the sessions of the Agent card's omp agents after a pause; it supersedes any read out, and asks for
// nothing when no agent is omp.
func askAgentSessions(m Model, after time.Duration) (Model, []Cmd) {
	var asks []AgentSessionAsk
	for _, a := range m.AgentCardRows() {
		if a.NoAddress != AgentNotOmp {
			asks = append(asks, AgentSessionAsk{Block: a.Block, PromptFile: a.PromptFile})
		}
	}
	if len(asks) == 0 {
		m.AgentSessions.looping = false
		return m, nil
	}
	m.AgentSessions.looping = true
	return m, []Cmd{ReadAgentSessions{Request: m.AgentSessions.reads.Ask(), After: after, Agents: asks}}
}

// restartAgentSessions starts the read loop of an open Agent card when the mapping brought it an omp agent and no read is out
// or scheduled; with a loop running, or no omp agent listed, it changes nothing.
func restartAgentSessions(m Model, cmds []Cmd) (Model, []Cmd) {
	if !m.AgentCard.Open || m.AgentSessions.looping {
		return m, cmds
	}
	m, reads := askAgentSessions(m, 0)
	return m, append(cmds, reads...)
}

// agentSessionsRead keeps the newest answer and, while the card is open, asks for the next read. A card closed meanwhile asks for
// nothing: the reads stop with it.
func agentSessionsRead(m Model, msg AgentSessionsRead) (Model, []Cmd) {
	if !m.AgentSessions.reads.IsCurrent(msg.Request) {
		return m, nil
	}
	if !m.AgentCard.Open {
		m.AgentSessions.looping = false
		return m, nil
	}
	m.AgentSessions.Sessions = slices.Clone(msg.Sessions)
	return askAgentSessions(m, AgentSessionEvery)
}

// AgentFact is one line of the Agent card's detail for the selected agent; Muted lines are the reasons a fact is missing.
type AgentFact struct {
	Text  string
	Muted bool
}

// agentLastLineMax bounds the last message's first line so a long one never widens the card.
const agentLastLineMax = 100

// AgentLastLine is the first non-blank line of the agent's last message as the relay gave it, "" when it has none.
func (m Model) AgentLastLine(block string) string {
	for _, s := range m.Agents.States {
		if s.Block != block {
			continue
		}
		for line := range strings.SplitSeq(s.LastMessage, "\n") {
			if line = oneLine(line); line != "" {
				return truncateRunes(line, agentLastLineMax)
			}
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// AgentFacts is the detail the Agent card shows for the agent in block: the session's model, tool calls, current tool and cost, and
// the first line of its last message. An agent whose session is not read yet, or has no session, gets the reason in muted words.
func (m Model) AgentFacts(block string) []AgentFact {
	s, ok := m.AgentSessionFor(block)
	var facts []AgentFact
	switch {
	case !ok && m.agentNotOmp(block):
		facts = append(facts, AgentFact{Text: AgentSessionNotOmpReason, Muted: true})
	case !ok:
		facts = append(facts, AgentFact{Text: "reading the session…", Muted: true})
	case s.Reason != "":
		facts = append(facts, AgentFact{Text: s.Reason, Muted: true})
	default:
		facts = append(facts, AgentFact{Text: sessionSummary(s)})
	}
	if last := m.AgentLastLine(block); last != "" {
		facts = append(facts, AgentFact{Text: "last: " + last})
	} else {
		facts = append(facts, AgentFact{Text: "no message from the agent yet", Muted: true})
	}
	return facts
}

// sessionSummary is "model · N tool calls · running bash · $0.34". A tail that cut the file marks the count "at least" and the cost approximate.
func sessionSummary(s AgentSessionFacts) string {
	parts := make([]string, 0, 4)
	if s.Model != "" {
		parts = append(parts, s.Model)
	} else {
		parts = append(parts, "model unknown")
	}
	calls := "no tool calls"
	switch {
	case s.ToolCalls == 1:
		calls = "1 tool call"
	case s.ToolCalls > 1:
		calls = strconv.Itoa(s.ToolCalls) + " tool calls"
	}
	if s.Partial && s.ToolCalls > 0 {
		calls = "at least " + calls
	}
	parts = append(parts, calls)
	if s.CurrentTool != "" {
		parts = append(parts, "running "+s.CurrentTool)
	} else {
		parts = append(parts, "no tool running")
	}
	if s.HasCost {
		cost := fmt.Sprintf("$%.2f", s.Cost)
		if s.Partial {
			cost = "~" + cost
		}
		parts = append(parts, cost)
	}
	return strings.Join(parts, " · ")
}

// AgentSessionNotOmpReason is what the card says of an agent that is not omp, which has no session to read.
const AgentSessionNotOmpReason = "not an omp agent: no session to read"

func (m Model) agentNotOmp(block string) bool {
	return slices.ContainsFunc(m.AgentMapping, func(e AgentEntry) bool { return e.Block == block && e.NoAddress == AgentNotOmp })
}
