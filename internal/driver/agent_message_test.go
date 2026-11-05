package driver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// ompAgent is the omp agent dana's Pane started on work#aa11, which loads tern-kata's extension.
var ompAgent = model.AgentEntry{IssueUID: "work-aa11", IssueRef: "work#aa11", ProjectID: 7, Block: "31", Teammate: "omp", Address: "dana/omp-31",
	StartedAt: now.Add(-time.Hour)}

// agentRaised is the Attention request dana's Pane raised for an agent named omp waiting on work#aa11.
func agentRaised(t *testing.T) model.AttentionRequest {
	return attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"dana","teammate":"omp","message":"which table?"}`, 5)
}

// messagePane is the Attention Page listing request with the given agents mapped and relayed in the given states.
func messagePane(t *testing.T, request model.AttentionRequest, states []model.AgentState, agents ...model.AgentEntry) driver.State {
	t.Helper()
	s := attentionPane(t)
	s.Model.AgentMapping = agents
	s = answerAttention(t, step(t, s, key("6")), []model.AttentionRequest{request}, 0)
	if states != nil {
		s = step(t, s, model.AgentStatesMsg{States: states}).State
	}
	return s
}

func sentMessage(t *testing.T, cmds []model.Cmd) model.SendAgentMessage {
	t.Helper()
	var found []model.SendAgentMessage
	for _, c := range cmds {
		if send, ok := c.(model.SendAgentMessage); ok {
			found = append(found, send)
		}
	}
	if len(found) != 1 {
		t.Fatalf("cmds %#v, want one message sent", cmds)
	}
	return found[0]
}

func TestAMessageToTheAgentThatRaisedARequestIsWrittenToItsOwnAddress(t *testing.T) {
	s := messagePane(t, agentRaised(t), nil, ompAgent)
	open := step(t, s, key("m"))
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || m.CurrentMode() != model.ModeAgentMessage || len(open.Cmds) != 0 {
		t.Fatalf("card %v mode %s cmds %v, want the Message card", m.TopCard(), m.CurrentMode(), open.Cmds)
	}
	wantShown(t, "the Message card", open.State, "Message omp", "work#aa11", "stays on work#aa11 until the agent clears it")
	if !strings.HasPrefix(open.State.Focus, view.LayerID(model.CardAgentMessage)) {
		t.Errorf("text focus %q is not on the Message card", open.State.Focus)
	}

	typed := step(t, open.State, tern.Key{Text: "  use the second table "}).State
	sending := step(t, typed, key("enter"))
	want := model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Actor: "dana", Address: "dana/omp-31", Message: "use the second table"}
	if got := sentMessage(t, sending.Cmds); got != want {
		t.Fatalf("sent %+v, want %+v", got, want)
	}
	if again := step(t, sending.State, key("enter")); len(again.Cmds) != 0 {
		t.Errorf("enter while the message is out sent %v", again.Cmds)
	}

	sent := step(t, sending.State, model.AgentMessageSent{UID: "work-aa11"}).State
	if sent.Model.TopCard() != model.NoCard || sent.Model.Notice != "Sent the message to omp on work#aa11." {
		t.Fatalf("card %v notice %q, want the card closed and the send confirmed", sent.Model.TopCard(), sent.Model.Notice)
	}
}

func TestAMessageOverOneTheAgentHasNotClearedAsksBeforeReplacingIt(t *testing.T) {
	s := step(t, messagePane(t, agentRaised(t), nil, ompAgent), key("m")).State
	s = step(t, step(t, s, tern.Key{Text: "no, the third"}).State, key("enter")).State
	const held = `{"from":"dana","message":"use the second table"}`
	asking := step(t, s, model.AgentMessageSent{UID: "work-aa11", Held: held, Unread: "use the second table"}).State
	if asking.Model.CurrentMode() != model.ModeAgentMessageReplace {
		t.Fatalf("mode %s, want the replace question", asking.Model.CurrentMode())
	}
	wantShown(t, "the replace question", asking, "has not cleared its last message", "use the second table", "Replace it", "Keep editing")

	kept := step(t, asking, key("escape"))
	if kept.State.Model.CurrentMode() != model.ModeAgentMessage || kept.State.Model.AgentMessage.Text != "no, the third" || len(kept.Cmds) != 0 {
		t.Fatalf("esc: mode %s text %q cmds %v, want back to the message as typed", kept.State.Model.CurrentMode(), kept.State.Model.AgentMessage.Text, kept.Cmds)
	}

	replaced := step(t, asking, key("enter"))
	if got := sentMessage(t, replaced.Cmds); got.Replacing != held || got.Message != "no, the third" {
		t.Fatalf("sent %+v, want the message replacing the value held", got)
	}
	failed := step(t, replaced.State, model.AgentMessageSent{UID: "work-aa11", Err: errors.New("daemon down")}).State
	if failed.Model.TopCard() != model.CardAgentMessage {
		t.Fatalf("a failed send closed the card")
	}
	wantShown(t, "the failure", failed, "daemon down")
}

func TestTheMessageCardRefusesABlankOrTooLongMessage(t *testing.T) {
	s := step(t, messagePane(t, agentRaised(t), nil, ompAgent), key("m")).State
	blank := step(t, step(t, s, tern.Key{Text: "   "}).State, key("enter"))
	if len(blank.Cmds) != 0 {
		t.Fatalf("a blank message was sent: %v", blank.Cmds)
	}
	wantShown(t, "the blank refusal", blank.State, "Type a message.")
	long := step(t, step(t, s, tern.Key{Text: strings.Repeat("é", 513)}).State, key("enter"))
	if len(long.Cmds) != 0 {
		t.Fatalf("a message over kata's limit was sent: %v", long.Cmds)
	}
	wantShown(t, "the length refusal", long.State, "1026 bytes", "at most 1024")
}

func TestMessagingIsOffWithAReasonWhenNoAgentCanReceiveIt(t *testing.T) {
	notOmp, older := ompAgent, ompAgent
	notOmp.Address, notOmp.NoAddress = "", model.AgentNotOmp
	older.Address = ""
	exitedRequest := attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"dana","teammate":"omp","message":"agent exited without closing work#aa11"}`, 5)
	for name, tc := range map[string]struct {
		request model.AttentionRequest
		states  []model.AgentState
		agents  []model.AgentEntry
		want    string
	}{
		"a request a person wrote":     {attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"bob","message":"please deploy"}`, 5), nil, []model.AgentEntry{ompAgent}, model.NoAgentRaisedRefusal},
		"another agent's name":         {attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"dana","teammate":"claude","message":"which table?"}`, 5), nil, []model.AgentEntry{ompAgent}, model.NoAgentRaisedRefusal},
		"an exited agent":              {agentRaised(t), []model.AgentState{{Block: "31", State: model.AgentExited}}, []model.AgentEntry{ompAgent}, model.AgentExitedRefusal},
		"an agent gone with its block": {exitedRequest, nil, nil, model.AgentExitedRefusal},
		"an agent that is not omp":     {agentRaised(t), nil, []model.AgentEntry{notOmp}, model.AgentNotOmpRefusal},
		"an agent started before":      {agentRaised(t), nil, []model.AgentEntry{older}, model.AgentWithoutAddressRefusal},
	} {
		t.Run(name, func(t *testing.T) {
			out := step(t, messagePane(t, tc.request, tc.states, tc.agents...), key("m"))
			if out.State.Model.TopCard() != model.NoCard || out.State.Model.Notice != tc.want || len(out.Cmds) != 0 {
				t.Errorf("card %v notice %q cmds %v, want no card and %q", out.State.Model.TopCard(), out.State.Model.Notice, out.Cmds, tc.want)
			}
		})
	}
}

func TestAMessageGoesToTheNewestWaitingAgentElseTheNewest(t *testing.T) {
	older, newer := ompAgent, ompAgent
	newer.Block, newer.Address, newer.StartedAt = "32", "dana/omp-32", ompAgent.StartedAt.Add(time.Minute)
	address := func(states []model.AgentState) string {
		t.Helper()
		s := step(t, messagePane(t, agentRaised(t), states, older, newer), key("m")).State
		return s.Model.AgentMessage.Address
	}
	if got := address([]model.AgentState{{Block: "31", State: model.AgentWaitingInput}, {Block: "32", State: model.AgentWorking}}); got != "dana/omp-31" {
		t.Errorf("with the older agent waiting the message goes to %q", got)
	}
	if got := address([]model.AgentState{{Block: "31", State: model.AgentIdle}, {Block: "32", State: model.AgentWorking}}); got != "dana/omp-32" {
		t.Errorf("with no agent waiting the message goes to %q, want the newest", got)
	}
}
