package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// withHistory is m with one undoable change in the Undo history.
func withHistory(m model.Model) model.Model {
	entry := model.UndoEntry{Kind: model.UndoPriority, UID: "zzz", ProjectID: 2, ShortID: "zzzz", InstanceUID: "i", Revision: 2}
	m, _ = model.Update(m, model.FieldWritten{UID: "zzz", Record: model.WriteRecord{Entry: &entry}})
	return m
}

func undoSays(m model.Model) string {
	m, cmds := model.Update(m, model.KeyMsg{Chord: "u"})
	if len(cmds) == 1 {
		if _, ok := cmds[0].(model.UndoChange); ok {
			return "undoing"
		}
	}
	return m.Notice
}

// TestEveryWriteWithoutAnInverseClearsTheHistory sends each write the Pane makes that `u` cannot take back, and checks that the
// history it held is gone with the write's reason, so `u` never reaches past it.
func TestEveryWriteWithoutAnInverseClearsTheHistory(t *testing.T) {
	press := func(m model.Model, chords ...string) model.Model {
		for _, c := range chords {
			m, _ = model.Update(m, model.KeyMsg{Chord: c})
		}
		return m
	}
	for _, tt := range []struct {
		name   string
		model  func() model.Model
		act    func(model.Model) model.Model
		answer model.Msg
		reason string
	}{
		{"comment", editorModel, func(m model.Model) model.Model {
			m = press(m, "c")
			m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "local"})
			return press(m, "meta+o")
		}, model.DraftSaved{UID: "a"}, "comment addition cannot be undone"},
		{"attach", func() model.Model { return attachPane(model.ColumnList, nil, 2) }, func(m model.Model) model.Model {
			m = press(m, "@")
			m, _ = model.Update(m, model.FieldValueMsg{Value: "/x"})
			return press(m, "enter")
		}, model.FileAttached{Ref: "issue", Name: "x"}, "attaching a file cannot be undone"},
		{"detach", func() model.Model { return attachPane(model.ColumnList, pointers("a.png"), 2) }, func(m model.Model) model.Model {
			m = press(m, "shift+u")
			m, _ = model.Update(m, model.FieldValueMsg{Value: "a.png"})
			return press(m, "enter")
		}, model.FileDetached{Ref: "issue", Name: "a.png", Cleared: true}, "removing an attachment cannot be undone"},
		{"recurrence made", recurrenceBase, func(m model.Model) model.Model {
			m, _ = saveRecurrence(openRecurrenceCard(m))
			return m
		}, model.RecurrenceResultMsg{UID: "parent"}, "making a recurrence cannot be undone"},
		{"recurrence changed", func() model.Model { return recurring("FREQ=DAILY;INTERVAL=1") }, func(m model.Model) model.Model {
			m, _ = saveRecurrence(openRecurrenceCard(m))
			return m
		}, model.RecurrenceResultMsg{UID: "parent"}, "changing a recurrence cannot be undone"},
		{"recurrence stopped", func() model.Model { return recurring("FREQ=DAILY;INTERVAL=1") }, func(m model.Model) model.Model {
			return press(openRecurrenceCard(m), "ctrl+x", "y")
		}, model.RecurrenceResultMsg{UID: "parent"}, "stopping a recurrence cannot be undone"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := withHistory(tt.model())
			if got := undoSays(m); got != "undoing" {
				t.Fatalf("before the write u says %q, want it to start an undo", got)
			}
			m, _ = model.Update(tt.act(m), tt.answer)
			if got, want := undoSays(m), "nothing to undo: "+tt.reason; got != want {
				t.Fatalf("after the write u says %q, want %q", got, want)
			}
		})
	}
}

func TestCreatingAnIssueClearsTheHistory(t *testing.T) {
	m := withHistory(model.Model{Workspace: "workspace", Actor: "actor", Project: model.Project{ID: 7}, SelectedUID: "parent", Issues: []model.Issue{{UID: "parent", ProjectID: 7}}})
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+n"})
	m, _ = model.Update(m, model.KeyMsg{Text: "New Issue"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
	if len(cmds) == 0 {
		t.Fatal("nothing sent")
	}
	m, _ = model.Update(m, model.IssueCreated{UID: "created"})
	if got := undoSays(m); got != "nothing to undo: issue creation cannot be undone" {
		t.Fatalf("u says %q", got)
	}
}

func TestAWriteWithAnInverseDoesNotClearTheHistory(t *testing.T) {
	m := withHistory(editorModel())
	m, _ = model.Update(m, model.KeyMsg{Chord: "+"})
	m, _ = model.Update(m, model.FieldLabelsLoaded{UID: "a", Labels: []string{"bug"}})
	m, _ = model.Update(m, model.FieldValueMsg{Value: "bug"})
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 1 {
		t.Fatalf("sent %#v", cmds)
	}
	m, _ = model.Update(m, model.FieldWritten{UID: "a"})
	if got := undoSays(m); got != "undoing" {
		t.Fatalf("u says %q after a label, want it to start an undo", got)
	}
}
