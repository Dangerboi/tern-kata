package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func reviewFlow() model.Workflow {
	return model.Workflow{UID: "W1", Name: "Review then test", EventUID: "E1", Revision: 1, Steps: []model.WorkflowStep{
		{Key: "review", Kind: model.StepPrompt, Prompt: "Review the change."},
		{Key: "test", Kind: model.StepCommand, Command: "make test", After: []string{"review"}, Retries: 1},
	}}
}

// workflowsPage is a bound Project's Workflows Page with the given workflows loaded, and the commands entering it sent.
func workflowsPage(flows ...model.Workflow) (model.Model, []model.Cmd) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "5"})
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
	m, _ = model.Update(m, model.KeyMsg{Chord: "5"})
	if m.Page != model.PageIssues || !strings.Contains(m.Notice, "Project") || !m.SidebarProjects {
		t.Errorf("page %v, notice %q, Projects level %v; want the Issues Page, a notice and the Menu's Projects", m.Page, m.Notice, m.SidebarProjects)
	}
}

func typedText(m model.Model, text string) model.Model {
	old := len(m.Workflows.Card.Editor.Text)
	m, _ = model.Update(m, model.EditMsg{Target: model.EditWorkflowBody, From: 0, To: old, Text: text, Cursor: len(text), Len: old})
	return m
}

func TestEditingAWorkflowShowsItsDefinitionAsTheTextAndSavesItGuardedOnItsVersion(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	if !m.Workflows.Card.Open || m.TopCard() != model.CardWorkflow || m.CurrentMode() != model.ModeEditor {
		t.Fatalf("e opened card %v in mode %s", m.TopCard(), m.CurrentMode())
	}
	if want := reviewFlow().Text(); m.Workflows.Card.Editor.Text != want {
		t.Fatalf("card text %q, want the workflow's own text", m.Workflows.Card.Editor.Text)
	}
	m = typedText(m, strings.Replace(m.Workflows.Card.Editor.Text, "make test", "make check", 1))
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || save.UID != "W1" || save.Expected != "E1" || save.ProjectID != 7 || save.Name != "Review then test" || !strings.Contains(save.Definition, `"command":"make check"`) {
		t.Fatalf("save sent %#v, want a replace of W1 guarded on E1", cmds)
	}
	if !m.Workflows.Card.Saving {
		t.Error("the card should show it is saving")
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
	m = typedText(m, `{"name":"Nightly","definition":{"version":1,"steps":[{"key":"build","kind":"command","command":"make"}]}}`)
	_, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	save, ok := firstCmd[model.SaveWorkflow](cmds)
	if !ok || save.UID != "" || save.Expected != "" || save.Name != "Nightly" {
		t.Fatalf("create sent %#v", cmds)
	}
}

func TestAWorkflowTheLocalCheckRefusesStaysInTheCardWithTheProblemNamed(t *testing.T) {
	for name, tc := range map[string]struct{ text, problem string }{
		"broken JSON":        {`{"name": "x", "definition": }`, "line 1"},
		"truncated JSON":     {`{"name": "x"`, "ends early"},
		"nothing":            {``, "empty"},
		"unknown field":      {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","when":"now"}]}}`, `"when"`},
		"no name":            {`{"name":" ","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"}]}}`, `"name"`},
		"wrong version":      {`{"name":"x","definition":{"version":2,"steps":[{"key":"a","kind":"command","command":"c"}]}}`, `"version"`},
		"no steps":           {`{"name":"x","definition":{"version":1,"steps":[]}}`, `"steps"`},
		"duplicate keys":     {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"},{"key":"a","kind":"command","command":"c"}]}}`, `"key" is used by an earlier step`},
		"unportable key":     {`{"name":"x","definition":{"version":1,"steps":[{"key":"a b","kind":"command","command":"c"}]}}`, `"key" must be a name`},
		"forward after":      {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","after":["b"]},{"key":"b","kind":"command","command":"c"}]}}`, `"after" names "b", which is not an earlier step`},
		"self after":         {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","after":["a"]}]}}`, `"after" names "a"`},
		"too many retries":   {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","retries":101}]}}`, `"retries" must be between 0 and 100`},
		"negative retries":   {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","retries":-1}]}}`, `"retries"`},
		"fractional retries": {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","retries":1.5}]}}`, "whole number"},
		"command and prompt": {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","prompt":"p"}]}}`, "command step needs"},
		"prompt and command": {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"prompt","command":"c","prompt":"p"}]}}`, "prompt step needs"},
		"neither":            {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command"}]}}`, "command step needs"},
		"unknown kind":       {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"shell","command":"c"}]}}`, `"kind" must be`},
		"secret in options":  {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c","options":{"deep":{"Token":"t"}}}]}}`, "credentials or local paths"},
		"text after":         {`{"name":"x","definition":{"version":1,"steps":[{"key":"a","kind":"command","command":"c"}]}} {}`, "more text follows"},
	} {
		t.Run(name, func(t *testing.T) {
			m := onWorkflows(t)
			m, _ = model.Update(m, model.KeyMsg{Chord: "n"})
			m = typedText(m, tc.text)
			m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
			card := m.Workflows.Card
			if len(cmds) != 0 || card.Saving {
				t.Fatalf("a refused text sent %#v", cmds)
			}
			if card.Err == nil || !strings.Contains(card.Err.Error(), tc.problem) {
				t.Fatalf("error %v, want it to name %q", card.Err, tc.problem)
			}
			if !card.Open || card.Editor.Text != tc.text {
				t.Errorf("the card kept %q (open %v), want the edited text kept", card.Editor.Text, card.Open)
			}
		})
	}
}

func TestASyntaxErrorNamesItsLine(t *testing.T) {
	_, err := model.ParseWorkflow("{\n  \"name\": \"x\",\n  \"definition\": }\n}")
	if err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error %v, want line 3", err)
	}
}

func TestAConflictingSaveKeepsTheEditedTextUntilTheUserOverwritesOrDiscards(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	m, _ = model.Update(m, model.KeyMsg{Chord: "e"})
	edited := strings.Replace(m.Workflows.Card.Editor.Text, "make test", "make check", 1)
	m = typedText(m, edited)
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	newer := reviewFlow()
	newer.EventUID, newer.Revision = "E2", 2
	newer.Steps[0].Prompt = "Review it twice."
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &newer})

	card := m.Workflows.Card
	if m.CurrentMode() != model.ModeConflict || card.Conflict == nil || card.Conflict.EventUID != "E2" || card.Editor.Text != edited || card.Saving {
		t.Fatalf("after the conflict: mode %s, conflict %+v, text kept %v, saving %v", m.CurrentMode(), card.Conflict, card.Editor.Text == edited, card.Saving)
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
	m = typedText(m, `{"name":"Nightly","definition":{"version":1,"steps":[{"key":"build","kind":"command","command":"make"}]}}`)
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
	text := m.Workflows.Card.Editor.Text
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Err: errString("daemon gone")})
	if card := m.Workflows.Card; !card.Open || card.Saving || card.Err == nil || card.Editor.Text != text {
		t.Errorf("card %+v", card)
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
	m, _ = model.Update(m, model.DraftEditorClosed{UID: model.WorkflowDraftUID, Text: "{}\r\n"})
	if m.Workflows.Card.Editor.Text != "{}\n" || m.Workflows.Card.Err != nil {
		t.Errorf("text %q, error %v; want the editor's text with LF line breaks", m.Workflows.Card.Editor.Text, m.Workflows.Card.Err)
	}
}

func TestArchivingAndRestoringAreGuardedOnTheVersionOnScreen(t *testing.T) {
	archived := reviewFlow()
	archived.UID, archived.Name, archived.Archived, archived.EventUID = "W2", "Zed", true, "E7"
	m := onWorkflows(t, reviewFlow(), archived)
	m, cmds := model.Update(m, model.KeyMsg{Chord: "x"})
	if want := (model.ArchiveWorkflow{Workspace: "/repo", ProjectID: 7, ProjectUID: "p", UID: "W1", Expected: "E1"}); !hasCmd(cmds, want) {
		t.Fatalf("x sent %#v, want %#v", cmds, want)
	}
	if len(m.VisibleWorkflows()) != 1 {
		t.Fatalf("archived workflows are shown by default: %d rows", len(m.VisibleWorkflows()))
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "a"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	m, cmds = model.Update(m, model.KeyMsg{Chord: "r"})
	if want := (model.RestoreWorkflow{Workspace: "/repo", ProjectID: 7, ProjectUID: "p", UID: "W2", Expected: "E7"}); !hasCmd(cmds, want) {
		t.Fatalf("r sent %#v, want %#v", cmds, want)
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "e"})
	if m.Workflows.Card.Open || !strings.Contains(m.Notice, "restore") || len(cmds) != 0 {
		t.Errorf("editing an archived workflow opened the card (%v) or said %q", m.Workflows.Card.Open, m.Notice)
	}
}

func TestArchiveRefusedByKataShowsItsMessageAndChangesNothing(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	refusal := "invalid cron document: workflow W1 is used by live job J1; update or delete that job first"
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Err: errString(refusal)})
	if m.Notice != refusal || len(m.VisibleWorkflows()) != 1 {
		t.Errorf("notice %q, rows %d", m.Notice, len(m.VisibleWorkflows()))
	}
}

func TestAnArchiveOrRestoreOfAChangedWorkflowWritesNothingAndShowsTheCurrentOne(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
	newer := reviewFlow()
	newer.EventUID, newer.Name = "E2", "Renamed"
	m, _ = model.Update(m, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Current: &newer})
	if w, _ := m.SelectedWorkflow(); w.EventUID != "E2" || !strings.Contains(m.Notice, "changed meanwhile") {
		t.Errorf("selected %+v, notice %q", w, m.Notice)
	}
}

func TestEnterOpensTheStepsAndEscClosesThemBeforeLeavingTheMenu(t *testing.T) {
	m := onWorkflows(t, reviewFlow())
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
