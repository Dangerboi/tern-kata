package driver_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func reviewFlow() model.Workflow {
	return model.Workflow{UID: "W1", Name: "Review then test", Author: "dana", EventUID: "E1", Revision: 3, About: "Review a change, then run the tests.", Steps: []model.WorkflowStep{
		{Key: "review", Kind: cronjob.StepPrompt, Prompt: "Review the change."},
		{Key: "test", Kind: cronjob.StepCommand, Command: "make test", After: []string{"review"}, Retries: 2},
	}}
}

func nightlyFlow() model.Workflow {
	return model.Workflow{UID: "W2", Name: "Nightly build", Author: "dana", EventUID: "E5", Steps: []model.WorkflowStep{{Key: "build", Kind: cronjob.StepCommand, Command: "make"}}}
}

// workflowsPane is projectPane at the given width, on the Workflows Page with the workflows loaded.
func workflowsPane(t *testing.T, cols int, flows ...model.Workflow) driver.State {
	t.Helper()
	out := feed(t, projectPane(t, cols, boardCard("a")), key("5"))
	if !hasLoad(out) {
		t.Fatalf("the Page asked for no workflows: %#v", out.Cmds)
	}
	return step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: flows}).State
}

func hasLoad(out driver.Output) bool {
	for _, cmd := range out.Cmds {
		if load, ok := cmd.(model.LoadWorkflows); ok && load.ProjectID == 7 && load.ProjectUID == "p" {
			return true
		}
	}
	return false
}

// screen is everything the view draws, as one searchable text.
func screen(t *testing.T, s driver.State) string {
	t.Helper()
	var all strings.Builder
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		all.WriteString(wireText(n))
		all.WriteByte('\n')
	}
	return all.String()
}

func mustShow(t *testing.T, s driver.State, what string, texts ...string) {
	t.Helper()
	drawn := screen(t, s)
	for _, text := range texts {
		if !strings.Contains(drawn, text) {
			t.Errorf("%s: the Pane does not draw %q", what, text)
		}
	}
}

func TestWorkflowsPageListsTheProjectsWorkflowsAndOpensTheirStepsWithDependenciesAndRetries(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	pageBarEntry(t, s.Model, "5") // fails the test when the Page bar draws none
	if !workflowRowDrawn(t, s.Model, "W1") || !workflowRowDrawn(t, s.Model, "W2") {
		t.Fatalf("the Workflows Page draws a row per workflow, got %v", drawnIDs(t, view.Render(s.Model, now)))
	}
	mustShow(t, s, "list", "Review then test", "2 steps", "1 step")

	opened := step(t, s, key("enter")).State
	mustShow(t, opened, "inspector", "Review a change, then run the tests.", "review", "prompt", "Review the change.", "test", "command", "make test", "after review", "retries 2")

	narrowed := workflowsPane(t, 80, reviewFlow())
	if !workflowRowDrawn(t, narrowed.Model, "W1") {
		t.Fatal("a narrow Pane draws the list")
	}
	narrowOpen := step(t, narrowed, key("enter")).State
	if workflowRowDrawn(t, narrowOpen.Model, "W1") {
		t.Error("a narrow Pane draws the steps in place of the list, not both")
	}
	mustShow(t, narrowOpen, "narrow inspector", "after review")
	back := step(t, narrowOpen, key("escape")).State
	if !workflowRowDrawn(t, back.Model, "W1") {
		t.Error("esc brings the list back")
	}
}

func TestClickingARowSelectsItsWorkflow(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	s = step(t, s, &tern.Select{ID: view.WorkflowListID, Item: workflowRow(t, s.Model, "W2")}).State
	if s.Model.Workflows.Selected != "W2" {
		t.Errorf("selected %q after a click on the second row", s.Model.Workflows.Selected)
	}
}

func TestWorkflowsPageWithoutAProjectSaysSoAndOpensTheMenusProjects(t *testing.T) {
	s := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}})
	m := step(t, s, key("5")).State.Model
	if m.Page != model.PageIssues || !strings.Contains(m.Notice, "Project") || m.Focus != model.ColumnCollections || !m.SidebarProjects {
		t.Errorf("page %v, notice %q, focus %v, Projects level %v", m.Page, m.Notice, m.Focus, m.SidebarProjects)
	}
}

func TestWorkflowsPageSaysWhyItIsEmpty(t *testing.T) {
	mustShow(t, workflowsPane(t, 160), "no workflows", "Press n to write one")
	archived := nightlyFlow()
	archived.Archived = true
	mustShow(t, workflowsPane(t, 160, archived), "only archived", "Every workflow here is archived")
	failed := step(t, feed(t, projectPane(t, 160, boardCard("a")), key("5")).State, model.WorkflowsLoaded{ProjectUID: "p", Err: errText("kata server not responding")}).State
	mustShow(t, failed, "load failure", "kata server not responding")
}

type errText string

func (e errText) Error() string { return string(e) }

func TestWorkflowCardEditsTheDefinitionAsJSONAndRefusesAnInvalidOneKeepingTheText(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	s = step(t, s, key("e")).State
	if s.Model.TopCard() != model.CardWorkflow {
		t.Fatalf("e opened card %v", s.Model.TopCard())
	}
	mustShow(t, s, "card", "Edit workflow Review then test", `\"after\": [`, `make test`)
	cardInput(t, s.Model) // fails the test when the card draws no text editor

	broken := strings.Replace(reviewFlow().Text(), `"review"`, `"test"`, 1)
	old := len(s.Model.Workflows.Card.Editor.Text)
	s = step(t, s, &tern.Edit{ID: cardInput(t, s.Model), From: 0, To: old, Text: broken, Cursor: len(broken), Len: old}).State
	refused := step(t, s, key("ctrl+o"))
	if len(refused.Cmds) != 0 {
		t.Fatalf("a workflow with duplicate keys was sent: %#v", refused.Cmds)
	}
	mustShow(t, refused.State, "refusal", "is used by an earlier step")
	if got := refused.State.Model.Workflows.Card.Editor.Text; got != broken {
		t.Errorf("the card holds %q after the refusal, want the edited text kept", got)
	}

	// The card is a Draft: leaving it with changes asks first.
	asked := step(t, refused.State, key("escape")).State
	if asked.Model.CurrentMode() != model.ModeDiscard {
		t.Fatalf("esc with edited text: mode %s", asked.Model.CurrentMode())
	}
}

func TestWorkflowCardSavesAndShowsAConflictBesideTheKeptText(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	s = step(t, s, key("e")).State
	edited := strings.Replace(reviewFlow().Text(), "make test", "make check", 1)
	old := len(s.Model.Workflows.Card.Editor.Text)
	s = step(t, s, &tern.Edit{ID: cardInput(t, s.Model), From: 0, To: old, Text: edited, Cursor: len(edited), Len: old}).State
	saved := step(t, s, key("ctrl+o"))
	var save model.SaveWorkflow
	for _, cmd := range saved.Cmds {
		if c, ok := cmd.(model.SaveWorkflow); ok {
			save = c
		}
	}
	if save.UID != "W1" || save.Expected != "E1" || save.ProjectID != 7 {
		t.Fatalf("save sent %#v", saved.Cmds)
	}

	newer := reviewFlow()
	newer.EventUID, newer.Revision = "E2", 4
	newer.Steps[0].Prompt = "Review it twice."
	conflict := step(t, saved.State, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &newer}).State
	mustShow(t, conflict, "conflict", "Workflow changed: your text is kept", "make check", "Review it twice.", "revision 4", "Overwrite", "overwrite new version")
	drawn := screen(t, conflict)
	if choice, texts := strings.Index(drawn, "Overwrite"), strings.Index(drawn, "Your text"); choice < 0 || texts < 0 || choice > texts {
		t.Errorf("the buttons are at %d, after the texts at %d", choice, texts)
	}
	if got := conflict.Model.Workflows.Card.Editor.Text; got != edited {
		t.Errorf("the conflict lost the edited text: %q", got)
	}

	overwrite := step(t, conflict, key("o"))
	again := false
	for _, cmd := range overwrite.Cmds {
		if c, ok := cmd.(model.SaveWorkflow); ok && c.Expected == "E2" {
			again = true
		}
	}
	if !again {
		t.Errorf("o sent %#v, want the text saved against E2", overwrite.Cmds)
	}
}

func TestWorkflowConflictOnAnArchivedWorkflowOffersNoOverwrite(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	s = step(t, s, key("e")).State
	edited := strings.Replace(reviewFlow().Text(), "make test", "make check", 1)
	old := len(s.Model.Workflows.Card.Editor.Text)
	s = step(t, s, &tern.Edit{ID: cardInput(t, s.Model), From: 0, To: old, Text: edited, Cursor: len(edited), Len: old}).State
	s = step(t, s, key("ctrl+o")).State
	gone := reviewFlow()
	gone.EventUID, gone.Revision, gone.Archived = "E2", 4, true
	conflict := step(t, s, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &gone}).State
	mustShow(t, conflict, "archived conflict", "Workflow changed: your text is kept", "archived", "Discard Draft")
	drawn := screen(t, conflict)
	if strings.Contains(drawn, "Overwrite") {
		t.Errorf("the archived conflict still draws Overwrite:\n%s", drawn)
	}
	if strings.Contains(drawn, "overwrite new version") || !strings.Contains(drawn, "discard Draft") {
		t.Errorf("the archived conflict's footer hints are wrong:\n%s", drawn)
	}
	if choice, texts := strings.Index(drawn, "Discard Draft"), strings.Index(drawn, "Your text"); choice < 0 || texts < 0 || choice > texts {
		t.Errorf("the choice is at %d, after the texts at %d", choice, texts)
	}
}

func TestNewWorkflowStartsFromATemplateAndCreatesWithoutAGuard(t *testing.T) {
	s := workflowsPane(t, 160)
	s = step(t, s, key("n")).State
	mustShow(t, s, "template", "New workflow", `\"steps\"`)
	text := `{"name":"Nightly","definition":{"version":1,"steps":[{"key":"build","kind":"command","command":"make"}]}}`
	old := len(s.Model.Workflows.Card.Editor.Text)
	s = step(t, s, &tern.Edit{ID: cardInput(t, s.Model), From: 0, To: old, Text: text, Cursor: len(text), Len: old}).State
	out := step(t, s, key("ctrl+o"))
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.SaveWorkflow); ok && c.UID == "" && c.Expected == "" && c.Name == "Nightly" {
			return
		}
	}
	t.Fatalf("ctrl+o sent %#v, want a create", out.Cmds)
}

func TestWorkflowCardOpensEditorOnTheTextAndTakesItBack(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	s = step(t, s, key("e")).State
	out := step(t, s, key("ctrl+e"))
	if out.Editor == nil || out.Editor.UID != model.WorkflowDraftUID || out.Editor.Text != reviewFlow().Text() || out.Editor.Suffix != ".json" {
		t.Fatalf("ctrl+e routed %#v to $EDITOR", out.Editor)
	}
	back := step(t, out.State, model.DraftEditorClosed{UID: model.WorkflowDraftUID, Text: "{}\n"}).State
	if back.Model.Workflows.Card.Editor.Text != "{}\n" {
		t.Errorf("the card holds %q", back.Model.Workflows.Card.Editor.Text)
	}
}

func TestArchivingAWorkflowALiveJobUsesShowsKatasRefusalAndKeepsTheRow(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	out := step(t, s, key("x"))
	var archive model.ArchiveWorkflow
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.ArchiveWorkflow); ok {
			archive = c
		}
	}
	if archive.UID != "W1" || archive.Expected != "E1" {
		t.Fatalf("x sent %#v", out.Cmds)
	}
	refusal := "invalid cron document: workflow W1 is used by live job J1; update or delete that job first"
	refused := step(t, out.State, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Err: errText(refusal)}).State
	mustShow(t, refused, "refusal", refusal, "Review then test")
}

func TestArchivedWorkflowsAreHiddenUntilAskedForAndRestoredFromTheList(t *testing.T) {
	archived := nightlyFlow()
	archived.Archived = true
	s := workflowsPane(t, 160, reviewFlow(), archived)
	if workflowRowDrawn(t, s.Model, "W2") {
		t.Fatal("an archived workflow is listed before it is asked for")
	}
	s = feed(t, s, key("a"), key("j")).State
	mustShow(t, s, "archived shown", "Nightly build", "archived")
	out := step(t, s, key("r"))
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.RestoreWorkflow); ok && c.UID == "W2" && c.Expected == "E5" {
			restored := nightlyFlow()
			after := step(t, out.State, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpRestore, UID: "W2", Workflow: &restored}).State
			mustShow(t, after, "restored", "Nightly build")
			if strings.Contains(screen(t, after), `"text":"archived"`) {
				t.Error("the restored workflow still reads archived")
			}
			return
		}
	}
	t.Fatalf("r sent %#v, want a restore of W2 guarded on E5", out.Cmds)
}

func TestEveryWorkflowsFooterHintClicksLikeItsKey(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	if s.Model.CurrentMode() != model.ModeWorkflows {
		t.Fatalf("mode %s", s.Model.CurrentMode())
	}
	for binding := range model.BindingsForMode(model.ModeWorkflows, true) {
		t.Run(binding.Chord, func(t *testing.T) {
			keyboard, keyCmds := keys(binding.Chord)(s)
			pointer, clickCmds := clicked(s, click("", view.KeyClick(binding.Chord)))
			sameAs(t, "hint differs from key", pointer, clickCmds, keyboard, keyCmds)
		})
	}
}

func TestWorkflowCardButtonsAreDrawnForEveryState(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	editing := step(t, s, key("e")).State
	old := len(editing.Model.Workflows.Card.Editor.Text)
	asking := step(t, step(t, editing, &tern.Edit{ID: cardInput(t, editing.Model), From: 0, To: old, Text: "x", Cursor: 1, Len: old}).State, key("escape")).State
	conflict := editing
	conflict.Model, _ = model.Update(editing.Model, model.KeyMsg{Chord: "ctrl+o"})
	newer := reviewFlow()
	newer.EventUID = "E2"
	conflict.Model, _ = model.Update(conflict.Model, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &newer})
	for name, tt := range map[string]struct {
		state driver.State
		mode  model.Mode
	}{"editing": {editing, model.ModeEditor}, "asking to discard": {asking, model.ModeDiscard}, "conflict": {conflict, model.ModeConflict}} {
		t.Run(name, func(t *testing.T) {
			if got := tt.state.Model.CurrentMode(); got != tt.mode {
				t.Fatalf("mode %s, want %s", got, tt.mode)
			}
			buttons, labels := cardButtonsOf(t, tt.state)
			if len(buttons) == 0 {
				t.Fatal("the card draws no buttons")
			}
			for id, act := range buttons {
				chord := strings.TrimPrefix(act, view.KeyClick(""))
				if _, ok := model.LookupBindingForMode(chord, tt.mode); !ok {
					t.Errorf("%s (%q) sends %q, which %s does not bind", id, labels[id], chord, tt.mode)
				}
			}
		})
	}
}

func TestWorkflowRowAndInspectorNameTheLiveJobsThatUseIt(t *testing.T) {
	out := feed(t, projectPane(t, 160, boardCard("a")), key("5"))
	var load model.LoadJobs
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.LoadJobs); ok {
			load = c
		}
	}
	if load.ProjectID != 7 || load.Archived {
		t.Fatalf("the Page asked for jobs with %#v, want the Project's live jobs", out.Cmds)
	}
	use := func(uid, name string, enabled bool) model.CronJob {
		return model.CronJob{UID: uid, Name: name, Definition: cronjob.Definition{Enabled: enabled, Action: cronjob.Action{Kind: "execute", WorkflowUID: "W1"}}}
	}
	archived := use("J3", "Old review", true)
	now := time.Now()
	archived.DeletedAt = &now
	s := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow(), nightlyFlow()}}).State
	s = step(t, s, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{use("J1", "Nightly review", true), use("J2", "Weekly review", false), archived}}).State
	mustShow(t, s, "row", "Nightly review +1")
	if strings.Contains(screen(t, s), "Old review") {
		t.Error("an archived job is named as using the workflow")
	}
	s = step(t, s, key("enter")).State
	mustShow(t, s, "inspector", "Weekly review", "disabled", "enabled")
}

func TestAFailedReadOfTheWorkflowsShowsAsItsOwnLineAboveTheHeldRows(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	s = feed(t, s, key("1")).State
	out := feed(t, s, key("5"))
	if !hasLoad(out) {
		t.Fatalf("reopening the Page asked for no workflows: %#v", out.Cmds)
	}
	failed := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Err: errText("kata server not responding")}).State
	mustShow(t, failed, "failed read", "kata server not responding", reviewFlow().Name, nightlyFlow().Name)
	nodes := wireNodes(t, view.Render(failed.Model, now))
	line := slices.IndexFunc(nodes, func(n tern.WireNode) bool { return strings.HasSuffix(n.ID, ".names.error") })
	rows := slices.IndexFunc(nodes, func(n tern.WireNode) bool { return n.ID == view.WorkflowListID })
	if line < 0 || rows < 0 || line > rows {
		t.Errorf("the read error (node %d) is not its own line above the held rows (node %d)", line, rows)
	}
}
