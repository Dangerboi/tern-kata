package model

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// The context menu of an Attachment row of the Issue detail, and the long press that lists the same. Each item is a key of the
// detail's mode that acts on the row under the cursor; a menu pick moves the cursor to its row and presses that key, so a click and
// the key never disagree.

const (
	OpenAttachmentBesideAction Action = "open-attachment-beside"
	OpenAttachmentSystemAction Action = "open-attachment-system"
	SaveAttachmentAction       Action = "save-attachment"
	CopyAttachmentAction       Action = "copy-attachment"
	UnlinkAttachmentAction     Action = "unlink-attachment"

	ConfirmUnlinkAttachmentAction Action = "confirm-unlink-attachment"
	KeepAttachmentAction          Action = "keep-attachment"
	AttachmentCopyClose           Action = "attachment-copy-close"
)

const (
	// ModeUnlinkAttachment is the question "Unlink <name> from <ref>? y/n".
	ModeUnlinkAttachment Mode = "unlink-attachment"
	// ModeAttachmentCopy is the card with the downloaded file's path and the kata command that fetches it.
	ModeAttachmentCopy Mode = "attachment-copy"
)

// AttachmentWay is what OpenAttachment does with the Attachment once it is downloaded into the cache folder.
type AttachmentWay int

const (
	// WayOpen is what enter and a click do: Tern's preview for the kinds Tern draws, else the OS opener for images, video, audio,
	// PDFs and text.
	WayOpen AttachmentWay = iota
	// WayBeside opens the file in a split beside the Pane, whatever the floating or side panel setting says.
	WayBeside
	// WaySystem hands the file to the OS opener of the machine the Pane runs on, for the types OSOpens names.
	WaySystem
	// WaySave copies the file into the Downloads folder of the machine the Pane runs on, under a name no file there has.
	WaySave
	// WayCopy only downloads, for the Copy card.
	WayCopy
)

// ternDrawnExtensions are the extensions Tern's own file block draws in place beside the Pane: images, plain text, Markdown and JSON.
// Video, audio and PDFs open in a block that only hands them to the OS of the machine showing the window, and a CSV opens a table
// block that takes the focus, so those go to the OS opener of the session host.
var ternDrawnExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".txt", ".md", ".json"}

// TernDraws reports whether Tern's file block draws a file called name itself.
func TernDraws(name string) bool {
	return slices.Contains(ternDrawnExtensions, strings.ToLower(filepath.Ext(name)))
}

// osOpenedExtensions are the only extensions tern-kata hands to the OS opener, by enter, a click or Open with the system app: images,
// video, audio, PDFs and plain text. Anything else is only saved, because what the OS does with a file it knows (run it, follow a
// link inside it, launch a handler registered for it) cannot be told from the name alone, and anyone who can write the Project,
// agents included, can name an Attachment.
var osOpenedExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic", ".tif", ".tiff",
	".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v",
	".mp3", ".wav", ".ogg", ".flac", ".m4a",
	".pdf",
	".txt", ".md", ".log", ".csv", ".json",
}

// OSOpens reports whether tern-kata hands a file called name to the OS opener. The name is not trimmed: Windows reads `x.png.` and
// `x.png ` as other names than x.png, so they must not count as images.
func OSOpens(name string) bool {
	return slices.Contains(osOpenedExtensions, strings.ToLower(filepath.Ext(name)))
}

// verb is how a failure of way names what it was doing.
func (w AttachmentWay) verb() string {
	switch w {
	case WaySave:
		return "save"
	case WayCopy:
		return "copy"
	}
	return "open"
}

// AttachmentRefusal is why way cannot run on the Attachment a, or "" when it can: the context menu disables its row with it and
// the key says it as a Notice. Only WayBeside and WaySystem refuse.
func AttachmentRefusal(a Attachment, way AttachmentWay) string {
	if way == WayBeside && !TernDraws(a.Name) {
		return "Tern draws only images, text, Markdown and JSON beside the Pane; open this one another way."
	}
	if way == WaySystem && !OSOpens(a.Name) {
		return "tern-kata hands only images, video, audio, PDFs and text to the system app, since the OS may run anything else; save it (s) and open it yourself."
	}
	return ""
}

// AttachmentMenuPickMsg is a pick from the context menu of the Attachment row with Key: the detail's cursor moves to it, then Chord
// is pressed in the detail's mode.
type AttachmentMenuPickMsg struct{ Key, Chord string }

func (AttachmentMenuPickMsg) msg() {}

// UnlinkAsk is the unlink question waiting for its answer: the Attachment and the Issue it leaves, named as the question says.
type UnlinkAsk struct {
	UID, Ref   string
	ProjectID  int64
	Attachment Attachment
}

// AttachmentCopyState is the Copy card: the downloaded file's path, and the command that fetches the Attachment from kata, which
// each have a copy button.
type AttachmentCopyState struct {
	Open                bool
	Name, Path, Command string
}

// attachmentUnderCursor is the Attachment of the detail's cursor row, with the Issue that holds it.
func (m Model) attachmentUnderCursor() (Issue, Attachment, bool) {
	m = m.withRows()
	row, ok := m.cursorRow()
	if !ok || row.Kind != RowAttachment || m.authority == nil || m.authority.Issue == nil {
		return Issue{}, Attachment{}, false
	}
	return *m.authority.Issue, row.Activity.File, true
}

func (m Model) attachmentOnCursor() bool {
	_, _, ok := m.attachmentUnderCursor()
	return ok
}

// openAttachmentAs is the key of an open way: it downloads the Attachment under the cursor and then acts as way says.
func openAttachmentAs(way AttachmentWay) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		issue, a, _ := m.attachmentUnderCursor()
		if reason := AttachmentRefusal(a, way); reason != "" {
			m.Notice = reason
			return m, nil
		}
		return m, []Cmd{OpenAttachment{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Attachment: a, Way: way}}
	}
}

func askUnlinkAttachment(m Model) (Model, []Cmd) {
	issue, a, _ := m.attachmentUnderCursor()
	if m.Stale {
		m.Notice = "Daemon disconnected; reconnect before unlinking an attachment."
		return m, nil
	}
	m.AskUnlink = &UnlinkAsk{UID: issue.UID, Ref: issue.Qualified(), ProjectID: issue.ProjectID, Attachment: a}
	return m, nil
}

func confirmUnlinkAttachment(m Model) (Model, []Cmd) {
	ask := m.AskUnlink
	m.AskUnlink = nil
	if ask == nil {
		return m, nil
	}
	return m, []Cmd{UnlinkAttachment{Workspace: m.Workspace, UID: ask.UID, ProjectID: ask.ProjectID, Actor: m.Actor, ID: ask.Attachment.ID, Name: ask.Attachment.Name, FromQuestion: true}}
}

// attachmentMenuPick moves the cursor to the Attachment row the menu was built for and presses its key.
func attachmentMenuPick(m Model, msg AttachmentMenuPickMsg) (Model, []Cmd) {
	if m.covered() {
		return m, nil
	}
	m = m.chooseRow(TabAttachments, msg.Key)
	if row, ok := m.cursorRow(); !ok || row.Kind != RowAttachment || row.Key != msg.Key {
		return m, nil
	}
	m.Notice = ""
	next, cmds, _ := runChord(m, msg.Chord, ModeDetail)
	return next, cmds
}

// AttachmentCopyCommand is the shell command that downloads the Attachment into the current directory.
func AttachmentCopyCommand(issueRef, attachmentID string) string {
	return "kata docbank attachments get " + issueRef + " " + attachmentID
}

// attachmentOpened takes the answer of OpenAttachment. A failure and each way's success are said as a Notice, a toast for a save,
// or the Copy card.
func attachmentOpened(m Model, msg AttachmentOpened) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot %s %s: %v", msg.Way.verb(), msg.Name, msg.Err)
		return m, nil
	}
	switch msg.Way {
	case WaySave:
		m, wait := m.raiseToast(Toast{Text: "Saved " + msg.Path})
		return m, []Cmd{wait}
	case WayCopy:
		issue := m.issueByUID(msg.UID)
		if m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == msg.UID {
			issue = m.authority.Issue
		}
		if issue == nil {
			m.Notice = "That Issue is no longer listed."
			return m, nil
		}
		m.HelpOpen = false
		m.AttachmentCopy = AttachmentCopyState{Open: true, Name: msg.Name, Path: msg.Path, Command: AttachmentCopyCommand(issue.Qualified(), msg.AttachmentID)}
		return m, nil
	case WayBeside:
		m.Notice = fmt.Sprintf("Opened %s beside the Pane (%s)", msg.Name, msg.Path)
	case WaySystem:
		m.Notice = fmt.Sprintf("Opened %s with the system app (%s)", msg.Name, msg.Path)
	default:
		if msg.OutsideTern {
			m.Notice = fmt.Sprintf("Opened %s outside Tern (%s)", msg.Name, msg.Path)
		} else {
			m.Notice = fmt.Sprintf("Opened %s (%s)", msg.Name, msg.Path)
		}
	}
	return m, nil
}

// attachmentCopyKey is a key of the Copy card: esc closes it.
func attachmentCopyKey(m Model, msg KeyMsg) (Model, []Cmd) {
	if binding, ok := LookupBindingForMode(msg.Chord, ModeAttachmentCopy); ok && binding.Action == AttachmentCopyClose {
		m.AttachmentCopy = AttachmentCopyState{}
	}
	return m, nil
}

// attachmentUnlinked takes the answer of an unlink. The U card's own write goes through its form; one the question sent has no form,
// so it reports and reads the Issue again itself. The two can be in flight together, so the answer says which one it is.
func attachmentUnlinked(m Model, msg AttachmentUnlinked) (Model, []Cmd) {
	report := ""
	if issue := m.issueByUID(msg.UID); issue != nil {
		report = fmt.Sprintf("Unlinked %s from %s; its bytes stay in Docbank", msg.Name, issue.ShortID)
	}
	if !msg.FromQuestion {
		return fieldWritten(m, msg.UID, report, msg.Err)
	}
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot unlink %s: %v", msg.Name, msg.Err)
		return m, nil
	}
	m.Notice = report
	return snapshotChanged(m.issueWritten(msg.UID))
}
