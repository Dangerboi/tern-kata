package model

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

var errDraftReadOnly = errors.New("Read-only: reconnecting to kata; Draft kept")

type Draft struct {
	Editor                          TextEditor
	BaseText                        string // the Issue text the Draft started against
	ProjectID, BaseRevision         int64
	Comment, Saving, DiscardConfirm bool
	Conflict                        *Issue
	Err                             error
	// ReplyTo is the UID of the comment a comment Draft answers ("" for a plain comment), ReplyAuthor its author and
	// ReplyKind what the reply is.
	ReplyTo, ReplyAuthor string
	ReplyKind            ReplyKind
}

// ReplyKind is what a Reply is to the comment it answers.
type ReplyKind string

const (
	ReplyPlain     ReplyKind = "reply"
	ReplyConfirm   ReplyKind = "confirm"
	ReplyRefute    ReplyKind = "refute"
	ReplySupersede ReplyKind = "supersede"
)

// ReplyKinds are the kinds of a Reply, in the order the kind key cycles them.
var ReplyKinds = []ReplyKind{ReplyPlain, ReplyConfirm, ReplyRefute, ReplySupersede}

// DuplicateReply is kata's refusal of a Reply as a duplicate of an earlier one of the same kind. It is a Draft's Err, and
// Send anyway can override it.
type DuplicateReply struct{ Message string }

func (e DuplicateReply) Error() string { return e.Message }

// Duplicate says kata refused the last send as a duplicate Reply.
func (d Draft) Duplicate() bool {
	_, ok := errors.AsType[DuplicateReply](d.Err)
	return ok
}

// minEvidence is the length kata asks of a confirm or refute, counted in characters once whitespace collapses.
const minEvidence = 40

// validReply is the refusal of a Reply the daemon would reject for too little evidence, nil when it may be sent.
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
}

func (SaveDraft) cmd() {}

type OpenDraftEditor struct{ UID, Text string }

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

type DraftSaved struct{ UID string }

func (DraftSaved) msg() {}

type DraftFailed struct {
	UID string
	Err error
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
	if _, ok := m.Drafts[uid]; !ok {
		issue := m.shownIssue()
		text := issue.Body
		if comment {
			text = ""
		}
		m = putDraft(m, uid, Draft{Editor: NewTextEditor(text), ProjectID: issue.ProjectID, BaseText: issue.Body, BaseRevision: issue.Revision, Comment: comment})
	}
	m.EditorUID = uid
	return m.focusDetail()
}

// startReply is `R`: on a comment row it opens the Issue's comment Draft as a reply to that comment, keeping the Draft's text.
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

func saveDraft(m Model) (Model, []Cmd) { return sendDraft(m, false) }

func sendDraft(m Model, force bool) (Model, []Cmd) {
	draft, _ := m.ActiveDraft()
	if m.Stale {
		draft.Err = errDraftReadOnly
		return putDraft(m, m.EditorUID, draft), nil
	}
	if err := draft.validReply(); err != nil {
		draft.Err = err
		return putDraft(m, m.EditorUID, draft), nil
	}
	draft.Saving = true
	draft.Err = nil
	m = putDraft(m, m.EditorUID, draft)
	return m, []Cmd{SaveDraft{Workspace: m.Workspace, UID: m.EditorUID, ProjectID: draft.ProjectID, BaseText: draft.BaseText, Text: draft.Editor.Text, Comment: draft.Comment, Actor: m.Actor, ReplyTo: draft.ReplyTo, ReplyKind: draft.ReplyKind, Force: force}}
}

// replyKey handles the keys only a Reply Draft has: the kind key cycles the kind and Send anyway resends a duplicate with force.
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
		m, cmds := sendDraft(m, true)
		return m, cmds, true
	}
	return m, nil, false
}

func updateEditor(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case DraftSaved:
		m = dropDraft(m, msg.UID)
		return m.issueWritten(msg.UID), nil, true
	case DraftFailed:
		if draft, ok := m.Drafts[msg.UID]; ok {
			draft.Saving = false
			draft.Err = msg.Err
			m = putDraft(m, msg.UID, draft)
		}
		return m, nil, true
	case DraftConflict:
		if draft, ok := m.Drafts[msg.UID]; ok {
			draft.Saving = false
			draft.Conflict = &msg.Issue
			draft.Err = nil
			m = putDraft(m, msg.UID, draft)
		}
		return m, nil, true
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

// draftCard is the Draft editor's part of the shared Draft key handler: its text is the body of the
// Draft being edited and saving sends it back to kata.
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
