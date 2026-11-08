package model_test

import (
	"errors"
	"github.com/Dangerboi/tern-kata/internal/model"
	"strings"
	"testing"
)

var closeSave = model.KeyMsg{Chord: "ctrl+o"}

func TestCloseSequences(t *testing.T) {
	for _, tt := range []struct {
		name           string
		msgs           []model.Msg
		wantError      string
		submit, reopen bool
	}{
		{name: "missing message", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, closeSave}, wantError: "at least 40"},
		{name: "missing evidence", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseMessage, Value: "Implemented the requested change and verified the result."}, closeSave}, wantError: "requires commit"},
		{name: "bad sha", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseMessage, Value: "Implemented the requested change and verified the result."}, model.CloseFieldMsg{Field: model.CloseCommit, Value: "invalid"}, closeSave}, wantError: "hexadecimal"},
		{name: "valid close", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseMessage, Value: "Implemented the requested change and verified the result."}, model.CloseFieldMsg{Field: model.CloseTest, Value: "go test ./model"}, closeSave}, submit: true},
		{name: "invalid reason", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseReason, Value: "other"}, closeSave}, wantError: "Reason must"},
		{name: "reopen", msgs: []model.Msg{model.KeyMsg{Chord: "r"}}, reopen: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Revision: 7}}}
			var cmds []model.Cmd
			for _, msg := range tt.msgs {
				m, cmds = model.Update(m, msg)
			}
			submitted, reopened := false, false
			for _, cmd := range cmds {
				switch cmd := cmd.(type) {
				case model.CloseIssueCmd:
					submitted = true
					if cmd.Quick || cmd.Reason != "done" || cmd.Message == "" || len(cmd.Evidence) != 1 || cmd.Evidence[0] != (model.CloseEvidence{Kind: model.EvidenceTest, Value: "go test ./model"}) {
						t.Fatalf("form fields: %+v", cmd)
					}
				case model.ReopenIssueCmd:
					reopened = true
					if cmd.UID != "issue" || cmd.ProjectID != 2 {
						t.Fatal(cmd)
					}
				}
			}
			if submitted != tt.submit || reopened != tt.reopen {
				t.Fatalf("commands: %#v", cmds)
			}
			if tt.wantError != "" && (m.CloseForm.Err == nil || !strings.Contains(m.CloseForm.Err.Error(), tt.wantError)) {
				t.Fatalf("error: %v", m.CloseForm.Err)
			}
		})
	}
}

func TestACloseRefusalKeepsTheFormOpenWithTheDaemonsWords(t *testing.T) {
	message := "refusing: issue has 1 open children:\n  abcd Child\nClose children first\nReview and retry"
	m := model.Model{CloseForm: model.CloseFormState{Open: true, Saving: true, UID: "issue"}}
	m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: errors.New(message)})
	if !m.CloseForm.Open || m.CloseForm.Saving || m.CloseForm.Err.Error() != message {
		t.Fatalf("refusal: %+v", m.CloseForm)
	}
}
