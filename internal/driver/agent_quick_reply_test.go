package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// quickReplyTexts are the three replies in key order.
var quickReplyTexts = map[string]string{
	"1": "continue",
	"2": "wrap up: commit what you have and comment your status",
	"3": "commit and close if the acceptance criteria hold",
}

// cardPane is the Agent card's pane with agents mapped for dana, the first selected.
func cardPane(t *testing.T, agents ...model.AgentEntry) driver.State {
	t.Helper()
	pane := withAgentRows(agentPanes(t, agentIssue("t1"))["list"])
	pane.Model.AgentMapping, pane.Model.Actor = agents, "dana"
	return step(t, pane, key("w")).State
}

func cardAgent(block string, at time.Time) model.AgentEntry {
	return model.AgentEntry{IssueUID: "t1", IssueRef: "work#t1", ProjectID: 7, Block: block, Teammate: "omp", Address: "dana/omp-" + block, StartedAt: at, Placement: model.AgentSplit}
}

func TestEachQuickReplyIsSentAsTheTypedMessageWouldBe(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)
	surfaces := map[string]func(t *testing.T) (driver.State, model.SendAgentMessage){
		"the Attention Page": func(t *testing.T) (driver.State, model.SendAgentMessage) {
			s := messagePane(t, agentRaised(t), nil, ompAgent)
			return s, model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Actor: "dana", Address: "dana/omp-31"}
		},
		"the Agent card": func(t *testing.T) (driver.State, model.SendAgentMessage) {
			return cardPane(t, cardAgent("31", at)), model.SendAgentMessage{Workspace: "/repo", ProjectID: 7, UID: "t1", Actor: "dana", Address: "dana/omp-31"}
		},
	}
	for name, surface := range surfaces {
		for pick, text := range quickReplyTexts {
			t.Run(name+" "+pick, func(t *testing.T) {
				s, want := surface(t)
				want.Message = text

				typed := step(t, step(t, s, key("m")).State, tern.Key{Text: text}).State
				wantTyped := sentMessage(t, step(t, typed, key("enter")).Cmds)

				opened := step(t, s, key("r"))
				if m := opened.State.Model; m.TopCard() != model.CardAgentMessage || m.CurrentMode() != model.ModeAgentQuickReply || len(opened.Cmds) != 0 {
					t.Fatalf("card %v mode %s cmds %v, want the quick-reply picker", m.TopCard(), m.CurrentMode(), opened.Cmds)
				}
				wantShown(t, "the picker", opened.State, "Quick reply to omp", quickReplyTexts["1"], quickReplyTexts["2"], quickReplyTexts["3"])
				sending := step(t, opened.State, key(pick))
				if got := sentMessage(t, sending.Cmds); got != want || got != wantTyped {
					t.Fatalf("sent %+v, want %+v, as typed %+v", got, want, wantTyped)
				}
				if again := step(t, sending.State, key("1")); len(again.Cmds) != 0 {
					t.Errorf("a second pick while the message is out sent %v", again.Cmds)
				}
				done := step(t, sending.State, model.AgentMessageSent{UID: want.UID}).State
				if done.Model.TopCard() != model.NoCard || !strings.HasPrefix(done.Model.Notice, "Sent the message to omp on ") {
					t.Errorf("card %v notice %q, want the card closed and the send confirmed", done.Model.TopCard(), done.Model.Notice)
				}
			})
		}
	}
}

func TestEscCancelsTheQuickReplyPickerAndOtherKeysSendNothing(t *testing.T) {
	for name, s := range map[string]driver.State{
		"the Attention Page": messagePane(t, agentRaised(t), nil, ompAgent),
		"the Agent card":     cardPane(t, cardAgent("31", now)),
	} {
		t.Run(name, func(t *testing.T) {
			opened := step(t, s, key("r")).State
			for _, k := range []string{"4", "x", "enter"} {
				if got := step(t, opened, key(k)); len(got.Cmds) != 0 || got.State.Model.CurrentMode() != model.ModeAgentQuickReply {
					t.Errorf("%s: mode %s cmds %v, want the picker still open and nothing sent", k, got.State.Model.CurrentMode(), got.Cmds)
				}
			}
			if closed := step(t, opened, key("escape")); closed.State.Model.TopCard() != model.NoCard || len(closed.Cmds) != 0 {
				t.Errorf("esc: card %v cmds %v, want the picker closed", closed.State.Model.TopCard(), closed.Cmds)
			}
		})
	}
}

func TestQuickReplyIsRefusedWhereMessageIs(t *testing.T) {
	notOmp, older := ompAgent, ompAgent
	notOmp.Address, notOmp.NoAddress = "", model.AgentNotOmp
	older.Address = ""
	for name, tc := range map[string]struct {
		states []model.AgentState
		agent  model.AgentEntry
		want   string
	}{
		"an exited agent":          {[]model.AgentState{{Block: "31", State: model.AgentExited}}, ompAgent, model.AgentExitedRefusal},
		"an agent that is not omp": {nil, notOmp, model.AgentNotOmpRefusal},
		"an agent started before":  {nil, older, model.AgentWithoutAddressRefusal},
	} {
		t.Run(name+" on the Attention Page", func(t *testing.T) {
			s := messagePane(t, agentRaised(t), tc.states, tc.agent)
			quick, typed := step(t, s, key("r")), step(t, s, key("m"))
			if m := quick.State.Model; m.TopCard() != model.NoCard || m.Notice != tc.want || m.Notice != typed.State.Model.Notice || len(quick.Cmds) != 0 {
				t.Errorf("card %v notice %q cmds %v, want no card and %q as `m` gives", m.TopCard(), m.Notice, quick.Cmds, tc.want)
			}
		})
		t.Run(name+" on the Agent card", func(t *testing.T) {
			onCard := tc.agent
			onCard.IssueUID, onCard.IssueRef = "t1", "work#t1"
			s := cardPane(t, onCard)
			s.Model.Agents = model.AgentStatesRelay{Received: true, States: tc.states}
			quick, typed := step(t, s, key("r")), step(t, s, key("m"))
			if m := quick.State.Model; m.TopCard() != model.CardAgent || m.Notice != tc.want || m.Notice != typed.State.Model.Notice || len(quick.Cmds) != 0 {
				t.Errorf("card %v notice %q cmds %v, want the Agent card and %q as `m` gives", m.TopCard(), m.Notice, quick.Cmds, tc.want)
			}
			if why := quick.State.Model.AgentCardUnavailable(model.AgentCardQuickReply); why != tc.want {
				t.Errorf("the button is off with %q, want %q", why, tc.want)
			}
		})
	}
}

func TestAQuickReplyOverAnUnreadMessageAsksBeforeReplacingIt(t *testing.T) {
	const held = `{"from":"dana","message":"use the second table"}`
	for name, s := range map[string]driver.State{
		"the Attention Page": messagePane(t, agentRaised(t), nil, ompAgent),
		"the Agent card":     cardPane(t, cardAgent("31", now)),
	} {
		t.Run(name, func(t *testing.T) {
			sending := step(t, step(t, s, key("r")).State, key("2"))
			sent := sentMessage(t, sending.Cmds)
			asking := step(t, sending.State, model.AgentMessageSent{UID: sent.UID, Held: held, Unread: "use the second table"}).State
			if asking.Model.CurrentMode() != model.ModeAgentMessageReplace {
				t.Fatalf("mode %s, want the replace question", asking.Model.CurrentMode())
			}
			wantShown(t, "the replace question", asking, "has not cleared its last message", "use the second table", "Replace it")
			replaced := sentMessage(t, step(t, asking, key("enter")).Cmds)
			if replaced.Message != quickReplyTexts["2"] || replaced.Replacing != held {
				t.Errorf("sent %+v, want the wrap-up replacing the value held", replaced)
			}
		})
	}
}
