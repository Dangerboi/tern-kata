package model

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// ternDocFloor is the first Tern whose doc element this Pane has seen work: in-place editing, and the Comment bubble on a
// selection. An older Tern draws the Issue body as Markdown and edits it in the overlay.
const ternDocFloor = "v0.9.26"

// TernDrawsDoc reports whether a Tern of this version (as the hello reply names it, "0.9.26") draws a doc. A pre-release of the floor
// is older than the floor; a version that cannot be read is not enough.
func TernDrawsDoc(version string) bool {
	v := "v" + strings.TrimPrefix(strings.TrimSpace(version), "v")
	return semver.IsValid(v) && semver.Compare(v, ternDocFloor) >= 0
}

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

// bodyEdited holds the doc's new text as a Draft on the revision the edit began on; leaving the doc saves it. A Draft the user
// opened in the overlay is theirs and stays.
func bodyEdited(m Model, text string) Model {
	issue := m.shownIssue()
	if issue == nil || !m.InPlaceBody {
		return m
	}
	draft, held := m.Drafts[issue.UID]
	if held && (!draft.FromDoc || draft.Comment || draft.Saving) {
		return m
	}
	_, hidden := m.BodyShown(*issue)
	whole := normalizeLineBreaks(hidden + text)
	if whole == normalizeLineBreaks(issue.Body) {
		if held {
			m = dropDraft(m, issue.UID)
		}
		return m
	}
	if !held {
		draft = Draft{ProjectID: issue.ProjectID, BaseText: issue.Body, BaseRevision: issue.Revision, FromDoc: true, Hidden: normalizeLineBreaks(hidden)}
	}
	draft.Editor = NewTextEditor(whole)
	return putDraft(m, issue.UID, draft)
}

// bodyEditing saves the in-place edit when the user leaves the doc, through the write the overlay's Save makes.
func bodyEditing(m Model, on bool) (Model, []Cmd) {
	issue := m.shownIssue()
	if on || issue == nil || !m.InPlaceBody {
		return m, nil
	}
	draft, held := m.Drafts[issue.UID]
	if !held || !draft.FromDoc || draft.Comment || draft.Saving || draft.Conflict != nil {
		return m, nil
	}
	return sendDraftOf(m, issue.UID, false)
}

// selectionComment writes a comment on the Issue that quotes the selection above the note. The comment is a Draft like any other, so
// a failure keeps it; an Issue with a Draft already holds no second one.
func selectionComment(m Model, msg SelectionComment) (Model, []Cmd) {
	issue := m.shownIssue()
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
	m = putDraft(m, issue.UID, Draft{Editor: NewTextEditor(text), ProjectID: issue.ProjectID, BaseText: issue.Body, BaseRevision: issue.Revision, Comment: true, FromDoc: true})
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
