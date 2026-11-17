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
// the open edit's (the doc sends its last second of typing after the move), never the Issue now shown.
func bodyEdited(m Model, uid, text string) Model {
	m = m.noteDocChange(uid)
	if !m.InPlaceBody {
		return m
	}
	if issue := m.docIssue(); issue != nil && issue.UID == uid {
		if m.docEdit != nil && !m.editsDoc(uid) { // the edit began on an Issue the doc no longer shows, and left it
			return m
		}
		if !m.BodyDoc(*issue).Editable {
			return m
		}
		if !m.editsDoc(uid) {
			m = m.startDocEdit(*issue)
		}
	} else if !m.editsDoc(uid) {
		return m
	}
	draft, held := m.Drafts[uid]
	hidden, base := m.docEdit.hidden, m.docEdit.base
	if held {
		switch {
		case draft.InPlaceEdit():
			hidden, base = draft.Doc.HiddenPrefix, draft.DraftBase
		case draft.Saving && !draft.Comment && draft.Doc == nil: // the user opened the in-place edit's Draft in the card while it saved: the text still belongs to it
		default: // the overlay's or a comment's: not the doc's to change
			return m
		}
	}
	whole := normalizeLineBreaks(hidden + text)
	if held && draft.Saving { // the save in flight carries the text before this one: this is saved once that save settles
		draft.Late = &whole
		return putDraft(m, uid, draft)
	}
	if whole == normalizeLineBreaks(base.BaseText) {
		if held {
			m = dropDraft(m, uid)
		}
		return m
	}
	if !held {
		draft = Draft{DraftBase: base, Doc: &DocPart{HiddenPrefix: normalizeLineBreaks(hidden), Ref: m.docEdit.ref}}
	}
	draft.Editor, draft.Err = NewTextEditor(whole), nil
	return putDraft(m, uid, draft)
}

// withLate gives draft, whose save did not go through, the text the doc sent for it after the save left: the Draft the user keeps is
// what they last typed.
func withLate(draft Draft) Draft {
	if draft.Late != nil {
		draft.Editor = NewTextEditor(*draft.Late)
		draft.Late = nil
	}
	return draft
}

// docSaved settles the save of Issue uid's body edit, which sent saved: the text the doc sent after it goes on to a save of its own,
// against the text just saved. The edit still open in the doc is now written against that text too.
func (m Model) docSaved(uid string, draft Draft) (Model, []Cmd) {
	saved := draft.Editor.Text
	if m.editsDoc(uid) {
		edit := *m.docEdit
		edit.base.BaseText = saved
		m.docEdit = &edit
	}
	if draft.Late == nil || *draft.Late == saved {
		return m, nil
	}
	doc := draft.Doc
	if doc == nil { // the user opened the Draft in the card meanwhile: the follow-up is the doc's again, so a failure or conflict is announced as any edit of the doc's
		doc = &DocPart{}
		if m.editsDoc(uid) {
			doc.HiddenPrefix, doc.Ref = m.docEdit.hidden, m.docEdit.ref
		} else if issue := m.issueByUID(uid); issue != nil {
			_, hidden := m.BodyShown(*issue)
			doc.HiddenPrefix, doc.Ref = normalizeLineBreaks(hidden), issue.QualifiedID
		}
	}
	next := Draft{DraftBase: draft.DraftBase, Doc: doc, Editor: NewTextEditor(*draft.Late)}
	next.BaseText = saved
	m = putDraft(m, uid, next)
	return sendDraftOf(m, uid, false)
}

// deadEditWithin is how soon after an editing on an editing off with no change in between is a dead edit: Tern closed the edit
// itself, which it does for the first double click after the doc's text changed.
const deadEditWithin = 500 * time.Millisecond

// deadEditGuard is how long keys that would run a binding are swallowed after a dead edit: the user types on as if the edit were open.
const deadEditGuard = 2 * time.Second

const deadEditNotice = "The body did not open for editing. Double-click it again; keys are ignored for a moment."

// docOpened is an editing on of the doc that has not ended: when it began, and whether a change came since.
type docOpened struct {
	uid   string
	at    time.Time
	typed bool
}

// noteDocEditing keeps track of the doc's editing on and off, and starts the key guard on a dead edit.
func (m Model) noteDocEditing(uid string, on bool) Model {
	if on {
		m.docOpened = &docOpened{uid: uid, at: m.Now}
		return m
	}
	opened := m.docOpened
	if opened == nil || opened.uid != uid {
		return m
	}
	m.docOpened = nil
	if !opened.typed && m.Now.Sub(opened.at) <= deadEditWithin {
		m.docGuardUntil = m.Now.Add(deadEditGuard)
		m.Notice = deadEditNotice
	}
	return m
}

// noteDocChange notes that the doc sent a change of Issue uid's body since it began editing.
func (m Model) noteDocChange(uid string) Model {
	if opened := m.docOpened; opened != nil && opened.uid == uid && !opened.typed {
		typed := *opened
		typed.typed = true
		m.docOpened = &typed
	}
	return m
}

// EndDocGuard ends the key guard: the pointer acted, so the user knows where they are.
func (m Model) EndDocGuard() Model {
	m.docGuardUntil = time.Time{}
	return m
}

// docKeyGuarded reports whether key is one the key guard swallows: a printable key or enter, which would run a binding. Escape, the
// arrows and the like pass.
func (m Model) docKeyGuarded(key KeyMsg) bool {
	return m.Now.Before(m.docGuardUntil) && (key.Text != "" || key.Chord == "enter")
}

// bodyEditing starts an in-place edit, or saves it when the user leaves the doc, through the write the overlay's Save makes.
func bodyEditing(m Model, uid string, on bool) (Model, []Cmd) {
	return applyBodyEditing(m.noteDocEditing(uid, on), uid, on)
}

func applyBodyEditing(m Model, uid string, on bool) (Model, []Cmd) {
	if !m.InPlaceBody {
		return m, nil
	}
	if on {
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
	m.docEdit = nil
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
