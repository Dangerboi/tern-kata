package data

import (
	"context"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

const reviewThenTest = `{
  "name": "Review then test",
  "definition": {
    "version": 1,
    "about": "Review a change, then run the tests.",
    "steps": [
      {"key": "review", "kind": "prompt", "prompt": "Review the change.", "options": {"profile": "careful", "limits": {"turns": 12}}},
      {"key": "test", "kind": "command", "command": "make test", "after": ["review"], "retries": 2}
    ]
  }
}`

func parsed(t *testing.T, text string) model.WorkflowDocument {
	t.Helper()
	doc, err := model.ParseWorkflow(text)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func saveWorkflow(t *testing.T, gateway *Gateway, workspace string, project model.Project, uid, expected, text string) model.WorkflowWritten {
	t.Helper()
	doc := parsed(t, text)
	return as[model.WorkflowWritten](t, run(t, gateway, model.SaveWorkflow{Workspace: workspace, Actor: "tester", ProjectID: project.ID, ProjectUID: project.UID, UID: uid, Expected: expected, Name: doc.Name, Definition: doc.Definition}))
}

func loadWorkflows(t *testing.T, gateway *Gateway, workspace string, project model.Project) []model.Workflow {
	t.Helper()
	loaded := as[model.WorkflowsLoaded](t, run(t, gateway, model.LoadWorkflows{Workspace: workspace, ProjectID: project.ID, ProjectUID: project.UID}))
	if loaded.Err != nil {
		t.Fatal(loaded.Err)
	}
	return loaded.Workflows
}

func TestWorkflowsAreCreatedListedAndEditedAsTheirTextShows(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")

	created := saveWorkflow(t, gateway, workspace, bound, "", "", reviewThenTest)
	if created.Err != nil || created.Workflow == nil {
		t.Fatalf("created %+v", created)
	}
	w := created.Workflow
	if w.Name != "Review then test" || w.About == "" || len(w.Steps) != 2 || w.Archived || w.EventUID == "" {
		t.Fatalf("created workflow %+v", w)
	}
	test := w.Steps[1]
	if test.Key != "test" || test.Kind != model.StepCommand || test.Command != "make test" || test.Retries != 2 || len(test.After) != 1 || test.After[0] != "review" {
		t.Errorf("second step %+v, want the command step after review with 2 retries", test)
	}
	if listed := loadWorkflows(t, gateway, workspace, bound); len(listed) != 1 || listed[0].UID != w.UID || listed[0].EventUID != w.EventUID {
		t.Fatalf("listed %+v, want the created workflow", listed)
	}

	// The text an edit starts from sends back unchanged, options included, so editing never drops what the editor was not shown.
	edited := saveWorkflow(t, gateway, workspace, bound, w.UID, w.EventUID, strings.Replace(w.Text(), "make test", "make check", 1))
	if edited.Err != nil || edited.Workflow == nil || edited.Workflow.Steps[1].Command != "make check" {
		t.Fatalf("edited %+v", edited)
	}
	if edited.Workflow.EventUID == w.EventUID {
		t.Error("a write must give the workflow a new definition_event_uid")
	}
	if got := edited.Workflow.Steps[0].Options; !strings.Contains(string(got), `"turns":12`) {
		t.Errorf("step options %s lost the nested number", got)
	}
}

func TestSavingAWorkflowChangedMeanwhileWritesNothingAndReturnsTheCurrentOne(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")
	original := saveWorkflow(t, gateway, workspace, bound, "", "", reviewThenTest).Workflow

	someoneElse := saveWorkflow(t, gateway, workspace, bound, original.UID, original.EventUID, strings.Replace(original.Text(), "Review the change.", "Review it twice.", 1)).Workflow
	mine := saveWorkflow(t, gateway, workspace, bound, original.UID, original.EventUID, strings.Replace(original.Text(), "make test", "make check", 1))

	if mine.Err != nil || mine.Workflow != nil || mine.Current == nil {
		t.Fatalf("stale save %+v, want the current workflow and no write", mine)
	}
	if mine.Current.EventUID != someoneElse.EventUID || mine.Current.Steps[0].Prompt != "Review it twice." {
		t.Errorf("current workflow %+v, want the other writer's version", mine.Current)
	}
	if listed := loadWorkflows(t, gateway, workspace, bound); listed[0].EventUID != someoneElse.EventUID || listed[0].Steps[1].Command != "make test" {
		t.Errorf("after the refused save the workflow is %+v, want the other writer's, untouched", listed[0])
	}

	// Saving again on the version just seen is the overwrite the card offers.
	overwritten := saveWorkflow(t, gateway, workspace, bound, original.UID, mine.Current.EventUID, strings.Replace(original.Text(), "make test", "make check", 1))
	if overwritten.Err != nil || overwritten.Workflow == nil || overwritten.Workflow.Steps[1].Command != "make check" {
		t.Errorf("overwrite %+v", overwritten)
	}
}

func TestWorkflowsAreArchivedAndRestoredGuardedOnTheirVersion(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")
	w := *saveWorkflow(t, gateway, workspace, bound, "", "", reviewThenTest).Workflow

	archived := as[model.WorkflowWritten](t, run(t, gateway, model.ArchiveWorkflow{Workspace: workspace, Actor: "tester", ProjectID: bound.ID, ProjectUID: bound.UID, UID: w.UID, Expected: w.EventUID}))
	if archived.Err != nil || archived.Workflow == nil || !archived.Workflow.Archived {
		t.Fatalf("archived %+v", archived)
	}
	if listed := loadWorkflows(t, gateway, workspace, bound); len(listed) != 1 || !listed[0].Archived {
		t.Errorf("listed %+v, want the archived workflow still listed as archived", listed)
	}

	stale := as[model.WorkflowWritten](t, run(t, gateway, model.RestoreWorkflow{Workspace: workspace, Actor: "tester", ProjectID: bound.ID, ProjectUID: bound.UID, UID: w.UID, Expected: w.EventUID}))
	if stale.Err != nil || stale.Workflow != nil || stale.Current == nil || !stale.Current.Archived || stale.Current.EventUID != archived.Workflow.EventUID {
		t.Fatalf("stale restore %+v, want nothing written and the archived workflow as current", stale)
	}

	restored := as[model.WorkflowWritten](t, run(t, gateway, model.RestoreWorkflow{Workspace: workspace, Actor: "tester", ProjectID: bound.ID, ProjectUID: bound.UID, UID: w.UID, Expected: archived.Workflow.EventUID}))
	if restored.Err != nil || restored.Workflow == nil || restored.Workflow.Archived {
		t.Fatalf("restored %+v", restored)
	}
}

func TestArchivingAWorkflowALiveJobUsesIsRefusedWithKatasMessage(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")
	w := *saveWorkflow(t, gateway, workspace, bound, "", "", reviewThenTest).Workflow

	kataClient, err := client.New(kata.URL)
	if err != nil {
		t.Fatal(err)
	}
	title, actor := "Result", "tester"
	created, err := kataClient.CreateCronJobWithResponse(context.Background(), &g.CreateCronJobRequestOptions{PathParams: &g.CreateCronJobPath{ProjectID: bound.ID}, Body: &g.PutCronJobBody{
		Name: "Nightly review", Actor: &actor,
		Definition: g.CronJobDefinition{Version: 1, Kind: "job", Enabled: true, Overlap: "forbid", Catchup: "skip",
			Trigger: g.Trigger{Kind: "manual"},
			Action:  g.Action{Kind: "execute", WorkflowUID: &w.UID},
			Issue:   &g.IssuePolicy{Kind: "per-run", Title: &title}},
	}})
	if err != nil || created.JSON201 == nil {
		t.Fatalf("job create: %v", err)
	}
	job := created.JSON201.Job

	refused := as[model.WorkflowWritten](t, run(t, gateway, model.ArchiveWorkflow{Workspace: workspace, Actor: "tester", ProjectID: bound.ID, ProjectUID: bound.UID, UID: w.UID, Expected: w.EventUID}))
	want := "invalid cron document: workflow " + w.UID + " is used by live job " + job.UID + "; update or delete that job first"
	if refused.Err == nil || refused.Err.Error() != want {
		t.Fatalf("archive refusal %v, want %q", refused.Err, want)
	}
	if listed := loadWorkflows(t, gateway, workspace, bound); listed[0].Archived {
		t.Error("a refused archive must leave the workflow live")
	}
}
