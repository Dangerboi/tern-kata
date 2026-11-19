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
	Attachments  []Attachment
	Suggestion   int
	Caret        Caret
	Found        *DeletedIssue
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
	UID    string
	Err    error
	Record WriteRecord
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

func (m Model) LoadedIssue(uid string) (Issue, bool) {
	if issue := m.issueByUID(uid); issue != nil {
		return *issue, true
	}
	return Issue{}, false
}

type fieldOpen int

const (
	openInput         fieldOpen = iota
	openProjectLabels           // a picker of the Project's labels, loaded from the daemon
	openIssueLabels
	openAttachments // a picker of the Issue's Attachments; none says so as the Notice
	openImmediate
)

type fieldSpec struct {
	title string
	// label is the word above the input and placeholder the format it takes, shown while it is empty.
	label, placeholder string
	needsValue         bool
	clean              func(string) string
	check              func(string) error
	open               fieldOpen
	send               func(m Model) (Model, []Cmd)
}

var fieldSpecs = map[Action]fieldSpec{
	AddLabelAction:       {title: "Add label", label: "Label", placeholder: "Name", needsValue: true, open: openProjectLabels, send: sendFieldWrite},
	RemoveLabelAction:    {title: "Remove label", label: "Label", placeholder: "Name", needsValue: true, open: openIssueLabels, send: sendFieldWrite},
	AssignOwnerAction:    {title: "Assign owner", label: "Owner", placeholder: "Name", needsValue: true, send: sendFieldWrite},
	ClearOwnerAction:     {title: "Clear owner", open: openImmediate, send: sendFieldWrite},
	PriorityAction:       {title: "Set priority", label: "Priority", placeholder: "P0 to P4, or - to clear", clean: trimPriority, check: checkPriority, send: sendFieldWrite},
	SetParentAction:      {title: "Set parent", label: "Parent", placeholder: "Issue id", needsValue: true, clean: trimHash, send: sendFieldWrite},
	TimedAssignAction:    {title: "Assign for a time", label: "Duration", placeholder: "1h (1m to 24h)", needsValue: true, check: checkAssignmentDuration, send: sendTimedAssignment},
	AddBlockerAction:     {title: "Block an Issue", label: "Blocked Issue", placeholder: "Issue id", needsValue: true, clean: trimHash, send: sendFieldWrite},
	AddRelatedAction:     {title: "Add related Issue", label: "Issue", placeholder: "Issue id", needsValue: true, clean: trimHash, send: sendFieldWrite},
	AttachFileAction:     {title: "Attach file", label: "File", placeholder: "File path", needsValue: true, send: sendAttach},
	DetachFileAction:     {title: "Unlink attachment", label: "Attachment", placeholder: "Name", open: openAttachments, send: sendDetach},
	RestoreDeletedAction: {title: "Restore Issue", label: "Issue", placeholder: "Short id or project#id", needsValue: true, send: sendRestore},
}

func FieldTitle(action Action) string { return fieldSpecs[action].title }

func (m Model) FieldFormTitle() string { return FieldTitle(m.FieldForm.Action) }

// FieldFormLabel is the word above the open form's input, "" for a form with no input.
func (m Model) FieldFormLabel() string { return fieldSpecs[m.FieldForm.Action].label }

// FieldFormPlaceholder is the format the open form's empty input takes.
func (m Model) FieldFormPlaceholder() string { return fieldSpecs[m.FieldForm.Action].placeholder }

// FieldFormMeta is the Issue the open form acts on, named as the Pane names it; "" for a form that names none (Restore
// finds its Issue from what is typed).
func (m Model) FieldFormMeta() string {
	if issue := m.issueByUID(m.FieldForm.UID); issue != nil && m.FieldForm.UID != "" {
		return m.RefName(m.projectName(issue.ProjectID), issue.ShortID)
	}
	return ""
}

// assignmentSeconds reads a timed assignment's duration: a Go duration of whole seconds from 1m to 24h.
func assignmentSeconds(v string) (int64, error) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < time.Minute || d > 24*time.Hour || d%time.Second != 0 {
		return 0, errors.New("Use a whole number of seconds from 1m to 24h.")
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
	return writeFieldValue(m, strconv.FormatInt(seconds, 10))
}

func trimHash(v string) string { return strings.TrimPrefix(v, "#") }

func PriorityText(p *int64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("P%d", *p)
}

// trimPriority lets a priority be typed as the badge shows it, P2, as well as the number.
func trimPriority(v string) string { return strings.TrimLeft(v, "Pp") }

func checkPriority(v string) error {
	if n, err := strconv.Atoi(v); v != "-" && (err != nil || n < 0 || n > 4) {
		return errors.New(Cannot("set", "priority", errors.New("use P0 to P4, or - to clear")))
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
	if action == PriorityAction {
		// The picker opens on the Issue's priority; none is "-", the word kata's priority write clears with.
		f.Value = "-"
		if issue.Priority != nil {
			f.Value = strconv.FormatInt(*issue.Priority, 10)
		}
	}
	if action == AttachFileAction && m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == issue.UID && m.authority.AttachmentsOff {
		f.Err = errors.New(DocbankOffMessage)
	}
	switch spec.open {
	case openImmediate:
		return submitField(m)
	case openProjectLabels:
		return m, []Cmd{LoadFieldLabels{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID}}
	case openIssueLabels:
		f.Labels = issue.Labels
	case openAttachments:
		f.Attachments = Attachments(issue.Metadata)
		f.Labels = AttachmentLabels(f.Attachments)
		if len(f.Labels) == 0 {
			m.FieldForm = FieldFormState{}
			m = m.say(m.RefName(m.projectName(issue.ProjectID), issue.ShortID) + " has no attachments.")
		}
	}
	return m, nil
}

func submitField(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New(Cannot("save", "", ErrDisconnected))
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

func sendFieldWrite(m Model) (Model, []Cmd) { return writeFieldValue(m, m.FieldForm.Value) }

// writeFieldValue sends value, which may differ from what the card holds, so a refused write can be retried as typed.
func writeFieldValue(m Model, value string) (Model, []Cmd) {
	f := &m.FieldForm
	f.Saving = true
	f.Err = nil
	return m, []Cmd{WriteField{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Action: f.Action, Value: value, Actor: m.Actor}}
}

func sendAttach(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	if m.issueByUID(f.UID) == nil {
		f.Err = errors.New("That Issue is no longer listed.")
		return m, nil
	}
	f.Saving = true
	return m, []Cmd{AttachFile{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, Path: f.Value}}
}

// sendDetach unlinks the Attachment typed in full, else the highlighted one.
func sendDetach(m Model) (Model, []Cmd) {
	f := &m.FieldForm
	issue := m.issueByUID(f.UID)
	if issue == nil {
		f.Err = errors.New("That Issue is no longer listed.")
		return m, nil
	}
	i := slices.Index(f.Labels, f.Value)
	if i < 0 {
		suggestions := m.FieldSuggestions()
		if len(suggestions) == 0 {
			f.Err = fmt.Errorf("%s has no attachment named %q.", issue.ShortID, f.Value)
			return m, nil
		}
		i = slices.Index(f.Labels, suggestions[f.Suggestion%len(suggestions)])
	}
	if i < 0 || i >= len(f.Attachments) {
		f.Err = fmt.Errorf("%s no longer has that attachment.", issue.ShortID)
		return m, nil
	}
	f.Saving = true
	return m, []Cmd{UnlinkAttachment{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, ID: f.Attachments[i].ID, Name: f.Attachments[i].Name}}
}

func updateFieldKey(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.FieldForm.Saving {
		return m, nil
	}
	if m.FieldForm.Action == PriorityAction {
		switch {
		case slices.Contains([]string{"0", "1", "2", "3", "4", "-"}, msg.Chord):
			// A digit or a dash sets or clears the priority at once, as the key after the priority key always did.
			m.FieldForm.Value = msg.Chord
			return submitField(m)
		case slices.Contains([]string{"left", "up"}, msg.Chord):
			return stepFieldChoice(m, true), nil
		case slices.Contains([]string{"right", "down", "space"}, msg.Chord):
			return stepFieldChoice(m, false), nil
		case msg.Chord != "escape" && msg.Chord != "enter" && msg.Chord != "ctrl+o" && msg.Chord != "ctrl+s":
			// Any other key closes the field and is handled as if it had been pressed with no field open.
			m.FieldForm = FieldFormState{}
			return update(m, msg)
		}
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
		var edited bool
		if m.FieldForm.Value, edited = editText(m.FieldForm.Value, &m.FieldForm.Caret, "", false, msg); edited {
			m.FieldForm.Found = nil
		}
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
		m = m.say(report)
	}
	return snapshotChanged(m.issueWritten(uid))
}
