package model

import "fmt"

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
	// FileLeft: the copy was not deleted because it is on another host, or outside an Attachment store.
	FileLeft FileFate = "left"
	// FileGone: the copy was already missing on this machine.
	FileGone FileFate = "gone"
)

// FileDetached answers DetachFile. The pointer is cleared whenever Err is nil; Fate says what became of the file at Path on Host.
type FileDetached struct {
	Ref, Name, Host, Path string
	Fate                  FileFate
	Err                   error
}

func (FileDetached) msg() {}

// Report is the sentence the Pane and the command line show for a successful detach.
func (d FileDetached) Report() string {
	switch d.Fate {
	case FileDeleted:
		return fmt.Sprintf("Removed %s and deleted %s", d.Name, d.Path)
	case FileGone:
		return fmt.Sprintf("Removed %s; the file %s was already gone", d.Name, d.Path)
	}
	return fmt.Sprintf("Removed %s; the file was left on %s at %s", d.Name, d.Host, d.Path)
}
