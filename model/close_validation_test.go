package model_test

import (
	"errors"
	"github.com/Dangerboi/tern-kata/model"
	"strings"
	"testing"
)

func TestCloseSequences(t *testing.T) {
	for _, tt := range []struct {
		name           string
		msgs           []model.Msg
		wantError      string
		submit, reopen bool
	}{
		{name: "missing message", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseSubmitMsg{}}, wantError: "at least 40"},
		{name: "missing evidence", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: 1, Value: "Implemented the requested change and verified the result."}, model.CloseSubmitMsg{}}, wantError: "requires commit"},
		{name: "bad sha", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: 1, Value: "Implemented the requested change and verified the result."}, model.CloseFieldMsg{Field: 2, Value: "invalid"}, model.CloseSubmitMsg{}}, wantError: "hexadecimal"},
		{name: "valid close", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: 1, Value: "Implemented the requested change and verified the result."}, model.CloseFieldMsg{Field: 4, Value: "go test ./model"}, model.CloseSubmitMsg{}}, submit: true},
		{name: "invalid reason", msgs: []model.Msg{model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: 0, Value: "other"}, model.CloseSubmitMsg{}}, wantError: "Reason must"},
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
					if cmd.Body.Source != nil || cmd.Body.Message == nil || len(cmd.Body.Evidence) != 1 || cmd.Body.Evidence[0].Type != "test" {
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

func TestCloseRefusalsPreserveReadableMessage(t *testing.T) {
	for _, tt := range []struct{ code, message string }{
		{"revision_conflict", "issue revision is 8"},
		{"parent_has_open_children", "refusing: issue has 1 open children:\n  abcd Child\nClose children first"},
		{"sibling_throttle", "sibling-close throttle: Slow down and review"},
		{"duplicate_message", "Use specific prose for this Issue"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			refusal := errors.New(tt.message + "\nReview and retry")
			m := model.Model{CloseForm: model.CloseFormState{Open: true, Pending: true, UID: "issue"}}
			m, _ = model.Update(m, model.CloseResultMsg{UID: "issue", Err: refusal})
			if !m.CloseForm.Open || m.CloseForm.Pending || m.CloseForm.Err.Error() != tt.message+"\nReview and retry" {
				t.Fatalf("refusal: %+v", m.CloseForm)
			}
		})
	}
}
