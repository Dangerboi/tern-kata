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
	kata := startKata(t, "0.20.0")
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
	}), "0.20.0")
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
