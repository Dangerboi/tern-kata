package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func peerEnv(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func running(entries ...model.AgentEntry) []model.IssueAgent {
	return model.RunningAgents(entries, nil)
}

func omp(ref, block, address string) model.AgentEntry {
	_, short, _ := strings.Cut(ref, "#")
	return model.AgentEntry{IssueUID: "uid-" + short, IssueRef: ref, ProjectID: 3, Block: block, Teammate: "omp", Address: address}
}

// sender records the SendAgentMessage commands and answers each as answer says.
type sender struct {
	sent   []model.SendAgentMessage
	answer func(model.SendAgentMessage) model.AgentMessageSent
}

func (s *sender) send(cmd model.Cmd) model.Msg {
	c := cmd.(model.SendAgentMessage)
	s.sent = append(s.sent, c)
	if s.answer == nil {
		return model.AgentMessageSent{UID: c.UID}
	}
	return s.answer(c)
}

func message(t *testing.T, env map[string]string, args ...string) LaunchPlan {
	t.Helper()
	plan, err := PlanLaunch(append([]string{"message"}, args...), peerEnv(env), "test")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestMessageTakesTheTextVerbatimAndTheSendersIdentityFromTheEnvironment(t *testing.T) {
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_TEAMMATE": "builder-12", "KATA_INBOX_USER": " dana/builder-12 "}, "demo#aa11", "-m", "--help me, -h")
	if plan.Subcommand != "message" || plan.TargetRef != "demo#aa11" || plan.MessageText != "--help me, -h" || plan.Actor != "dana" || plan.Teammate != "builder-12" || plan.InboxUser != "dana/builder-12" {
		t.Fatalf("plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "dana"}, "-m", "hi", "dana/omp-1", "--as", "bob"); plan.TargetRef != "dana/omp-1" || plan.Actor != "bob" || plan.Teammate != "" {
		t.Fatalf("plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "me-agent", "KATA_INBOX_USER": "dana/omp-2", "KATA_TEAMMATE": "omp-2"}, "dana/omp-1", "-m", "hi"); plan.Actor != "dana" || plan.Teammate != "omp-2" {
		t.Fatalf("plan %+v", plan)
	}
	for _, args := range [][]string{{"demo#aa11"}, {"-m", "hi"}, {"a", "b", "-m", "hi"}, {"a", "-m"}, {"a", "-m", "x", "-m", "y"}, {"a", "-m", "x", "--bogus"}} {
		if _, err := PlanLaunch(append([]string{"message"}, args...), peerEnv(nil), "test"); err == nil {
			t.Errorf("message %v was accepted", args)
		}
	}
}

func TestAgentsTakesOnlyAnIssue(t *testing.T) {
	plan, err := PlanLaunch([]string{"agents", "--issue", "demo#aa11"}, peerEnv(nil), "test")
	if err != nil || plan.Subcommand != "agents" || plan.TargetRef != "demo#aa11" {
		t.Fatalf("plan %+v, %v", plan, err)
	}
	for _, args := range [][]string{{"agents", "demo#aa11"}, {"agents", "--issue"}, {"agents", "--bogus"}, {"agents", "--as", "bob"}} {
		if _, err := PlanLaunch(args, peerEnv(nil), "test"); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	if plan, err := PlanLaunch([]string{"agents", "--help"}, peerEnv(nil), "test"); err != nil || !plan.Help {
		t.Errorf("--help: %+v, %v", plan, err)
	}
	if plan, err := PlanLaunch([]string{"message", "x", "-m", "--help"}, peerEnv(nil), "test"); err != nil || plan.Help || plan.MessageText != "--help" {
		t.Errorf("-m --help: %+v, %v", plan, err)
	}
}

func TestAgentsListsTheRunningOnesAndCutsToAnIssue(t *testing.T) {
	list := running(omp("demo#aa11", "1", "dana/omp-1"), omp("demo#bb22", "2", "dana/omp-2"), omp("demo#aa11", "3", "dana/omp-3"),
		model.AgentEntry{IssueRef: "demo#aa11", IssueUID: "uid-aa11", Block: "4", Teammate: "other", NoAddress: model.AgentNotOmp})
	list[0].State = model.AgentWaitingInput
	list = slices.Delete(list, 2, 3) // block 3 exited: the relay does not list it among the running
	var all, one bytes.Buffer
	if err := runAgents(LaunchPlan{}, list, &all); err != nil {
		t.Fatal(err)
	}
	if err := runAgents(LaunchPlan{TargetRef: "aa11"}, list, &one); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"demo#aa11  omp    waiting_input  dana/omp-1", "demo#bb22  omp    -              dana/omp-2", "demo#aa11  other  -              -"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("all agents:\n%s\nwant a line %q", all.String(), want)
		}
	}
	if strings.Contains(all.String(), "omp-3") || strings.Contains(one.String(), "bb22") || !strings.Contains(one.String(), "omp-1") {
		t.Errorf("cut to aa11:\n%s", one.String())
	}
	var none bytes.Buffer
	if err := runAgents(LaunchPlan{TargetRef: "zz99"}, list, &none); err != nil || !strings.Contains(none.String(), "No running agents.") {
		t.Errorf("no match: %q, %v", none.String(), err)
	}
}

func TestMessageToAnAddressWritesFromTheActorWithTheSendersTeammate(t *testing.T) {
	s := &sender{}
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_TEAMMATE": "builder-12", "KATA_INBOX_USER": "dana/builder-12"}, "dana/omp-2", "-m", " I changed the API ")
	var out bytes.Buffer
	if err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1"), omp("demo#bb22", "2", "dana/omp-2")), s.send, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	want := model.SendAgentMessage{Workspace: "/w", ProjectID: 3, UID: "uid-bb22", Actor: "dana", Teammate: "builder-12", Address: "dana/omp-2", Message: "I changed the API"}
	if len(s.sent) != 1 || s.sent[0] != want {
		t.Fatalf("sent %+v, want %+v", s.sent, want)
	}
	if !strings.Contains(out.String(), "Sent the message to dana/omp-2 on demo#bb22.") {
		t.Errorf("output %q", out.String())
	}
}

func TestMessageFromAnAgentWithoutATeammateNameWritesNothing(t *testing.T) {
	s := &sender{}
	var out bytes.Buffer
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_INBOX_USER": "dana/omp-2"}, "dana/omp-1", "-m", "hi")
	err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	var failed subcommandError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "no teammate name") || len(s.sent) != 0 {
		t.Errorf("err %v, sent %+v, want a refusal naming the missing teammate and no write", err, s.sent)
	}
}

func TestMessageToAnAddressHoldingAnUnreadOneWritesNothingAndExitsNonZero(t *testing.T) {
	s := &sender{answer: func(c model.SendAgentMessage) model.AgentMessageSent {
		return model.AgentMessageSent{UID: c.UID, Held: `{"from":"dana"}`, Unread: "rebase first\nthen test"}
	}}
	var out bytes.Buffer
	err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	var failed subcommandError
	if !errors.As(err, &failed) {
		t.Fatalf("err %v, want a subcommandError (exit 1)", err)
	}
	if !strings.Contains(out.String(), "dana/omp-1 on demo#aa11 holds a message it has not cleared (rebase first); nothing was written there.") {
		t.Errorf("output %q", out.String())
	}
	if len(s.sent) != 1 || s.sent[0].Replacing != "" {
		t.Errorf("sent %+v: a held message must never be replaced", s.sent)
	}
}

func TestMessageToARefReachesEveryAgentOnTheIssueButTheSenderAndNamesTheOnesNotWritten(t *testing.T) {
	s := &sender{answer: func(c model.SendAgentMessage) model.AgentMessageSent {
		if c.Address == "dana/omp-2" {
			return model.AgentMessageSent{UID: c.UID, Held: "x", Unread: "busy"}
		}
		return model.AgentMessageSent{UID: c.UID}
	}}
	list := running(omp("demo#aa11", "1", "dana/omp-1"), omp("demo#aa11", "2", "dana/omp-2"), omp("demo#aa11", "3", "dana/omp-3"), omp("demo#aa11", "4", "dana/builder-4"), omp("demo#bb22", "5", "dana/omp-5"))
	var out bytes.Buffer
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_INBOX_USER": "dana/builder-4", "KATA_TEAMMATE": "builder-4"}, "demo#aa11", "-m", "heads up")
	err := runMessage(plan, "/w", list, s.send, &out)
	var addrs []string
	for _, c := range s.sent {
		addrs = append(addrs, c.Address)
	}
	if want := []string{"dana/omp-1", "dana/omp-2", "dana/omp-3"}; !slices.Equal(addrs, want) {
		t.Errorf("written to %v, want %v (not the sender, not another Issue)", addrs, want)
	}
	if err == nil || !strings.Contains(err.Error(), "not written to 1 of 3 agents") {
		t.Errorf("err %v, want the count of agents not written", err)
	}
	for _, want := range []string{"Sent the message to dana/omp-1 on demo#aa11.", "dana/omp-2 on demo#aa11 holds a message it has not cleared (busy)", "Sent the message to dana/omp-3 on demo#aa11."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
}

func TestMessageRefusesWhatReachesNobodyBeforeWritingAnything(t *testing.T) {
	list := running(omp("demo#aa11", "1", "dana/omp-1"), model.AgentEntry{IssueRef: "demo#bb22", IssueUID: "uid-bb22", Block: "2", NoAddress: model.AgentNotOmp})
	old := omp("demo#cc33", "3", "dana/omp-3")
	old.ProjectID = 0
	list = append(list, running(old)...)
	for name, tc := range map[string]struct {
		to, text, actor, want string
	}{
		"unknown address":      {"dana/omp-9", "hi", "dana", "no running agent"},
		"unknown ref":          {"demo#zz99", "hi", "dana", "no running agent"},
		"agent without an omp": {"demo#bb22", "hi", "dana", "can receive a message"},
		"empty text":           {"dana/omp-1", "  ", "dana", "Type a message."},
		"too long":             {"dana/omp-1", strings.Repeat("a", 1025), "dana", "kata takes at most 1024"},
		"another actor's":      {"dana/omp-1", "hi", "bob", "acts for dana"},
		"an older entry":       {"dana/omp-3", "hi", "dana", "does not name the Issue's Project"},
	} {
		s := &sender{}
		var out bytes.Buffer
		plan := message(t, map[string]string{"KATA_AUTHOR": tc.actor}, tc.to, "-m", tc.text)
		err := runMessage(plan, "/w", list, s.send, &out)
		if err == nil || !strings.Contains(err.Error()+out.String(), tc.want) || len(s.sent) != 0 {
			t.Errorf("%s: err %v, output %q, sent %+v, want a refusal naming %q", name, err, out.String(), s.sent, tc.want)
		}
	}
}
