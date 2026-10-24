package model

import (
	"errors"
	"fmt"
	"strings"
)

// The Delete card soft-deletes the selected Issue through kata's delete action, which kata only accepts with a confirmation
// naming the Issue exactly as the CLI's `kata delete` asks for it. The user types the Issue's qualified id to give it, and may
// add a reason. A deleted Issue stays in kata and can be restored, by `u` right after or from the Menu. Purge, which cannot be
// undone, is not offered.

const (
	ModeDeleteForm Mode   = "delete-form"
	DeleteSubmit   Action = "delete-submit"
	DeleteCancel   Action = "delete-cancel"
	OpenDelete     Action = "open-delete"
)

// The Delete card's fields.
const (
	DeleteID     Field = "id"
	DeleteReason Field = "reason"
)

// DeleteFields is the Delete card's field list, in tab order.
var DeleteFields = []FieldSpec{{DeleteID, true}, {DeleteReason, true}}

// DeleteFormState is the Delete card. QualifiedID, ShortID and Title are the Issue as the card opened; ID is the confirmation
// typed.
type DeleteFormState struct {
	Open, Saving bool
	Field        Field
	UID          string
	ProjectID    int64
	QualifiedID  string
	ShortID      string
	Title        string
	ID, Reason   string
	Caret        Caret
	Err          error
}

func (f *DeleteFormState) at(field Field) *string {
	if field == DeleteReason {
		return &f.Reason
	}
	return &f.ID
}

// DeleteIssueCmd is one request to kata's delete action. QualifiedID is the Issue's project#short id, which the gateway sends
// in kata's confirmation header as "DELETE <QualifiedID>".
type DeleteIssueCmd struct {
	Workspace, UID, Actor, QualifiedID, Reason string
	ProjectID                                  int64
}

func (DeleteIssueCmd) cmd() {}

// IssueDeleted answers DeleteIssueCmd. Record says what the delete did to the Undo history.
type IssueDeleted struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (IssueDeleted) msg() {}

// qualifiedID is the Issue as kata names it across Projects: project#short id.
func qualifiedID(i *Issue) string {
	if i.QualifiedID != "" {
		return i.QualifiedID
	}
	return i.ProjectName + "#" + i.ShortID
}

// openDelete is `X`: the card for the selected Issue, with the cursor in the confirmation.
func openDelete(m Model) Model {
	if m.Stale || m.DeleteForm.Saving {
		return m
	}
	issue := closeIssue(m)
	if issue == nil {
		return m
	}
	m.DeleteForm = DeleteFormState{Open: true, Field: DeleteID, UID: issue.UID, ProjectID: issue.ProjectID, QualifiedID: qualifiedID(issue), ShortID: issue.ShortID, Title: issue.Title}
	return m
}

// submitDelete sends the delete once the typed id matches the Issue's qualified id.
func submitDelete(m Model) (Model, []Cmd) {
	f := &m.DeleteForm
	if f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before deleting.")
		return m, nil
	}
	if strings.TrimSpace(f.ID) != f.QualifiedID {
		f.Err = fmt.Errorf("Type %s to confirm the delete.", f.QualifiedID)
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{DeleteIssueCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, QualifiedID: f.QualifiedID, Reason: strings.TrimSpace(f.Reason)}}
}

// updateDelete applies kata's answer to the delete: a refusal stays on the card, and a delete closes it, leaves the Issue's
// detail, and tells the user how to take it back.
func updateDelete(m Model, msg IssueDeleted) (Model, []Cmd) {
	f := &m.DeleteForm
	if !f.Open || f.UID != msg.UID {
		return m, nil
	}
	f.Saving = false
	if msg.Err != nil {
		f.Err = msg.Err
		return m, nil
	}
	qualified := f.QualifiedID
	m.DeleteForm = DeleteFormState{}
	m = issueGone(m, msg.UID)
	m.Notice = fmt.Sprintf("Deleted %s. Press u to restore it.", qualified)
	return snapshotChanged(m.issueWritten(msg.UID))
}

func updateDeleteFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.DeleteForm
	if form.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeDeleteForm)
	switch {
	case bound && binding.Action == DeleteSubmit:
		return submitDelete(m)
	case bound && binding.Action == DeleteCancel:
		m.DeleteForm = DeleteFormState{}
	default:
		text := form.at(form.Field)
		var edited bool
		if *text, edited = editText(*text, &form.Caret, form.Field, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
