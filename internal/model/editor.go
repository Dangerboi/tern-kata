package model

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var errDraftReadOnly = errors.New("Read-only: reconnecting to kata; Draft kept")

// DraftKeptHint is what follows the error of a failed save of a doc edit or comment: that the Draft is kept (unless the error says so
// itself) and how to open it. where is the phrase that places the Issue when the user may be on another one (" on that Issue"), "" when
// they are on it.
func DraftKeptHint(err error, where string) string {
	if errors.Is(err, errDraftReadOnly) { // its text already says the Draft is kept
		return ". Press " + openPhrase(where) + " it."
	}
	return ". The Draft is kept: press " + openPhrase(where) + " it."
}

// ConflictSentence says that what (a Draft's "edit of work#1", "comment on the Issue") conflicts with a newer change, and how to open
// the conflict card; where is as in DraftKeptHint. The Notice and the line under the body say it the same way.
func ConflictSentence(what, where string) string {
	return "Your " + what + " conflicts with a newer change. Press " + openPhrase(where) + " the conflict card."
}

// openPhrase is the phrase "e or c to open" (with where after the keys), which leads to what the keys open.
func openPhrase(where string) string { return "e or c" + where + " to open" }

type Draft struct {
	Editor TextEditor
	DraftBase
	Comment, Saving, DiscardConfirm bool
	Conflict                        *Issue
	Err                             error
	ReplyTo, ReplyAuthor            string
	ReplyKind                       ReplyKind
	// Doc is set on a Draft the Issue body's doc made, which the overlay does not show: an in-place edit, or a comment on a
	// selection. Opening the Draft in the overlay makes it an ordinary one.
	Doc *DocPart
	Keyed
}

// DocPart is what only a Draft made in the Issue body's doc has.
type DocPart struct {
	// HiddenPrefix is the stored text in front of what the doc draws (a Parent section it leaves out).
	HiddenPrefix string
	// Ref names the Issue in a message raised when the Issue is no longer the one shown.
	Ref string
	// Shown is set once the Draft's save failed or conflicted: the doc then draws the Draft's text, and keeps drawing it while a retry
	// or an overwrite saves, however the failure is cleared.
	Shown bool
}

// DraftBase is what a Draft was begun from: the Issue's Project, its body and its revision then. A save is written against it.
type DraftBase struct {
	BaseText                string
	ProjectID, BaseRevision int64
}

func baseOf(issue Issue) DraftBase {
	return DraftBase{BaseText: issue.Body, ProjectID: issue.ProjectID, BaseRevision: issue.Revision}
}

// InPlaceEdit reports whether the Draft is an edit of the Issue body made in the doc, as against a comment from it or a Draft of the overlay.
func (d Draft) InPlaceEdit() bool { return d.Doc != nil && !d.Comment }

// DocCanAct reports whether the Issue body's doc can act on the Draft: an in-place edit that is neither being saved nor in conflict.
func (d Draft) DocCanAct() bool { return d.InPlaceEdit() && !d.Saving && d.Conflict == nil }

// withShown returns the Draft with its doc set to draw it, which an in-place edit does once its save failed or conflicted. Any other
// Draft comes back as it is.
func (d Draft) withShown() Draft {
	if d.InPlaceEdit() && !d.Doc.Shown {
		doc := *d.Doc // Drafts are copied by value: the DocPart they share is not changed
		doc.Shown = true
		d.Doc = &doc
	}
	return d
}

// DocText is the text of an in-place edit as the doc holds it.
func (d Draft) DocText() string { return strings.TrimPrefix(d.Editor.Text, d.Doc.HiddenPrefix) }

type Keyed struct {
	Key  string
	sent any
}

type ReplyKind string

const (
	ReplyPlain     ReplyKind = "reply"
	ReplyConfirm   ReplyKind = "confirm"
	ReplyRefute    ReplyKind = "refute"
	ReplySupersede ReplyKind = "supersede"
)

var ReplyKinds = []ReplyKind{ReplyPlain, ReplyConfirm, ReplyRefute, ReplySupersede}

// DuplicateReply is kata's refusal of a Reply as a duplicate of an earlier one of the same kind. It is a Draft's Err, and
// Send anyway can override it.
type DuplicateReply struct{ Message string }

func (e DuplicateReply) Error() string { return e.Message }

func (d Draft) Duplicate() bool {
	_, ok := errors.AsType[DuplicateReply](d.Err)
	return ok
}

// minEvidence is the length kata asks of a confirm or refute, counted in characters once whitespace collapses.
const minEvidence = 40

func (d Draft) validReply() error {
	if d.ReplyTo == "" || (d.ReplyKind != ReplyConfirm && d.ReplyKind != ReplyRefute) {
		return nil
	}
	if utf8.RuneCountInString(strings.Join(strings.Fields(d.Editor.Text), " ")) < minEvidence {
		return fmt.Errorf("A %s needs at least %d characters, not counting extra whitespace. Nothing was sent.", d.ReplyKind, minEvidence)
	}
	return nil
}

type SaveDraft struct {
	Workspace, UID, Text, Actor string
	BaseText                    string
	ProjectID                   int64
	Comment                     bool
	// ReplyTo and ReplyKind make the comment a Reply to the comment with that UID; Force sends it although kata sees a duplicate.
	ReplyTo   string
	ReplyKind ReplyKind
	Force     bool
	// Key is kata's Idempotency-Key for a comment or Reply; a body edit has none.
	Key string
}

func (SaveDraft) cmd() {}

// OpenDraftEditor opens text in $EDITOR; Suffix is the extension of the temporary file, "" for a Draft's Markdown.
type OpenDraftEditor struct{ UID, Text, Suffix string }

func (OpenDraftEditor) cmd() {}

type EditMsg struct {
	Target      EditTarget
	From, To    int
	Text        string
	Cursor, Len int
}

func (EditMsg) msg() {}

type UndoMsg struct{ Target EditTarget }

func (UndoMsg) msg() {}

type DraftSaved struct {
	UID    string
	Record WriteRecord
}

func (DraftSaved) msg() {}

type DraftFailed struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (DraftFailed) msg() {}

type DraftConflict struct {
	UID   string
	Issue Issue
}

func (DraftConflict) msg() {}

type DraftEditorClosed struct {
	UID, Text string
	Err       error
}

func (DraftEditorClosed) msg() {}

func (m Model) ActiveDraft() (Draft, bool) {
	draft, ok := m.Drafts[m.EditorUID]
	return draft, ok && m.EditorUID != ""
}
func (m Model) EditorMode() Mode {
	draft, ok := m.ActiveDraft()
	if !ok {
		return ""
	}
	if draft.Conflict != nil {
		return ModeConflict
	}
	if draft.DiscardConfirm {
		return ModeDiscard
	}
	return ModeEditor
}
func putDraft(m Model, uid string, draft Draft) Model {
	m.Drafts = maps.Clone(m.Drafts)
	if m.Drafts == nil {
		m.Drafts = make(map[string]Draft)
	}
	m.Drafts[uid] = draft
	return m
}
func dropDraft(m Model, uid string) Model {
	if draft, ok := m.Drafts[uid]; ok && draft.Conflict != nil {
		m = m.conflictResolved(uid, draft.Conflict)
	}
	m.Drafts = maps.Clone(m.Drafts)
	delete(m.Drafts, uid)
	if m.EditorUID == uid {
		m.EditorUID = ""
	}
	return m
}
func startDraft(m Model, comment bool) Model {
	if m.shownIssue() == nil {
		return m
	}
	uid := m.SelectedUID
	if draft, ok := m.Drafts[uid]; ok && draft.Doc != nil {
		draft.Doc = nil
		m = putDraft(m, uid, draft)
	}
	if _, ok := m.Drafts[uid]; !ok {
		issue := m.shownIssue()
		text := issue.Body
		if comment {
			text = ""
		}
		m = putDraft(m, uid, Draft{Editor: NewTextEditor(normalizeLineBreaks(text)), DraftBase: baseOf(*issue), Comment: comment})
	}
	m.EditorUID = uid
	return m.focusDetail()
}

// A body Draft already open stays as it is, as `c` leaves it.
func startReply(m Model) Model {
	row, ok := m.withRows().cursorRow()
	if !ok || row.Kind != RowComment || m.shownIssue() == nil {
		return m
	}
	m = startDraft(m, true)
	if draft, ok := m.ActiveDraft(); ok && draft.Comment {
		draft.ReplyTo, draft.ReplyAuthor, draft.ReplyKind = row.Key, row.Activity.Author, ReplyPlain
		draft.Err = nil
		m = putDraft(m, m.EditorUID, draft)
	}
	return m
}

// draftWhat names the doc Draft for a Notice: "edit of work#aaaa" or "comment on work#aaaa".
func draftWhat(draft Draft) string {
	what := "edit of"
	if draft.Comment {
		what = "comment on"
	}
	return what + " " + cmp.Or(draft.Doc.Ref, "the Issue")
}

// draftRefused keeps draft of Issue uid with err, which a save failed with. A Draft of the doc has no card showing the error, and the doc
// may be on another Issue by now, so a Notice names the Issue.
func draftRefused(m Model, uid string, draft Draft, err error) Model {
	draft.Err = err
	m = putDraft(m, uid, draft.withShown())
	if draft.Doc != nil && m.EditorUID != uid {
		kept := DraftKeptHint(err, " on that Issue")
		m.Notice = fmt.Sprintf("Cannot save your %s: %s%s", draftWhat(draft), err, kept)
	}
	return m
}

func saveDraft(m Model) (Model, []Cmd) { return sendDraftOf(m, m.EditorUID, false) }

// sendDraftOf sends the Draft of Issue uid, which the overlay need not show.
func sendDraftOf(m Model, uid string, force bool) (Model, []Cmd) {
	draft := m.Drafts[uid]
	if m.Stale {
		return draftRefused(m, uid, draft, errDraftReadOnly), nil
	}
	if err := draft.validReply(); err != nil {
		draft.Err = err
		return putDraft(m, uid, draft), nil
	}
	draft.Saving = true
	draft.Err = nil
	text := draft.Editor.Text
	if !draft.Comment && text == normalizeLineBreaks(draft.BaseText) {
		text = draft.BaseText // the Draft opened with line breaks normalised; unedited, it saves the stored bytes, so kata sees no change; an edited body saves with LF line breaks
	}
	cmd := SaveDraft{Workspace: m.Workspace, UID: uid, ProjectID: draft.ProjectID, BaseText: draft.BaseText, Text: text, Comment: draft.Comment, Actor: m.Actor, ReplyTo: draft.ReplyTo, ReplyKind: draft.ReplyKind, Force: force}
	if draft.Comment {
		request := cmd
		request.BaseText, request.Force = "", false // kata fingerprints neither, so a resend that differs in them keeps its key
		m, draft.Keyed = m.mintKey(draft.Keyed, request)
		cmd.Key = draft.Key
	}
	m = putDraft(m, uid, draft)
	return m, []Cmd{cmd}
}

// mintKey is the idempotency key for sending request: the key of k's previous send when the request is unchanged (kata
// fingerprints a request with its key, so only an identical resend may reuse it), a new one otherwise.
// request must carry no key.
func (m Model) mintKey(k Keyed, request any) (Model, Keyed) {
	if k.Key == "" || !reflect.DeepEqual(k.sent, request) {
		m.keys++
		k = Keyed{Key: m.nonce + "-" + strconv.Itoa(m.keys), sent: request}
	}
	return m, k
}

func replyKey(m Model, b Binding) (Model, []Cmd, bool) {
	d, _ := m.ActiveDraft()
	switch b.Action {
	case CycleReplyKind:
		if d.ReplyTo != "" {
			d.ReplyKind = ReplyKinds[(slices.Index(ReplyKinds, d.ReplyKind)+1)%len(ReplyKinds)]
			d.Err = nil
			m = putDraft(m, m.EditorUID, d)
		}
		return m, nil, true
	case SendReplyAnyway:
		if !d.Duplicate() {
			return m, nil, true
		}
		m, cmds := sendDraftOf(m, m.EditorUID, true)
		return m, cmds, true
	}
	return m, nil, false
}

func updateEditor(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case DraftSaved:
		draft, ok := m.Drafts[msg.UID]
		inPlace := ok && draft.InPlaceEdit()
		if inPlace {
			m = m.bodySaved(msg.UID, draft.Editor.Text)
		}
		m = dropDraft(m, msg.UID).issueWritten(msg.UID)
		var cmds []Cmd
		if inPlace {
			m, cmds = m.docSaved(msg.UID, draft)
		}
		return m, cmds, true
	case DraftFailed:
		if draft, ok := m.Drafts[msg.UID]; ok {
			draft.Saving = false
			m, draft = m.withLate(msg.UID, draft)
			m = draftRefused(m, msg.UID, draft, msg.Err)
		}
		return m, nil, true
	case DraftConflict:
		if draft, ok := m.Drafts[msg.UID]; ok {
			draft.Saving = false
			m, draft = m.withLate(msg.UID, draft)
			draft.Conflict = &msg.Issue
			draft.Err = nil
			draft = draft.withShown()
			m = putDraft(m, msg.UID, draft)
			switch {
			case draft.Doc == nil:
			case m.EditorUID == "": // the card is how a conflict is settled, so it opens over the doc
				m.EditorUID = msg.UID
				m = m.focusDetail()
			case m.EditorUID != msg.UID: // another Issue's card is open: it stays, and a Notice says where the conflict is
				m.Notice = ConflictSentence(draftWhat(draft), " on that Issue")
			}
		}
		return m, nil, true
	case BodyEdited:
		return bodyEdited(m, msg.UID, msg.Text), nil, true
	case BodyEditing:
		m, cmds := bodyEditing(m, msg.UID, msg.On)
		return m, cmds, true
	case SelectionComment:
		m, cmds := selectionComment(m, msg)
		return m, cmds, true
	case DraftEditorClosed:
		if draft, ok := m.Drafts[msg.UID]; ok {
			draft.Err = msg.Err
			if msg.Err == nil {
				text := normalizeLineBreaks(msg.Text)
				draft.Editor = draft.Editor.ApplyEdit(0, len16(draft.Editor.Text), text, len16(text), len16(draft.Editor.Text))
			}
			m = putDraft(m, msg.UID, draft)
		}
		return m, nil, true
	}
	return m, nil, false
}

var draftCard = draftCardKeys{
	draft: func(m Model) Draft { d, _ := m.ActiveDraft(); return d },
	set:   func(m Model, d Draft) Model { return putDraft(m, m.EditorUID, d) },
	close: func(m Model) Model { return dropDraft(m, m.EditorUID) },
	save:  saveDraft,
	empty: func(m Model) bool { d, _ := m.ActiveDraft(); return d.Editor.Text == "" },
	own: func(m Model, b Binding, _ KeyMsg) (Model, []Cmd, bool) {
		if b.Action != ExternalEditorAction {
			return replyKey(m, b)
		}
		d, _ := m.ActiveDraft()
		return m, []Cmd{OpenDraftEditor{UID: m.EditorUID, Text: d.Editor.Text}}, true
	},
	edit: func(m Model, b Binding, bound bool, msg KeyMsg) Model {
		d, _ := m.ActiveDraft()
		d.Editor = editorKey(d.Editor, b, bound, msg)
		return putDraft(m, m.EditorUID, d)
	},
}
