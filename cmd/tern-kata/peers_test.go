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

// sender records the SendAgentMessage commands and answers each as answer says; it answers the auth capability read as verified
// says and counts the reads.
type sender struct {
	sent     []model.SendAgentMessage
	resolved []model.ResolveComment
	comments map[string]string // handle -> comment ULID this fake Issue holds
	answer   func(model.SendAgentMessage) model.AgentMessageSent
	verified bool
	account  string // the actor a verified daemon reports for the credential
	reads    int
	// remote is what the daemon's Agent records hold for other hosts (the agents of every Issue a scan reads, the ones on the
	// Issue a lookup names); issues are Issues that exist without such an agent. remoteErr fails a scan, issueErr a lookup.
	remote    []model.IssueAgent
	issues    []string
	remoteErr error
	issueErr  error
	loads     []model.LoadRemoteAgents
}

func (s *sender) send(cmd model.Cmd) model.Msg {
	if l, ok := cmd.(model.LoadRemoteAgents); ok {
		s.loads = append(s.loads, l)
		if l.Issue == "" {
			return model.RemoteAgentsLoaded{Agents: s.remote, Err: s.remoteErr}
		}
		var found []model.IssueAgent
		for _, a := range s.remote {
			if a.OnIssue(l.Issue) {
				found = append(found, a)
			}
		}
		return model.RemoteAgentsLoaded{Agents: found, NotFound: len(found) == 0 && !slices.Contains(s.issues, l.Issue), Err: s.issueErr}
	}
	if _, ok := cmd.(model.LoadAuthCapabilities); ok {
		s.reads++
		return model.AuthCapabilitiesLoaded{Account: true, VerifiedSenders: s.verified, Actor: s.account}
	}
	if r, ok := cmd.(model.ResolveComment); ok {
		s.resolved = append(s.resolved, r)
		if uid, found := s.comments[model.CommentHandle(r.Handle)]; found {
			return model.CommentResolved{CommentUID: uid}
		}
		return model.CommentResolved{Err: model.ErrNoSuchComment}
	}
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
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_TEAMMATE": "builder-12", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": " dana/builder-12 "}, "demo#aa11", "-m", "--help me, -h")
	if plan.Subcommand != "message" || plan.TargetRef != "demo#aa11" || plan.MessageText != "--help me, -h" || plan.Actor != "dana" || plan.Teammate != "builder-12" || plan.InboxUser != "dana/builder-12" {
		t.Fatalf("plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "dana"}, "-m", "hi", "dana/omp-1", "--as", "bob"); plan.TargetRef != "dana/omp-1" || plan.Actor != "bob" || plan.Teammate != "" {
		t.Fatalf("plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "me-agent", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": "dana/omp-2", "KATA_TEAMMATE": "omp-2"}, "dana/omp-1", "-m", "hi"); plan.Actor != "dana" || plan.Teammate != "omp-2" {
		t.Fatalf("plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "me", "KATA_INBOX_USER": "dana"}, "dana/omp-1", "-m", "hi", "--as", "dana"); plan.Actor != "dana" || plan.InboxUser != "" || plan.Teammate != "" {
		t.Fatalf("a person with an inbox user and --as: plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "me", "KATA_INBOX_USER": "dana"}, "dana/omp-1", "-m", "hi"); plan.Actor != "me" || plan.InboxUser != "" {
		t.Fatalf("a person with an inbox user: plan %+v", plan)
	}
	if plan := message(t, map[string]string{"KATA_AUTHOR": "me", "KATA_INBOX_USER": "dana/omp-2"}, "dana/omp-1", "-m", "hi"); plan.Actor != "me" || plan.InboxUser != "" {
		t.Fatalf("an address without KATA_REF is not an agent: plan %+v", plan)
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
	if err := runAgents(LaunchPlan{}, "/w", list, (&sender{}).send, &all); err != nil {
		t.Fatal(err)
	}
	if err := runAgents(LaunchPlan{TargetRef: "aa11"}, "/w", list, (&sender{}).send, &one); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"demo#aa11  omp    -     waiting_input  dana/omp-1", "demo#bb22  omp    -     -              dana/omp-2", "demo#aa11  other  -     -              -"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("all agents:\n%s\nwant a line %q", all.String(), want)
		}
	}
	if strings.Contains(all.String(), "omp-3") || strings.Contains(one.String(), "bb22") || !strings.Contains(one.String(), "omp-1") {
		t.Errorf("cut to aa11:\n%s", one.String())
	}
	var none bytes.Buffer
	if err := runAgents(LaunchPlan{TargetRef: "zz99"}, "/w", list, (&sender{}).send, &none); err != nil || !strings.Contains(none.String(), "No running agents.") {
		t.Errorf("no match: %q, %v", none.String(), err)
	}
}

func TestMessageToAnAddressWritesFromTheActorWithTheSendersTeammate(t *testing.T) {
	s := &sender{}
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_TEAMMATE": "builder-12", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": "dana/builder-12"}, "dana/omp-2", "-m", " I changed the API ")
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
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": "dana/omp-2"}, "dana/omp-1", "-m", "hi")
	err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	var failed subcommandError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "no teammate name") || len(s.sent) != 0 {
		t.Errorf("err %v, sent %+v, want a refusal naming the missing teammate and no write", err, s.sent)
	}
}

func TestMessageToAnAddressHoldingAnUnreadOneWritesNothingAndExitsNonZero(t *testing.T) {
	s := &sender{answer: func(c model.SendAgentMessage) model.AgentMessageSent {
		return model.AgentMessageSent{UID: c.UID, Held: `{"from":"dana"}`, Unread: "rebase first", UnreadFrom: "dana-phone"}
	}}
	var out bytes.Buffer
	err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	var failed subcommandError
	if !errors.As(err, &failed) {
		t.Fatalf("err %v, want a subcommandError (exit 1)", err)
	}
	if !strings.Contains(out.String(), "dana/omp-1 on demo#aa11 holds a message from dana-phone it has not cleared (rebase first); nothing was written there. To clear it: kata notify demo#aa11 --to dana/omp-1 --clear") {
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
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": "dana/builder-4", "KATA_TEAMMATE": "builder-4"}, "demo#aa11", "-m", "heads up")
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
		"another actor's":      {"dana/omp-1", "hi", "bob", "message_from"},
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

func TestMessageFromAnotherActorFollowsTheSenderRule(t *testing.T) {
	listed := omp("demo#aa11", "1", "dana/omp-1")
	listed.MessageFrom = "dana-laptop"
	for name, tc := range map[string]struct {
		actor     string
		verified  bool
		wantSent  bool
		wantReads int
		wantText  string
	}{
		"the agent's own actor, unverified daemon":  {"dana", false, true, 1, "Sent the message"},
		"a listed actor on a verified daemon":       {"dana-laptop", true, true, 1, "Sent the message"},
		"a listed actor on an unverified daemon":    {"dana-laptop", false, false, 1, "[auth] require_token_identity"},
		"an unlisted actor on a verified daemon":    {"bob", true, false, 1, "message_from"},
		"an unlisted actor on an unverified daemon": {"bob", false, false, 1, "message_from"},
	} {
		s := &sender{verified: tc.verified}
		var out bytes.Buffer
		plan := message(t, map[string]string{"KATA_AUTHOR": tc.actor}, "dana/omp-1", "-m", "hi")
		err := runMessage(plan, "/w", running(listed), s.send, &out)
		if (err == nil) != tc.wantSent || (len(s.sent) == 1) != tc.wantSent || s.reads != tc.wantReads || !strings.Contains(out.String(), tc.wantText) {
			t.Errorf("%s: err %v, output %q, sent %+v, reads %d", name, err, out.String(), s.sent, s.reads)
		}
	}
}

func TestMessageReWritesTheCommentTheSlotAsksToAnswer(t *testing.T) {
	const uid = "01JABCDEFGHJKMNPQRSTVWXYZ0"
	s := &sender{comments: map[string]string{"c:vwxyz0": uid}}
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "why?", "--re", "c:VWXYZ0")
	if plan.MessageRe != "c:VWXYZ0" {
		t.Fatalf("plan %+v", plan)
	}
	var out bytes.Buffer
	if err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(s.sent) != 1 || s.sent[0].Re != uid || s.sent[0].Message != "why?" || len(s.resolved) != 1 || s.resolved[0].UID != "uid-aa11" || s.resolved[0].ProjectID != 3 {
		t.Errorf("sent %+v, resolved %+v", s.sent, s.resolved)
	}

	s = &sender{}
	out.Reset()
	err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	if err == nil || len(s.sent) != 0 || !strings.Contains(out.String(), "--re c:VWXYZ0") {
		t.Errorf("a handle that names no comment: err %v, sent %+v, output %q", err, s.sent, out.String())
	}

	s = &sender{}
	if err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil || len(s.resolved) != 0 || s.sent[0].Re != "" {
		t.Errorf("without --re: err %v, resolved %+v, sent %+v", err, s.resolved, s.sent)
	}
}

func TestMessageReTakesAValueAndOnlyOnce(t *testing.T) {
	if plan := message(t, nil, "dana/omp-1", "--re=c:abc123", "-m", "hi"); plan.MessageRe != "c:abc123" || plan.TargetRef != "dana/omp-1" {
		t.Errorf("plan %+v", plan)
	}
	for _, args := range [][]string{{"a", "-m", "x", "--re"}, {"a", "-m", "x", "--re", "--as"}, {"a", "-m", "x", "--re", "c:1", "--re", "c:2"}, {"a", "-m", "x", "--re="}} {
		if _, err := PlanLaunch(append([]string{"message"}, args...), peerEnv(nil), "test"); err == nil {
			t.Errorf("message %v was accepted", args)
		}
	}
	if plan, err := PlanLaunch([]string{"message", "a", "-m", "x", "--re", "-h"}, peerEnv(nil), "test"); err != nil || plan.Help || plan.MessageRe != "-h" {
		t.Errorf("--re -h: %+v, %v", plan, err)
	}
}

// remoteOmp is an omp agent another host published a record for, as the daemon's loader returns it.
func remoteOmp(ref, block, address, host string, messageFrom ...string) model.IssueAgent {
	_, short, _ := strings.Cut(ref, "#")
	return model.RemoteIssueAgent(model.AgentRecord{Address: address, Host: host, HostID: "id-" + host, Block: block, Name: "omp", Actor: model.AgentAddressActor(address), MessageFrom: messageFrom}, ref, "uid-"+short, 3)
}

func TestAgentsHasAHostColumnAndListsAgentsOnOtherHostsWithNoState(t *testing.T) {
	local := running(omp("demo#aa11", "1", "dana/omp-1"))
	local[0].State = model.AgentWorking
	s := &sender{remote: []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop"), remoteOmp("demo#bb22", "8", "dana/omp-8", "desk")}}
	var out bytes.Buffer
	if err := runAgents(LaunchPlan{}, "/w", local, s.send, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ISSUE      AGENT  HOST    STATE    ADDRESS", "demo#aa11  omp    -       working  dana/omp-1", "demo#aa11  omp    laptop  -        dana/omp-7", "demo#bb22  omp    desk    -        dana/omp-8"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output:\n%s\nwant a line %q", out.String(), want)
		}
	}
}

func TestAgentsWithoutAnIssueScansKataRefsProjectAndWithOneReadsThatIssue(t *testing.T) {
	s := &sender{remote: []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop"), remoteOmp("demo#bb22", "8", "dana/omp-8", "desk")}}
	var scanned, cut bytes.Buffer
	scan, err := PlanLaunch([]string{"agents"}, peerEnv(map[string]string{"KATA_REF": " demo#cc33 "}), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := runAgents(scan, "/w", nil, s.send, &scanned); err != nil {
		t.Fatal(err)
	}
	one, err := PlanLaunch([]string{"agents", "--issue", "bb22"}, peerEnv(map[string]string{"KATA_REF": "demo#cc33"}), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := runAgents(one, "/w", nil, s.send, &cut); err != nil {
		t.Fatal(err)
	}
	if want := (model.LoadRemoteAgents{Workspace: "/w", ScopeRef: "demo#cc33"}); s.loads[0] != want {
		t.Errorf("scan asked %+v, want %+v", s.loads[0], want)
	}
	if want := (model.LoadRemoteAgents{Workspace: "/w", Issue: "bb22", ScopeRef: "demo#cc33"}); s.loads[1] != want {
		t.Errorf("--issue asked %+v, want %+v", s.loads[1], want)
	}
	if !strings.Contains(scanned.String(), "omp-7") || !strings.Contains(scanned.String(), "omp-8") {
		t.Errorf("scan:\n%s\nwant both remote agents", scanned.String())
	}
	if strings.Contains(cut.String(), "omp-7") || !strings.Contains(cut.String(), "omp-8") {
		t.Errorf("--issue bb22:\n%s\nwant only the agent on bb22", cut.String())
	}
}

func TestAgentsKeepsTheLocalListWhenTheRemoteOnesCannotBeRead(t *testing.T) {
	local := running(omp("demo#aa11", "1", "dana/omp-1"))
	var noScope bytes.Buffer
	s := &sender{remoteErr: model.ErrNoRemoteScope}
	if err := runAgents(LaunchPlan{}, "/w", local, s.send, &noScope); err != nil {
		t.Fatalf("no Project to scan: %v", err)
	}
	if !strings.Contains(noScope.String(), "dana/omp-1") || !strings.Contains(noScope.String(), "Agents on other hosts are not listed: no Project to look in") {
		t.Errorf("output:\n%s\nwant the local agent and why no other host's are listed", noScope.String())
	}
	var down bytes.Buffer
	s = &sender{remoteErr: errors.New("daemon unreachable")}
	err := runAgents(LaunchPlan{}, "/w", local, s.send, &down)
	var failed subcommandError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "daemon unreachable") || !strings.Contains(down.String(), "dana/omp-1") {
		t.Errorf("err %v, output %q, want the local agent and an exit status 1 naming the failure", err, down.String())
	}
	var none bytes.Buffer
	s = &sender{}
	if err := runAgents(LaunchPlan{}, "/w", nil, s.send, &none); err != nil || !strings.Contains(none.String(), "No running agents.") {
		t.Errorf("nothing anywhere: %q, %v", none.String(), err)
	}
}

func TestMessageToAnAddressOnAnotherHostIsFoundByScanAndNeedsAVerifiedDaemon(t *testing.T) {
	remote := []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop")}
	for name, tc := range map[string]struct {
		verified bool
		wantSent bool
		wantText string
	}{
		"verified":   {true, true, "Sent the message to dana/omp-7 on demo#aa11 (host laptop)."},
		"unverified": {false, false, "[auth] require_token_identity"},
	} {
		s := &sender{remote: remote, verified: tc.verified}
		var out bytes.Buffer
		plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_REF": "demo#bb22"}, "dana/omp-7", "-m", "hi")
		err := runMessage(plan, "/w", running(omp("demo#bb22", "2", "dana/omp-2")), s.send, &out)
		if (err == nil) != tc.wantSent || (len(s.sent) == 1) != tc.wantSent || !strings.Contains(out.String(), tc.wantText) || s.reads != 1 {
			t.Errorf("%s: err %v, output %q, sent %+v, reads %d", name, err, out.String(), s.sent, s.reads)
		}
		if len(s.loads) != 2 || s.loads[1] != (model.LoadRemoteAgents{Workspace: "/w", ScopeRef: "demo#bb22"}) {
			t.Errorf("%s: loads %+v, want the Issue lookup then a scan of KATA_REF's Project", name, s.loads)
		}
		if tc.wantSent && (s.sent[0].UID != "uid-aa11" || s.sent[0].ProjectID != 3 || s.sent[0].Actor != "dana") {
			t.Errorf("%s: sent %+v, want the slot on the record's Issue as dana", name, s.sent)
		}
	}
}

func TestMessageToALocalAddressReadsNothingFromOtherHosts(t *testing.T) {
	s := &sender{remote: []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop")}}
	var out bytes.Buffer
	if err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(s.loads) != 0 || s.reads != 1 {
		t.Errorf("loads %+v, auth reads %d: a local message reads nothing from other hosts, and the daemon once for the sender", s.loads, s.reads)
	}
}

func TestMessageIsWrittenAsTheAccountOfADaemonThatVerifiesActors(t *testing.T) {
	// kata records the token's account as the event actor of the write, and the agent's extension delivers a live slot only when that
	// actor is the slot's from: a slot from the person's own name would be dropped.
	s := &sender{verified: true, account: "dana"}
	var out bytes.Buffer
	plan := message(t, map[string]string{"KATA_AUTHOR": "dangerboi"}, "dana/omp-1", "-m", "hi")
	if err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(s.sent) != 1 || s.sent[0].Actor != "dana" {
		t.Errorf("sent %+v, want one message written as the account dana", s.sent)
	}
}

func TestMessageKeepsTheActorWhenTheDaemonDoesNotVerifyActors(t *testing.T) {
	s := &sender{account: "dana"}
	var out bytes.Buffer
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-1", "-m", "hi")
	if err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil || len(s.sent) != 1 || s.sent[0].Actor != "dana" {
		t.Fatalf("err %v, sent %+v", err, s.sent)
	}
	s = &sender{account: "ops"}
	out.Reset()
	if err := runMessage(plan, "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out); err != nil || len(s.sent) != 1 || s.sent[0].Actor != "dana" {
		t.Fatalf("err %v, sent %+v: an unverified daemon's account is not the sender", err, s.sent)
	}
}

func TestMessageToARefReachesLocalAndRemoteAgentsOnTheIssueButTheSender(t *testing.T) {
	s := &sender{verified: true, remote: []model.IssueAgent{
		remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop"), remoteOmp("demo#aa11", "8", "dana/builder-8", "desk"), remoteOmp("demo#bb22", "9", "dana/omp-9", "laptop"),
	}}
	list := running(omp("demo#aa11", "1", "dana/omp-1"), omp("demo#bb22", "2", "dana/omp-2"))
	var out bytes.Buffer
	plan := message(t, map[string]string{"KATA_AUTHOR": "dana", "KATA_REF": "demo#aa11", "KATA_INBOX_USER": "dana/builder-8", "KATA_TEAMMATE": "builder-8"}, "demo#aa11", "-m", "heads up")
	if err := runMessage(plan, "/w", list, s.send, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	var addrs []string
	for _, c := range s.sent {
		addrs = append(addrs, c.Address)
	}
	if want := []string{"dana/omp-1", "dana/omp-7"}; !slices.Equal(addrs, want) {
		t.Errorf("written to %v, want %v (local first; not the sender, not another Issue)", addrs, want)
	}
	if len(s.loads) != 1 || s.loads[0].Issue != "demo#aa11" {
		t.Errorf("loads %+v, want one lookup of the Issue", s.loads)
	}
}

func TestMessageToARefOnAnUnverifiedDaemonRefusesRemoteAgentsButNotLocalOnes(t *testing.T) {
	s := &sender{remote: []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop")}}
	list := running(omp("demo#aa11", "1", "dana/omp-1"))
	var out bytes.Buffer
	err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "demo#aa11", "-m", "hi"), "/w", list, s.send, &out)
	if err == nil || !strings.Contains(err.Error(), "not written to 1 of 2 agents") {
		t.Errorf("err %v, want the one remote agent counted as not written", err)
	}
	if len(s.sent) != 1 || s.sent[0].Address != "dana/omp-1" {
		t.Errorf("sent %+v, want only the local agent", s.sent)
	}
	for _, want := range []string{"Sent the message to dana/omp-1 on demo#aa11.", "Cannot message dana/omp-7 on demo#aa11 (host laptop): ", "[auth] require_token_identity"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
}

func TestMessageToARemoteAgentTakesTheRecordsMessageFrom(t *testing.T) {
	remote := []model.IssueAgent{remoteOmp("demo#aa11", "7", "dana/omp-7", "laptop", "dana-laptop")}
	for name, tc := range map[string]struct {
		actor    string
		verified bool
		wantSent bool
		wantText string
	}{
		"a listed actor, verified":   {"dana-laptop", true, true, "Sent the message"},
		"a listed actor, unverified": {"dana-laptop", false, false, "[auth] require_token_identity"},
		"an unlisted actor":          {"bob", true, false, "message_from"},
		"the agent's own actor":      {"dana", true, true, "Sent the message"},
	} {
		s := &sender{remote: remote, verified: tc.verified}
		var out bytes.Buffer
		plan := message(t, map[string]string{"KATA_AUTHOR": tc.actor}, "demo#aa11", "-m", "hi")
		err := runMessage(plan, "/w", nil, s.send, &out)
		if (err == nil) != tc.wantSent || (len(s.sent) == 1) != tc.wantSent || !strings.Contains(out.String(), tc.wantText) {
			t.Errorf("%s: err %v, output %q, sent %+v", name, err, out.String(), s.sent)
		}
	}
}

func TestMessageToAnUnknownTargetNamesWhyTheOtherHostsCouldNotBeRead(t *testing.T) {
	var out bytes.Buffer
	s := &sender{remoteErr: errors.New("daemon unreachable")}
	err := runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-9", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	if err == nil || !strings.Contains(err.Error(), "no running agent") || !strings.Contains(err.Error(), "daemon unreachable") || len(s.sent) != 0 {
		t.Errorf("scan failing: err %v, sent %+v", err, s.sent)
	}
	s = &sender{remoteErr: model.ErrNoRemoteScope}
	err = runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "dana/omp-9", "-m", "hi"), "/w", nil, s.send, &out)
	if err == nil || !strings.Contains(err.Error(), "no running agent") || strings.Contains(err.Error(), "could not be read") {
		t.Errorf("no scope: err %v, want only the unknown-target refusal", err)
	}
	s = &sender{issueErr: errors.New("daemon unreachable")}
	err = runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "demo#aa11", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	var failed subcommandError
	if !errors.As(err, &failed) || len(s.sent) != 1 || !strings.Contains(out.String(), "daemon unreachable") {
		t.Errorf("lookup failing: err %v, sent %+v, output %q, want the local agent messaged, the failure said and exit 1", err, s.sent, out.String())
	}
	s = &sender{issueErr: errors.New("daemon unreachable")}
	err = runMessage(message(t, map[string]string{"KATA_AUTHOR": "dana"}, "demo#zz99", "-m", "hi"), "/w", running(omp("demo#aa11", "1", "dana/omp-1")), s.send, &out)
	if err == nil || !strings.Contains(err.Error(), "daemon unreachable") || len(s.sent) != 0 {
		t.Errorf("lookup failing with no local target: err %v, sent %+v, want the read error", err, s.sent)
	}
}
