package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
)

// "Restore a deleted Issue" is a Menu entry that opens the single-field form for one reference: a short id, project#id or a
// ULID. Enter reads that Issue with deleted Issues included and shows its title and that it is deleted; Enter again restores it.

// RestoreDeletedAction is the Menu entry's form.
const RestoreDeletedAction Action = "restore-deleted"

// RestoreTitle is the Menu entry and the title of its form; RestoreKey names the Menu row.
const (
	RestoreTitle = "Restore a deleted Issue"
	RestoreKey   = "restore"
)

// DeletedIssue is the Issue a restore reads: Deleted says kata holds it as deleted.
type DeletedIssue struct {
	UID         string
	ProjectID   int64
	ShortID     string
	Title       string
	QualifiedID string
	Deleted     bool
}

// LookupDeleted reads an Issue with deleted Issues included. Typed is the reference as the user typed it and Ref the part kata
// takes with ProjectID; ByUID says Ref is a ULID, which needs no Project.
type LookupDeleted struct {
	Workspace, Typed, Ref string
	ProjectID             int64
	ByUID                 bool
}

func (LookupDeleted) cmd() {}

// DeletedLooked answers LookupDeleted; Ref is the reference as typed.
type DeletedLooked struct {
	Ref   string
	Issue DeletedIssue
	Err   error
}

func (DeletedLooked) msg() {}

// RestoreIssueCmd is one request to kata's restore action.
type RestoreIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
}

func (RestoreIssueCmd) cmd() {}

// IssueRestored answers RestoreIssueCmd. Changed is false when kata held the Issue as not deleted.
type IssueRestored struct {
	UID     string
	Changed bool
	Err     error
}

func (IssueRestored) msg() {}

// projectName is the name of the catalog Project with the id, "" when the Collection does not list it.
func (m Model) projectName(id int64) string {
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.ID == id {
			return entry.Project.Name
		}
	}
	return ""
}

// openRestore is the Menu entry: the form with nothing found yet.
func openRestore(m Model) Model {
	if m.Stale || m.FieldForm.Saving {
		return m
	}
	m.FieldForm = FieldFormState{Open: true, Action: RestoreDeletedAction}
	return m
}

// lookupDeleted turns the typed reference into the read: project#id and a bare short id name their Project (a bare one the
// bound Project), a ULID needs none.
func (m Model) lookupDeleted(ref string) (LookupDeleted, error) {
	cmd := LookupDeleted{Workspace: m.Workspace, Typed: ref, Ref: ref}
	name, short, qualified := strings.Cut(ref, "#")
	if qualified {
		if short == "" {
			return cmd, errors.New("Enter an Issue id after #.")
		}
		cmd.Ref = short
		if name == "" {
			return m.inBoundProject(cmd)
		}
		for _, entry := range m.Snapshot.Catalog {
			if entry.Project.Name == name {
				cmd.ProjectID = entry.Project.ID
				return cmd, nil
			}
		}
		return cmd, fmt.Errorf("kata Project %q not found", name)
	}
	if _, err := ulid.ParseStrict(ref); err == nil {
		cmd.ByUID = true
		return cmd, nil
	}
	return m.inBoundProject(cmd)
}

func (m Model) inBoundProject(cmd LookupDeleted) (LookupDeleted, error) {
	if !m.Bound || m.Project.ID == 0 {
		return cmd, errors.New("This Pane has no Project; name it as project#id.")
	}
	cmd.ProjectID = m.Project.ID
	return cmd, nil
}

// sendRestore is Enter in the form: read the Issue first, restore it once it is shown.
func sendRestore(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if found := f.Found; found != nil {
		f.Saving, f.Err = true, nil
		return m, []Cmd{RestoreIssueCmd{Workspace: m.Workspace, UID: found.UID, ProjectID: found.ProjectID, Actor: m.Actor}}
	}
	cmd, err := m.lookupDeleted(f.Value)
	if err != nil {
		f.Err = err
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{cmd}
}

// deletedLooked applies the read: the Issue is shown with its title and whether it is deleted. An answer for a reference no
// longer in the form is dropped.
func deletedLooked(m Model, msg DeletedLooked) Model {
	f := &m.FieldForm
	if !f.Open || f.Action != RestoreDeletedAction || !f.Saving || f.Found != nil || f.Value != msg.Ref {
		return m
	}
	f.Saving = false
	if msg.Err != nil {
		f.Err = msg.Err
		return m
	}
	found := msg.Issue
	found.QualifiedID = m.projectName(found.ProjectID) + "#" + found.ShortID
	f.Found = &found
	return m
}

// issueRestored applies kata's answer to the restore: it closes the form, or keeps it open with kata's refusal, or with the
// word that kata held the Issue as not deleted.
func issueRestored(m Model, msg IssueRestored) (Model, []Cmd) {
	f := &m.FieldForm
	if !f.Open || f.Action != RestoreDeletedAction || f.Found == nil || f.Found.UID != msg.UID {
		return m, nil
	}
	f.Saving = false
	switch {
	case msg.Err != nil:
		f.Err = msg.Err
	case !msg.Changed:
		f.Err = fmt.Errorf("%s is not deleted; kata restored nothing.", f.Found.QualifiedID)
	default:
		qualified := f.Found.QualifiedID
		m.FieldForm = FieldFormState{}
		m.Notice = fmt.Sprintf("Restored %s.", qualified)
		return snapshotChanged(m.issueWritten(msg.UID))
	}
	return m, nil
}
