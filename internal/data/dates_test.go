package data

import (
	"context"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (f attachFixture) patchKey(t *testing.T, key string, value any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("rival"), Patch: map[string]any{key: value}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f attachFixture) setDates(t *testing.T, changes ...model.DateChange) model.DatesResultMsg {
	t.Helper()
	return as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Changes: changes}))
}

func TestSetDatesSetsChangesAndClearsEachKey(t *testing.T) {
	f := newAttachFixture(t)
	scheduled := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30"}
	if got := f.setDates(t, scheduled, deadline); got.Err != nil || len(got.Applied) != 2 {
		t.Fatalf("set: %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if meta["scheduled_on"] != "2026-10-12" || meta["deadline_on"] != "2026-10-30" {
		t.Fatalf("metadata %v", meta)
	}
	scheduled.Was, scheduled.Value = "2026-10-12", "2026-10-15"
	deadline.Was, deadline.Value = "2026-10-30", ""
	if got := f.setDates(t, scheduled, deadline); got.Err != nil || len(got.Applied) != 2 {
		t.Fatalf("change and clear: %+v", got)
	}
	meta = f.issue(t, f.uid).Metadata
	if _, still := meta["deadline_on"]; still || meta["scheduled_on"] != "2026-10-15" {
		t.Fatalf("metadata %v", meta)
	}
}

func TestSetDatesReportsAKeyChangedMeanwhileAndStopsThere(t *testing.T) {
	f := newAttachFixture(t)
	f.patchKey(t, "scheduled_on", "2026-10-20")
	first := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-15", Was: "2026-10-12"}
	second := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30"}
	got := f.setDates(t, first, second)
	if got.Err == nil || !strings.Contains(got.Err.Error(), "Scheduled changed elsewhere") || len(got.Applied) != 0 {
		t.Fatalf("got %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if meta["scheduled_on"] != "2026-10-20" {
		t.Fatalf("the other write was overwritten: %v", meta)
	}
	if _, wrote := meta["deadline_on"]; wrote {
		t.Fatal("a write went on after the failure")
	}

	// A key absent when the card opened but set meanwhile is refused too, and an earlier write stays reported.
	f.patchKey(t, "deadline_on", "2026-11-01")
	first.Was = "2026-10-20"
	got = f.setDates(t, first, second)
	if got.Err == nil || !strings.Contains(got.Err.Error(), "Deadline changed elsewhere") || len(got.Applied) != 1 || got.Applied[0].Key != "scheduled_on" {
		t.Fatalf("got %+v", got)
	}
	if meta := f.issue(t, f.uid).Metadata; meta["deadline_on"] != "2026-11-01" || meta["scheduled_on"] != "2026-10-15" {
		t.Fatalf("metadata %v", meta)
	}
}

func TestSetDatesLeavesAHeldTimeAloneWhenGuardedOnIt(t *testing.T) {
	f := newAttachFixture(t)
	f.patchKey(t, "deadline_on", "2026-11-02T17:30:00")
	change := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-11-05", Was: "2026-11-02T17:30:00"}
	if got := f.setDates(t, change); got.Err != nil {
		t.Fatal(got.Err)
	}
	if got := f.issue(t, f.uid).Metadata["deadline_on"]; got != "2026-11-05" {
		t.Fatalf("deadline %v", got)
	}
}
