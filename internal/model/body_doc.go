package model

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// ternDocFloor is the first Tern whose doc element this Pane has seen work: in-place editing, and the Comment bubble on a
// selection. An older Tern draws the Issue body as Markdown and edits it in the overlay.
const ternDocFloor = "v0.9.26"

// TernDrawsDoc reports whether a Tern of this version (as the hello reply names it, "0.9.26") draws a doc. A pre-release of the floor
// is older than the floor; a version that cannot be read is not enough.
func TernDrawsDoc(version string) bool { return TernAtLeast(version, ternDocFloor) }

var picture = regexp.MustCompile(`(?i)<img|!\[[^\]]*\]\(`)

// HasPicture reports whether body holds an HTML <img> tag or a Markdown image. Such a body is drawn as Markdown, not as a doc, which
// draws neither at a picture's size nor opens it large. A picture inside code counts too: that body only keeps the Markdown look.
func HasPicture(body string) bool { return picture.MatchString(body) }

// BodyEdited is the new Markdown source of the doc that shows Issue UID's body: the user edited it in place. The doc may send it after
// the selection moved on to another Issue, so UID, not the Issue shown, says whose it is.
type BodyEdited struct{ UID, Text string }

func (BodyEdited) msg() {}

// BodyEditing is the doc that shows Issue UID's body starting (On) or ending an in-place edit.
type BodyEditing struct {
	UID string
	On  bool
}

func (BodyEditing) msg() {}

// SelectionComment is a comment the user wrote on a selection of the doc that shows Issue UID's body.
type SelectionComment struct{ UID, Quote, Note string }

func (SelectionComment) msg() {}

var (
	atxHeading   = regexp.MustCompile(`^#{1,6}(\s|$)`)
	parentHeader = regexp.MustCompile(`^##\s+Parent\s*$`)
)

// BodyShown is the body the detail draws for issue, and the stored text before it: a leading `## Parent` section that names the
// parent is not drawn, so an edit of what is drawn saves with that section back in front of it.
func (m Model) BodyShown(issue Issue) (shown, hidden string) {
	shown = issue.Body
	if parent, ok := m.DetailParent(); ok {
		shown = withoutParentSection(issue.Body, parent.Ref)
	}
	return shown, issue.Body[:len(issue.Body)-len(shown)]
}

// withoutParentSection drops a leading `## Parent` section (up to the next heading) that names the parent
// by ref (#short or project#short) as a whole word; display only, the stored body is untouched.
func withoutParentSection(body, ref string) string {
	trimmed := strings.TrimLeft(body, "\r\n")
	first, rest, _ := strings.Cut(trimmed, "\n")
	if !parentHeader.MatchString(first) {
		return body
	}
	section, after := rest, ""
	for off, line := 0, rest; ; {
		if atxHeading.MatchString(line) {
			section, after = rest[:off], rest[off:]
			break
		}
		nl := strings.IndexByte(line, '\n')
		if nl < 0 {
			break
		}
		off += nl + 1
		line = rest[off:]
	}
	short := ref[strings.LastIndex(ref, "#")+1:]
	named := slices.ContainsFunc(strings.Fields(section), func(word string) bool {
		word = strings.Trim(word, ".,;:()[]<>\"'`*_")
		return word == short || strings.HasSuffix(word, "#"+short)
	})
	if !named {
		return body
	}
	return strings.TrimLeft(after, "\r\n")
}

// docIssue is the Issue the body's doc shows: the detail's own, which stays on screen while the selection's Issue loads. The doc's
// events belong to it, whatever the selection is.
func (m Model) docIssue() *Issue {
	if detail := m.DetailAuthority(); detail != nil {
		return detail.Issue
	}
	return nil
}

// BodyDoc is what the Issue body's doc draws.
type BodyDoc struct {
	// Text is the stored body, or the kept edit once its save failed or conflicted. While the user edits the doc it is the text the
	// doc held when the edit began: Tern ends an edit when the text changes, and the kept edit changes with every pause in typing.
	Text string
	// Editable is false while the Issue holds a Draft the doc cannot take (one of the overlay, a comment, one whose save is in flight
	// or in conflict): the doc then offers neither an edit nor a comment on a selection.
	Editable bool
	// Err is the failure of a kept doc Draft (an in-place edit or a comment) that no card shows; nil otherwise.
	Err error
	// Conflicted is set when a kept doc Draft (an in-place edit or a comment on a selection) conflicted and no card shows it: the line
	// under the doc says so, since a Notice does not outlive the next key.
	Conflicted bool
	// What names that Draft for the line, "edit" or "comment"; "" when Conflicted is false.
	What string
}

// BodyDoc is the state of issue's body doc.
func (m Model) BodyDoc(issue Issue) BodyDoc {
	draft, held := m.Drafts[issue.UID]
	doc := BodyDoc{Text: m.docText(issue), Editable: !held || draft.DocCanAct()}
	if m.editsDoc(issue.UID) {
		doc.Text = m.docEdit.drawn
	}
	if held && draft.Doc != nil && m.EditorUID != issue.UID {
		doc.Err = draft.Err
		if doc.Conflicted = draft.Conflict != nil; doc.Conflicted {
			doc.What = "edit"
			if draft.Comment {
				doc.What = "comment"
			}
		}
	}
	return doc
}

func (m Model) docText(issue Issue) string {
	body, _ := m.BodyShown(issue)
	if draft, ok := m.Drafts[issue.UID]; ok && draft.InPlaceEdit() && draft.Doc.Shown {
		return draft.DocText()
	}
	return body
}

// startDocEdit notes that the user edits issue in the doc: what the doc draws until the edit ends, and what the edit is written
// against, as the doc showed the Issue now.
func (m Model) startDocEdit(issue Issue) Model {
	_, hidden := m.BodyShown(issue)
	m.docEdit = &docEdit{uid: issue.UID, ref: issue.QualifiedID, drawn: m.docText(issue), hidden: hidden, base: baseOf(issue)}
	return m
}

// bodyEdited holds the doc's new text as a Draft on the revision the edit began on; leaving the doc saves it. A Draft the user
// opened in the overlay is theirs and stays. The text belongs to Issue uid: when the doc has since moved on to another Issue, it is
// the open edit's (the doc sends its last second of typing after the move), never the Issue now shown. A change that comes after the
// edit ended and its Draft is gone (the card's save settled) is lateAfterEnd's.
func bodyEdited(m Model, uid, text string) (Model, []Cmd) {
	m = m.noteDocChange(uid)
	if !m.InPlaceBody {
		return m, nil
	}
	if ended := m.docEnded; ended != nil && ended.uid == uid && !m.editsDoc(uid) {
		if _, held := m.Drafts[uid]; !held {
			return lateAfterEnd(m, *ended, text)
		}
	}
	if issue := m.docIssue(); issue != nil && issue.UID == uid {
		if m.docEdit != nil && !m.editsDoc(uid) { // the edit began on an Issue the doc no longer shows, and left it
			return m, nil
		}
		if !m.BodyDoc(*issue).Editable {
			return m, nil
		}
		if !m.editsDoc(uid) {
			m = m.startDocEdit(*issue)
		}
	} else if !m.editsDoc(uid) {
		return m, nil
	}
	draft, held := m.Drafts[uid]
	hidden, base := m.docEdit.hidden, m.docEdit.base
	if held {
		switch {
		case draft.InPlaceEdit():
			hidden, base = draft.Doc.HiddenPrefix, draft.DraftBase
		case draft.Saving && !draft.Comment && draft.Doc == nil: // the user opened the in-place edit's Draft in the card while it saved: the text still belongs to it
		default: // the overlay's or a comment's: not the doc's to change
			return m, nil
		}
	}
	whole := normalizeLineBreaks(hidden + text)
	if held && draft.Saving { // the save in flight carries the text before this one: this is saved once that save settles
		draft.Late = LateText{Text: whole, Held: true}
		return putDraft(m, uid, draft), nil
	}
	if whole == normalizeLineBreaks(base.BaseText) {
		if held {
			m = dropDraft(m, uid)
		}
		return m, nil
	}
	if !held {
		draft = Draft{DraftBase: base, Doc: &DocPart{HiddenPrefix: normalizeLineBreaks(hidden), Ref: m.docEdit.ref}}
	}
	draft.Editor, draft.Err = NewTextEditor(whole), nil
	return putDraft(m, uid, draft), nil
}

// lateAfterEnd saves the text the doc sent for Issue uid after its edit ended and the save of that edit settled, which no Draft
// holds any more. It goes on the base the edit began on, so a save that a card made meanwhile opens the conflict card (the user sees
// it) instead of being overwritten; an edit whose save the doc settled has had that base moved to the text it saved (docSaved).
func lateAfterEnd(m Model, ended docEdit, text string) (Model, []Cmd) {
	whole := normalizeLineBreaks(ended.hidden + text)
	if whole == normalizeLineBreaks(ended.base.BaseText) {
		return m, nil
	}
	m = putDraft(m, ended.uid, Draft{DraftBase: ended.base, Doc: &DocPart{HiddenPrefix: normalizeLineBreaks(ended.hidden), Ref: ended.ref}, Editor: NewTextEditor(whole)})
	return sendDraftOf(m, ended.uid, false)
}

// withLate gives draft, whose save did not go through, the text the doc sent for it after the save left: the Draft the user keeps is
// what they last typed.
func withLate(draft Draft) Draft {
	if draft.Late.Held {
		draft.Editor = NewTextEditor(draft.Late.Text)
		draft.Late = LateText{}
	}
	return draft
}

// docSaved settles the save of Issue uid's body edit, which sent saved: the text the doc sent after it goes on to a save of its own,
// against the text just saved. The edit still open in the doc, or the one that ended, is now written against that text too, unless
// a card made the save: that text is not the doc's, and a change that comes after it is checked against the base the edit began on.
func (m Model) docSaved(uid string, draft Draft) (Model, []Cmd) {
	saved := draft.Editor.Text
	if m.editsDoc(uid) {
		edit := *m.docEdit
		edit.base.BaseText = saved
		m.docEdit = &edit
	}
	if ended := m.docEnded; ended != nil && ended.uid == uid && draft.Doc != nil {
		edit := *ended
		edit.base.BaseText = saved
		m.docEnded = &edit
	}
	if !draft.Late.Held || draft.Late.Text == saved {
		return m, nil
	}
	next := Draft{DraftBase: draft.DraftBase, Doc: m.followUpDoc(uid, draft), Editor: NewTextEditor(draft.Late.Text)}
	next.BaseText = saved
	m = putDraft(m, uid, next)
	return sendDraftOf(m, uid, false)
}

// followUpDoc is the DocPart of the save that follows the one of draft, whose late text is saved next. A Draft the user opened in the
// card meanwhile has none: the follow-up is the doc's again, so a failure or conflict is announced as any edit of the doc's, naming
// the Issue and keeping the Parent section that Issue's own doc hides (kept with the Draft, not read off whatever detail is shown).
func (m Model) followUpDoc(uid string, draft Draft) *DocPart {
	if draft.Doc != nil {
		return draft.Doc
	}
	if from := draft.FromDoc; from != nil {
		return &DocPart{HiddenPrefix: from.HiddenPrefix, Ref: from.Ref}
	}
	switch issue := m.docIssue(); {
	case m.editsDoc(uid):
		return &DocPart{HiddenPrefix: normalizeLineBreaks(m.docEdit.hidden), Ref: m.docEdit.ref}
	case issue != nil && issue.UID == uid:
		_, hidden := m.BodyShown(*issue)
		return &DocPart{HiddenPrefix: normalizeLineBreaks(hidden), Ref: issue.QualifiedID}
	}
	if issue := m.issueByUID(uid); issue != nil {
		return &DocPart{Ref: issue.QualifiedID}
	}
	return &DocPart{}
}

// deadEditWithin is how soon after an editing on an editing off with no change in between is a dead edit: Tern closed the edit
// itself, which it does for the first double click after the doc's text changed.
const deadEditWithin = 500 * time.Millisecond

// DocGuardWindow is how long keys that would run a binding are swallowed after a dead edit: the user types on as if the edit were open.
const DocGuardWindow = 2 * time.Second

const deadEditNotice = "The body did not open for editing. Double-click it again; keys are ignored for a moment."

// WaitDocGuard is the pause of DocGuardWindow after the key guard Seq began. It reads nothing; its answer DocGuardExpired ends that
// guard, and the Notice that says why keys are ignored, when no input has.
type WaitDocGuard struct{ Seq int }

func (WaitDocGuard) cmd() {}

type DocGuardExpired struct{ Seq int }

func (DocGuardExpired) msg() {}

func docGuardExpired(m Model, msg DocGuardExpired) Model {
	if msg.Seq == m.docGuardSeq {
		return m.EndDocGuard()
	}
	return m
}

// docOpened is an editing on of the doc that has not ended: when it began, whether the text the doc draws differed then from what it
// drew when its last edit session ended, and whether a change came since.
type docOpened struct {
	uid     string
	at      time.Time
	changed bool
	typed   bool
}

// docRedrawn reports whether the doc, which draws Issue uid's body now, draws other text than it did when the last edit session of any
// Issue ended (a save, someone else's edit, another Issue), or before the first one since the Pane opened: Tern keeps that state across nodes, and ends the next edit at once.
func (m Model) docRedrawn(uid string) bool {
	issue := m.docIssue()
	return issue != nil && issue.UID == uid && (m.docRested == nil || m.docText(*issue) != *m.docRested)
}

// restDoc notes the text the doc drew during the edit session that ends now, as the text that session ended on. It is the text the
// doc held when the edit began, not what it draws after the flush: a kept Draft is drawn at once, and that redraw comes after the session.
func (m Model) restDoc(uid string) Model {
	if m.editsDoc(uid) {
		drawn := m.docEdit.drawn
		m.docRested = &drawn
		return m
	}
	if issue := m.docIssue(); issue != nil && issue.UID == uid {
		drawn := m.docText(*issue)
		m.docRested = &drawn
	}
	return m
}

// noteDocEditing keeps track of the doc's editing on and off. A dead edit, which Tern's bug can have caused (the text the doc draws
// changed since its last edit session, and the edit ended within deadEditWithin with no change), starts the key guard: the command
// that ends it comes back. An edit that stayed open past deadEditWithin was alive, so it ends the guard.
func (m Model) noteDocEditing(uid string, on bool) (Model, []Cmd) {
	if on {
		m.docOpened = &docOpened{uid: uid, at: m.Now, changed: m.docRedrawn(uid)}
		return m, nil
	}
	opened := m.docOpened
	if opened == nil || opened.uid != uid {
		return m, nil
	}
	m.docOpened = nil
	switch {
	case m.Now.Sub(opened.at) > deadEditWithin:
		return m.EndDocGuard(), nil
	case !opened.typed && opened.changed:
		m.docGuardSeq++
		m.docGuardUntil = m.Now.Add(DocGuardWindow)
		m.Notice = deadEditNotice
		return m, []Cmd{WaitDocGuard{Seq: m.docGuardSeq}}
	}
	return m, nil
}

// noteDocChange notes that the doc sent a change of Issue uid's body since it began editing; an edit ends the key guard.
func (m Model) noteDocChange(uid string) Model {
	if opened := m.docOpened; opened != nil && opened.uid == uid && !opened.typed {
		typed := *opened
		typed.typed = true
		m.docOpened = &typed
	}
	return m.EndDocGuard()
}

// EndDocGuard ends the key guard, and takes its Notice away: the user knows where they are (the pointer acted, a change came), or the
// window ran out.
func (m Model) EndDocGuard() Model {
	m.docGuardUntil = time.Time{}
	if m.Notice == deadEditNotice {
		m.Notice = ""
	}
	return m
}

// docKeyGuarded reports whether key is one the key guard swallows: a printable key or enter that would run a Pane binding on the
// detail that shows the doc. Escape, the arrows and the like pass, and so does any key a card, the Menu, a picker or a text input
// has, and any key while an edit is open past deadEditWithin (the doc has it).
func (m Model) docKeyGuarded(key KeyMsg) bool {
	if !m.Now.Before(m.docGuardUntil) || !(key.Text != "" || key.Chord == "enter") {
		return false
	}
	if opened := m.docOpened; opened != nil && m.Now.Sub(opened.at) > deadEditWithin {
		return false
	}
	if m.docIssue() == nil {
		return false
	}
	switch m.CurrentMode() {
	case ModeList, ModeDetail, ModeBoard, ModeLane:
		return true
	}
	return false
}

// bodyEditing starts an in-place edit, or saves it when the user leaves the doc, through the write the overlay's Save makes.
func bodyEditing(m Model, uid string, on bool) (Model, []Cmd) {
	m, guard := m.noteDocEditing(uid, on)
	if !on {
		m = m.restDoc(uid)
	}
	m, cmds := applyBodyEditing(m, uid, on)
	return m, append(cmds, guard...)
}

// applyBodyEditing is the doc's editing on or off as an edit of the body: on notes the edit and what it is written against, off
// takes it out of the doc and sends the Draft it left. The edit that ended is kept as docEnded, which a change that comes late
// finds.
func applyBodyEditing(m Model, uid string, on bool) (Model, []Cmd) {
	if !m.InPlaceBody {
		return m, nil
	}
	if on {
		m.docEnded = nil
		issue := m.docIssue()
		if issue == nil || issue.UID != uid { // a doc that is no longer the one shown starts nothing
			return m, nil
		}
		// a doc node removed without an editing off must not leave its edit behind
		if !m.BodyDoc(*issue).Editable { // a save in flight: the edit would begin on a body the save is about to change
			m.docEdit = nil
			return m, nil
		}
		return m.startDocEdit(*issue), nil
	}
	if !m.editsDoc(uid) { // no edit of this doc is open (an editing on the doc ignored, say): nothing was edited, so a Draft held for the Issue is not sent
		return m, nil
	}
	m.docEnded, m.docEdit = m.docEdit, nil
	return flushDocEdit(m, uid)
}

// flushDocEdit sends the in-place edit held for Issue uid, if there is one: leaving the doc does, and so does the doc moving on to
// another Issue while the edit is open.
func flushDocEdit(m Model, uid string) (Model, []Cmd) {
	draft, held := m.Drafts[uid]
	if !held || !draft.DocCanAct() {
		return m, nil
	}
	return sendDraftOf(m, uid, false)
}

// selectionComment writes a comment on the Issue that quotes the selection above the note. The comment is a Draft like any other, so
// a failure keeps it; an Issue with a Draft already holds no second one.
func selectionComment(m Model, msg SelectionComment) (Model, []Cmd) {
	if !m.InPlaceBody {
		return m, nil
	}
	quote, note := strings.TrimSpace(normalizeLineBreaks(msg.Quote)), strings.TrimSpace(normalizeLineBreaks(msg.Note))
	if quote == "" || note == "" {
		return m, nil
	}
	issue := m.docIssue()
	if issue == nil || issue.UID != msg.UID { // the selection moved on while the doc held the note: it is the first Issue's
		if issue = m.issueByUID(msg.UID); issue == nil {
			m, wait := m.raiseToast(Toast{Text: "Your comment on the selection was not sent: its Issue is no longer in the list. Select it and comment again."})
			return m, []Cmd{wait}
		}
	}
	if _, held := m.Drafts[issue.UID]; held {
		m, wait := m.raiseToast(Toast{Text: "This Issue has a Draft open: send or discard it, then comment on the selection again."})
		return m, []Cmd{wait}
	}
	lines := strings.Split(quote, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	text := strings.Join(lines, "\n") + "\n\n" + note
	m = putDraft(m, issue.UID, Draft{Editor: NewTextEditor(text), DraftBase: baseOf(*issue), Comment: true, Doc: &DocPart{Ref: issue.QualifiedID}})
	return sendDraftOf(m, issue.UID, false)
}

// bodySaved shows text as the body of Issue uid until the refresh that follows the write reads it, so the doc does not go back to
// the old text in between.
func (m Model) bodySaved(uid, text string) Model {
	if uid == m.uid && m.authority != nil && m.authority.Issue != nil {
		detail := *m.authority
		issue := *detail.Issue
		issue.Body = text
		detail.Issue = &issue
		m.authority = &detail
	}
	return m
}

// docEdit is an in-place edit that is open in the doc.
type docEdit struct {
	// uid is the Issue whose body the user is editing, and ref its name in a message.
	uid, ref string
	// drawn is the text the doc held when the edit began (BodyDoc's Text), base what the edit is written against, and hidden the
	// stored text in front of drawn.
	drawn, hidden string
	base          DraftBase
}

// editsDoc reports whether an in-place edit of Issue uid is open in the doc.
func (m Model) editsDoc(uid string) bool { return m.docEdit != nil && m.docEdit.uid == uid }
