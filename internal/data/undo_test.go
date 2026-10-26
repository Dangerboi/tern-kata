package data

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

type undoFixture struct {
	t         *testing.T
	kata      *kataServer
	gateway   *Gateway
	workspace string
	project   model.Project
	next      uint64
}

func newUndoFixture(t *testing.T) *undoFixture {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	return &undoFixture{t: t, kata: kata, gateway: gateway, workspace: workspace, project: demo}
}

func (f *undoFixture) issue(title string) string {
	return createIssue(f.t, f.gateway, f.workspace, f.project, title)
}

func (f *undoFixture) detail(uid string) model.IssueDetailData {
	f.t.Helper()
	return *as[model.DetailLoaded](f.t, run(f.t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: uid})).Selected
}

func (f *undoFixture) field(uid string, action model.Action, value string) model.WriteRecord {
	f.t.Helper()
	written := as[model.FieldWritten](f.t, run(f.t, f.gateway, model.WriteField{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Action: action, Value: value}))
	if written.Err != nil {
		f.t.Fatalf("%s %q: %v", action, value, written.Err)
	}
	return written.Record
}

// entry is the change a write recorded, numbered as the Pane's history numbers it.
func (f *undoFixture) entry(rec model.WriteRecord) model.UndoEntry {
	f.t.Helper()
	if rec.Entry == nil {
		f.t.Fatalf("the write recorded %+v, want an undo entry", rec)
	}
	f.next++
	e := *rec.Entry
	e.ID = f.next
	return e
}

func (f *undoFixture) undo(e model.UndoEntry) model.ChangeUndone {
	f.t.Helper()
	return as[model.ChangeUndone](f.t, run(f.t, f.gateway, model.UndoChange{Workspace: f.workspace, Entry: e}))
}

// restores undoes the change and expects it to have been written.
func (f *undoFixture) restores(e model.UndoEntry) model.ChangeUndone {
	f.t.Helper()
	got := f.undo(e)
	if got.Result != model.UndoChanged || got.Err != nil {
		f.t.Fatalf("undo of %s: %+v, want it written", e.Kind, got)
	}
	return got
}

// refuses undoes the change and expects the refusal reason with nothing written: the revision stays where it was.
func (f *undoFixture) refuses(uid string, e model.UndoEntry, reason string) {
	f.t.Helper()
	before := f.detail(uid).Issue.Revision
	got := f.undo(e)
	if got.Result != model.UndoConflict || got.Conflict != reason {
		f.t.Fatalf("undo of %s: %+v, want the refusal %q", e.Kind, got, reason)
	}
	if after := f.detail(uid).Issue.Revision; after != before {
		f.t.Fatalf("the refused undo wrote: revision %d, was %d", after, before)
	}
}

// rival writes to the Issue as another account would, around the gateway.
func (f *undoFixture) rival(uid string, write func(c *Client)) {
	f.t.Helper()
	c, err := connect(context.Background(), f.kata.target())
	if err != nil {
		f.t.Fatal(err)
	}
	write(c)
}

func (f *undoFixture) rivalField(uid string, action model.Action, value string) {
	f.t.Helper()
	f.rival(uid, func(c *Client) {
		if err := c.WriteField(context.Background(), model.WriteField{UID: uid, ProjectID: f.project.ID, Actor: "rival", Action: action, Value: value}); err != nil {
			f.t.Fatal(err)
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestUndoRestoresEachChange(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Undo me")

	// Priority, from none and from a value.
	e := f.entry(f.field(uid, model.PriorityAction, "2"))
	f.restores(e)
	if p := f.detail(uid).Issue.Priority; p != nil {
		t.Fatalf("priority %d after undoing the first set", *p)
	}
	f.field(uid, model.PriorityAction, "1")
	e = f.entry(f.field(uid, model.PriorityAction, "3"))
	f.restores(e)
	if p := f.detail(uid).Issue.Priority; p == nil || *p != 1 {
		t.Fatalf("priority %v, want the earlier 1", p)
	}

	// Owner, from none and from another owner.
	e = f.entry(f.field(uid, model.AssignOwnerAction, "alice"))
	f.restores(e)
	if o := f.detail(uid).Issue.Owner; o != nil {
		t.Fatalf("owner %q, want it unassigned again", *o)
	}
	f.field(uid, model.AssignOwnerAction, "alice")
	e = f.entry(f.field(uid, model.AssignOwnerAction, "bob"))
	f.restores(e)
	if o := f.detail(uid).Issue.Owner; o == nil || *o != "alice" {
		t.Fatalf("owner %v, want alice", o)
	}
	e = f.entry(f.field(uid, model.ClearOwnerAction, ""))
	f.restores(e)
	if o := f.detail(uid).Issue.Owner; o == nil || *o != "alice" {
		t.Fatalf("owner %v after undoing an unassign, want alice", o)
	}

	// Labels.
	e = f.entry(f.field(uid, model.AddLabelAction, "bug"))
	f.restores(e)
	if labels := f.detail(uid).Issue.Labels; slices.Contains(labels, "bug") {
		t.Fatalf("labels %v", labels)
	}
	f.field(uid, model.AddLabelAction, "bug")
	e = f.entry(f.field(uid, model.RemoveLabelAction, "bug"))
	f.restores(e)
	if labels := f.detail(uid).Issue.Labels; !slices.Contains(labels, "bug") {
		t.Fatalf("labels %v, want bug back", labels)
	}

	// Body.
	saved := as[model.DraftSaved](t, run(t, f.gateway, model.SaveDraft{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, BaseText: "body", Text: "rewritten", Actor: "tester"}))
	e = f.entry(saved.Record)
	f.restores(e)
	if body := f.detail(uid).Issue.Body; body != "body" {
		t.Fatalf("body %q", body)
	}

	// Close and reopen.
	closed := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Quick: true, Reason: model.ReasonDone, Key: "k-1"}))
	e = f.entry(closed.Record)
	f.restores(e)
	if status := f.detail(uid).Issue.Status; status != "open" {
		t.Fatalf("status %q after undoing a close", status)
	}
	closed = as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Quick: true, Reason: model.ReasonDone, Key: "k-2"}))
	reopened := as[model.CloseResultMsg](t, run(t, f.gateway, model.ReopenIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester"}))
	_ = closed
	e = f.entry(reopened.Record)
	f.restores(e)
	if issue := f.detail(uid).Issue; issue.Status != "closed" || issue.ClosedReason == nil || *issue.ClosedReason != "done" {
		t.Fatalf("issue %+v, want it closed as done again", issue)
	}
}

func TestUndoRemovesTheLinkAddedByType(t *testing.T) {
	f := newUndoFixture(t)
	first, second := f.issue("First"), f.issue("Second")
	ref := f.detail(second).Issue.ShortID
	for _, action := range []model.Action{model.AddBlockerAction, model.AddRelatedAction} {
		e := f.entry(f.field(first, action, ref))
		if e.Link == nil || e.Link.FromUID != first || e.Link.ToUID != second || e.Link.ID == 0 {
			t.Fatalf("%s recorded %+v", action, e.Link)
		}
		if links := f.detail(first).Links; len(links) != 1 {
			t.Fatalf("links %+v", links)
		}
		f.restores(e)
		if links := f.detail(first).Links; len(links) != 0 {
			t.Fatalf("%s: links %+v after the undo", action, links)
		}
		if again := f.undo(e); again.Result != model.UndoAlready {
			t.Fatalf("undoing it twice: %+v, want already restored", again)
		}
	}
}

func TestUndoPutsALaneMoveBack(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("On the Board")
	f.field(uid, model.AddLabelAction, "lane:todo")
	moved := as[model.MoveResultMsg](t, run(t, f.gateway, model.MoveIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Add: "lane:doing", Remove: []string{"lane:todo"}}))
	e := f.entry(moved.Record)
	if e.Kind != model.UndoLaneMove || !slices.Equal(e.Added, []string{"lane:doing"}) || !slices.Equal(e.Removed, []string{"lane:todo"}) {
		t.Fatalf("entry %+v", e)
	}
	f.restores(e)
	if labels := f.detail(uid).Issue.Labels; !slices.Contains(labels, "lane:todo") || slices.Contains(labels, "lane:doing") {
		t.Fatalf("labels %v, want the card back in todo", labels)
	}
}

func TestUndoOfALaneMoveCompletesAfterTheSecondLabelWriteFailed(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("On the Board")
	f.field(uid, model.AddLabelAction, "lane:todo")
	moved := as[model.MoveResultMsg](t, run(t, f.gateway, model.MoveIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Add: "lane:doing", Remove: []string{"lane:todo"}}))
	e := f.entry(moved.Record)

	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/labels/") {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"status":422,"error":{"code":"x","message":"no"}}`))
		return true
	}
	f.kata.intercept.Store(&hook)
	if got := f.undo(e); got.Result != model.UndoFailed || got.Err == nil {
		t.Fatalf("undo %+v, want the second label write to fail", got)
	}
	f.kata.intercept.Store(nil)
	if labels := f.detail(uid).Issue.Labels; !slices.Contains(labels, "lane:doing") || !slices.Contains(labels, "lane:todo") {
		t.Fatalf("labels %v, want only the first label write applied", labels)
	}
	f.restores(e)
	if labels := f.detail(uid).Issue.Labels; !slices.Contains(labels, "lane:todo") || slices.Contains(labels, "lane:doing") {
		t.Fatalf("labels %v, want the card back in todo", labels)
	}
}

func TestUndoOfALaneMoveRefusesWhenAnotherActorMovedTheCardOn(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("On the Board")
	f.field(uid, model.AddLabelAction, "lane:todo")
	moved := as[model.MoveResultMsg](t, run(t, f.gateway, model.MoveIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Add: "lane:doing", Remove: []string{"lane:todo"}}))
	e := f.entry(moved.Record)
	f.rivalField(uid, model.RemoveLabelAction, "lane:doing")
	f.rivalField(uid, model.AddLabelAction, "lane:review")
	f.refuses(uid, e, "issue changed since the Lane move")
	if labels := f.detail(uid).Issue.Labels; slices.Contains(labels, "lane:todo") || !slices.Contains(labels, "lane:review") {
		t.Fatalf("labels %v, want the rival's Lane kept", labels)
	}
}

func TestUndoOfALaneMoveFromNoLaneRefusesWhenAnotherActorMovedTheCardOn(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("On the Board")
	moved := as[model.MoveResultMsg](t, run(t, f.gateway, model.MoveIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Add: "lane:doing"}))
	e := f.entry(moved.Record)
	f.rivalField(uid, model.RemoveLabelAction, "lane:doing")
	f.rivalField(uid, model.AddLabelAction, "lane:done")
	f.refuses(uid, e, "issue changed since the Lane move")
	if labels := f.detail(uid).Issue.Labels; !slices.Equal(labels, []string{"lane:done"}) {
		t.Fatalf("labels %v, want the rival's Lane kept", labels)
	}
}

func TestUndoOfDatesCompletesAfterTheSecondWriteFailed(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Calendar entry")
	set := func(changes ...model.DateChange) model.DatesResultMsg {
		return as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Changes: changes}))
	}
	scheduled := model.DateChange{Key: model.ScheduledKey, Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: model.DeadlineKey, Label: "Deadline", Value: "2026-10-30"}
	e := f.entry(set(scheduled, deadline).Record)

	patches := 0
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/metadata") {
			return false
		}
		if patches++; patches != 2 {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"status":422,"error":{"code":"x","message":"no"}}`))
		return true
	}
	f.kata.intercept.Store(&hook)
	if got := f.undo(e); got.Err == nil {
		t.Fatalf("undo %+v, want the second date write to fail", got)
	}
	f.kata.intercept.Store(nil)
	meta := f.detail(uid).Issue.Metadata
	if _, still := meta[model.ScheduledKey]; still || meta[model.DeadlineKey] != "2026-10-30" {
		t.Fatalf("dates %v, want only the first one restored", meta)
	}
	f.restores(e)
	if meta := f.detail(uid).Issue.Metadata; meta[model.ScheduledKey] != nil || meta[model.DeadlineKey] != nil {
		t.Fatalf("dates %v, want both cleared", meta)
	}
}

func TestUndoPutsBothDatesBack(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Calendar entry")
	set := func(changes ...model.DateChange) model.DatesResultMsg {
		return as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Changes: changes}))
	}
	scheduled := model.DateChange{Key: model.ScheduledKey, Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: model.DeadlineKey, Label: "Deadline", Value: "2026-10-30"}
	if got := set(scheduled, deadline); got.Err != nil {
		t.Fatal(got.Err)
	}
	scheduled.Was, scheduled.Value, deadline.Was, deadline.Value = "2026-10-12", "2026-10-20", "2026-10-30", ""
	changed := set(scheduled, deadline)
	e := f.entry(changed.Record)
	f.restores(e)
	meta := f.detail(uid).Issue.Metadata
	if meta[model.ScheduledKey] != "2026-10-12" || meta[model.DeadlineKey] != "2026-10-30" {
		t.Fatalf("dates %v, want the earlier two back", meta)
	}
	// A first date is cleared by undoing it.
	other := f.issue("Zebra crossing")
	first := as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.workspace, UID: other, ProjectID: f.project.ID, Actor: "tester", Changes: []model.DateChange{{Key: model.ScheduledKey, Label: "Scheduled", Value: "2026-11-01"}}}))
	f.restores(f.entry(first.Record))
	if _, still := f.detail(other).Issue.Metadata[model.ScheduledKey]; still {
		t.Fatalf("metadata %v, want the date cleared", f.detail(other).Issue.Metadata)
	}
}

func TestUndoRefusesWithoutWritingWhenTheIssueMovedOn(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Contested")

	e := f.entry(f.field(uid, model.PriorityAction, "2"))
	f.rivalField(uid, model.PriorityAction, "4")
	f.refuses(uid, e, "issue priority changed since the action")

	e = f.entry(f.field(uid, model.AssignOwnerAction, "alice"))
	f.rivalField(uid, model.AssignOwnerAction, "carol")
	f.refuses(uid, e, "issue owner changed since the action")

	e = f.entry(f.field(uid, model.AddLabelAction, "bug"))
	f.rivalField(uid, model.AssignOwnerAction, "dave")
	f.refuses(uid, e, "issue changed since the label edit")

	saved := as[model.DraftSaved](t, run(t, f.gateway, model.SaveDraft{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, BaseText: "body", Text: "mine", Actor: "tester"}))
	e = f.entry(saved.Record)
	f.rival(uid, func(c *Client) {
		if _, err := c.EditIssueWithResponse(context.Background(), &g.EditIssueRequestOptions{PathParams: &g.EditIssuePath{ProjectID: fmt.Sprint(f.project.ID), Ref: uid}, Body: &g.EditIssueBody{Body: ptr("theirs"), Actor: ptr("rival")}}); err != nil {
			t.Fatal(err)
		}
	})
	f.refuses(uid, e, "issue body changed since the action")

	closed := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Quick: true, Reason: model.ReasonDone, Key: "k-1"}))
	e = f.entry(closed.Record)
	f.rival(uid, func(c *Client) {
		if err := c.Reopen(context.Background(), model.ReopenIssueCmd{UID: uid, ProjectID: f.project.ID, Actor: "rival"}); err != nil {
			t.Fatal(err)
		}
	})
	// Reopened elsewhere: the Issue is already as the undo would leave it.
	if got := f.undo(e); got.Result != model.UndoAlready {
		t.Fatalf("undo %+v, want already restored", got)
	}

	other := f.issue("Linked")
	link := f.entry(f.field(uid, model.AddRelatedAction, f.detail(other).Issue.ShortID))
	f.rivalField(uid, model.AssignOwnerAction, "erin")
	f.refuses(uid, link, "created link changed since the action")

	identity := f.entry(f.field(uid, model.PriorityAction, "1"))
	identity.ProjectUID = "01JOTHERPROJECT0000000000"
	f.refuses(uid, identity, "issue identity changed")
}

func TestUndoRefusesWhenADateChangedSinceTheAction(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Calendar entry")
	set := func(change model.DateChange) model.DatesResultMsg {
		return as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Changes: []model.DateChange{change}}))
	}
	e := f.entry(set(model.DateChange{Key: model.ScheduledKey, Label: "Scheduled", Value: "2026-10-12"}).Record)
	f.rival(uid, func(c *Client) {
		if _, err := c.patchMetadataKey(context.Background(), f.project.ID, uid, "rival", model.ScheduledKey, "2026-10-19", ptr(`"2026-10-12"`)); err != nil {
			t.Fatal(err)
		}
	})
	f.refuses(uid, e, "issue dates changed since the action")
	if got := f.detail(uid).Issue.Metadata[model.ScheduledKey]; got != "2026-10-19" {
		t.Fatalf("scheduled %v, want the rival's date kept", got)
	}
}

func TestUndoRefusesAnotherDaemonOrAccount(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Elsewhere")
	e := f.entry(f.field(uid, model.PriorityAction, "2"))
	rewrite := func(change func(map[string]any)) {
		hook := func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/api/v1/instance" {
				return false
			}
			rec := httptest.NewRecorder()
			f.kata.handler.ServeHTTP(rec, r)
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			change(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return true
		}
		f.kata.intercept.Store(&hook)
	}
	rewrite(func(b map[string]any) { b["instance_uid"] = "01JOTHERINSTANCE0000000000" })
	f.refuses(uid, e, "daemon instance changed")
	rewrite(func(b map[string]any) { b["auth"] = map[string]any{"kind": "token", "actor": "somebody-else"} })
	f.refuses(uid, e, "daemon principal changed")
	f.kata.intercept.Store(nil)
	f.restores(e)
}

func TestUndoOfAReopenAsksForEvidenceWhenKataRequiresIt(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Evidence")
	as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Quick: true, Reason: model.ReasonDone, Key: "k-1"}))
	e := f.entry(as[model.CloseResultMsg](t, run(t, f.gateway, model.ReopenIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester"})).Record)
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/instance" {
			return false
		}
		rec := httptest.NewRecorder()
		f.kata.handler.ServeHTTP(rec, r)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		// The principal must stay what the change ran as, so only the evidence flag is added to it.
		auth, _ := body["auth"].(map[string]any)
		auth["close_requires_evidence"] = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	f.kata.intercept.Store(&hook)
	// The entry was taken before the flag, so its principal differs: take it again under the flag.
	e.Principal = ""
	account, err := connectAccount(f)
	if err != nil {
		t.Fatal(err)
	}
	e.Principal = account.principal
	if got := f.undo(e); got.Result != model.UndoNeedsEvidence || got.Err != nil {
		t.Fatalf("undo %+v, want it to ask for evidence", got)
	}
	if status := f.detail(uid).Issue.Status; status != "open" {
		t.Fatalf("status %q, want it untouched", status)
	}
	evidence := model.CloseIssueCmd{Key: "k-undo", Message: "Finished once more after the reopen, so it is done again.", Evidence: []model.CloseEvidence{{Kind: model.EvidenceCommit, Value: "0123456789abcdef"}}}
	got := as[model.ChangeUndone](t, run(t, f.gateway, model.UndoChange{Workspace: f.workspace, Entry: e, Close: &evidence}))
	if got.Result != model.UndoChanged || got.Err != nil {
		t.Fatalf("undo with evidence %+v", got)
	}
	if issue := f.detail(uid).Issue; issue.Status != "closed" || issue.ClosedReason == nil || *issue.ClosedReason != "done" {
		t.Fatalf("issue %+v", issue)
	}
}

func connectAccount(f *undoFixture) (daemonAccount, error) {
	c, err := connect(context.Background(), f.kata.target())
	if err != nil {
		return daemonAccount{}, err
	}
	return c.account(context.Background())
}

func TestAWriteRecordsAnUnknownOutcomeOnlyWhenItMayHaveLanded(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Flaky")
	for status, unknown := range map[int]bool{http.StatusInternalServerError: true, http.StatusConflict: false, http.StatusUnprocessableEntity: false} {
		hook := func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/priority") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"status":` + http.StatusText(0) + `0,"error":{"code":"x","message":"no"}}`))
			return true
		}
		f.kata.intercept.Store(&hook)
		written := as[model.FieldWritten](t, run(t, f.gateway, model.WriteField{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Action: model.PriorityAction, Value: "1"}))
		if written.Err == nil || written.Record.Unknown != unknown || written.Record.Entry != nil {
			t.Errorf("status %d: %+v, want unknown=%v", status, written, unknown)
		}
	}
}

func TestBoundariesTheDaemonDecidesAreRecordedWithTheirReason(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("Recurring or leased")
	// An owner edit that involves an expiry cannot be taken back.
	f.field(uid, model.TimedAssignAction, "3600")
	rec := f.field(uid, model.AssignOwnerAction, "bob")
	if rec.Entry != nil || rec.Boundary != "owner edit involving an expiry cannot be undone" {
		t.Fatalf("record %+v", rec)
	}
	// A write that changed nothing leaves the history alone.
	if rec := f.field(uid, model.AssignOwnerAction, "bob"); rec.Entry != nil || rec.Boundary != "" || rec.Unknown {
		t.Fatalf("a write that changed nothing recorded %+v", rec)
	}
}
