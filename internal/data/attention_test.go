package data

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// attentionFixture is an in-process daemon with two Projects, each bound to its own workspace.
type attentionFixture struct {
	kata    *kataServer
	gateway *Gateway
	work    string
	other   string
	workP   model.Project
	otherP  model.Project
}

func newAttentionFixture(t *testing.T) attentionFixture {
	t.Helper()
	kata := startKata(t, "0.21.0")
	gateway := newGateway(t, newLocator(kata.target()))
	work, workP := project(t, gateway, "work")
	other, otherP := project(t, gateway, "other")
	return attentionFixture{kata, gateway, work, other, workP, otherP}
}

// put writes value under the notification slot of address on an Issue.
func (f attentionFixture) put(t *testing.T, project model.Project, uid, address string, value any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: project.ID, Ref: uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.NotificationKey(address): value}},
	})
	if err != nil {
		t.Fatalf("writing the slot of %s: %v", address, err)
	}
}

func (f attentionFixture) read(t *testing.T, address string) model.AttentionLoaded {
	t.Helper()
	return as[model.AttentionLoaded](t, run(t, f.gateway, model.LoadAttention{Workspace: f.work, Address: address, Request: 5}))
}

func TestLoadAttentionReturnsOnlyOpenIssuesHoldingTheAddressesSlotAcrossProjects(t *testing.T) {
	f := newAttentionFixture(t)
	ours := createIssue(t, f.gateway, f.work, f.workP, "Ours")
	theirs := createIssue(t, f.gateway, f.other, f.otherP, "Theirs")
	closed := createIssue(t, f.gateway, f.work, f.workP, "Closed")
	someoneElses := createIssue(t, f.gateway, f.work, f.workP, "For someone else")
	createIssue(t, f.gateway, f.other, f.otherP, "No slot")
	f.put(t, f.workP, ours, "dana", map[string]any{"from": "bob", "message": "look at this\nsecond line"})
	f.put(t, f.otherP, theirs, "dana", map[string]any{"from": "pip", "teammate": "scout", "message": "reply please", "re": "01JABCDEFGHJKMNPQRSTVWXYZ0", "broadcast": true})
	f.put(t, f.workP, closed, "dana", map[string]any{"from": "bob", "message": "stale"})
	f.put(t, f.workP, someoneElses, "erin", map[string]any{"from": "bob", "message": "not for dana"})
	if done := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.work, UID: closed, ProjectID: f.workP.ID, Actor: "tester", Quick: true, Key: "k-attention-closed"})); done.Err != nil {
		t.Fatal(done.Err)
	}

	got := f.read(t, "dana")
	if got.Err != nil || got.Request != 5 || got.Skipped != 0 || len(got.Requests) != 2 {
		t.Fatalf("answer %+v, want two requests of two Projects and the request number echoed", got)
	}
	byUID := map[string]model.AttentionRequest{}
	for _, r := range got.Requests {
		byUID[r.IssueUID] = r
	}
	a, b := byUID[ours], byUID[theirs]
	if a.ProjectName != "work" || a.ProjectID != f.workP.ID || a.Title != "Ours" || a.ShortID == "" || a.Updated.IsZero() ||
		a.Slot.From != "bob" || a.Slot.Message != "look at this\nsecond line" || a.Slot.Address != "dana" || !strings.Contains(a.Slot.Raw, `"look at this`) {
		t.Errorf("work request %+v", a)
	}
	if b.ProjectName != "other" || b.ProjectID != f.otherP.ID || b.Sender() != "pip/scout" || !b.Slot.Broadcast || b.CommentHandle() != "c:vwxyz0" {
		t.Errorf("other request %+v", b)
	}
	if got.Requests[0].IssueUID != theirs {
		t.Errorf("the Issue written last is not first: %v then %v", got.Requests[0].QualifiedID(), got.Requests[1].QualifiedID())
	}
	if none := f.read(t, "nobody"); none.Err != nil || len(none.Requests) != 0 || none.Skipped != 0 {
		t.Errorf("an address with no slot: %+v", none)
	}
}

func TestLoadAttentionCountsASlotThatDoesNotReadAsSkipped(t *testing.T) {
	f := newAttentionFixture(t)
	good := createIssue(t, f.gateway, f.work, f.workP, "Good")
	noSender := createIssue(t, f.gateway, f.other, f.otherP, "No sender")
	notObject := createIssue(t, f.gateway, f.other, f.otherP, "Not an object")
	f.put(t, f.workP, good, "dana", map[string]any{"from": "bob", "message": "fine", "kind": 3})
	f.put(t, f.otherP, noSender, "dana", map[string]any{"message": "who sent this"})
	f.put(t, f.otherP, notObject, "dana", "just text")

	got := f.read(t, "dana")
	if got.Err != nil || got.Skipped != 2 || len(got.Requests) != 1 || got.Requests[0].IssueUID != good {
		t.Fatalf("answer %+v, want the good request and two skipped", got)
	}
	if got.Requests[0].Tag() != "request" {
		t.Errorf("a malformed optional field kept in the tag: %q", got.Requests[0].Tag())
	}
}

func TestLoadAttentionOrdersNewestIssueUpdateFirst(t *testing.T) {
	f := newAttentionFixture(t)
	first := createIssue(t, f.gateway, f.work, f.workP, "First")
	second := createIssue(t, f.gateway, f.other, f.otherP, "Second")
	f.put(t, f.workP, first, "dana", map[string]any{"from": "bob", "message": "one"})
	f.put(t, f.otherP, second, "dana", map[string]any{"from": "bob", "message": "two"})
	if got := f.read(t, "dana"); len(got.Requests) != 2 || got.Requests[0].IssueUID != second {
		t.Fatalf("order %+v, want the later write first", got.Requests)
	}
	// Rewriting the older request moves its Issue's update time, so it comes first.
	f.put(t, f.workP, first, "dana", map[string]any{"from": "bob", "message": "one again"})
	if got := f.read(t, "dana"); len(got.Requests) != 2 || got.Requests[0].IssueUID != first {
		t.Fatalf("order %+v, want the rewritten request first", got.Requests)
	}
}

func TestLoadAttentionFailureCarriesTheRequestNumberAndTheDaemonsMessage(t *testing.T) {
	failing := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"the daemon hiccuped"}}`))
	}), "0.21.0")
	gateway := newGateway(t, newLocator(failing.target()))
	got := as[model.AttentionLoaded](t, run(t, gateway, model.LoadAttention{Workspace: t.TempDir(), Address: "dana", Request: 9}))
	if got.Request != 9 || got.Err == nil || got.Err.Error() != "the daemon hiccuped" || len(got.Requests) != 0 {
		t.Fatalf("answer %+v, want request 9 failing with the daemon's message", got)
	}
}

func TestLoadAttentionWithNoDaemonAnswersWithTheRequestNumberAndTheError(t *testing.T) {
	boom := errors.New("boom")
	l := newLocator(Target{})
	l.err.Store(&boom)
	if got := run(t, newGateway(t, l), model.LoadAttention{Request: 4}); !reflect.DeepEqual(got, model.AttentionLoaded{Request: 4, Err: boom}) {
		t.Fatalf("got %#v", got)
	}
}

func (f attentionFixture) clear(t *testing.T, project model.Project, uid, address, seen string) model.AttentionCleared {
	t.Helper()
	return as[model.AttentionCleared](t, run(t, f.gateway, model.ClearAttention{
		Workspace: f.work, ProjectID: project.ID, UID: uid, Key: model.NotificationKey(address), Seen: seen, Actor: "dana",
	}))
}

func TestClearAttentionWithTheSeenValueRemovesTheSlot(t *testing.T) {
	f := newAttentionFixture(t)
	ours := createIssue(t, f.gateway, f.work, f.workP, "Ours")
	kept := createIssue(t, f.gateway, f.work, f.workP, "Kept")
	f.put(t, f.workP, ours, "dana", map[string]any{"from": "bob", "message": "look at this"})
	f.put(t, f.workP, kept, "dana", map[string]any{"from": "bob", "message": "and at this"})
	seen := ""
	for _, r := range f.read(t, "dana").Requests {
		if r.IssueUID == ours {
			seen = r.Slot.Raw
		}
	}
	if got := f.clear(t, f.workP, ours, "dana", seen); got.Err != nil || got.Changed || got.UID != ours {
		t.Fatalf("clear: %+v", got)
	}
	if left := f.read(t, "dana").Requests; len(left) != 1 || left[0].IssueUID != kept {
		t.Fatalf("requests after the clear: %+v, want only the other Issue's", left)
	}
}

func TestClearAttentionAfterTheSlotChangedReportsItAndLeavesTheNewValue(t *testing.T) {
	f := newAttentionFixture(t)
	ours := createIssue(t, f.gateway, f.work, f.workP, "Ours")
	f.put(t, f.workP, ours, "dana", map[string]any{"from": "bob", "message": "first ask"})
	seen := f.read(t, "dana").Requests[0].Slot.Raw
	f.put(t, f.workP, ours, "dana", map[string]any{"from": "bob", "message": "a newer ask"})

	if got := f.clear(t, f.workP, ours, "dana", seen); got.Err != nil || !got.Changed {
		t.Fatalf("clear: %+v, want the conflict reported", got)
	}
	left := f.read(t, "dana").Requests
	if len(left) != 1 || left[0].Slot.Message != "a newer ask" {
		t.Fatalf("requests after the refused clear: %+v, want the new request kept", left)
	}
}

func TestClearAttentionOfAnUnknownIssueAnswersWithTheDaemonsError(t *testing.T) {
	f := newAttentionFixture(t)
	got := f.clear(t, f.workP, "01JZZZZZZZZZZZZZZZZZZZZZZZ", "dana", `{"from":"bob","message":"m"}`)
	if got.Err == nil || got.Changed {
		t.Fatalf("clear: %+v, want an error", got)
	}
}

func TestATypedReplyToTheCommentARequestNamesRemovesItFromTheNextRead(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Discuss")
	other := createIssue(t, f.gateway, f.work, f.workP, "Elsewhere")
	detail := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.work, UID: uid})).Selected
	}
	draft := func(actor, text, replyTo string, kind model.ReplyKind) {
		t.Helper()
		as[model.DraftSaved](t, run(t, f.gateway, model.SaveDraft{Workspace: f.work, UID: uid, ProjectID: f.workP.ID, BaseText: detail().Issue.Body, Text: text, Comment: true, Actor: actor, ReplyTo: replyTo, ReplyKind: kind}))
	}
	draft("asker", "which one should I pick?", "", model.ReplyPlain)
	question := detail().Comments[0]
	f.put(t, f.workP, uid, "answerer", map[string]any{"from": "asker", "message": "please answer the question", "re": question.UID})
	f.put(t, f.workP, other, "answerer", map[string]any{"from": "asker", "message": "a request on another Issue"})

	before := f.read(t, "answerer")
	if len(before.Requests) != 2 {
		t.Fatalf("requests before the reply: %+v", before.Requests)
	}
	var named model.AttentionRequest
	for _, r := range before.Requests {
		if r.IssueUID == uid {
			named = r
		}
	}
	if named.Slot.Re != question.UID {
		t.Fatalf("the read request names %q, want the comment %q", named.Slot.Re, question.UID)
	}

	draft("answerer", "pick the second one: it reproduces on every run I tried and matches the spec.", question.UID, model.ReplyConfirm)
	after := f.read(t, "answerer").Requests
	if len(after) != 1 || after[0].IssueUID != other {
		t.Fatalf("requests after the reply: %+v, want only the request on the other Issue", after)
	}
}

func TestAgentAttentionReplacesAnEarlierRequestAndClears(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Waiting")
	f.put(t, f.workP, uid, "dana", map[string]any{"from": "bob", "message": "an earlier request"})
	raise := func(message string) {
		t.Helper()
		got := as[model.AgentAttentionWritten](t, run(t, f.gateway, model.RaiseAgentAttention{
			Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Message: message,
		}))
		if got.Err != nil || got.UID != uid || got.Clear {
			t.Fatalf("raise: %+v", got)
		}
	}
	raise("which table?")
	got := f.read(t, "dana").Requests
	if len(got) != 1 || got[0].Slot.From != "dana-agent" || got[0].Slot.Message != "which table?" {
		t.Fatalf("requests after the raise: %+v, want the agent's message replacing the earlier one", got)
	}
	raise("which table?") // the same value again changes nothing and is no error
	cleared := as[model.AgentAttentionWritten](t, run(t, f.gateway, model.ClearAgentAttention{
		Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Messages: []string{"which table?"},
	}))
	if cleared.Err != nil || !cleared.Clear || cleared.UID != uid {
		t.Fatalf("clear: %+v", cleared)
	}
	if left := f.read(t, "dana").Requests; len(left) != 0 {
		t.Fatalf("requests after the clear: %+v", left)
	}
}

func TestAgentAttentionClearLeavesARequestSomeoneElseWroteSince(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Waiting")
	as[model.AgentAttentionWritten](t, run(t, f.gateway, model.RaiseAgentAttention{
		Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Message: "which table?",
	}))
	f.put(t, f.workP, uid, "dana", map[string]any{"from": "bob", "message": "a newer ask"})
	clear := func() model.AgentAttentionWritten {
		return as[model.AgentAttentionWritten](t, run(t, f.gateway, model.ClearAgentAttention{
			Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Messages: []string{"which table?"},
		}))
	}
	if got := clear(); got.Err != nil || !got.Clear {
		t.Fatalf("clear: %+v, want a refused guard to count as done", got)
	}
	if left := f.read(t, "dana").Requests; len(left) != 1 || left[0].Slot.From != "bob" || left[0].Slot.Message != "a newer ask" {
		t.Fatalf("requests after the clear: %+v, want the newer request kept", left)
	}
	f.put(t, f.workP, uid, "dana", nil)
	if got := clear(); got.Err != nil {
		t.Fatalf("clear of a slot that is gone: %+v", got)
	}
}

func TestAgentAttentionClearTakesAnyMessageAnAgentRaisedAndKeepsAnyOtherRequest(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Waiting")
	clear := func() model.AgentAttentionWritten {
		return as[model.AgentAttentionWritten](t, run(t, f.gateway, model.ClearAgentAttention{
			Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Messages: []string{"msgA", "msgB"},
		}))
	}
	// Another Pane's agent on the Issue overwrote this Pane's request with msgB: still a request an agent raised.
	f.put(t, f.workP, uid, "dana", map[string]any{"from": "dana-agent", "message": "msgB"})
	if got := clear(); got.Err != nil || !got.Clear {
		t.Fatalf("clear: %+v", got)
	}
	if left := f.read(t, "dana").Requests; len(left) != 0 {
		t.Fatalf("requests after the clear: %+v, want the leftover request cleared", left)
	}
	// Someone else's request, and this actor's with a message no agent raised, both survive.
	f.put(t, f.workP, uid, "dana", map[string]any{"from": "bob", "message": "msgA"})
	clear()
	if left := f.read(t, "dana").Requests; len(left) != 1 || left[0].Slot.From != "bob" {
		t.Fatalf("requests after the clear: %+v, want another person's request kept", left)
	}
	f.put(t, f.workP, uid, "dana", map[string]any{"from": "dana-agent", "message": "typed by hand"})
	clear()
	if left := f.read(t, "dana").Requests; len(left) != 1 || left[0].Slot.Message != "typed by hand" {
		t.Fatalf("requests after the clear: %+v, want a message no agent raised kept", left)
	}
}

func TestAgentAttentionRaisedWithATeammateNamesItAndIsClearedWithoutOne(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Waiting")
	raise := func(teammate, message string) {
		t.Helper()
		got := as[model.AgentAttentionWritten](t, run(t, f.gateway, model.RaiseAgentAttention{
			Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Teammate: teammate, Address: "dana", Message: message,
		}))
		if got.Err != nil {
			t.Fatalf("raise: %+v", got)
		}
	}
	raise("builder", "which table?")
	got := f.read(t, "dana").Requests
	if len(got) != 1 || got[0].Slot.From != "dana-agent" || got[0].Slot.Teammate != "builder" || got[0].Sender() != "dana-agent/builder" {
		t.Fatalf("requests after the first raise: %+v, want sender dana-agent/builder", got)
	}
	raise("reviewer", "which column?")
	got = f.read(t, "dana").Requests
	if len(got) != 1 || got[0].Sender() != "dana-agent/reviewer" {
		t.Fatalf("requests after the second raise: %+v, want sender dana-agent/reviewer", got)
	}
	raise("", "which column?")
	if got = f.read(t, "dana").Requests; len(got) != 1 || got[0].Slot.Teammate != "" || got[0].Sender() != "dana-agent" {
		t.Fatalf("requests after a raise with no name: %+v, want no teammate", got)
	}
	raise("reviewer", "which column?")
	cleared := as[model.AgentAttentionWritten](t, run(t, f.gateway, model.ClearAgentAttention{
		Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana-agent", Address: "dana", Messages: []string{"which column?"},
	}))
	if cleared.Err != nil || !cleared.Clear {
		t.Fatalf("clear: %+v", cleared)
	}
	if left := f.read(t, "dana").Requests; len(left) != 0 {
		t.Fatalf("requests after the clear: %+v, want the request with a teammate cleared", left)
	}
}

func TestAMessageToAnAgentIsWrittenOnlyOverWhatThePersonAgreedToReplace(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Agent at work")
	const agent = "dana/omp-31"
	send := func(message, replacing string) model.AgentMessageSent {
		t.Helper()
		return as[model.AgentMessageSent](t, run(t, f.gateway, model.SendAgentMessage{
			Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana", Address: agent, Message: message, Replacing: replacing,
		}))
	}
	slot := func() model.AttentionRequest {
		t.Helper()
		got := f.read(t, agent).Requests
		if len(got) != 1 {
			t.Fatalf("requests for the agent: %+v, want one", got)
		}
		return got[0]
	}
	if got := send("use the second table", ""); got.Err != nil || got.UID != uid || got.Held != "" {
		t.Fatalf("first message: %+v", got)
	}
	first := slot()
	if first.Slot.From != "dana" || first.Slot.Message != "use the second table" || first.Slot.Teammate != "" {
		t.Fatalf("slot after the first message: %+v", first.Slot)
	}
	// The agent has not cleared it: a second message writes nothing and says what is held.
	held := send("no, the third", "")
	if held.Err != nil || held.Held != first.Slot.Raw || held.Unread != "use the second table" {
		t.Fatalf("second message over an unread one: %+v", held)
	}
	if got := slot(); got.Slot.Message != "use the second table" {
		t.Fatalf("the unread message was replaced without asking: %+v", got.Slot)
	}
	// Someone rewrites the slot after the person agreed to replace the first message: the new value is held instead.
	f.put(t, f.workP, uid, agent, map[string]any{"from": "bob", "message": "from bob"})
	again := send("no, the third", held.Held)
	if again.Err != nil || again.Held == "" || again.Held == held.Held || again.Unread != "from bob" {
		t.Fatalf("replacing a value that changed meanwhile: %+v", again)
	}
	if got := send("no, the third", again.Held); got.Err != nil || got.Held != "" {
		t.Fatalf("replacing the value held: %+v", got)
	}
	if got := slot(); got.Slot.From != "dana" || got.Slot.Message != "no, the third" {
		t.Fatalf("slot after the agreed replacement: %+v", got.Slot)
	}
	// The agent cleared the slot after the person agreed to replace it: the message is written all the same.
	replaced := slot().Slot.Raw
	f.put(t, f.workP, uid, agent, nil)
	if got := send("one more thing", replaced); got.Err != nil || got.Held != "" {
		t.Fatalf("replacing a slot the agent cleared meanwhile: %+v", got)
	}
	if got := slot(); got.Slot.Message != "one more thing" {
		t.Fatalf("slot after the agent cleared it: %+v", got.Slot)
	}
}

func TestAMessageAskingAnAgentToAnswerACommentCarriesTheHandleInTheSlot(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Agent at work")
	const agent = "dana/omp-31"
	sent := as[model.AgentMessageSent](t, run(t, f.gateway, model.SendAgentMessage{
		Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana", Address: agent, Message: "is this still true?", Re: "c:abc123",
	}))
	if sent.Err != nil || sent.Held != "" {
		t.Fatalf("message about a comment: %+v", sent)
	}
	got := f.read(t, agent).Requests
	if len(got) != 1 || got[0].Slot.Message != "is this still true?" || got[0].Slot.Re != "c:abc123" || got[0].CommentHandle() != "c:abc123" {
		t.Fatalf("requests for the agent: %+v, want one naming c:abc123", got)
	}
}

func TestAnExitedAgentsAddressIsClearedWhoeverWroteToIt(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Agent gone")
	const agent = "dana/omp-31"
	clear := func() model.AgentAddressCleared {
		return as[model.AgentAddressCleared](t, run(t, f.gateway, model.ClearAgentAddress{Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana", Address: agent}))
	}
	for _, from := range []string{"dana", "bob"} {
		f.put(t, f.workP, uid, agent, map[string]any{"from": from, "message": "unread"})
		if got := clear(); got.Err != nil || got.UID != uid {
			t.Fatalf("clear of %s's message: %+v", from, got)
		}
		if left := f.read(t, agent).Requests; len(left) != 0 {
			t.Fatalf("after the clear of %s's message: %+v", from, left)
		}
	}
	if got := clear(); got.Err != nil {
		t.Fatalf("clear of an empty Agent address: %+v", got)
	}
}

func TestHandOffAgentWritesTheAttentionMetadataAndSwapsTheReadyLabel(t *testing.T) {
	f := newAttentionFixture(t)
	uid := createIssue(t, f.gateway, f.work, f.workP, "Stopped")
	if w := as[model.FieldWritten](t, run(t, f.gateway, model.WriteField{Workspace: f.work, UID: uid, ProjectID: f.workP.ID, Actor: "tester", Action: model.AddLabelAction, Value: "ready-for-agent"})); w.Err != nil {
		t.Fatal(w.Err)
	}
	got := as[model.AgentHandedOff](t, run(t, f.gateway, model.HandOffAgent{
		Workspace: f.work, ProjectID: f.workP.ID, UID: uid, Actor: "dana", Message: model.AgentStopHandOffMessage, SwapLabels: true,
	}))
	if got.Err != nil || got.UID != uid {
		t.Fatalf("hand-off: %+v", got)
	}
	shown := issueFixture{kata: f.kata}.issue(t, uid)
	for key, want := range map[string]string{"work.attention": "needs-human", "work.attention_msg": model.AgentStopHandOffMessage} {
		if got := shown.Metadata[key]; got != want {
			t.Errorf("%s = %v, want %s", key, got, want)
		}
	}
	if labels := shown.Labels; !reflect.DeepEqual(labels, []string{"ready-for-human"}) {
		t.Errorf("labels %v, want ready-for-human only", labels)
	}

	plain := createIssue(t, f.gateway, f.work, f.workP, "No ready label")
	if got := as[model.AgentHandedOff](t, run(t, f.gateway, model.HandOffAgent{Workspace: f.work, ProjectID: f.workP.ID, UID: plain, Actor: "dana", Message: "m"})); got.Err != nil {
		t.Fatal(got.Err)
	}
	if labels := (issueFixture{kata: f.kata}).issue(t, plain).Labels; len(labels) != 0 {
		t.Errorf("labels %v, want none touched", labels)
	}
}
