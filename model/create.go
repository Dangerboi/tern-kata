package model

import (
	"errors"
	"maps"
	"strconv"
	"strings"
)

const (
	NewIssue       Action = "new-issue"
	NewChild       Action = "new-child"
	CreateNext     Action = "create-next"
	CreatePrevious Action = "create-previous"
	CreateComplete Action = "create-complete"
	ModeCreate     Mode   = "create"
	CreateDraftUID        = "create-draft"
)

type CreateState struct {
	Draft
	Open                                      bool
	Field                                     int
	Title, Labels, Owner, Priority, ParentUID string
	Completion                                FieldFormState
}
type CreateIssueCmd struct {
	Workspace, Actor, Title, Body, Owner, ParentUID string
	ProjectID                                       int64
	Labels                                          []string
	Priority                                        *int64
}

func (CreateIssueCmd) cmd() {}

type IssueCreated struct {
	UID string
	Err error
}

func (IssueCreated) msg() {}

func openCreate(m Model, child bool) (Model, []Cmd) {
	if m.Stale {
		return m, nil
	}
	projectID := m.Project.ID
	if entry, ok := CatalogEntry(m.Snapshot, m.Collection.ProjectUID); ok {
		projectID = entry.Project.ID
	}
	issue := closeIssue(m)
	parent := ""
	if child {
		if issue == nil {
			return m, nil
		}
		projectID = issue.ProjectID
		parent = issue.UID
	} else if !m.Collection.IsProject() && issue != nil {
		projectID = issue.ProjectID
	}
	m.Create = CreateState{Open: true, Draft: Draft{ProjectID: projectID, Editor: NewTextEditor("")}, ParentUID: parent}
	if projectID == 0 {
		m.Create.Err = errors.New("Select a Project before creating an Issue.")
		return m, nil
	}
	return m, []Cmd{LoadFieldLabels{Workspace: m.Workspace, UID: CreateDraftUID, ProjectID: projectID}}
}
func (m Model) CreateSuggestions() []string {
	f := m.Create.Completion
	parts := strings.Split(m.Create.Labels, ",")
	f.Value = strings.TrimSpace(parts[len(parts)-1])
	m.FieldForm = f
	return m.FieldSuggestions()
}

// createFieldCount is the Create card's fields: title, body, labels, owner and priority.
const createFieldCount = 5

func createValue(f *CreateState) *string {
	switch f.Field {
	case 0:
		return &f.Title
	case 2:
		return &f.Labels
	case 3:
		return &f.Owner
	case 4:
		return &f.Priority
	}
	return nil
}
func submitCreate(m Model) (Model, []Cmd) {
	f := &m.Create
	if f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errDraftReadOnly
		return m, nil
	}
	if strings.TrimSpace(f.Title) == "" {
		f.Err = errors.New("Title is required.")
		return m, nil
	}
	if f.ProjectID == 0 {
		f.Err = errors.New("Select a Project before creating an Issue.")
		return m, nil
	}
	var priority *int64
	if p := strings.TrimSpace(f.Priority); p != "" && p != "-" {
		n, e := strconv.ParseInt(p, 10, 64)
		if e != nil || n < 0 || n > 4 {
			f.Err = errors.New("Priority must be 0-4, or - to clear.")
			return m, nil
		}
		priority = &n
	}
	labels := []string{}
	for _, label := range strings.Split(f.Labels, ",") {
		if label = strings.TrimSpace(label); label != "" {
			labels = append(labels, label)
		}
	}
	f.Saving = true
	f.Err = nil
	return m, []Cmd{CreateIssueCmd{Workspace: m.Workspace, Actor: m.Actor, ProjectID: f.ProjectID, Title: strings.TrimSpace(f.Title), Body: f.Editor.Text, Labels: labels, Owner: strings.TrimSpace(f.Owner), Priority: priority, ParentUID: f.ParentUID}}
}
func updateCreate(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case IssueCreated:
		if !m.Create.Open || !m.Create.Saving {
			return m, nil, true
		}
		m.Create.Saving = false
		m.Create.Err = msg.Err
		if msg.Err != nil {
			return m, nil, true
		}
		m = m.selectAfterCreate(msg.UID)
		m.ExpandedUIDs = maps.Clone(m.ExpandedUIDs)
		if m.ExpandedUIDs == nil {
			m.ExpandedUIDs = map[string]bool{}
		}
		if m.Create.ParentUID != "" {
			m.ExpandedUIDs[m.Create.ParentUID] = true
		}
		m.Create = CreateState{}
		m.Focus = ColumnList
		next, cmds := snapshotChanged(m)
		return next, cmds, true
	case FieldLabelsLoaded:
		if msg.UID == CreateDraftUID && m.Create.Open {
			m.Create.Completion.Labels = msg.Labels
			m.Create.Err = msg.Err
			return m, nil, true
		}
	case EditMsg:
		if m.Create.Open && msg.ID == "layer.create.body" {
			if !m.Create.Saving && !m.Create.DiscardConfirm {
				m.Create.Editor = m.Create.Editor.ApplyEdit(msg.From, msg.To, msg.Text, msg.Cursor, msg.Len)
			}
			return m, nil, true
		}
	case UndoMsg:
		if m.Create.Open && msg.ID == "layer.create.body" {
			if !m.Create.Saving && !m.Create.DiscardConfirm {
				m.Create.Editor = m.Create.Editor.Undo()
			}
			return m, nil, true
		}
	case KeyMsg:
		if !m.Create.Open {
			b, ok := LookupBindingForMode(msg.Chord, m.CurrentMode())
			if ok && (b.Action == NewIssue || b.Action == NewChild) {
				next, cmds := openCreate(m, b.Action == NewChild)
				return next, cmds, true
			}
			return m, nil, false
		}
		if m.Create.Saving {
			return m, nil, true
		}
		mode := ModeCreate
		if m.Create.DiscardConfirm {
			mode = ModeDiscard
		}
		b, ok := LookupBindingForMode(msg.Chord, mode)
		if m.Create.DiscardConfirm {
			if ok && b.Action == DiscardDraft {
				m.Create = CreateState{}
			} else if ok && b.Action == KeepDraft {
				m.Create.DiscardConfirm = false
			}
			return m, nil, true
		}
		if ok {
			switch b.Action {
			case SaveDraftAction:
				next, cmds := submitCreate(m)
				return next, cmds, true
			case CancelDraft:
				f := m.Create
				if f.Title+f.Editor.Text+f.Labels+f.Owner+f.Priority == "" {
					m.Create = CreateState{}
				} else {
					m.Create.DiscardConfirm = true
				}
				return m, nil, true
			case CreateNext:
				cycleField(&m, 1)
				return m, nil, true
			case CreatePrevious:
				cycleField(&m, -1)
				return m, nil, true
			case CreateComplete:
				if m.Create.Field == 2 {
					s := m.CreateSuggestions()
					if len(s) > 0 {
						parts := strings.Split(m.Create.Labels, ",")
						parts[len(parts)-1] = s[m.Create.Completion.Suggestion%len(s)]
						m.Create.Labels = strings.Join(parts, ",")
					}
				}
				return m, nil, true
			}
		}
		if m.Create.Field == 1 {
			b, ok = LookupBindingForMode(msg.Chord, ModeEditor)
			if ok {
				m.Create.Editor = m.Create.Editor.Key(b.Action, editorSelectionChord(msg.Chord))
			} else if msg.Text != "" {
				m.Create.Editor = m.Create.Editor.Insert(msg.Text)
			}
		} else if value := createValue(&m.Create); value != nil {
			b, ok = LookupBindingForMode(msg.Chord, ModeFieldForm)
			if ok && b.Action == FieldNext {
				m.Create.Completion.Suggestion++
			} else if ok && b.Action == FieldPrevious {
				m.Create.Completion.Suggestion = max(0, m.Create.Completion.Suggestion-1)
			} else {
				*value, _ = editText(*value, b, ok, msg)
			}
		}
		m.Create.Err = nil
		return m, nil, true
	}
	return m, nil, false
}
