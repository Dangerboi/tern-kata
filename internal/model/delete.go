package model

import (
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

const (
	DeleteID     Field = "id"
	DeleteReason Field = "reason"
)

var deleteTable = fieldTable[DeleteFormState]{
	form: func(m *Model) *DeleteFormState { return &m.DeleteForm },
	fields: []formField[DeleteFormState]{
		textField(DeleteID, func(f *DeleteFormState) *string { return &f.ID }),
		textField(DeleteReason, func(f *DeleteFormState) *string { return &f.Reason }),
	},
}

var DeleteFields = deleteTable.specs()

type DeleteFormState struct {
	formCard
	UID         string
	ProjectID   int64
	QualifiedID string
	ShortID     string
	Title       string
	ID, Reason  string
}

// DeleteIssueCmd is one request to kata's delete action. QualifiedID is the Issue's project#short id, which the gateway sends
// in kata's confirmation header as "DELETE <QualifiedID>".
type DeleteIssueCmd struct {
	Workspace, UID, Actor, QualifiedID, Reason string
	ProjectID                                  int64
}

func (DeleteIssueCmd) cmd() {}

type IssueDeleted struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (IssueDeleted) msg() {}

// notIssueScoped is the condition of what kata refuses to a credential limited to one Issue (delete): the Pane offers it only
// while the connection is not known to be Issue-scoped.
func notIssueScoped(m Model) bool { return !m.IssueScoped }

func openDelete(m Model) Model {
	if m.Stale || m.DeleteForm.Saving {
		return m
	}
	issue := closeIssue(m)
	if issue == nil {
		return m
	}
	m.DeleteForm = DeleteFormState{Open: true, Field: DeleteID, UID: issue.UID, ProjectID: issue.ProjectID, QualifiedID: issue.Qualified(), ShortID: issue.ShortID, Title: issue.Title}
	return m
}

// submitDelete sends the delete once the typed text is the Issue's qualified id or kata tui's `DELETE <qualified id>`.
func submitDelete(m Model) (Model, []Cmd) {
	f := &m.DeleteForm
	if f.refuseSubmit(m, "deleting") {
		return m, nil
	}
	if typed := strings.TrimSpace(f.ID); typed != f.QualifiedID && typed != "DELETE "+f.QualifiedID {
		f.Err = fmt.Errorf("Type %s or DELETE %s to confirm the delete.", f.QualifiedID, f.QualifiedID)
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{DeleteIssueCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, QualifiedID: f.QualifiedID, Reason: strings.TrimSpace(f.Reason)}}
}

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
	m, wait := m.raiseToast(Toast{Text: fmt.Sprintf("Deleted %s. Press u to restore it.", qualified), Label: "Undo", Chord: "u"})
	m, cmds := snapshotChanged(m.issueWritten(msg.UID))
	return m, append(cmds, wait)
}
