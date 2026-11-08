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

func TestAMessagePassesOverAnotherActorsNewerAgentOnTheSameIssue(t *testing.T) {
	own, theirs := ompAgent, ompAgent
	theirs.Block, theirs.Address, theirs.StartedAt = "40", "sam/omp-40", ompAgent.StartedAt.Add(time.Minute)
	states := []model.AgentState{{Block: "31", State: model.AgentWaitingInput}, {Block: "40", State: model.AgentWaitingInput}}
	out := step(t, messagePane(t, agentRaised(t), states, own, theirs), key("m"))
	if m := out.State.Model; m.TopCard() != model.CardAgentMessage || m.AgentMessage.Address != "dana/omp-31" {
		t.Errorf("card %v address %q notice %q, want the Message card for dana/omp-31", m.TopCard(), m.AgentMessage.Address, m.Notice)
	}
}

// commentPane is the Comments tab of work#aa11 with the comment named namedComment under the cursor, with agents mapped.
func commentPane(t *testing.T, agents ...model.AgentEntry) driver.State {
	t.Helper()
	s := attentionPane(t)
	s.Model.AgentMapping = agents
	issue := &model.Issue{UID: "work-aa11", QualifiedID: "work#aa11", ProjectID: 7}
	s = step(t, s, model.DetailLoaded{UID: "work-aa11", Selected: &model.IssueDetailData{Issue: issue, Comments: commentsOf("x1", namedComment, "x3")}}).State
	s = answerAttention(t, step(t, s, key("6")), attentionRequestsNaming(t, "work", 7, "aa11", namedComment), 0)
	s = step(t, s, key("enter")).State
	if m := s.Model; m.ActiveTab() != model.TabComments || detailRowKey(m) != namedComment {
		t.Fatalf("tab %v row %q, want the Comments tab on %s", m.ActiveTab(), detailRowKey(m), namedComment)
	}
	return s
}

func TestAskingTheAgentAboutACommentSendsTheMessageNamingItsHandle(t *testing.T) {
	open := step(t, commentPane(t, ompAgent), key("m"))
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || len(open.Cmds) != 0 {
		t.Fatalf("card %v cmds %v, want the Message card", m.TopCard(), open.Cmds)
	}
	wantShown(t, "the Message card", open.State, "Message omp", "work#aa11", "answer comment c:vwxyz0")
	sending := step(t, step(t, open.State, tern.Key{Text: "is this still true?"}).State, key("enter"))
	want := model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Actor: "dana", Address: "dana/omp-31", Message: "is this still true?", Re: namedComment}
	if got := sentMessage(t, sending.Cmds); got != want {
		t.Fatalf("sent %+v, want %+v", got, want)
	}
}

func TestAskingAboutACommentPassesOverANewerAgentThatCannotBeMessaged(t *testing.T) {
	older, newer := ompAgent, ompAgent
	newer.Block, newer.Address, newer.StartedAt = "32", "dana/omp-32", ompAgent.StartedAt.Add(time.Minute)
	s := step(t, commentPane(t, older, newer), model.AgentStatesMsg{States: []model.AgentState{{Block: "31", State: model.AgentIdle}, {Block: "32", State: model.AgentExited}}}).State
	open := step(t, s, key("m"))
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || m.AgentMessage.Address != "dana/omp-31" {
		t.Fatalf("card %v address %q notice %q, want the Message card for dana/omp-31", m.TopCard(), m.AgentMessage.Address, m.Notice)
	}
	sending := step(t, step(t, open.State, tern.Key{Text: "still true?"}).State, key("enter"))
	if got := sentMessage(t, sending.Cmds); got.Address != "dana/omp-31" {
		t.Fatalf("sent to %q, want dana/omp-31", got.Address)
	}
}

func TestAskingAboutACommentWithoutAnAgentThatCanBeMessagedOnlySaysWhy(t *testing.T) {
	other := ompAgent
	other.IssueUID = "other-bb22"
	for name, c := range map[string]struct {
		agents []model.AgentEntry
		reason string
	}{
		"no agent on the Issue":       {[]model.AgentEntry{other}, model.NoAgentOnIssueRefusal},
		"an agent without an address": {[]model.AgentEntry{{IssueUID: "work-aa11", Block: "31", Teammate: "omp"}}, model.AgentWithoutAddressRefusal},
	} {
		t.Run(name, func(t *testing.T) {
			got := step(t, commentPane(t, c.agents...), key("m"))
			if m := got.State.Model; m.TopCard() != model.NoCard || m.Notice != c.reason || len(got.Cmds) != 0 {
				t.Fatalf("card %v notice %q cmds %v, want only the notice %q", m.TopCard(), m.Notice, got.Cmds, c.reason)
			}
		})
	}
}

// A daemon that verifies actors records the credential's account as the actor of every write, and the agent's extension delivers a
// live slot only when that actor is the slot's from: the message is written as the account, whatever name the Pane resolved.
func TestAMessageOnADaemonThatVerifiesActorsIsWrittenAsItsAccount(t *testing.T) {
	s := messagePane(t, agentRaised(t), nil, ompAgent)
	s.Model.Actor, s.Model.VerifiedSenders, s.Model.Daemon = "dangerboi", true, model.Daemon{Account: "dana"}
	typed := step(t, step(t, s, key("m")).State, tern.Key{Text: "use the second table"}).State
	want := model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Actor: "dana", Address: "dana/omp-31", Message: "use the second table"}
	if got := sentMessage(t, step(t, typed, key("enter")).Cmds); got != want {
		t.Fatalf("sent %+v, want %+v", got, want)
	}
}
