package model

import (
	"regexp"
	"slices"
	"strings"
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

// BodyEdited is the new Markdown source of the Issue body's doc: the user edited it in place.
type BodyEdited struct{ Text string }

func (BodyEdited) msg() {}

// BodyEditing is the Issue body's doc starting (On) or ending an in-place edit.
type BodyEditing struct{ On bool }

func (BodyEditing) msg() {}

// SelectionComment is a comment the user wrote on a selection of the Issue body's doc.
type SelectionComment struct{ Quote, Note string }

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
	// Err is the failure of a kept in-place edit that no card shows; nil otherwise.
	Err error
}

// BodyDoc is the state of issue's body doc.
func (m Model) BodyDoc(issue Issue) BodyDoc {
	draft, held := m.Drafts[issue.UID]
	doc := BodyDoc{Text: m.docText(issue), Editable: !held || draft.DocCanAct()}
	if m.docEditUID() == issue.UID {
		doc.Text = m.docEdit.drawn
	}
	if held && draft.Doc != nil && m.EditorUID != issue.UID {
		doc.Err = draft.Err
	}
	return doc
}

func (m Model) docText(issue Issue) string {
	body, _ := m.BodyShown(issue)
	if draft, ok := m.Drafts[issue.UID]; ok && draft.InPlaceEdit() && (draft.Err != nil || draft.Conflict != nil) {
		return draft.DocText()
	}
	return body
}

// startDocEdit notes that the user edits issue in the doc: what the doc draws until the edit ends, and what the edit is written
// against, as the doc showed the Issue now.
func (m Model) startDocEdit(issue Issue) Model {
	_, hidden := m.BodyShown(issue)
	m.docEdit = &docEdit{uid: issue.UID, drawn: m.docText(issue), hidden: hidden, base: baseOf(issue)}
	return m
}

// bodyEdited holds the doc's new text as a Draft on the revision the edit began on; leaving the doc saves it. A Draft the user
// opened in the overlay is theirs and stays.
func bodyEdited(m Model, text string) Model {
	issue := m.docIssue()
	if issue == nil || !m.InPlaceBody || (m.docEditUID() != "" && m.docEditUID() != issue.UID) { // the latter: the edit began on an Issue the doc no longer shows, and left it
		return m
	}
	if !m.BodyDoc(*issue).Editable {
		return m
	}
	if m.docEditUID() != issue.UID {
		m = m.startDocEdit(*issue)
	}
	draft, held := m.Drafts[issue.UID]
	hidden, base := m.docEdit.hidden, m.docEdit.base
	if held {
		hidden, base = draft.Doc.HiddenPrefix, draft.DraftBase
	}
	whole := normalizeLineBreaks(hidden + text)
	if whole == normalizeLineBreaks(base.BaseText) {
		if held {
			m = dropDraft(m, issue.UID)
		}
		return m
	}
	if !held {
		draft = Draft{DraftBase: base, Doc: &DocPart{HiddenPrefix: normalizeLineBreaks(hidden), Ref: issue.QualifiedID}}
	}
	draft.Editor, draft.Err = NewTextEditor(whole), nil
	return putDraft(m, issue.UID, draft)
}

// bodyEditing starts an in-place edit, or saves it when the user leaves the doc, through the write the overlay's Save makes.
func bodyEditing(m Model, on bool) (Model, []Cmd) {
	issue := m.docIssue()
	if issue == nil || !m.InPlaceBody {
		return m, nil
	}
	if on { // a doc node removed without an editing off must not leave its edit behind
		if !m.BodyDoc(*issue).Editable { // a save in flight: the edit would begin on a body the save is about to change
			m.docEdit = nil
			return m, nil
		}
		return m.startDocEdit(*issue), nil
	}
	uid := m.docEditUID()
	if uid == "" {
		uid = issue.UID
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
	issue := m.docIssue()
	if issue == nil || !m.InPlaceBody {
		return m, nil
	}
	quote, note := strings.TrimSpace(normalizeLineBreaks(msg.Quote)), strings.TrimSpace(normalizeLineBreaks(msg.Note))
	if quote == "" || note == "" {
		return m, nil
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
	// uid is the Issue whose body the user is editing.
	uid string
	// drawn is the text the doc held when the edit began (BodyDoc's Text), base what the edit is written against, and hidden the
	// stored text in front of drawn.
	drawn, hidden string
	base          DraftBase
}

// docEditUID is the UID of the Issue whose body is edited in the doc, "" when none.
func (m Model) docEditUID() string {
	if m.docEdit == nil {
		return ""
	}
	return m.docEdit.uid
}
