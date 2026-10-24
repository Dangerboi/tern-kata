package model

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	AddLabelAction    Action = "add-label"
	RemoveLabelAction Action = "remove-label"
	AssignOwnerAction Action = "assign-owner"
	ClearOwnerAction  Action = "clear-owner"
	PriorityAction    Action = "priority"
	SetParentAction   Action = "set-parent"
	TimedAssignAction Action = "timed-assign"
	AddBlockerAction  Action = "add-blocker"
	AddRelatedAction  Action = "add-related"
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
	Caret        Caret
	Err          error
}
type WriteField struct {
	Workspace, UID, Actor, Value string
	ProjectID                    int64
	Action                       Action
}

func (WriteField) cmd() {}

// RemoveLinkCmd deletes the link LinkID from the Issue UID.
type RemoveLinkCmd struct {
	Workspace, UID, Actor string
	ProjectID, LinkID     int64
}

func (RemoveLinkCmd) cmd() {}

// LinkRemoved answers RemoveLinkCmd.
type LinkRemoved struct {
	UID    string
	LinkID int64
	Err    error
}

func (LinkRemoved) msg() {}

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

// issueByUID is the Issue uid, as the detail shows it or as the list holds it.
func (m Model) issueByUID(uid string) *Issue {
	if issue := m.shownIssue(); issue != nil && issue.UID == uid {
		return issue
	}
	for i := range m.Issues {
		if m.Issues[i].UID == uid {
			return &m.Issues[i]
		}
	}
	return nil
}

// fieldOpen says how the form of an action opens.
type fieldOpen int

const (
	openInput         fieldOpen = iota // a plain input
	openProjectLabels                  // a picker of the Project's labels, loaded from the daemon
	openIssueLabels                    // a picker of the Issue's labels
	openAttachments                    // a picker of the Issue's Attachments; none says so as the Notice
	openImmediate                      // submits at once
)

// fieldSpec describes one single-field form action. Opening and submitting read it instead of branching on the action.
type fieldSpec struct {
	title      string
	needsValue bool
	clean      func(string) string
	check      func(string) error
	open       fieldOpen
	refuse     bool // the Binding's refusal shows as the form's error
	send       func(m Model) (Model, []Cmd)
}

var fieldSpecs = map[Action]fieldSpec{
	AddLabelAction:    {title: "Add label", needsValue: true, open: openProjectLabels, send: sendFieldWrite},
	RemoveLabelAction: {title: "Remove label", needsValue: true, open: openIssueLabels, send: sendFieldWrite},
	AssignOwnerAction: {title: "Assign owner", needsValue: true, send: sendFieldWrite},
	ClearOwnerAction:  {title: "Clear owner", open: openImmediate, send: sendFieldWrite},
	PriorityAction:    {title: "Priority (0-4, or - to clear)", check: checkPriority, send: sendFieldWrite},
	SetParentAction:   {title: "Set parent (Issue id)", needsValue: true, clean: trimHash, send: sendFieldWrite},
	TimedAssignAction: {title: "Assignment duration (1m..24h, e.g. 1h)", needsValue: true, check: checkAssignmentDuration, send: sendTimedAssignment},
	AddBlockerAction:  {title: "This Issue blocks (Issue id)", needsValue: true, clean: trimHash, send: sendFieldWrite},
	AddRelatedAction:  {title: "Related to (Issue id)", needsValue: true, clean: trimHash, send: sendFieldWrite},
	AttachFileAction:  {title: "Attach a file (path)", needsValue: true, refuse: true, send: sendAttach},
	DetachFileAction:  {title: "Remove attachment", open: openAttachments, refuse: true, send: sendDetach},
}

// FieldTitle is the title of the form of an action.
func FieldTitle(action Action) string { return fieldSpecs[action].title }

// FieldFormTitle is the title of the open form: FieldTitle, with the Issue named where kata tui names it.
func (m Model) FieldFormTitle() string {
	f := m.FieldForm
	if f.Action == TimedAssignAction {
		if issue := m.issueByUID(f.UID); issue != nil {
			return fmt.Sprintf("Assignment duration for #%s (1m..24h, e.g. 1h)", issue.ShortID)
		}
	}
	return FieldTitle(f.Action)
}

// assignmentSeconds reads a timed assignment's duration: a Go duration of whole seconds from 1m to 24h.
func assignmentSeconds(v string) (int64, error) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < time.Minute || d > 24*time.Hour || d%time.Second != 0 {
		return 0, errors.New("duration must be a whole number of seconds from 1m through 24h")
	}
	return int64(d / time.Second), nil
}

func checkAssignmentDuration(v string) error {
	_, err := assignmentSeconds(v)
	return err
}

// sendTimedAssignment sends the duration as seconds, the unit kata's claim action takes.
func sendTimedAssignment(m Model) (Model, []Cmd) {
	seconds, err := assignmentSeconds(m.FieldForm.Value)
	if err != nil {
		m.FieldForm.Err = err
		return m, nil
	}
	m.FieldForm.Value = strconv.FormatInt(seconds, 10)
	return sendFieldWrite(m)
}

// trimHash drops the "#" of a reference typed as #id.
func trimHash(v string) string { return strings.TrimPrefix(v, "#") }

func checkPriority(v string) error {
	if n, err := strconv.Atoi(v); v != "-" && (err != nil || n < 0 || n > 4) {
		return errors.New("Priority must be 0-4, or - to clear.")
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
	spec := fieldSpecs[action]
	m.FieldForm = FieldFormState{Open: true, Action: action, UID: issue.UID, ProjectID: issue.ProjectID}
	f := &m.FieldForm
	if spec.refuse {
		f.Err = AttachRefusal(m.Project, issue.ProjectID, issue.ShortID, issue.ProjectName)
	}
	switch spec.open {
	case openImmediate:
		return submitField(m)
	case openProjectLabels:
		return m, []Cmd{LoadFieldLabels{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID}}
	case openIssueLabels:
		f.Labels = issue.Labels
	case openAttachments:
		for _, a := range Attachments(issue.Metadata) {
			f.Labels = append(f.Labels, a.Name)
		}
		if f.Err == nil && len(f.Labels) == 0 {
			m.FieldForm = FieldFormState{}
			m.Notice = fmt.Sprintf("%s has no attachments.", issue.ShortID)
		}
	}
	return m, nil
}

func submitField(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if f.Saving || m.Stale {
		return m, nil
	}
	spec := fieldSpecs[f.Action]
	f.Value = strings.TrimSpace(f.Value)
	if spec.clean != nil {
		f.Value = spec.clean(f.Value)
	}
	if spec.check != nil {
		if f.Err = spec.check(f.Value); f.Err != nil {
			return m, nil
		}
	}
	if spec.needsValue && f.Value == "" {
		f.Err = errors.New("Enter a value.")
		return m, nil
	}
	return spec.send(m)
}

func sendFieldWrite(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	f.Saving = true
	f.Err = nil
	return m, []Cmd{WriteField{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Action: f.Action, Value: f.Value, Actor: m.Actor}}
}

// attachIssue is the Issue of an attachment form, or the form's error when it is gone or the Binding refuses it.
func attachIssue(m *Model) *Issue {
	f := &m.FieldForm
	issue := m.issueByUID(f.UID)
	if issue == nil {
		f.Err = errors.New("That Issue is no longer listed.")
		return nil
	}
	if f.Err = AttachRefusal(m.Project, issue.ProjectID, issue.ShortID, issue.ProjectName); f.Err != nil {
		return nil
	}
	return issue
}

func sendAttach(m Model) (Model, []Cmd) {
	if attachIssue(&m) == nil {
		return m, nil
	}
	f := &m.FieldForm
	f.Saving = true
	return m, []Cmd{AttachFile{Workspace: m.Workspace, Ref: f.UID, Actor: m.Actor, Path: f.Value}}
}

// sendDetach removes the Attachment typed in full, else the highlighted one.
func sendDetach(m Model) (Model, []Cmd) {
	issue := attachIssue(&m)
	if issue == nil {
		return m, nil
	}
	f := &m.FieldForm
	name := f.Value
	if !slices.Contains(f.Labels, name) {
		suggestions := m.FieldSuggestions()
		if len(suggestions) == 0 {
			f.Err = NoAttachmentError(issue.ShortID, name)
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
		case FieldNext:
			m.FieldForm.Suggestion++
		case FieldPrevious:
			m.FieldForm.Suggestion = max(0, m.FieldForm.Suggestion-1)
		case FieldComplete:
			s := m.FieldSuggestions()
			if len(s) > 0 {
				m.FieldForm.Value = s[m.FieldForm.Suggestion%len(s)]
				m.FieldForm.Caret = Caret{}
			}
		}
	} else {
		m.FieldForm.Value, _ = editText(m.FieldForm.Value, &m.FieldForm.Caret, "", false, msg)
	}
	m.FieldForm.Err = nil
	return m, nil
}

// fieldWritten closes the form of a write to the Issue uid when it succeeded, leaving a non-empty report as the Notice, and
// keeps it open with the failure otherwise. A write for another Issue than the form's is ignored.
func fieldWritten(m Model, uid, report string, err error) (Model, []Cmd) {
	if m.FieldForm.UID != uid {
		return m, nil
	}
	m.FieldForm.Saving = false
	if err != nil {
		m.FieldForm.Open = true
		m.FieldForm.Err = err
		return m, nil
	}
	m.FieldForm = FieldFormState{}
	if report != "" {
		m.Notice = report
	}
	return snapshotChanged(m.issueWritten(uid))
}
