package model

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

const (
	AddLabelAction    Action = "add-label"
	RemoveLabelAction Action = "remove-label"
	AssignOwnerAction Action = "assign-owner"
	ClearOwnerAction  Action = "clear-owner"
	PriorityAction    Action = "priority"
	FieldSubmit       Action = "field-submit"
	FieldCancel       Action = "field-cancel"
	FieldComplete     Action = "field-complete"
	FieldNext         Action = "field-next"
	FieldPrevious     Action = "field-previous"
	ModeFieldForm     Mode   = "field-form"
)

type FieldFormState struct {
	Open, Saving bool
	Action       Action
	UID, Value   string
	ProjectID    int64
	Labels       []string
	Suggestion   int
	Err          error
}
type WriteField struct {
	Workspace, UID, Actor, Value string
	ProjectID                    int64
	Action                       Action
}

func (WriteField) cmd() {}

type LoadFieldLabels struct {
	Workspace, UID string
	ProjectID      int64
}

func (LoadFieldLabels) cmd() {}

type FieldLabelsLoaded struct {
	UID    string
	Labels []string
	Err    error
}

func (FieldLabelsLoaded) msg() {}

type FieldValueMsg struct{ Value string }

func (FieldValueMsg) msg() {}

type FieldWritten struct {
	UID string
	Err error
}

func (FieldWritten) msg() {}

func (m Model) FieldSuggestions() []string {
	labels := []string{}
	for _, label := range m.FieldForm.Labels {
		if strings.HasPrefix(strings.ToLower(label), strings.ToLower(m.FieldForm.Value)) {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels
}

func fieldAction(action Action) bool {
	return action == AddLabelAction || action == RemoveLabelAction || action == AssignOwnerAction || action == ClearOwnerAction || action == PriorityAction
}
func openField(m Model, action Action) (Model, []Cmd) {
	if m.Stale || m.FieldForm.Saving {
		return m, nil
	}
	issue := m.shownIssue()
	if issue == nil {
		for i := range m.Issues {
			if m.Issues[i].UID == m.SelectedUID {
				issue = &m.Issues[i]
				break
			}
		}
	}
	if issue == nil {
		return m, nil
	}
	m.FieldForm = FieldFormState{Open: true, Action: action, UID: issue.UID, ProjectID: issue.ProjectID}
	if action == RemoveLabelAction {
		m.FieldForm.Labels = issue.Labels
	}
	if action == ClearOwnerAction {
		return submitField(m)
	}
	if action == AddLabelAction {
		return m, []Cmd{LoadFieldLabels{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID}}
	}
	return m, nil
}
func submitField(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if f.Saving || m.Stale {
		return m, nil
	}
	f.Value = strings.TrimSpace(f.Value)
	if f.Action == PriorityAction && f.Value != "-" {
		n, err := strconv.Atoi(f.Value)
		if err != nil || n < 0 || n > 4 {
			f.Err = errors.New("Priority must be 0-4, or - to clear.")
			return m, nil
		}
	}
	if (f.Action == AddLabelAction || f.Action == RemoveLabelAction || f.Action == AssignOwnerAction) && f.Value == "" {
		f.Err = errors.New("Enter a value.")
		return m, nil
	}
	f.Saving = true
	f.Err = nil
	return m, []Cmd{WriteField{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Action: f.Action, Value: f.Value, Actor: m.Actor}}
}
func updateFieldKey(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.FieldForm.Saving {
		return m, nil
	}
	binding, ok := LookupBindingForMode(msg.Chord, ModeFieldForm)
	if ok && binding.Mode == ModeFieldForm {
		switch binding.Action {
		case FieldSubmit:
			return submitField(m)
		case FieldCancel:
			m.FieldForm = FieldFormState{}
			return m, nil
		case DeleteInput, ClearInput:
			m.FieldForm.Value, _ = editText(m.FieldForm.Value, binding, true, msg)
		case FieldNext:
			m.FieldForm.Suggestion++
		case FieldPrevious:
			m.FieldForm.Suggestion = max(0, m.FieldForm.Suggestion-1)
		case FieldComplete:
			s := m.FieldSuggestions()
			if len(s) > 0 {
				m.FieldForm.Value = s[m.FieldForm.Suggestion%len(s)]
			}
		}
	} else {
		m.FieldForm.Value, _ = editText(m.FieldForm.Value, binding, false, msg)
	}
	m.FieldForm.Err = nil
	return m, nil
}
