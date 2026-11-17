package model

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// The context menu of an Attachment row of the Issue detail, and the long press that lists the same. Each item is a key of the
// detail's mode that acts on the row under the cursor; a menu pick moves the cursor to its row and presses that key, so a click and
// the key never disagree. attachmentActions is the one table that the keys, the menu rows, the refusals and the failure words read.

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
	// WaySave copies the file into the Downloads folder, under a name no file there has: the folder of the machine that shows the
	// window when the Pane runs in Tern, else of the machine the Pane runs on.
	WaySave
	// WayCopy only downloads, for the Copy card.
	WayCopy
)

// fileKind is a named group of extensions, so the text that says what a refusal allows is built from the same lists that decide it.
type fileKind struct {
	name string
	exts []string
	// since is the first Tern release that draws the kind; "" for a kind every Tern draws. Only ternDrawnKinds sets it.
	since string
}

func hasKind(kinds []fileKind, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return slices.ContainsFunc(kinds, func(k fileKind) bool { return slices.Contains(k.exts, ext) })
}

// kindNames says the kinds as a list: "a, b and c".
func kindNames(kinds []fileKind) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.name
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// ternDrawnKinds are the kinds Tern's own file block shows in place beside the Pane. Tern 0.9.26 plays video and audio and shows
// PDFs there; an older Tern hands them to the OS of the machine showing the window, so a kind with a since release goes to the OS
// opener of the session host on an older Tern or one whose version cannot be read (ternCannotDraw). A CSV opens a table block that
// takes the focus, so it goes to the OS opener too, and so do .avi and .mkv, which Tern does not play. Known limit: a Windows window
// with Tern-managed ABKit installed cannot play media in the preview and shows a card that says it plays in the default app, and
// `tern open --preview` still succeeds there, so tern-kata cannot tell; Open with the system app (o) opens it.
var ternDrawnKinds = []fileKind{
	{name: "images", exts: []string{".png", ".jpg", ".jpeg", ".gif"}},
	{name: "text", exts: []string{".txt"}},
	{name: "Markdown", exts: []string{".md"}},
	{name: "JSON", exts: []string{".json"}},
	{name: "Word documents", exts: []string{".docx"}, since: "v0.9.21"},
	{name: "video", exts: []string{".mp4", ".mov", ".m4v", ".webm"}, since: "v0.9.26"},
	{name: "audio", exts: []string{".mp3", ".wav", ".ogg", ".flac", ".m4a"}, since: "v0.9.26"},
	{name: "PDFs", exts: []string{".pdf"}, since: "v0.9.26"},
}

// TernDraws reports whether Tern's file block draws a file called name itself. A kind listed here but not in osOpenedKinds (a
// Word document) is shown only by Tern and is never handed to the OS; a video, audio or PDF is in both.
func TernDraws(name string) bool { return hasKind(ternDrawnKinds, name) }

// TernNeeds says which Tern release first draws a file called name: the name of its kind, as the text of a refusal says it, and
// the release (a semver with a leading v). since is "" when every Tern that draws files draws it, or name is no drawn kind.
func TernNeeds(name string) (kind, since string) {
	ext := strings.ToLower(filepath.Ext(name))
	for _, k := range ternDrawnKinds {
		if slices.Contains(k.exts, ext) {
			return k.name, k.since
		}
	}
	return "", ""
}

// osOpenedKinds are the only kinds tern-kata hands to the OS opener, by enter, a click or Open with the system app. Anything else
// is only saved, because what the OS does with a file it knows (run it, follow a link inside it, launch a handler registered for
// it) cannot be told from the name alone, and anyone who can write the Project, agents included, can name an Attachment.
var osOpenedKinds = []fileKind{
	{name: "images", exts: []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic", ".tif", ".tiff"}},
	{name: "video", exts: []string{".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v"}},
	{name: "audio", exts: []string{".mp3", ".wav", ".ogg", ".flac", ".m4a"}},
	{name: "PDFs", exts: []string{".pdf"}},
	{name: "text", exts: []string{".txt", ".md", ".log", ".csv", ".json"}},
}

// OSOpens reports whether tern-kata hands a file called name to the OS opener. The name is not trimmed: Windows reads `x.png.` and
// `x.png ` as other names than x.png, so they must not count as images. A name with a colon is never handed over: on NTFS
// `x.txt:name.png` is a stream of x.txt.
func OSOpens(name string) bool {
	return !strings.Contains(name, ":") && hasKind(osOpenedKinds, name)
}

// wayUnlink is the table row that is no way of OpenAttachment: unlinking asks a question first.
const wayUnlink AttachmentWay = -1

// AttachmentAction is one thing the Attachment row's keys and context menu do with an Attachment.
type AttachmentAction struct {
	// Action is the key of the detail's mode.
	Action Action
	// Label is the menu item's text.
	Label string
	way   AttachmentWay
	// verb says what a failure of the way was doing.
	verb    string
	refusal func(Attachment) string
}

// Refusal is why the action cannot run on a, or "".
func (r AttachmentAction) Refusal(a Attachment) string {
	if r.refusal == nil {
		return ""
	}
	return r.refusal(a)
}

var attachmentActions = []AttachmentAction{
	{Action: DrillIn, Label: "Open", way: WayOpen, verb: "open"},
	{Action: OpenAttachmentBesideAction, Label: "Open beside", way: WayBeside, verb: "open", refusal: func(a Attachment) string {
		if TernDraws(a.Name) {
			return ""
		}
		return "Tern draws only " + kindNames(ternDrawnKinds) + " beside the Pane; open this one another way."
	}},
	{Action: OpenAttachmentSystemAction, Label: "Open with the system app", way: WaySystem, verb: "open", refusal: func(a Attachment) string {
		if OSOpens(a.Name) {
			return ""
		}
		return "tern-kata hands only " + kindNames(osOpenedKinds) + " to the system app, since the OS may run anything else; save it (s) and open it yourself."
	}},
	{Action: SaveAttachmentAction, Label: "Save to Downloads", way: WaySave, verb: "save"},
	{Action: CopyAttachmentAction, Label: "Copy…", way: WayCopy, verb: "copy"},
	{Action: UnlinkAttachmentAction, Label: "Unlink…", way: wayUnlink},
}

// AttachmentActions are the Attachment row's actions in menu order. The slice is shared: callers do not change it.
func AttachmentActions() []AttachmentAction { return attachmentActions }

func attachmentActionOf(way AttachmentWay) AttachmentAction {
	return attachmentActions[slices.IndexFunc(attachmentActions, func(r AttachmentAction) bool { return r.way == way })]
}

// attachmentHandler is the key handler of the table's row for action.
func attachmentHandler(action Action) func(Model) (Model, []Cmd) {
	row := attachmentActions[slices.IndexFunc(attachmentActions, func(r AttachmentAction) bool { return r.Action == action })]
	if row.way == wayUnlink {
		return askUnlinkAttachment
	}
	return openAttachmentAs(row.way)
}

// AttachmentRefusal is why way cannot run on the Attachment a, or "" when it can: the context menu disables its row with it and
// the key says it as a Notice.
func AttachmentRefusal(a Attachment, way AttachmentWay) string {
	return attachmentActionOf(way).Refusal(a)
}

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
	issue := m.shownIssue()
	if !ok || row.Kind != RowAttachment || issue == nil {
		return Issue{}, Attachment{}, false
	}
	return *issue, row.Activity.File, true
}

func (m Model) attachmentOnCursor() bool {
	_, _, ok := m.attachmentUnderCursor()
	return ok
}

// openAttachmentAs is the key of an open way: it downloads the Attachment under the cursor and then acts as way says.
func openAttachmentAs(way AttachmentWay) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		issue, a, ok := m.attachmentUnderCursor()
		if !ok {
			return m, nil
		}
		if reason := AttachmentRefusal(a, way); reason != "" {
			m.Notice = reason
			return m, nil
		}
		return m, []Cmd{OpenAttachment{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Attachment: a, Way: way}}
	}
}

func askUnlinkAttachment(m Model) (Model, []Cmd) {
	issue, a, ok := m.attachmentUnderCursor()
	if !ok {
		return m, nil
	}
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

// attachmentMenuPick is a ContextMenuPickMsg from the menu of the Attachment row with msg.Row: the detail's cursor moves to that
// row of the shown Issue, which gives the detail the focus as a click on the row does, and the key is pressed there.
func attachmentMenuPick(m Model, msg ContextMenuPickMsg) (Model, []Cmd) {
	if issue := m.shownIssue(); m.covered() || issue == nil || issue.UID != msg.UID {
		return m, nil
	}
	next := m.chooseRow(TabAttachments, msg.Row)
	if row, ok := next.cursorRow(); !ok || row.Kind != RowAttachment || row.Key != msg.Row {
		return m, nil
	}
	m = next
	m.Notice = ""
	next, cmds, _ := runChord(m, msg.Chord, msg.Mode)
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
		m.Notice = fmt.Sprintf("Cannot %s %s: %v", attachmentActionOf(msg.Way).verb, msg.Name, msg.Err)
		return m, nil
	}
	switch msg.Way {
	case WaySave:
		text := "Saved " + msg.Path
		if msg.Remote {
			text += " on the computer showing this window"
		}
		if msg.Note != "" {
			text += " on the Pane's computer: " + msg.Note
		}
		m, wait := m.raiseToast(Toast{Text: text})
		return m, []Cmd{wait}
	case WayCopy:
		issue := m.issueByUID(msg.UID)
		if issue == nil {
			m.Notice = "That Issue is no longer listed."
			return m, nil
		}
		copied := AttachmentCopyState{Open: true, Name: msg.Name, Path: msg.Path, Command: AttachmentCopyCommand(issue.Qualified(), msg.AttachmentID)}
		if m.covered() {
			m, wait := m.raiseToast(Toast{Text: fmt.Sprintf("%s is at %s; fetch it with: %s", msg.Name, copied.Path, copied.Command)})
			return m, []Cmd{wait}
		}
		m.AttachmentCopy = copied
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
		if msg.Note != "" {
			m.Notice += ": " + msg.Note
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
