package model

import (
	"fmt"
	"strings"
)

// AttachFile uploads the file at Path through kata's attachment API to the Issue UID of ProjectID. Path is as typed: quoted, starting
// with ~, or relative to Workspace.
type AttachFile struct {
	Workspace, UID, Actor, Path string
	ProjectID                   int64
}

func (AttachFile) cmd() {}

type FileAttached struct {
	UID, ID, Name string
	Err           error
}

func (FileAttached) msg() {}

// UnlinkAttachment unlinks the Attachment ID from the Issue UID of ProjectID by clearing its meta key. The file's bytes stay in Docbank:
// kata has no call that deletes them. Name is for the report. FromQuestion is set when the unlink question sent it rather than the U
// card, and the answer repeats it.
type UnlinkAttachment struct {
	Workspace, UID, Actor, ID, Name string
	ProjectID                       int64
	FromQuestion                    bool
}

func (UnlinkAttachment) cmd() {}

type AttachmentUnlinked struct {
	UID, Name    string
	FromQuestion bool
	Err          error
}

func (AttachmentUnlinked) msg() {}

// OpenAttachment downloads the Attachment to the cache folder, unless the folder already holds it, and then does what Way says. By
// default it opens it: in Tern's preview block when the Pane runs in Tern and Tern previews its kind, else with the program the OS
// chooses for its type when it is an image, video, audio, PDF or text file.
type OpenAttachment struct {
	Workspace, UID string
	ProjectID      int64
	Attachment     Attachment
	Way            AttachmentWay
}

func (OpenAttachment) cmd() {}

type AttachmentOpened struct {
	UID, AttachmentID, Name string
	// Path is where the file is: in the cache folder, or for WaySave the copy in Downloads.
	Path string
	Way  AttachmentWay
	Err  error
	// OutsideTern is set when the file went to the OS opener rather than Tern's preview block.
	OutsideTern bool
}

func (AttachmentOpened) msg() {}

// MigrateAttachments uploads every file that an old attachment key (tern-kata.attachment.<name>, or from 0.3 attachment.<name>,
// holding {host, path}) of the workspace's Project points to on this machine, replaces the key with a native Attachment and
// deletes the local copy. Keys that point to another machine, or to a file that is not there, are left and listed.
type MigrateAttachments struct{ Workspace, Actor string }

func (MigrateAttachments) cmd() {}

// MigratedAttachment is one old Attachment key the migration looked at. ID is the new Attachment's id when it moved; Reason says
// why a key was left, and Failed whether that is a failure rather than a file for another machine to move.
type MigratedAttachment struct {
	ShortID, Name, ID, Host, Path, Reason string
	Failed                                bool
}

// AttachmentsMigrated answers MigrateAttachments. Err is set when the migration could not start or when a key failed to move.
type AttachmentsMigrated struct {
	Moved, Left []MigratedAttachment
	Err         error
}

func (AttachmentsMigrated) msg() {}

func (a AttachmentsMigrated) Report() string {
	var b strings.Builder
	if len(a.Moved) == 0 && len(a.Left) == 0 {
		return "No old attachment keys to move."
	}
	fmt.Fprintf(&b, "Moved %d attachment(s) to Docbank.", len(a.Moved))
	for _, m := range a.Moved {
		fmt.Fprintf(&b, "\n  %s  %s -> %s", m.ShortID, m.Name, m.ID)
	}
	if len(a.Left) > 0 {
		fmt.Fprintf(&b, "\nLeft %d attachment(s) where they are:", len(a.Left))
		for _, l := range a.Left {
			fmt.Fprintf(&b, "\n  %s  %s: %s", l.ShortID, l.Name, l.Reason)
		}
	}
	return b.String()
}
