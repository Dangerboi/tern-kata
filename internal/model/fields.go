package model

import (
	"errors"
	"fmt"
	"slices"
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
	SetParentAction   Action = "set-parent"
	AttachFileAction  Action = "attach-file"
	DetachFileAction  Action = "detach-file"
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

// fieldIssue is the Issue the field keys act on: the one the Issue detail shows, else the selected one in the list.
func (m Model) fieldIssue() *Issue {
	if issue := m.shownIssue(); issue != nil {
		return issue
	}
	for i := range m.Issues {
		if m.Issues[i].UID == m.SelectedUID {
			return &m.Issues[i]
		}
	}
	return nil
}

func openField(m Model, action Action) (Model, []Cmd) {
	if m.Stale || m.FieldForm.Saving {
		return m, nil
	}
	issue := m.fieldIssue()
	if issue == nil {
		return m, nil
	}
	m.FieldForm = FieldFormState{Open: true, Action: action, UID: issue.UID, ProjectID: issue.ProjectID}
	if action == AttachFileAction || action == DetachFileAction {
		return openAttachment(m, issue)
	}
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

// attachRefusal is why this workspace cannot attach to or detach from the Issue, or nil.
func (m Model) attachRefusal(issue *Issue) error {
	return AttachRefusal(m.Project, issue.ProjectID, issue.ShortID, issue.ProjectName)
}

// openAttachment finishes opening the form of u or U. A refusal, or no Attachments to remove, shows as the form's error.
func openAttachment(m Model, issue *Issue) (Model, []Cmd) {
	f := &m.FieldForm
	f.Err = m.attachRefusal(issue)
	if f.Action == DetachFileAction {
		for _, a := range Attachments(issue.Metadata) {
			f.Labels = append(f.Labels, a.Name)
		}
		if f.Err == nil && len(f.Labels) == 0 {
			f.Err = fmt.Errorf("%s has no attachments.", issue.ShortID)
		}
	}
	return m, nil
}

func submitField(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if f.Saving || m.Stale {
		return m, nil
	}
	f.Value = strings.TrimSpace(f.Value)
	if f.Action == SetParentAction {
		f.Value = strings.TrimPrefix(f.Value, "#")
	}
	if f.Action == PriorityAction && f.Value != "-" {
		n, err := strconv.Atoi(f.Value)
		if err != nil || n < 0 || n > 4 {
			f.Err = errors.New("Priority must be 0-4, or - to clear.")
			return m, nil
		}
	}
	if (f.Action == AddLabelAction || f.Action == RemoveLabelAction || f.Action == AssignOwnerAction || f.Action == SetParentAction || f.Action == AttachFileAction) && f.Value == "" {
		f.Err = errors.New("Enter a value.")
		return m, nil
	}
	if f.Action == AttachFileAction || f.Action == DetachFileAction {
		return submitAttachment(m)
	}
	f.Saving = true
	f.Err = nil
	return m, []Cmd{WriteField{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Action: f.Action, Value: f.Value, Actor: m.Actor}}
}
// submitAttachment sends the form of u or U. U removes the Attachment typed in full, else the highlighted one.
func submitAttachment(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	issue := m.fieldIssue()
	if issue == nil {
		return m, nil
	}
	if f.Err = m.attachRefusal(issue); f.Err != nil {
		return m, nil
	}
	if f.Action == AttachFileAction {
		f.Saving = true
		return m, []Cmd{AttachFile{Workspace: m.Workspace, Ref: f.UID, Actor: m.Actor, Path: f.Value}}
	}
	name := f.Value
	if !slices.Contains(f.Labels, name) {
		suggestions := m.FieldSuggestions()
		if len(suggestions) == 0 {
			f.Err = fmt.Errorf("%s has no attachment named %q.", issue.ShortID, name)
			return m, nil
		}
		name = suggestions[f.Suggestion%len(suggestions)]
	}
	f.Saving = true
	return m, []Cmd{DetachFile{Workspace: m.Workspace, Ref: f.UID, Actor: m.Actor, Name: name}}
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

// attachmentWritten closes the form of u or U for the Issue uid when the write succeeded, leaving report as the Notice, and
// keeps it open with the failure otherwise.
func attachmentWritten(m Model, uid, report string, err error) (Model, []Cmd) {
	if m.FieldForm.UID != uid {
		return m, nil
	}
	m.FieldForm.Saving = false
	if err != nil {
		m.FieldForm.Err = err
		return m, nil
	}
	m.FieldForm = FieldFormState{}
	m.Notice = report
	return snapshotChanged(m.issueWritten(uid))
}
