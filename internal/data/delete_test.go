package data

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (f *undoFixture) qualified(uid string) string {
	f.t.Helper()
	return f.detail(uid).Issue.QualifiedID
}

func (f *undoFixture) deleteCmd(uid, qualified, reason string) model.DeleteIssueCmd {
	return model.DeleteIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", QualifiedID: qualified, Reason: reason}
}

func (f *undoFixture) deleteIssue(uid, reason string) model.IssueDeleted {
	f.t.Helper()
	return as[model.IssueDeleted](f.t, run(f.t, f.gateway, f.deleteCmd(uid, f.qualified(uid), reason)))
}

// listed says the Collection snapshot of the Project holds the Issue.
func (f *undoFixture) listed(uid string) bool {
	f.t.Helper()
	view := "project"
	loaded := as[model.SnapshotLoaded](f.t, run(f.t, f.gateway, model.ReadSnapshot{Workspace: f.workspace, Request: 1, Query: model.SnapshotQuery{View: &view, ProjectUID: &f.project.UID}}))
	for _, issue := range loaded.Snapshot.Issues {
		if issue.UID == uid {
			return true
		}
	}
	return false
}

func (f *undoFixture) lookup(ref string, projectID int64, byUID bool) model.DeletedLooked {
	f.t.Helper()
	return as[model.DeletedLooked](f.t, run(f.t, f.gateway, model.LookupDeleted{Workspace: f.workspace, Typed: ref, Ref: ref, ProjectID: projectID, ByUID: byUID}))
}

func (f *undoFixture) restore(uid string) model.IssueRestored {
	f.t.Helper()
	return as[model.IssueRestored](f.t, run(f.t, f.gateway, model.RestoreIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester"}))
}

// commentBodies are the bodies of the comments the Issue holds, read with deleted Issues included.
func (f *undoFixture) commentBodies(uid string) []string {
	f.t.Helper()
	client, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		f.t.Fatal(err)
	}
	shown, err := client.showIssueIncludingDeleted(context.Background(), f.project.ID, uid)
	if err != nil {
		f.t.Fatal(err)
	}
	var bodies []string
	for _, c := range shown.Comments {
		bodies = append(bodies, c.Body)
	}
	return bodies
}

func TestDeleteKeepsTheReasonAsACommentAndLookupDeletedReadsItBack(t *testing.T) {
	f := newUndoFixture(t)
	for reason, want := range map[string]string{"dup": "dup", "": ""} {
		uid := f.issue("Gone " + reason)
		if got := f.deleteIssue(uid, reason); got.Err != nil {
			t.Fatal(got.Err)
		}
		bodies := f.commentBodies(uid)
		if want == "" && len(bodies) != 0 {
			t.Errorf("no reason: comments %q, want none", bodies)
		}
		if want != "" && (len(bodies) != 1 || bodies[0] != "Deleted: dup") {
			t.Errorf("comments %q, want the reason as a comment", bodies)
		}
		got := f.lookup(uid, 0, true)
		if got.Err != nil || !got.Issue.Deleted {
			t.Fatalf("lookup %+v", got)
		}
		if got.Issue.DeletedBy != "tester" || got.Issue.DeletedAt.IsZero() || got.Issue.Reason != want {
			t.Errorf("reason %q: deleted by %q at %v with reason %q", reason, got.Issue.DeletedBy, got.Issue.DeletedAt, got.Issue.Reason)
		}
	}
}

func TestLookupDeletedReadsTheNewestDeletesReasonOnly(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Twice")
	if got := f.deleteIssue(uid, "first"); got.Err != nil {
		t.Fatal(got.Err)
	}
	if got := f.restore(uid); got.Err != nil {
		t.Fatal(got.Err)
	}
	// A comment by the same actor without the prefix is not a reason.
	client, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	actor := "tester"
	if _, err := client.CreateCommentWithResponse(context.Background(), &g.CreateCommentRequestOptions{
		PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(f.project.ID), Ref: uid},
		Body:       &g.CreateCommentBody{Body: "plain remark", Actor: &actor},
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.deleteIssue(uid, ""); got.Err != nil {
		t.Fatal(got.Err)
	}
	if got := f.lookup(uid, 0, true); got.Err != nil || got.Issue.Reason != "" {
		t.Errorf("lookup %+v, want the second delete's empty reason", got)
	}
}

func TestLookupDeletedStillFindsTheDeleteAfterAnotherIssueWasPurged(t *testing.T) {
	f := newUndoFixture(t)
	other := f.issue("Purge me")
	client, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	confirm := "PURGE " + f.qualified(other)
	purged, err := client.PurgeIssueWithResponse(context.Background(), &g.PurgeIssueRequestOptions{
		PathParams: &g.PurgeIssuePath{ProjectID: f.project.ID, Ref: other},
		Header:     &g.PurgeIssueHeaders{XKataConfirm: &confirm},
		Body:       &g.PurgeIssueBody{Actor: "tester"},
	})
	if err != nil || purged.StatusCode >= 300 {
		t.Fatalf("purge: %v %d", err, purged.StatusCode)
	}
	uid := f.issue("Gone after a purge")
	if got := f.deleteIssue(uid, "dup"); got.Err != nil {
		t.Fatal(got.Err)
	}
	got := f.lookup(uid, 0, true)
	if got.Err != nil || !got.Issue.Described || got.Issue.DeletedBy != "tester" || got.Issue.Reason != "dup" {
		t.Errorf("lookup %+v, want the actor and reason although the log asked for a reset", got)
	}
}

func TestLookupDeletedReadsOnlyTheReasonAfterTheLastRestore(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Round trip")
	for _, step := range []func() error{
		func() error { return f.deleteIssue(uid, "first").Err },
		func() error { return f.restore(uid).Err },
		func() error { return f.deleteIssue(uid, "").Err },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	// The first delete's comment sits before the restore, so it cannot be the second delete's reason.
	got := f.lookup(uid, 0, true)
	if !got.Issue.Described || got.Issue.Reason != "" {
		t.Errorf("lookup %+v, want a described delete with no reason", got)
	}
}

func TestOneActorsReasonIsNotAnotherActorsDelete(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Two hands")
	client, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	other := "someone-else"
	if _, err := client.CreateCommentWithResponse(context.Background(), &g.CreateCommentRequestOptions{
		PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(f.project.ID), Ref: uid},
		Body:       &g.CreateCommentBody{Body: "Deleted: not mine", Actor: &other},
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.deleteIssue(uid, "mine"); got.Err != nil {
		t.Fatal(got.Err)
	}
	if got := f.lookup(uid, 0, true); got.Issue.Reason != "mine" {
		t.Errorf("reason %q, want the deleting actor's own comment", got.Issue.Reason)
	}
}

func TestARetriedDeleteAfterARefusalPostsTheReasonOnce(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Retry me")
	refuse := true
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if !refuse || r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/actions/delete") {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"try again"}}`))
		return true
	}
	f.kata.intercept.Store(&hook)
	if got := f.deleteIssue(uid, "dup"); got.Err == nil {
		t.Fatal("the first delete was not refused")
	}
	refuse = false
	if got := f.deleteIssue(uid, "dup"); got.Err != nil {
		t.Fatal(got.Err)
	}
	if bodies := f.commentBodies(uid); len(bodies) != 1 || bodies[0] != "Deleted: dup" {
		t.Errorf("comments %q, want the reason once", bodies)
	}
}

func TestADeleteAfterARestoreWithTheSameReasonPostsItAgain(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Again")
	for i := 0; i < 2; i++ {
		if got := f.deleteIssue(uid, "dup"); got.Err != nil {
			t.Fatal(got.Err)
		}
		if got := f.restore(uid); got.Err != nil {
			t.Fatal(got.Err)
		}
	}
	if bodies := f.commentBodies(uid); len(bodies) != 2 {
		t.Errorf("comments %q, want one per delete", bodies)
	}
}

func TestAFailingReasonCommentSendsNoDelete(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Keep me")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/comments") {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"disk full"}}`))
		return true
	}
	f.kata.intercept.Store(&hook)
	got := f.deleteIssue(uid, "dup")
	if got.Err == nil || got.Record.Entry != nil {
		t.Fatalf("delete %+v, want the comment's error and no undo entry", got)
	}
	if !f.listed(uid) {
		t.Error("the Issue was deleted although its reason could not be written")
	}
}

func TestDeleteSendsKatasConfirmationHeaderAndTheActor(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Delete me")
	qualified := f.qualified(uid)
	if !strings.HasPrefix(qualified, "demo#") {
		t.Fatalf("qualified id %q", qualified)
	}

	var confirm, body string
	// The hook records the request and leaves it intact for kata.
	record := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/delete") {
			confirm = r.Header.Get("X-Kata-Confirm")
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		return false
	}
	f.kata.intercept.Store(&record)

	deleted := f.deleteIssue(uid, "duplicate of another")
	if deleted.Err != nil || deleted.UID != uid {
		t.Fatalf("delete %+v", deleted)
	}
	if confirm != "DELETE "+qualified {
		t.Errorf("X-Kata-Confirm %q, want %q", confirm, "DELETE "+qualified)
	}
	if strings.Contains(body, "reason") || !strings.Contains(body, `"actor":"tester"`) {
		t.Errorf("delete body %s, want the actor and no reason", body)
	}
	if deleted.Record.Entry == nil || deleted.Record.Entry.Kind != model.UndoDelete {
		t.Errorf("record %+v, want an undoable delete", deleted.Record)
	}
}

func TestDeleteIsAcceptedOnlyWithKatasConfirmation(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Careful")
	qualified := f.qualified(uid)

	for _, wrong := range []string{"demo#nope", strings.ToUpper(qualified), ""} {
		got := as[model.IssueDeleted](t, run(t, f.gateway, f.deleteCmd(uid, wrong, "")))
		if got.Err == nil || got.Record.Entry != nil {
			t.Errorf("confirmation for %q: %+v, want kata's refusal and no undo entry", wrong, got)
		}
		if !f.listed(uid) {
			t.Fatalf("the Issue left the Collection after the refused confirmation %q", wrong)
		}
	}

	// kata's own words reach the card.
	got := as[model.IssueDeleted](t, run(t, f.gateway, f.deleteCmd(uid, "demo#nope", "")))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "X-Kata-Confirm header value does not match") || !strings.Contains(got.Err.Error(), "expected DELETE "+qualified) {
		t.Errorf("refusal %v", got.Err)
	}

	// Without the header kata asks for it.
	c, err := connect(context.Background(), f.kata.target())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.DeleteIssueWithResponse(context.Background(), &g.DeleteIssueRequestOptions{
		PathParams: &g.DeleteIssuePath{ProjectID: f.project.ID, Ref: uid},
		Body:       &g.DeleteIssueBody{Actor: "tester"},
	})
	if err == nil || daemonCode(err) != "confirm_required" {
		t.Errorf("a delete without the header: %v, want confirm_required", err)
	}

	if f.deleteIssue(uid, "").Err != nil {
		t.Fatal("the exact confirmation was refused")
	}
}

func TestADeletedIssueLeavesTheCollectionAndRestoreBringsItBackWithItsHistory(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Come back")
	qualified := f.qualified(uid)
	f.field(uid, model.AssignOwnerAction, "alice")
	before := f.detail(uid).History

	if got := f.deleteIssue(uid, ""); got.Err != nil {
		t.Fatal(got.Err)
	}
	if f.listed(uid) {
		t.Fatal("a deleted Issue is still in the Collection snapshot")
	}

	// It is read with deleted Issues included, by ULID and by short id.
	_, issueShort, _ := strings.Cut(qualified, "#")
	for name, got := range map[string]model.DeletedLooked{
		"ULID":     f.lookup(uid, 0, true),
		"short id": f.lookup(issueShort, f.project.ID, false),
	} {
		if got.Err != nil || got.Issue.UID != uid || got.Issue.Title != "Come back" || !got.Issue.Deleted || got.Issue.ProjectID != f.project.ID || got.Issue.ShortID != issueShort {
			t.Errorf("%s: lookup %+v, want the deleted Issue with its title", name, got)
		}
	}

	restored := f.restore(uid)
	if restored.Err != nil || !restored.Changed {
		t.Fatalf("restore %+v", restored)
	}
	if !f.listed(uid) {
		t.Fatal("the restored Issue is not back in the Collection")
	}
	after := f.detail(uid)
	if after.Issue.Title != "Come back" || after.Issue.Owner == nil || *after.Issue.Owner != "alice" {
		t.Errorf("restored Issue %+v lost what it held", after.Issue)
	}
	if len(after.History) <= len(before) {
		t.Fatalf("history has %d events after delete and restore, had %d before; want the old ones and the new", len(after.History), len(before))
	}
	for _, e := range before {
		if !slices.ContainsFunc(after.History, func(a model.Event) bool { return a.UID == e.UID }) {
			t.Errorf("the restore lost history event %s %s", e.UID, e.Type)
		}
	}
	if got := f.lookup(issueShort, f.project.ID, false); got.Err != nil || got.Issue.Deleted {
		t.Errorf("after the restore the lookup says %+v, want an Issue that is not deleted", got)
	}
}

func TestRestoringAnIssueThatIsNotDeletedReportsKatasAnswer(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Never deleted")
	got := f.restore(uid)
	if got.Err != nil || got.Changed || got.UID != uid {
		t.Fatalf("restore of a live Issue %+v, want kata's answer: no change", got)
	}
}

func TestLookupOfAnUnknownDeletedReferenceReportsKatasRefusal(t *testing.T) {
	f := newUndoFixture(t)
	f.issue("Something")
	got := f.lookup("zzzz", f.project.ID, false)
	if got.Err == nil || got.Issue.UID != "" || got.Ref != "zzzz" {
		t.Fatalf("lookup %+v, want kata's refusal", got)
	}
}

func TestUndoOfADeleteRestoresTheIssue(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Undo my delete")
	e := f.entry(f.deleteIssue(uid, "oops").Record)
	if e.Kind != model.UndoDelete || e.UID != uid {
		t.Fatalf("entry %+v", e)
	}
	if f.listed(uid) {
		t.Fatal("the delete did not take")
	}

	got := f.restores(e)
	if got.Ref == "" {
		t.Errorf("undo named no Issue: %+v", got)
	}
	if !f.listed(uid) {
		t.Fatal("undo did not bring the Issue back")
	}

	// Undoing it again: the Issue is no longer deleted.
	if again := f.undo(e); again.Result != model.UndoAlready || again.Err != nil {
		t.Fatalf("second undo %+v, want already restored", again)
	}
}

func TestUndoOfADeleteSaysAlreadyRestoredWhenSomeoneRestoredIt(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Restored elsewhere")
	e := f.entry(f.deleteIssue(uid, "").Record)
	f.rival(uid, func(c *Client) {
		if _, err := c.restoreRaw(context.Background(), f.project.ID, uid, "rival"); err != nil {
			t.Fatal(err)
		}
	})
	got := f.undo(e)
	if got.Result != model.UndoAlready || got.Err != nil {
		t.Fatalf("undo %+v, want already restored", got)
	}
}

func TestUndoOfADeleteRefusesAnotherAccount(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Wrong place")
	e := f.entry(f.deleteIssue(uid, "").Record)
	e.InstanceUID = "another-instance"
	if got := f.undo(e); got.Result != model.UndoConflict || got.Conflict != "daemon instance changed" {
		t.Fatalf("undo %+v", got)
	}
	if f.listed(uid) {
		t.Fatal("a refused undo restored the Issue")
	}
}
