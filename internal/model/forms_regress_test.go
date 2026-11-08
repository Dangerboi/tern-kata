package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTimedAssignmentCanBeRetriedAfterAFailedWrite(t *testing.T) {
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}}
	m.FieldForm = model.FieldFormState{Open: true, Action: model.TimedAssignAction, UID: "i", Value: "1h"}
	m, cmds := clickPress(m, "enter")
	if writtenValue(cmds) != "3600" || m.FieldForm.Value != "1h" {
		t.Fatalf("first send: %#v value %q", cmds, m.FieldForm.Value)
	}
	m, _ = model.Update(m, model.FieldWritten{UID: "i", Err: errors.New("refused")})
	m, cmds = clickPress(m, "enter")
	if writtenValue(cmds) != "3600" || m.FieldForm.Err != nil {
		t.Fatalf("retry: %#v err %v", cmds, m.FieldForm.Err)
	}
}

func TestCloseCardCancelClearsItsValidationError(t *testing.T) {
	m := model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Revision: 7}}}
	m, _ = clickPress(m, "x", "ctrl+o")
	if m.CloseForm.Err == nil {
		t.Fatal("ctrl+o with no message raised no error")
	}
	m, _ = clickPress(m, "escape")
	if m.CloseForm.Open || m.CloseForm.Err != nil {
		t.Fatalf("after esc: open %v err %v", m.CloseForm.Open, m.CloseForm.Err)
	}
}

func TestFieldCardRefusesWithAMessageWhenTheDaemonDropsMidEdit(t *testing.T) {
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}}
	m, _ = clickPress(m, "!")
	m, _ = model.Update(m, model.FieldValueMsg{Value: "2"})
	m.Stale = true
	m, cmds := clickPress(m, "enter")
	if len(cmds) != 0 || m.FieldForm.Saving || m.FieldForm.Err == nil || !strings.HasPrefix(m.FieldForm.Err.Error(), "Daemon disconnected; reconnect before") {
		t.Fatalf("cmds %#v saving %v err %v", cmds, m.FieldForm.Saving, m.FieldForm.Err)
	}
}

func TestLanesCardRefusesToShareWhileTheDaemonIsGone(t *testing.T) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, _ = clickPress(m, "b")
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest(), Lanes: []string{"triage"}, Shared: true, ProjectID: 7, Revision: 4})
	m, _ = clickPress(m, "shift+b")
	m.Stale = true
	m, cmds := clickPress(m, "enter")
	if len(cmds) != 0 || m.LanesForm.Saving || m.LanesForm.Err == nil || !strings.HasPrefix(m.LanesForm.Err.Error(), "Daemon disconnected; reconnect before") {
		t.Fatalf("cmds %#v saving %v err %v", cmds, m.LanesForm.Saving, m.LanesForm.Err)
	}
}

func TestLateLabelLoadKeepsTheCardsError(t *testing.T) {
	m := model.Model{SelectedUID: "i", Issues: []model.Issue{{UID: "i"}}, Create: model.CreateState{Open: true}}
	m.Create.Err = errors.New("Title is required.")
	m, _ = model.Update(m, model.FieldLabelsLoaded{UID: model.CreateDraftUID, Labels: []string{"bug"}})
	if m.Create.Err == nil || m.Create.Err.Error() != "Title is required." {
		t.Fatalf("create err %v", m.Create.Err)
	}
	m.FieldForm = model.FieldFormState{Open: true, UID: "i", Action: model.AddLabelAction}
	m.FieldForm.Err = errors.New("Enter a value.")
	m, _ = model.Update(m, model.FieldLabelsLoaded{UID: "i", Err: errors.New("load failed")})
	if m.FieldForm.Err.Error() != "Enter a value." {
		t.Fatalf("field err %v", m.FieldForm.Err)
	}
	m.FieldForm.Err = nil
	m, _ = model.Update(m, model.FieldLabelsLoaded{UID: "i", Err: errors.New("load failed")})
	if m.FieldForm.Err == nil || m.FieldForm.Err.Error() != "load failed" {
		t.Fatalf("load error not shown: %v", m.FieldForm.Err)
	}
}

func writtenValue(cmds []model.Cmd) string {
	for _, c := range cmds {
		if w, ok := c.(model.WriteField); ok {
			return w.Value
		}
	}
	return ""
}
