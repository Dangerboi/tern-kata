package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const ModeCloseForm Mode = "close-form"
const (
	OpenClose     Action = "close-issue"
	ReopenAction  Action = "reopen-issue"
	CloseNext     Action = "close-next"
	ClosePrevious Action = "close-previous"
	CloseSave     Action = "close-save"
	CloseQuick    Action = "close-quick"
	CloseCancel   Action = "close-cancel"
)

// The Close card's fields.
const (
	CloseReason    Field = "reason"
	CloseMessage   Field = "message"
	CloseCommit    Field = "commit"
	ClosePR        Field = "pr"
	CloseTest      Field = "test"
	CloseReference Field = "reference"
	CloseRationale Field = "rationale"
)

// CloseFields is the Close card's field list, in tab order.
var CloseFields = []FieldSpec{{CloseReason, true}, {CloseMessage, true}, {CloseCommit, true}, {ClosePR, true}, {CloseTest, true}, {CloseReference, true}, {CloseRationale, true}}

// CloseValues holds what each Close field has been typed.
type CloseValues struct{ Reason, Message, Commit, PR, Test, Reference, Rationale string }

// at is the value the field called name holds, nil for a name that is not a Close field.
func (v *CloseValues) at(name Field) *string {
	switch name {
	case CloseReason:
		return &v.Reason
	case CloseMessage:
		return &v.Message
	case CloseCommit:
		return &v.Commit
	case ClosePR:
		return &v.PR
	case CloseTest:
		return &v.Test
	case CloseReference:
		return &v.Reference
	case CloseRationale:
		return &v.Rationale
	}
	return nil
}

// Value is what the field called name holds, "" for a name that is not a Close field.
func (v CloseValues) Value(name Field) string {
	if value := v.at(name); value != nil {
		return *value
	}
	return ""
}

type CloseFormState struct {
	Open, Saving bool
	Field        Field
	UID          string
	ProjectID    int64
	Values       CloseValues
	Err          error
}
type CloseFieldMsg struct {
	Field Field
	Value string
}

func (CloseFieldMsg) msg() {}

type CloseResultMsg struct {
	UID string
	Err error
}

func (CloseResultMsg) msg() {}

// CloseReasonKind is why an Issue is closed.
type CloseReasonKind string

const (
	ReasonDone          CloseReasonKind = "done"
	ReasonWontfix       CloseReasonKind = "wontfix"
	ReasonDuplicate     CloseReasonKind = "duplicate"
	ReasonSuperseded    CloseReasonKind = "superseded"
	ReasonAuditNoChange CloseReasonKind = "audit-no-change"
)

// CloseEvidenceKind names what a piece of close evidence is.
type CloseEvidenceKind string

const (
	EvidenceCommit       CloseEvidenceKind = "commit"
	EvidencePR           CloseEvidenceKind = "pr"
	EvidenceTest         CloseEvidenceKind = "test"
	EvidenceDuplicateOf  CloseEvidenceKind = "duplicate-of"
	EvidenceSupersededBy CloseEvidenceKind = "superseded-by"
	EvidenceAudit        CloseEvidenceKind = "no-change-audit"
)

// CloseEvidence is one trimmed evidence entry: a SHA, URL, test command, Issue reference or audit rationale.
type CloseEvidence struct {
	Kind  CloseEvidenceKind
	Value string
}

// CloseIssueCmd describes a close in plain values; the Daemon gateway turns it into kata's request.
// Quick is the owner-local close: reason done, no message, no evidence.
type CloseIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	Quick                 bool
	Reason                CloseReasonKind
	Message               string
	Evidence              []CloseEvidence
}

func (CloseIssueCmd) cmd() {}

type ReopenIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
}

func (ReopenIssueCmd) cmd() {}

func closeIssue(m Model) *Issue {
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

func openClose(m Model) Model {
	if m.Stale {
		return m
	}
	issue := closeIssue(m)
	if issue == nil {
		return m
	}
	m.CloseForm = CloseFormState{Open: true, Field: CloseFields[0].Name, UID: issue.UID, ProjectID: issue.ProjectID}
	m.CloseForm.Values.Reason = string(ReasonDone)
	return m
}

func submitClose(m Model, quick bool) (Model, []Cmd) {
	f := &m.CloseForm
	if !f.Open || f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before closing.")
		return m, nil
	}
	cmd := CloseIssueCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, Quick: quick, Reason: CloseReasonKind(strings.TrimSpace(f.Values.Reason))}
	if quick {
		cmd.Reason = ReasonDone
	} else {
		cmd.Message = strings.TrimSpace(f.Values.Message)
		for _, e := range []CloseEvidence{
			{EvidenceCommit, f.Values.Commit},
			{EvidencePR, f.Values.PR},
			{EvidenceTest, f.Values.Test},
			{referenceKind(cmd.Reason), f.Values.Reference},
			{EvidenceAudit, f.Values.Rationale},
		} {
			if e.Value = strings.TrimSpace(e.Value); e.Value != "" {
				cmd.Evidence = append(cmd.Evidence, e)
			}
		}
		if err := validateClose(cmd); err != nil {
			f.Err = err
			return m, nil
		}
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{cmd}
}

var closeSHA = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func referenceKind(reason CloseReasonKind) CloseEvidenceKind {
	if reason == ReasonSuperseded {
		return EvidenceSupersededBy
	}
	return EvidenceDuplicateOf
}

func validateClose(cmd CloseIssueCmd) error {
	reason := cmd.Reason
	floor := 40
	switch reason {
	case ReasonDone, ReasonAuditNoChange:
	case ReasonWontfix:
		floor = 60
	case ReasonDuplicate, ReasonSuperseded:
		floor = 20
	default:
		return errors.New("Reason must be done, wontfix, duplicate, superseded, or audit-no-change.")
	}
	message := strings.TrimRight(strings.ToLower(strings.Join(strings.Fields(cmd.Message), " ")), ".?!")
	if utf8.RuneCountInString(message) < floor {
		return fmt.Errorf("Explain this closure in at least %d characters.", floor)
	}
	for _, e := range cmd.Evidence {
		if e.Kind == EvidenceCommit && !closeSHA.MatchString(e.Value) {
			return errors.New("Commit must be a 7–40 character hexadecimal SHA.")
		}
	}
	kinds := map[CloseEvidenceKind]int{}
	for _, e := range cmd.Evidence {
		kinds[e.Kind]++
	}
	switch reason {
	case ReasonDone:
		if kinds[EvidenceCommit]+kinds[EvidencePR]+kinds[EvidenceTest] == 0 {
			return errors.New("Done requires commit, PR, or test evidence; use Quick done for owner-local interactive closing.")
		}
		if kinds[EvidenceDuplicateOf]+kinds[EvidenceAudit] > 0 {
			return errors.New("Done accepts commit, PR, or test evidence, not reference or audit evidence.")
		}
	case ReasonDuplicate, ReasonSuperseded:
		kind := referenceKind(reason)
		if len(cmd.Evidence) != 1 || kinds[kind] != 1 {
			return errors.New("Duplicate/superseded requires exactly one Issue reference and no other evidence.")
		}
	case ReasonAuditNoChange:
		if len(cmd.Evidence) != 1 || kinds[EvidenceAudit] != 1 {
			return errors.New("Audit requires an audit rationale and no other evidence.")
		}
	case "wontfix":
		if kinds[EvidenceDuplicateOf]+kinds[EvidenceAudit] > 0 {
			return errors.New("Wontfix does not accept reference or audit evidence.")
		}
	}
	return nil
}

func updateClose(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case CloseFieldMsg:
		if value := m.CloseForm.Values.at(msg.Field); value != nil && m.CloseForm.Open && !m.CloseForm.Saving {
			*value = msg.Value
			m.CloseForm.Err = nil
		}
	case CloseResultMsg:
		if m.CloseForm.UID != msg.UID {
			return m, nil
		}
		m.CloseForm.Saving = false
		m.CloseForm.Err = msg.Err
		if msg.Err == nil {
			m.CloseForm.Open = false
			m = m.issueWritten(msg.UID)
			return snapshotChanged(m)
		}
	}
	return m, nil
}

// closeKey is one key for the Close card on top.
func closeKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.CloseForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeCloseForm)
	switch {
	case bound && binding.Action == CloseCancel:
		if !f.Saving {
			f.Open = false
		}
	case bound && binding.Action == CloseNext:
		cycleField(&m, 1)
	case bound && binding.Action == ClosePrevious:
		cycleField(&m, -1)
	case bound && binding.Action == CloseSave:
		return submitClose(m, false)
	case bound && binding.Action == CloseQuick:
		return submitClose(m, true)
	case !f.Saving:
		if value := f.Values.at(f.Field); value != nil {
			var edited bool
			if *value, edited = editText(*value, binding, bound, msg); edited {
				f.Err = nil
			}
		}
	}
	return m, nil
}
