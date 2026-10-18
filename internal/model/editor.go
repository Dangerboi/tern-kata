package model

import (
	"errors"
	"maps"
)

var errDraftReadOnly = errors.New("Read-only: reconnecting to kata; Draft kept")

type Draft struct {
	Editor                          TextEditor
	BaseText                        string // the Issue text the Draft started against
	ProjectID, BaseRevision         int64
	Comment, Saving, DiscardConfirm bool
	Conflict                        *Issue
	Err                             error
}

type SaveDraft struct {
	Workspace, UID, Text, Actor string
	BaseText                    string
	ProjectID, BaseRevision     int64
	Comment                     bool
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
func saveDraft(m Model) (Model, []Cmd) {
	draft, _ := m.ActiveDraft()
	if m.Stale {
		draft.Err = errDraftReadOnly
		return putDraft(m, m.EditorUID, draft), nil
	}
	draft.Saving = true
	draft.Err = nil
	m = putDraft(m, m.EditorUID, draft)
	return m, []Cmd{SaveDraft{Workspace: m.Workspace, UID: m.EditorUID, ProjectID: draft.ProjectID, BaseText: draft.BaseText, BaseRevision: draft.BaseRevision, Text: draft.Editor.Text, Comment: draft.Comment, Actor: m.Actor}}
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
			return m, nil, false
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
