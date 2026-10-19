package model

import (
	"errors"
	"fmt"
	"io/fs"
)

// AttachFile copies the file at Path into the Project's Attachment store and points an attachment.<name> meta key of the Issue
// Ref names at the copy. Ref is anything kata accepts: a short id in the workspace's Binding, <project>#<short id> or a ULID.
type AttachFile struct{ Workspace, Ref, Actor, Path string }

func (AttachFile) cmd() {}

// FileAttached answers AttachFile with the stored name and the absolute native path of the copy.
type FileAttached struct {
	Ref, Name, Path string
	Err             error
}

func (FileAttached) msg() {}

// DetachFile clears the attachment.<Name> meta key of the Issue Ref names and deletes the copy it pointed to when the copy is on this machine.
type DetachFile struct{ Workspace, Ref, Actor, Name string }

func (DetachFile) cmd() {}

// FileFate is what Detach did to the file an Attachment pointed to.
type FileFate string

const (
	// FileDeleted: the copy on this machine was deleted.
	FileDeleted FileFate = "deleted"
	// FileLeft: no file was deleted: the copy is on another host, outside this Issue's folder of the Attachment store, or the pointer named none.
	FileLeft FileFate = "left"
	// FileGone: the copy was already missing on this machine.
	FileGone FileFate = "gone"
)

// FileDetached answers DetachFile. Cleared says whether the pointer was cleared, whatever Err is: once it is, a later failure
// still reports Host, Path and Fate, the file's fate being FileLeft when it could not be deleted.
type FileDetached struct {
	Ref, Name, Host, Path string
	Fate                  FileFate
	Cleared               bool
	Err                   error
}

func (FileDetached) msg() {}

// Report is the sentence the Pane and the command line show for a detach that cleared the pointer.
func (d FileDetached) Report() string {
	if d.Err != nil {
		reason := d.Err
		if pathErr, ok := errors.AsType[*fs.PathError](d.Err); ok {
			reason = pathErr.Err
		}
		return fmt.Sprintf("Removed %s; could not delete %s: %v", d.Name, d.Path, reason)
	}
	switch d.Fate {
	case FileDeleted:
		return fmt.Sprintf("Removed %s and deleted %s", d.Name, d.Path)
	case FileGone:
		return fmt.Sprintf("Removed %s; the file %s was already gone", d.Name, d.Path)
	}
	if d.Host == "" && d.Path == "" {
		return fmt.Sprintf("Removed %s; it named no readable file, so no file was touched", d.Name)
	}
	return fmt.Sprintf("Removed %s; the file was left on %s at %s", d.Name, d.Host, d.Path)
}

// CanAttach is whether a workspace bound to binding may attach to or detach from an Issue of issueProject: only the
// Binding's own Project holds attachments.
func CanAttach(binding Project, issueProject int64) bool {
	return binding.UID != "" && binding.ID == issueProject
}

// AttachRefusal is why a workspace bound to binding cannot attach to or detach from the Issue shortID of Project
// issueProject, owner being that Project's name, or nil when it can. The Pane and the gateway both word the refusal here.
func AttachRefusal(binding Project, issueProject int64, shortID, owner string) error {
	if CanAttach(binding, issueProject) {
		return nil
	}
	if binding.UID == "" {
		return fmt.Errorf("This workspace is not bound to a kata Project, so it cannot hold attachments. To change the attachments of %s, work from a workspace bound to Project %q.", shortID, owner)
	}
	return fmt.Errorf("%s belongs to Project %q, but this workspace is bound to %q. To change its attachments, work from a workspace bound to %q.", shortID, owner, binding.Name, owner)
}

// NoAttachmentError is the refusal to detach a name the Issue shortID does not have.
func NoAttachmentError(shortID, name string) error {
	return fmt.Errorf("%s has no attachment named %q.", shortID, name)
}

// AttachmentChangedError is the refusal to detach a name whose pointer changed after it was read.
func AttachmentChangedError(shortID, name string) error {
	return fmt.Errorf("The attachment %q of %s changed since it was read, so nothing was removed.", name, shortID)
}
