package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func reviewFlow() model.Workflow {
	return model.Workflow{UID: "W1", Name: "Review then test", EventUID: "E1", Revision: 1, Steps: []model.WorkflowStep{
		{Key: "review", Kind: cronjob.StepPrompt, Prompt: "Review the change."},
		{Key: "test", Kind: cronjob.StepCommand, Command: "make test", After: []string{"review"}, Retries: 1},
	}}
}

// workflowsPage is a bound Project's Workflows Page with the given workflows loaded, and the commands entering it sent.
func workflowsPage(flows ...model.Workflow) (model.Model, []model.Cmd) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "4"})
	m, _ = model.Update(m, model.WorkflowsLoaded{ProjectUID: "p", Workflows: flows})
	return m, cmds
}

func onWorkflows(t *testing.T, flows ...model.Workflow) model.Model {
	t.Helper()
	m, cmds := workflowsPage(flows...)
	if want := (model.LoadWorkflows{Workspace: "/repo", ProjectID: 7, ProjectUID: "p"}); !hasCmd(cmds, want) {
		t.Fatalf("entering the Page sent %#v, want %#v", cmds, want)
	}
	return m
}

func hasCmd(cmds []model.Cmd, want model.Cmd) bool {
	for _, cmd := range cmds {
		if reflect.DeepEqual(cmd, want) {
			return true
		}
	}
	return false
}

func TestWorkflowsPageNeedsAProject(t *testing.T) {
	m, _ := loadFirst(model.Model{Cols: 160}, model.ProjectLoaded{}, model.SnapshotLoaded{})
	m, _ = model.Update(m, model.KeyMsg{Chord: "4"})
	if m.Page != model.PageIssues || !strings.Contains(m.Notice, "Project") || m.SidebarLevel != model.MenuProjects {
		t.Errorf("page %v, notice %q, Projects level %v; want the Issues Page, a notice and the Menu's Projects", m.Page, m.Notice, m.SidebarLevel == model.MenuProjects)
	}
}

// loadedText brings a workflow document back into the card as $EDITOR does: the form takes what it holds.
func loadedText(m model.Model, text string) model.Model {
	m, _ = model.Update(m, model.DraftEditorClosed{UID: model.WorkflowDraftUID, Text: text})
	return m
}

// editStepBody shows step number step (from zero) of the open card and replaces its command or prompt.
func editStepBody(m model.Model, step int, text string) model.Model {
	m, _ = model.Update(m, model.ChooseMsg{Field: model.WorkflowFieldStep, Index: step})
	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.WorkflowFieldBody})
	old := len(m.Workflows.Card.Form.Step().Body.Text)
	m, _ = model.Update(m, model.EditMsg{Target: model.EditWorkflowBody, From: 0, To: old, Text: text, Cursor: len(text), Len: old})
	return m
}

// typedInto focuses the field and types text into it.
func typedInto(m model.Model, field model.Field, text string) model.Model {
	m, _ = model.Update(m, model.FocusFieldMsg{Field: field})
	return typed(m, text)
}

func TestEditingAWorkflowShowsItsFieldsAndStepsAndSavesItGuardedOnItsVersion(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	if !m.Workflows.Card.Open || m.TopCard() != model.CardWorkflow || m.CurrentMode() != model.ModeWorkflowForm {
		t.Fatalf("e opened card %v in mode %s", m.TopCard(), m.CurrentMode())
	}
	form := m.Workflows.Card.Form
	if form.Name != "Review then test" || len(form.Steps) != 2 || form.Steps[1].Key != "test" || form.Steps[1].After != "review" || form.Steps[1].Retries != "1" || form.Steps[1].Body.Text != "make test" {
		t.Fatalf("the form holds %+v, want the workflow's own fields", form)
	}
	m = editStepBody(m, 1, "make check")
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || save.UID != "W1" || save.Expected != "E1" || save.ProjectID != 7 || save.Name != "Review then test" || !strings.Contains(save.Definition, `"command":"make check"`) || !strings.Contains(save.Definition, `"after":["review"]`) || !strings.Contains(save.Definition, `"retries":1`) {
		t.Fatalf("save sent %#v, want a replace of W1 guarded on E1", cmds)
	}
	if !m.Workflows.Card.Saving {
		t.Error("the card should show it is saving")
	}
}

func TestTheFormKeepsTheOptionsKataHoldsThroughASave(t *testing.T) {
	flow := reviewFlow()
	flow.Options = []byte(`{"mystery":true}`)
	flow.Steps[0].Options = []byte(`{"limits":{"turns":12}}`)
	m := onWorkflows(t, flow)
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	m = editStepBody(m, 0, "Review it.")
	_, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || !strings.Contains(save.Definition, `"mystery":true`) || !strings.Contains(save.Definition, `"turns":12`) {
		t.Fatalf("save sent %#v, want the options kept", cmds)
	}
}

func TestStepsAreAddedRemovedAndMovedFromTheCard(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	names := func(m model.Model) string {
		return strings.Join(m.Workflows.Card.Form.StepLabels(), ",")
	}
	if got := names(m); got != "1 review,2 test" {
		t.Fatalf("steps %q", got)
	}

	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+n"})
	form := m.Workflows.Card.Form
	if names(m) != "1 review,2 step,3 test" || form.At != 1 || form.Field != model.WorkflowFieldKey || form.Step().Kind != cronjob.StepPrompt {
		t.Errorf("after adding: steps %q, shown %d, focus %s; want a new prompt step after the shown one with its name focused", names(m), form.At, form.Field)
	}

	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+down"})
	if names(m) != "1 review,2 test,3 step" || m.Workflows.Card.Form.At != 2 {
		t.Errorf("after moving down: steps %q, shown %d", names(m), m.Workflows.Card.Form.At)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+down"})
	if names(m) != "1 review,2 test,3 step" {
		t.Errorf("the last step moved down to %q", names(m))
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+up"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+up"})
	if names(m) != "1 step,2 review,3 test" || m.Workflows.Card.Form.At != 0 {
		t.Errorf("after moving up twice: steps %q, shown %d", names(m), m.Workflows.Card.Form.At)
	}

	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+x"})
	if names(m) != "1 review,2 test" || m.Workflows.Card.Form.At != 0 {
		t.Errorf("after removing: steps %q, shown %d", names(m), m.Workflows.Card.Form.At)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+x"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+x"})
	if names(m) != "1 test" || m.Workflows.Card.Err == nil || !strings.Contains(m.Workflows.Card.Err.Error(), "at least one step") {
		t.Errorf("removing the last step left %q with error %v, want the step kept and the reason said", names(m), m.Workflows.Card.Err)
	}
}

func TestAddedStepsGetUniqueNames(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+n"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+n"})
	if got := strings.Join(m.Workflows.Card.Form.StepLabels(), ","); got != "1 step,2 step2,3 step3" {
		t.Errorf("steps %q", got)
	}
}

func TestAStepsKindAndTextAreEditedInTheCard(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m = typedInto(m, model.WorkflowFieldName, "Nightly")
	m = editStepBody(m, 0, "make")
	m, _ = model.Update(m, model.ChooseMsg{Field: model.WorkflowFieldKind, Index: 0})
	m = typedInto(m, model.WorkflowFieldKey, "x")
	m = typedInto(m, model.WorkflowFieldRetries, "2")
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || save.UID != "" || save.Name != "Nightly" ||
		!strings.Contains(save.Definition, `"key":"stepx"`) || !strings.Contains(save.Definition, `"kind":"command"`) ||
		!strings.Contains(save.Definition, `"command":"make"`) || !strings.Contains(save.Definition, `"retries":2`) {
		t.Fatalf("save sent %#v (error %v), want a command step stepx with 2 retries", cmds, m.Workflows.Card.Err)
	}
}

func TestSpaceMovesAChoiceOnAndTypesIntoText(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.WorkflowFieldKind})
	m, _ = model.Update(m, model.KeyMsg{Chord: "space"})
	if got := m.Workflows.Card.Form.Step().Kind; got != cronjob.StepCommand {
		t.Errorf("space on the kind left %q", got)
	}
	m = typedInto(m, model.WorkflowFieldAbout, "a")
	m, _ = model.Update(m, model.KeyMsg{Chord: "space"})
	m = typed(m, "b")
	if got := m.Workflows.Card.Form.About; got != "a b" {
		t.Errorf("about %q", got)
	}
}

func TestAFormLeftAsItOpenedClosesWithoutAskingAndAChangedOneAsks(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	closed, _ := model.Update(m, model.KeyMsg{Chord: "escape"})
	if closed.Workflows.Card.Open {
		t.Error("esc on an unchanged form left the card open")
	}
	changed := editStepBody(m, 0, "Something else.")
	asked, _ := model.Update(changed, model.KeyMsg{Chord: "escape"})
	if !asked.Workflows.Card.Open || asked.CurrentMode() != model.ModeDiscard {
		t.Fatalf("esc on a changed form: open %v, mode %s; want the discard question", asked.Workflows.Card.Open, asked.CurrentMode())
	}
	kept, _ := model.Update(asked, model.KeyMsg{Chord: "n"})
	if !kept.Workflows.Card.Open || kept.CurrentMode() != model.ModeWorkflowForm {
		t.Error("keeping the Draft did not return to the form")
	}
	dropped, _ := model.Update(asked, model.KeyMsg{Chord: "y"})
	if dropped.Workflows.Card.Open {
		t.Error("discarding left the card open")
	}
}

func TestSavingNeedsANameAndAWholeNumberOfRetries(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m = editStepBody(m, 0, "Do it.")
	noName, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) != 0 || noName.Workflows.Card.Err == nil || !strings.Contains(noName.Workflows.Card.Err.Error(), "needs a name") {
		t.Errorf("a form with no name sent %#v with error %v", cmds, noName.Workflows.Card.Err)
	}
	m = typedInto(m, model.WorkflowFieldName, "Flow")
	m = typedInto(m, model.WorkflowFieldRetries, "x")
	bad, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) != 0 || bad.Workflows.Card.Err == nil || !strings.Contains(bad.Workflows.Card.Err.Error(), "retries must be a whole number") {
		t.Errorf("retries x sent %#v with error %v", cmds, bad.Workflows.Card.Err)
	}
}

func firstCmd[T model.Cmd](cmds []model.Cmd) (T, bool) {
	for _, cmd := range cmds {
		if c, ok := cmd.(T); ok {
			return c, true
		}
	}
	var zero T
	return zero, false
}

func TestANewWorkflowIsCreatedWithoutAGuard(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m = loadedText(m, `{"name":"Nightly","definition":{"version":1,"steps":[{"key":"build","kind":"command","command":"make"}]}}`)
	_, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || save.UID != "" || save.Expected != "" || save.Name != "Nightly" {
		t.Fatalf("create sent %#v", cmds)
	}
}

func TestAWorkflowTheLocalCheckRefusesStaysInTheCardWithTheProblemNamed(t *testing.T) {
	for name, tc := range map[string]struct{ text, problem string }{
		"broken JSON":                  {`{"name": "x", "definition": }`, "line 1"},
		"syntax error on a later line": {"{\n  \"name\": \"x\",\n  \"definition\": }\n}", "line 3"},
		"truncated JSON":               {`{"name": "x"`, "ends early"},
		"nothing":                      {``, "empty"},
		"unknown field":                {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","when":"now"}]}}`, `"when"`},
		"type mismatch":                {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","retries":1.5}]}}`, "whole number"},
		"no name":                      {`{"name":" ","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"}]}}`, "needs a name"},
		"name too long":                {`{"name":"` + strings.Repeat("n", 257) + `","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"}]}}`, `"name" must be 1 to 256 characters`},
		"definition over 256 KiB":      {`{"name":"x","definition":{"version":1,"about":"` + strings.Repeat("a", 256<<10) + `","steps":[{"key":"a","kind":"command","command":"c"}]}}`, "over kata's 256 KiB limit"},
		"a kata rule broken":           {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"},{"key":"a","kind":"command","command":"c"}]}}`, `Step "a": "key" is used by an earlier step; keys must be unique.`},
		"text after":                   {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"}]}} {}`, "more text follows"},
	} {
		t.Run(name, func(t *testing.T) {
			m := onWorkflows(t)
			m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
			m = loadedText(m, tc.text)
			var cmds []model.Cmd
			if m.Workflows.Card.Err == nil {
				// Text that reads as a document is checked against kata's rules when it is saved.
				m, cmds = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
			}
			card := m.Workflows.Card
			if len(cmds) != 0 || card.Saving {
				t.Fatalf("a refused text sent %#v", cmds)
			}
			if card.Err == nil || !strings.Contains(card.Err.Error(), tc.problem) {
				t.Fatalf("error %v, want it to name %q", card.Err, tc.problem)
			}
			if !card.Open {
				t.Error("the card closed on a refused text")
			}
		})
	}
}

func TestAConflictingSaveKeepsTheEditedTextUntilTheUserOverwritesOrDiscards(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	m = editStepBody(m, 1, "make check")
	edited := m.Workflows.Card.Form.JSON()
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	newer := reviewFlow()
	newer.EventUID, newer.Revision = "E2", 2
	newer.Steps[0].Prompt = "Review it twice."
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &newer})

	card := m.Workflows.Card
	if m.CurrentMode() != model.ModeConflict || card.Conflict == nil || card.Conflict.EventUID != "E2" || card.Form.JSON() != edited || card.Saving {
		t.Fatalf("after the conflict: mode %s, conflict %+v, text kept %v, saving %v", m.CurrentMode(), card.Conflict, card.Form.JSON() == edited, card.Saving)
	}
	if got, _ := m.SelectedWorkflow(); got.EventUID != "E1" {
		t.Errorf("the list shows %s before anyone chose; the conflicting version must not replace it yet", got.EventUID)
	}

	t.Run("overwrite", func(t *testing.T) {
		_, cmds := model.Update(m, model.KeyMsg{Chord: "o"})
		save, ok := firstCmd[model.SaveWorkflow](cmds)
		if !ok || save.Expected != "E2" || save.UID != "W1" || !strings.Contains(save.Definition, "make check") {
			t.Fatalf("overwrite sent %#v, want the edited text guarded on E2", cmds)
		}
	})
	t.Run("discard", func(t *testing.T) {
		got, cmds := model.Update(m, model.KeyMsg{Chord: "d"})
		if len(cmds) != 0 || got.Workflows.Card.Open {
			t.Fatalf("discard left the card open (%v) or sent %#v", got.Workflows.Card.Open, cmds)
		}
		if w, _ := got.SelectedWorkflow(); w.EventUID != "E2" {
			t.Errorf("the list shows %s, want the newer version", w.EventUID)
		}
	})
}

func TestASavedWorkflowClosesTheCardAndJoinsTheList(t *testing.T) {
	m := onWorkflows(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
	m = loadedText(m, `{"name":"Nightly","definition":{"version":1,"steps":[{"key":"build","kind":"command","command":"make"}]}}`)
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	saved := model.Workflow{UID: "W9", Name: "Nightly", EventUID: "E9"}
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, Workflow: &saved})
	if m.Workflows.Card.Open || m.Workflows.Selected != "W9" || len(m.VisibleWorkflows()) != 1 {
		t.Errorf("card open %v, selected %q, rows %d", m.Workflows.Card.Open, m.Workflows.Selected, len(m.VisibleWorkflows()))
	}
}

func TestAFailedSaveKeepsTheTextAndSaysWhy(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	text := m.Workflows.Card.Form.JSON()
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Err: errString("daemon gone")})
	if card := m.Workflows.Card; !card.Open || card.Saving || card.Err == nil || card.Form.JSON() != text || !strings.Contains(card.Err.Error(), "Cannot save the workflow: daemon gone") {
		t.Errorf("card %+v", card)
	}
}

func TestARefusedWriteAfterAProjectSwitchSaysWhy(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "other", Op: model.WorkflowOpSave, UID: "W1", Name: "Review then test", Err: errString("daemon gone")})
	if !strings.Contains(m.Notice, "Review then test") || !strings.Contains(m.Notice, "daemon gone") {
		t.Errorf("notice = %q", m.Notice)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestExternalEditorReturnsTheEditedTextToTheCard(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+e"})
	want := model.OpenDraftEditor{UID: model.WorkflowDraftUID, Text: reviewFlow().Text(), Suffix: ".json"}
	if !hasCmd(cmds, want) {
		t.Fatalf("ctrl+e sent %#v, want %#v", cmds, want)
	}
	m = loadedText(m, "{\r\n\"name\": \"Renamed\",\r\n\"definition\": {\"version\": 1, \"steps\": [{\"key\": \"a\", \"kind\": \"command\", \"command\": \"make\"}]}}\r\n")
	if form := m.Workflows.Card.Form; form.Name != "Renamed" || len(form.Steps) != 1 || form.Steps[0].Key != "a" || form.Steps[0].Body.Text != "make" || m.Workflows.Card.Err != nil {
		t.Errorf("form %+v, error %v; want the editor's document in the form", form, m.Workflows.Card.Err)
	}
	broken := loadedText(m, "{")
	if broken.Workflows.Card.Form.Name != "Renamed" || broken.Workflows.Card.Err == nil || !strings.Contains(broken.Workflows.Card.Err.Error(), "Cannot read the editor's text") {
		t.Errorf("text that is not JSON left %+v with error %v, want the form kept and the reason said", broken.Workflows.Card.Form, broken.Workflows.Card.Err)
	}
}

func TestArchivingAndRestoringAreGuardedOnTheVersionOnScreen(t *testing.T) {
	archived := reviewFlow()
	archived.UID, archived.Name, archived.Archived, archived.EventUID = "W2", "Zed", true, "E7"
	m := onWorkflows(t, reviewFlow(), archived)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "x"})
	if want := (model.ArchiveWorkflow{Workspace: "/repo", ProjectID: 7, ProjectUID: "p", UID: "W1", Name: "Review then test", Expected: "E1"}); !hasCmd(cmds, want) {
		t.Fatalf("x sent %#v, want %#v", cmds, want)
	}
	if len(m.VisibleWorkflows()) != 1 {
		t.Fatalf("archived workflows are shown by default: %d rows", len(m.VisibleWorkflows()))
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "a"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "r"})
	if want := (model.RestoreWorkflow{Workspace: "/repo", ProjectID: 7, ProjectUID: "p", UID: "W2", Name: "Zed", Expected: "E7"}); !hasCmd(cmds, want) {
		t.Fatalf("r sent %#v, want %#v", cmds, want)
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "e"})
	if m.Workflows.Card.Open || !strings.Contains(m.Notice, "Restore it first") || len(cmds) != 0 {
		t.Errorf("editing an archived workflow opened the card (%v) or said %q", m.Workflows.Card.Open, m.Notice)
	}
}

func TestArchiveRefusedByKataShowsItsMessageAndChangesNothing(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	refusal := "invalid cron document: workflow W1 is used by live job J1; update or delete that job first"
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Err: errString(refusal)})
	if want := `Cannot archive "W1": ` + refusal + "."; m.Notice != want || m.NoticeKind != model.NoticeFail || len(m.VisibleWorkflows()) != 1 {
		t.Errorf("notice %q, rows %d", m.Notice, len(m.VisibleWorkflows()))
	}
}

func TestAnArchiveOrRestoreOfAChangedWorkflowWritesNothingAndShowsTheCurrentOne(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	newer := reviewFlow()
	newer.EventUID, newer.Name = "E2", "Renamed"
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Current: &newer})
	if w, _ := m.SelectedWorkflow(); w.EventUID != "E2" || !strings.Contains(m.Notice, "changed since you opened it") {
		t.Errorf("selected %+v, notice %q", w, m.Notice)
	}
}

func TestAnArchiveConflictThatHidesTheRowKeepsTheSelectionAtItsRow(t *testing.T) {
	var flows []model.Workflow
	for _, uid := range []string{"WA", "WB", "WC"} {
		w := reviewFlow()
		w.UID, w.Name = uid, uid
		flows = append(flows, w)
	}
	m := onWorkflows(t, flows...)
	m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	gone := flows[1]
	gone.EventUID, gone.Archived = "E2", true
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "WB", Current: &gone})
	if got, _ := m.SelectedWorkflow(); got.UID != "WC" {
		t.Errorf("selected %q, want WC: the row now at the old position", got.UID)
	}
}

func TestAConflictWhoseCurrentVersionIsArchivedCannotBeOverwritten(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	m = editStepBody(m, 1, "make check")
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	gone := reviewFlow()
	gone.EventUID, gone.Revision, gone.Archived = "E2", 2, true
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &gone})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "o"})
	if _, sent := firstCmd[model.SaveWorkflow](cmds); sent || m.Workflows.Card.Conflict == nil || !strings.Contains(m.Notice, "archived") {
		t.Errorf("overwrite sent %#v, conflict kept %v, notice %q", cmds, m.Workflows.Card.Conflict != nil, m.Notice)
	}
}

func TestEnterOpensTheStepsAndEscClosesThemBeforeLeavingTheMenu(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m.Cols = 100
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	if !m.Workflows.Inspecting {
		t.Fatal("enter did not open the inspector")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if m.Workflows.Inspecting || m.Page != model.PageWorkflows {
		t.Fatalf("esc: inspecting %v, page %v; want the steps closed and the Page kept", m.Workflows.Inspecting, m.Page)
	}
}

func TestEachPageKeyLeavesTheWorkflowsPage(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "1"})
	if m.Page != model.PageIssues || m.Workflows.Inspecting {
		t.Errorf("page %v after 1", m.Page)
	}
	if m.CurrentMode() != model.ModeList {
		t.Errorf("mode %s, want the list's", m.CurrentMode())
	}
}

func TestAWidePaneShowsTheStepsBesideTheListWithoutEnter(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	if m.Workflows.Inspecting {
		t.Error("enter opened an inspector in a Pane that already shows it beside the list")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if m.Page != model.PageWorkflows {
		t.Errorf("page %v after esc: with no inspector to close, esc leaves for the Menu, not another Page", m.Page)
	}
}
