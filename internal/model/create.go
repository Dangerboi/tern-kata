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
	CreateComplete Action = "create-complete"
	ModeCreate     Mode   = "create"
	CreateDraftUID        = "create-draft"
)

type CreateState struct {
	Draft
	Open                                      bool
	Field                                     Field
	Title, Labels, Owner, Priority, ParentUID string
	Dates                                     DatesPair
	Completion                                FieldFormState
	Caret                                     Caret
}
type CreateIssueCmd struct {
	Workspace, Actor, Title, Body, Owner, ParentUID string
	ProjectID                                       int64
	Labels                                          []string
	Priority                                        *int64
	// ScheduledOn and DeadlineOn are YYYY-MM-DD days, "" for none; kata gets only the ones set.
	ScheduledOn, DeadlineOn string
	// Key is kata's Idempotency-Key for this create.
	Key string
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
	} else if m.Collection.IsProject() {
		projectID = 0
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
	m.Create = CreateState{Open: true, Field: CreateFields[0].Name, Draft: Draft{ProjectID: projectID, Editor: NewTextEditor("")}, ParentUID: parent}
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

const (
	CreateTitle     Field = "title"
	CreateBody      Field = "body"
	CreateLabels    Field = "labels"
	CreateOwner     Field = "owner"
	CreatePriority  Field = "priority"
	CreateScheduled Field = "scheduled"
	CreateDeadline  Field = "deadline"
)

// createTable declares the Create card's fields, in tab order, and its busy condition. The body is the multi-line editor:
// it takes text like the others and so holds the caret, but its text is the Editor's, so the table reaches none.
var createTable = fieldTable[CreateState]{
	form:  func(m *Model) *CreateState { return &m.Create },
	busy:  func(f CreateState) bool { return f.Saving || f.DiscardConfirm },
	focus: func(f *CreateState) *Field { return &f.Field },
	caret: func(f *CreateState) *Caret { return &f.Caret },
	err:   func(f *CreateState) *error { return &f.Err },
	fields: []formField[CreateState]{
		textField(CreateTitle, func(f *CreateState) *string { return &f.Title }),
		textField(CreateBody, func(*CreateState) *string { return nil }),
		textField(CreateLabels, func(f *CreateState) *string { return &f.Labels }),
		textField(CreateOwner, func(f *CreateState) *string { return &f.Owner }),
		textField(CreatePriority, func(f *CreateState) *string { return &f.Priority }),
		textField(CreateScheduled, func(f *CreateState) *string { return &f.Dates.Scheduled }),
		textField(CreateDeadline, func(f *CreateState) *string { return &f.Dates.Deadline }),
	},
}

var CreateFields = createTable.specs()

func (m Model) CreatePreview(name Field) string {
	return datePreview(m.Create.Dates.Of(name), m.Calendar.Date)
}

func submitCreate(m Model) (Model, []Cmd) {
	f := &m.Create
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
	scheduled, err := dateDay("Scheduled", f.Dates.Scheduled, m.Calendar.Date)
	if err != nil {
		f.Err = err
		return m, nil
	}
	deadline, err := dateDay("Deadline", f.Dates.Deadline, m.Calendar.Date)
	if err != nil {
		f.Err = err
		return m, nil
	}
	labels := []string{}
	for _, label := range strings.Split(f.Labels, ",") {
		if label = strings.TrimSpace(label); label != "" {
			labels = append(labels, label)
		}
	}
	f.Saving = true
	f.Err = nil
	cmd := CreateIssueCmd{Workspace: m.Workspace, Actor: m.Actor, ProjectID: f.ProjectID, Title: strings.TrimSpace(f.Title), Body: f.Editor.Text, Labels: labels, Owner: strings.TrimSpace(f.Owner), Priority: priority, ParentUID: f.ParentUID, ScheduledOn: scheduled, DeadlineOn: deadline}
	var keyed Keyed
	m, keyed = m.mintKey(f.Keyed, cmd)
	f.Keyed, cmd.Key = keyed, keyed.Key
	return m, []Cmd{cmd}
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
		next, cmds := snapshotChanged(m.issueWritten(msg.UID))
		return next, cmds, true
	case FieldLabelsLoaded:
		if msg.UID == CreateDraftUID && m.Create.Open {
			m.Create.Completion.Labels = msg.Labels
			if m.Create.Err == nil {
				m.Create.Err = msg.Err
			}
			return m, nil, true
		}
	}
	return m, nil, false
}

var createCard = draftCardKeys{
	draft: func(m Model) Draft { return m.Create.Draft },
	set:   func(m Model, d Draft) Model { m.Create.Draft = d; return m },
	close: func(m Model) Model { m.Create = CreateState{}; return m },
	save:  submitCreate,
	empty: func(m Model) bool {
		f := m.Create
		return f.Title+f.Editor.Text+f.Labels+f.Owner+f.Priority+f.Dates.Scheduled+f.Dates.Deadline == ""
	},
	own: func(m Model, b Binding, _ KeyMsg) (Model, []Cmd, bool) {
		switch b.Action {
		case CreateComplete:
			if m.Create.Field == CreateLabels {
				if s := m.CreateSuggestions(); len(s) > 0 {
					parts := strings.Split(m.Create.Labels, ",")
					parts[len(parts)-1] = s[m.Create.Completion.Suggestion%len(s)]
					m.Create.Labels = strings.Join(parts, ",")
					m.Create.Caret = Caret{}
				}
			}
		default:
			return m, nil, false
		}
		return m, nil, true
	},
	edit: func(m Model, _ Binding, _ bool, msg KeyMsg) Model {
		if m.Create.Field == CreateBody {
			b, ok := LookupBindingForMode(msg.Chord, ModeEditor)
			m.Create.Editor = editorKey(m.Create.Editor, b, ok, msg)
		} else if value := createTable.textAt(&m.Create, m.Create.Field); value != nil {
			b, ok := LookupBindingForMode(msg.Chord, ModeFieldForm)
			if ok && b.Action == FieldNext {
				m.Create.Completion.Suggestion++
			} else if ok && b.Action == FieldPrevious {
				m.Create.Completion.Suggestion = max(0, m.Create.Completion.Suggestion-1)
			} else {
				*value, _ = editText(*value, &m.Create.Caret, m.Create.Field, ok, msg)
			}
		}
		return m
	},
}
