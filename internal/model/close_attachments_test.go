package model_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const closeMessage = "Implemented the requested change and verified the result."

// attachedIssue is a model with Issue "issue" listed, holding the attachments "a.txt" (id A) and "b.log" (id B).
func attachedIssue() model.Model {
	meta := map[string]any{"attachment.A": reference("a.txt", "text/plain", 1), "attachment.B": reference("b.log", "text/plain", 2)}
	return model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2, Metadata: meta}}}
}

func updated(m model.Model, msgs ...model.Msg) (model.Model, []model.Cmd) {
	var cmds []model.Cmd
	for _, msg := range msgs {
		m, cmds = model.Update(m, msg)
	}
	return m, cmds
}

func closeCommand(t *testing.T, cmds []model.Cmd) model.CloseIssueCmd {
	t.Helper()
	for _, cmd := range cmds {
		if c, ok := cmd.(model.CloseIssueCmd); ok {
			return c
		}
	}
	t.Fatalf("no close was sent: %#v", cmds)
	return model.CloseIssueCmd{}
}

func TestClosePickerListsTheIssuesAttachmentsAndCitesTheOnesPicked(t *testing.T) {
	m, _ := updated(attachedIssue(), model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseMessage, Value: closeMessage})
	if !m.CloseForm.ShowsAttachments() || len(m.CloseForm.Attachments) != 2 {
		t.Fatalf("the picker is not offered: %+v", m.CloseForm)
	}
	// A click on the second name picks it; space on the first one (after tab walks the focus there) picks that too.
	m, _ = updated(m, model.ChooseMsg{Field: model.CloseAttachments, Index: 1}, model.KeyMsg{Chord: "left"}, model.KeyMsg{Chord: "space"})
	if m.CloseForm.Field != model.CloseAttachments || !m.CloseForm.Cites("A") || !m.CloseForm.Cites("B") {
		t.Fatalf("picked %v", m.CloseForm.Cited)
	}
	m, cmds := updated(m, model.KeyMsg{Chord: "ctrl+o"})
	cmd := closeCommand(t, cmds)
	want := []model.CloseEvidence{{Kind: model.EvidenceAttachment, Value: "A"}, {Kind: model.EvidenceAttachment, Value: "B"}}
	if cmd.Reason != model.ReasonDone || !slices.Equal(cmd.Evidence, want) || m.CloseForm.Err != nil {
		t.Fatalf("sent %+v, error %v, want evidence %v", cmd, m.CloseForm.Err, want)
	}
}

func TestClosePickerTogglesOffAndSpaceStillTypesInTextFields(t *testing.T) {
	m, _ := updated(attachedIssue(), model.KeyMsg{Chord: "x"})
	m, _ = updated(m, model.ChooseMsg{Field: model.CloseAttachments, Index: 0}, model.ChooseMsg{Field: model.CloseAttachments, Index: 0})
	if len(m.CloseForm.Cited) != 0 {
		t.Fatalf("a second pick left %v", m.CloseForm.Cited)
	}
	m, _ = updated(m, model.FocusFieldMsg{Field: model.CloseMessage}, model.KeyMsg{Chord: "a"}, model.KeyMsg{Chord: "space"}, model.KeyMsg{Chord: "b"})
	if got := m.CloseForm.Values.Message; got != "a b" {
		t.Fatalf("message %q", got)
	}
}

func TestClosePickerIsOnlyOfferedToADoneCloseOfAnIssueWithAttachments(t *testing.T) {
	plain := model.Model{SelectedUID: "issue", Issues: []model.Issue{{UID: "issue", ProjectID: 2}}}
	m, _ := updated(plain, model.KeyMsg{Chord: "x"})
	if m.CloseForm.ShowsAttachments() {
		t.Fatal("an Issue with no attachments is offered the picker")
	}
	m, _ = updated(attachedIssue(), model.KeyMsg{Chord: "x"}, model.ChooseMsg{Field: model.CloseAttachments, Index: 0}, model.CloseFieldMsg{Field: model.CloseReason, Value: "wontfix"})
	if m.CloseForm.ShowsAttachments() {
		t.Fatal("a wontfix close is offered the picker")
	}
	// Tab skips the hidden picker.
	m, _ = updated(m, model.FocusFieldMsg{Field: model.CloseExternal}, model.KeyMsg{Chord: "tab"})
	if m.CloseForm.Field != model.CloseReference {
		t.Fatalf("tab went to %q, want the field after the hidden picker", m.CloseForm.Field)
	}
	// A pick made while the reason was done is not sent with another reason.
	m, cmds := updated(m, model.CloseFieldMsg{Field: model.CloseMessage, Value: strings.Repeat("because it is not wanted ", 4)}, model.KeyMsg{Chord: "ctrl+o"})
	if cmd := closeCommand(t, cmds); cmd.Reason != model.ReasonWontfix || len(cmd.Evidence) != 0 {
		t.Fatalf("sent %+v", cmd)
	}
}

func TestCloseWithoutEvidenceNamesTheAttachmentChoice(t *testing.T) {
	m, _ := updated(attachedIssue(), model.KeyMsg{Chord: "x"}, model.CloseFieldMsg{Field: model.CloseMessage, Value: closeMessage}, model.KeyMsg{Chord: "ctrl+o"})
	if m.CloseForm.Err == nil || !strings.Contains(m.CloseForm.Err.Error(), "attachment") {
		t.Fatalf("error %v", m.CloseForm.Err)
	}
}

func closeEventRow(t *testing.T, payload string, metadata map[string]any) string {
	t.Helper()
	event := model.Event{UID: "e", Type: "issue.closed", Actor: "dana", CreatedAt: time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC), Payload: payload}
	m := withDetail(model.Model{}, &model.IssueDetailData{Issue: &model.Issue{UID: "a", Metadata: metadata}, History: []model.Event{event}})
	row, ok := activityRow(m, model.TabEvents, 0)
	if !ok {
		t.Fatal("no history row")
	}
	return row.Body
}

func TestEventsLineNamesAttachmentEvidenceAndHowItStands(t *testing.T) {
	frozen := `{"name":"report.pdf","media_type":"application/pdf","size":9,"docbank_version":"6f1c2e8a-2222-4222-8222-000000000001"}`
	withCopy := `{"reason":"done","message":"shipped","evidence":[{"type":"attachment","attachment_id":"A","attachment":` + frozen + `}]}`
	now := reference("report.pdf", "application/pdf", 9)
	changed := reference("report.pdf", "application/pdf", 9)
	changed["docbank_version"] = "6f1c2e8a-2222-4222-8222-000000000002"
	for _, tt := range []struct {
		name, payload string
		metadata      map[string]any
		want          string
	}{
		{"present", withCopy, map[string]any{"attachment.A": now}, "shipped (attachment report.pdf)"},
		{"unlinked", withCopy, nil, "shipped (attachment report.pdf [removed])"},
		{"replaced", withCopy, map[string]any{"attachment.A": changed}, "shipped (attachment report.pdf [changed])"},
		{"no copy, present", `{"reason":"done","evidence":[{"type":"attachment","attachment_id":"A"}]}`, map[string]any{"attachment.A": now}, "attachment report.pdf"},
		{"no copy, gone", `{"reason":"done","evidence":[{"type":"attachment","attachment_id":"A"}]}`, nil, "Attachment removed"},
		{"bad copy", `{"reason":"done","evidence":[{"type":"attachment","attachment_id":"A","attachment":{"name":"x/y"}}]}`, nil, "Invalid attachment"},
		{"beside others", `{"reason":"done","evidence":[{"type":"commit","sha":"abc123def4"},{"type":"attachment","attachment_id":"A","attachment":` + frozen + `}]}`, map[string]any{"attachment.A": now}, "commit abc123d, attachment report.pdf"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := closeEventRow(t, tt.payload, tt.metadata); got != tt.want {
				t.Fatalf("Body %q, want %q", got, tt.want)
			}
		})
	}
}
