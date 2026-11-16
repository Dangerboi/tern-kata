package data

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (f issueFixture) patchKey(t *testing.T, key string, value any) {
	t.Helper()
	f.patchKeyOf(t, f.uid, key, value)
}

// patchKeyOf writes a meta key of the Issue uid straight to kata, bypassing every guard.
func (f issueFixture) patchKeyOf(t *testing.T, uid, key string, value any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("rival"), Patch: map[string]any{key: value}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f issueFixture) setDates(t *testing.T, changes ...model.DateChange) model.DatesResultMsg {
	t.Helper()
	return as[model.DatesResultMsg](t, run(t, f.gateway, model.SetDatesCmd{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Changes: changes}))
}

func TestSetDatesSetsChangesAndClearsEachKey(t *testing.T) {
	f := newIssueFixture(t)
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

func TestSetDatesWritesNothingWhenAnyKeyChangedMeanwhile(t *testing.T) {
	f := newIssueFixture(t)
	f.patchKey(t, "scheduled_on", "2026-10-20")
	first := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-15", Was: "2026-10-12"}
	second := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30"}
	got := f.setDates(t, first, second)
	if got.Err == nil || !strings.Contains(got.Err.Error(), "nothing was written. The Scheduled field now shows") || len(got.Applied) != 0 {
		t.Fatalf("got %+v", got)
	}
	var conflict model.DatesConflict
	if !errors.As(got.Err, &conflict) || !conflict.Read || conflict.Key != "scheduled_on" || conflict.Held != "2026-10-20" || conflict.Shown != "2026-10-20" {
		t.Fatalf("the refusal does not carry the current value: %+v", got.Err)
	}
	meta := f.issue(t, f.uid).Metadata
	if meta["scheduled_on"] != "2026-10-20" {
		t.Fatalf("the other write was overwritten: %v", meta)
	}
	if _, wrote := meta["deadline_on"]; wrote {
		t.Fatal("a write went on after the failure")
	}

	// A key absent when the card opened but set meanwhile is refused too, and the key before it is not written either.
	f.patchKey(t, "deadline_on", "2026-11-01")
	first.Was = "2026-10-20"
	got = f.setDates(t, first, second)
	if got.Err == nil || !strings.Contains(got.Err.Error(), "nothing was written. The Deadline field now shows") || len(got.Applied) != 0 {
		t.Fatalf("got %+v", got)
	}
	if meta := f.issue(t, f.uid).Metadata; meta["deadline_on"] != "2026-11-01" || meta["scheduled_on"] != "2026-10-20" {
		t.Fatalf("metadata %v", meta)
	}
}

func TestSetDatesLeavesAHeldTimeAloneWhenGuardedOnIt(t *testing.T) {
	f := newIssueFixture(t)
	f.patchKey(t, "deadline_on", "2026-11-02T17:30:00")
	change := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-11-05", Was: "2026-11-02T17:30:00"}
	if got := f.setDates(t, change); got.Err != nil {
		t.Fatal(got.Err)
	}
	if got := f.issue(t, f.uid).Metadata["deadline_on"]; got != "2026-11-05" {
		t.Fatalf("deadline %v", got)
	}
}

// metadataPatches counts the metadata writes the daemon sees. rival, when set, runs once before the first of them reaches kata.
func (f issueFixture) metadataPatches(rival func()) (count *int, stop func()) {
	patches := 0
	ran := false
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/metadata") {
			if rival != nil && !ran {
				ran = true
				rival()
			}
			patches++
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	return &patches, func() { f.kata.intercept.Store(nil) }
}

func TestSetDatesWritesBothDatesInOneRequest(t *testing.T) {
	f := newIssueFixture(t)
	patches, stop := f.metadataPatches(nil)
	defer stop()
	scheduled := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30"}
	got := f.setDates(t, scheduled, deadline)
	if got.Err != nil || len(got.Applied) != 2 || *patches != 1 {
		t.Fatalf("got %+v after %d writes, want both in one", got, *patches)
	}
	if got.Record.Entry == nil || len(got.Record.Entry.Dates) != 2 {
		t.Fatalf("record %+v, want one entry for both dates", got.Record)
	}
}

func TestSetDatesRetriesWhenAnotherChangeMovedTheRevision(t *testing.T) {
	f := newIssueFixture(t)
	patches, stop := f.metadataPatches(func() { f.patchKey(t, "unrelated", "x") })
	defer stop()
	scheduled := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30"}
	got := f.setDates(t, scheduled, deadline)
	if got.Err != nil || len(got.Applied) != 2 {
		t.Fatalf("got %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if meta["scheduled_on"] != "2026-10-12" || meta["deadline_on"] != "2026-10-30" || meta["unrelated"] != "x" {
		t.Fatalf("metadata %v", meta)
	}
	if *patches != 3 {
		t.Fatalf("%d metadata writes, want the rival, the refused one and the retry", *patches)
	}
}

func TestSetDatesRefusesWhenADateChangedDuringTheSave(t *testing.T) {
	f := newIssueFixture(t)
	f.patchKey(t, "deadline_on", "2026-11-01")
	_, stop := f.metadataPatches(func() { f.patchKey(t, "deadline_on", "2026-11-09") })
	defer stop()
	scheduled := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-12"}
	deadline := model.DateChange{Key: "deadline_on", Label: "Deadline", Value: "2026-10-30", Was: "2026-11-01"}
	got := f.setDates(t, scheduled, deadline)
	var conflict model.DatesConflict
	if !errors.As(got.Err, &conflict) || conflict.Key != "deadline_on" || conflict.Held != "2026-11-09" || len(got.Applied) != 0 {
		t.Fatalf("got %+v, want the Deadline refused with the date now held", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if _, wrote := meta["scheduled_on"]; wrote || meta["deadline_on"] != "2026-11-09" {
		t.Fatalf("metadata %v, want nothing saved", meta)
	}
}
