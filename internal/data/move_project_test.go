package data

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// moveFixture is a daemon with two Projects and one Issue in the first.
type moveFixture struct {
	gateway    *Gateway
	workspace  string
	from, to   model.Project
	uid, actor string
}

func newMoveFixture(t *testing.T) moveFixture {
	t.Helper()
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, from := project(t, gateway, "demo")
	created := as[model.ProjectSaved](t, run(t, gateway, model.CreateProject{Workspace: t.TempDir(), Actor: "tester", Name: "extra"}))
	if created.Err != nil {
		t.Fatal(created.Err)
	}
	return moveFixture{gateway: gateway, workspace: workspace, from: from, to: created.Project, uid: createIssue(t, gateway, workspace, from, "Move me"), actor: "tester"}
}

func (f moveFixture) issue(t *testing.T) *model.Issue {
	t.Helper()
	issue := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: f.uid})).Selected.Issue
	if issue == nil {
		t.Fatal("the Issue is gone")
	}
	return issue
}

func (f moveFixture) move(t *testing.T, dryRun bool, revision int64) model.ProjectMoveAnswer {
	t.Helper()
	return as[model.ProjectMoveAnswer](t, run(t, f.gateway, model.MoveToProjectCmd{Workspace: f.workspace, UID: f.uid, Actor: f.actor, ProjectID: f.from.ID, ToProjectUID: f.to.UID, DryRun: dryRun, Revision: revision}))
}

func TestGatewayMovePreviewChangesNothing(t *testing.T) {
	f := newMoveFixture(t)
	before := f.issue(t)

	preview := f.move(t, true, before.Revision)
	if preview.Err != nil || preview.Conflict || !preview.DryRun || preview.UID != f.uid || preview.ToProjectUID != f.to.UID {
		t.Fatalf("preview %+v", preview)
	}
	if preview.Revision != before.Revision {
		t.Errorf("preview reports revision %d, want the Issue's %d", preview.Revision, before.Revision)
	}
	// kata assigns the new short id only when it really moves, so a dry run cannot report it.
	if preview.NewShortID != "" {
		t.Errorf("preview names the new short id %q; kata's dry run is expected to leave it out", preview.NewShortID)
	}
	after := f.issue(t)
	if after.ProjectName != "demo" || after.ShortID != before.ShortID || after.Revision != before.Revision {
		t.Errorf("the dry run changed the Issue: %+v then %+v", before, after)
	}
}

func TestGatewayMovePreviewWithoutARevisionReadsTheCurrentOne(t *testing.T) {
	f := newMoveFixture(t)
	before := f.issue(t)
	as[model.FieldWritten](t, run(t, f.gateway, model.WriteField{Workspace: f.workspace, UID: f.uid, ProjectID: f.from.ID, Actor: "someone", Action: model.AssignOwnerAction, Value: "alice"}))
	current := f.issue(t)
	if current.Revision == before.Revision {
		t.Fatalf("an owner change left the revision at %d", before.Revision)
	}
	if fresh := f.move(t, true, 0); fresh.Err != nil || fresh.Conflict || fresh.Revision != current.Revision {
		t.Fatalf("fresh preview %+v, want the current revision %d", fresh, current.Revision)
	}
}

func TestGatewayMoveWithTheCurrentRevisionMovesTheIssue(t *testing.T) {
	f := newMoveFixture(t)
	before := f.issue(t)

	moved := f.move(t, false, before.Revision)
	if moved.Err != nil || moved.Conflict || moved.DryRun || moved.NewShortID == "" {
		t.Fatalf("moved %+v, want a move that reports the new short id", moved)
	}
	after := f.issue(t)
	if after.ProjectName != "extra" || after.ShortID != moved.NewShortID || after.UID != f.uid {
		t.Errorf("Issue now %s %s (%s), want extra %s", after.ProjectName, after.ShortID, after.UID, moved.NewShortID)
	}
	if after.Revision <= before.Revision {
		t.Errorf("revision %d after the move, was %d", after.Revision, before.Revision)
	}
}

func TestGatewayMoveWithAStaleRevisionIsRefusedAndMovesNothing(t *testing.T) {
	f := newMoveFixture(t)
	opened := f.issue(t)
	as[model.FieldWritten](t, run(t, f.gateway, model.WriteField{Workspace: f.workspace, UID: f.uid, ProjectID: f.from.ID, Actor: "someone", Action: model.AssignOwnerAction, Value: "alice"}))

	for _, dryRun := range []bool{true, false} {
		answer := f.move(t, dryRun, opened.Revision)
		if !answer.Conflict || answer.Err != nil || answer.NewShortID != "" {
			t.Fatalf("dry run %v with a stale revision: %+v, want a conflict", dryRun, answer)
		}
	}
	if after := f.issue(t); after.ProjectName != "demo" || after.ShortID != opened.ShortID {
		t.Errorf("a refused move changed the Issue: %+v", after)
	}
}

func TestGatewayMoveToTheIssuesOwnProjectShowsKatasRefusal(t *testing.T) {
	f := newMoveFixture(t)
	answer := as[model.ProjectMoveAnswer](t, run(t, f.gateway, model.MoveToProjectCmd{Workspace: f.workspace, UID: f.uid, Actor: f.actor, ProjectID: f.from.ID, ToProjectUID: f.from.UID, DryRun: true}))
	if answer.Err == nil || !strings.Contains(answer.Err.Error(), "current project") || answer.Conflict {
		t.Fatalf("answer %+v, want kata's same-project refusal", answer)
	}
}
