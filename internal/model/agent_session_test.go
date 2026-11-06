package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func sessionRead(t *testing.T, cmds []model.Cmd) model.ReadAgentSessions {
	t.Helper()
	read, ok := agentCmd[model.ReadAgentSessions](cmds)
	if !ok {
		t.Fatalf("no ReadAgentSessions in %#v", cmds)
	}
	return read
}

func TestTheAgentCardReadsSessionsWhenItOpensAndEveryTwoSecondsWhileOpen(t *testing.T) {
	notOmp := entry("a", "2", 2)
	notOmp.NoAddress = model.AgentNotOmp
	first := entry("a", "1", 1)
	first.PromptFile = "/state/agent-prompts/prompt-1.md"
	m, cmds := press(cardModel(first, notOmp, entry("other", "3", 3)), "w")

	open := sessionRead(t, cmds)
	if open.After != 0 {
		t.Errorf("the read at opening waits %v; want none", open.After)
	}
	wantAsks := []model.AgentSessionAsk{{Block: "1", PromptFile: "/state/agent-prompts/prompt-1.md", Omp: true}, {Block: "2", Omp: false}}
	if !reflect.DeepEqual(open.Agents, wantAsks) {
		t.Errorf("asked %#v, want the card's agents %#v", open.Agents, wantAsks)
	}

	m, cmds = model.Update(m, model.AgentSessionsRead{Request: open.Request, Sessions: []model.AgentSessionFacts{{Block: "1", Model: "m"}}})
	tick := sessionRead(t, cmds)
	if tick.After != model.AgentSessionEvery || tick.Request == open.Request {
		t.Errorf("tick %+v, want a new read after %v", tick, model.AgentSessionEvery)
	}
	if got, ok := m.AgentSessionFor("1"); !ok || got.Model != "m" {
		t.Errorf("session %+v, %v; want the answer kept", got, ok)
	}

	m, cmds = model.Update(m, model.AgentSessionsRead{Request: tick.Request})
	if _, again := agentCmd[model.ReadAgentSessions](cmds); !again {
		t.Errorf("an open card stopped reading: %#v", cmds)
	}

	closed, _ := press(m, "escape")
	_, cmds = model.Update(closed, model.AgentSessionsRead{Request: sessionRead(t, cmds).Request})
	if len(cmds) != 0 {
		t.Errorf("a closed card read again: %#v", cmds)
	}
}

func TestAnAnswerToAReadBeforeTheCardWasReopenedStartsNoSecondChain(t *testing.T) {
	m, cmds := press(cardModel(entry("a", "1", 1)), "w")
	stale := sessionRead(t, cmds)
	m, _ = press(m, "escape")
	m, cmds = press(m, "w")
	fresh := sessionRead(t, cmds)

	_, cmds = model.Update(m, model.AgentSessionsRead{Request: stale.Request})
	if len(cmds) != 0 {
		t.Errorf("the superseded read's answer asked for more: %#v", cmds)
	}
	_, cmds = model.Update(m, model.AgentSessionsRead{Request: fresh.Request})
	sessionRead(t, cmds)
}

func TestAgentFactsSayWhatTheSessionAndTheRelayHold(t *testing.T) {
	base := cardModel(entry("a", "1", 1))
	withRelay := func(m model.Model, last string) model.Model {
		m, _ = model.Update(m, model.AgentStatesMsg{States: []model.AgentState{{Block: "1", State: model.AgentWorking, LastMessage: last}}})
		return m
	}
	answered := func(m model.Model, s model.AgentSessionFacts) model.Model {
		m, cmds := press(m, "w")
		m, _ = model.Update(m, model.AgentSessionsRead{Request: sessionRead(t, cmds).Request, Sessions: []model.AgentSessionFacts{s}})
		return m
	}
	for name, tt := range map[string]struct {
		session model.AgentSessionFacts
		last    string
		want    []model.AgentFact
	}{
		"a working agent": {
			session: model.AgentSessionFacts{Block: "1", Model: "claude-x", ToolCalls: 40, CurrentTool: "bash", Cost: 1.234, HasCost: true},
			last:    "\n  Running the tests\nthen more",
			want: []model.AgentFact{
				{Text: "claude-x · 40 tool calls · running bash · $1.23"},
				{Text: "last: Running the tests"},
			},
		},
		"an idle agent": {
			session: model.AgentSessionFacts{Block: "1", Model: "claude-x", ToolCalls: 1, Cost: 0.5, HasCost: true},
			last:    "Done.",
			want:    []model.AgentFact{{Text: "claude-x · 1 tool call · no tool running · $0.50"}, {Text: "last: Done."}},
		},
		"a file the tail cut": {
			session: model.AgentSessionFacts{Block: "1", Model: "m", ToolCalls: 9, Cost: 2, HasCost: true, Partial: true},
			want:    []model.AgentFact{{Text: "m · at least 9 tool calls · no tool running · ~$2.00"}, {Text: "no message from the agent yet", Muted: true}},
		},
		"no session": {
			session: model.AgentSessionFacts{Block: "1", Reason: "no session file yet"},
			last:    "Hi",
			want:    []model.AgentFact{{Text: "no session file yet", Muted: true}, {Text: "last: Hi"}},
		},
	} {
		m := answered(withRelay(base, tt.last), tt.session)
		if got := m.AgentFacts("1"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: facts %#v, want %#v", name, got, tt.want)
		}
	}
	m, _ := press(base, "w")
	if got := m.AgentFacts("1"); len(got) == 0 || !got[0].Muted {
		t.Errorf("before the first read: %#v, want a muted line", got)
	}
}
