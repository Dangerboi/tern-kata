package model

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const ModeCloseForm Mode = "close-form"
const (
	OpenClose    Action = "close-issue"
	ReopenAction Action = "reopen-issue"
	CloseSave    Action = "close-save"
	CloseQuick   Action = "close-quick"
	CloseCancel  Action = "close-cancel"
)

const (
	CloseReason      Field = "reason"
	CloseMessage     Field = "message"
	CloseCommit      Field = "commit"
	ClosePR          Field = "pr"
	CloseTest        Field = "test"
	CloseReference   Field = "reference"
	CloseRationale   Field = "rationale"
	CloseReviewed    Field = "reviewed"
	CloseExternal    Field = "external"
	CloseAttachments Field = "attachments"
)

var closeTable = fieldTable[CloseFormState]{
	form: func(m *Model) *CloseFormState { return &m.CloseForm },
	fields: []formField[CloseFormState]{
		textField(CloseReason, func(f *CloseFormState) *string { return &f.Values.Reason }),
		textField(CloseMessage, func(f *CloseFormState) *string { return &f.Values.Message }),
		textField(CloseCommit, func(f *CloseFormState) *string { return &f.Values.Commit }),
		textField(ClosePR, func(f *CloseFormState) *string { return &f.Values.PR }),
		textField(CloseTest, func(f *CloseFormState) *string { return &f.Values.Test }),
		textField(CloseReviewed, func(f *CloseFormState) *string { return &f.Values.Reviewed }),
		textField(CloseExternal, func(f *CloseFormState) *string { return &f.Values.External }),
		closeAttachmentsField(),
		textField(CloseReference, func(f *CloseFormState) *string { return &f.Values.Reference }),
		textField(CloseRationale, func(f *CloseFormState) *string { return &f.Values.Rationale }),
	},
	hidden: func(m Model, name Field) bool { return name == CloseAttachments && !m.CloseForm.ShowsAttachments() },
}

// closeAttachmentsField is the picker of the Issue's Attachments. It takes several values at once: left and right (or the
// click on a name) move the cursor and space toggles the Attachment under it.
func closeAttachmentsField() formField[CloseFormState] {
	field := choiceField(CloseAttachments,
		func(f *CloseFormState, back bool) {
			f.AttachmentAt = cycleIndex(f.AttachmentAt, len(f.Attachments), back)
		},
		toggleCited)
	field.empty = func(f *CloseFormState) bool { return len(f.Attachments) == 0 }
	return field
}

func toggleCited(f *CloseFormState, i int) bool {
	if i < 0 || i >= len(f.Attachments) {
		return false
	}
	f.AttachmentAt = i
	id := f.Attachments[i].ID
	if f.Cites(id) {
		f.Cited = slices.DeleteFunc(slices.Clone(f.Cited), func(c string) bool { return c == id })
	} else {
		f.Cited = append(slices.Clone(f.Cited), id)
	}
	return true
}

func (f CloseFormState) Cites(id string) bool { return slices.Contains(f.Cited, id) }

// ShowsAttachments says whether the card offers the Attachment picker: only a done close takes an Attachment as evidence, and
// an Issue with none has nothing to pick.
func (f CloseFormState) ShowsAttachments() bool {
	return len(f.Attachments) > 0 && strings.TrimSpace(f.Values.Reason) == string(ReasonDone)
}

func (f CloseFormState) citedEvidence() []CloseEvidence {
	var out []CloseEvidence
	for _, a := range f.Attachments {
		if f.Cites(a.ID) {
			out = append(out, CloseEvidence{EvidenceAttachment, a.ID})
		}
	}
	return out
}

var CloseFields = closeTable.specs()

type CloseValues struct{ Reason, Message, Commit, PR, Test, Reviewed, External, Reference, Rationale string }

func (v CloseValues) Value(name Field) string {
	return closeTable.value(CloseFormState{Values: v}, name)
}

type CloseFormState struct {
	formCard
	UID       string
	ProjectID int64
	Values    CloseValues
	Keyed
	// UndoEntry is the Undo history entry whose inverse this card completes, 0 for a plain close.
	UndoEntry uint64
	// Attachments are the Issue's Attachments when the card opened, the ones the picker offers. Cited holds the ids of those
	// picked; AttachmentAt is the picker's cursor, an index into Attachments.
	Attachments  []Attachment
	Cited        []string
	AttachmentAt int
}
type CloseFieldMsg struct {
	Field Field
	Value string
}

func (CloseFieldMsg) msg() {}

type CloseResultMsg struct {
	UID    string
	Err    error
	Record WriteRecord
}

func (CloseResultMsg) msg() {}

type CloseReasonKind string

const (
	ReasonDone          CloseReasonKind = "done"
	ReasonWontfix       CloseReasonKind = "wontfix"
	ReasonDuplicate     CloseReasonKind = "duplicate"
	ReasonSuperseded    CloseReasonKind = "superseded"
	ReasonAuditNoChange CloseReasonKind = "audit-no-change"
)

type CloseEvidenceKind string

const (
	EvidenceCommit       CloseEvidenceKind = "commit"
	EvidencePR           CloseEvidenceKind = "pr"
	EvidenceTest         CloseEvidenceKind = "test"
	EvidenceDuplicateOf  CloseEvidenceKind = "duplicate-of"
	EvidenceSupersededBy CloseEvidenceKind = "superseded-by"
	EvidenceAudit        CloseEvidenceKind = "no-change-audit"
	EvidenceReviewed     CloseEvidenceKind = "reviewed-paths"
	EvidenceExternal     CloseEvidenceKind = "external"
	EvidenceAttachment   CloseEvidenceKind = "attachment"
)

type CloseEvidence struct {
	Kind  CloseEvidenceKind
	Value string
}

type CloseIssueCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	// Quick is the owner-local close: reason done, no message, no evidence.
	Quick    bool
	Reason   CloseReasonKind
	Message  string
	Evidence []CloseEvidence
	// Key is kata's Idempotency-Key for this close.
	Key string
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
	if reason := IssueRefusal(*issue, OpenClose); reason != "" {
		m.Notice = reason
		return m
	}
	m.CloseForm = CloseFormState{Open: true, Field: CloseFields[0].Name, UID: issue.UID, ProjectID: issue.ProjectID, Attachments: Attachments(issue.Metadata)}
	m.CloseForm.Values.Reason = string(ReasonDone)
	return m
}

func submitClose(m Model, quick bool) (Model, []Cmd) {
	f := &m.CloseForm
	if f.refuseSubmit(m, "closing") {
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
			{EvidenceReviewed, f.Values.Reviewed},
			{EvidenceExternal, f.Values.External},
			{referenceKind(cmd.Reason), f.Values.Reference},
			{EvidenceAudit, f.Values.Rationale},
		} {
			if e.Value = strings.TrimSpace(e.Value); e.Value != "" {
				cmd.Evidence = append(cmd.Evidence, e)
			}
		}
		if cmd.Reason == ReasonDone {
			cmd.Evidence = append(cmd.Evidence, f.citedEvidence()...)
		}
		if err := validateClose(cmd); err != nil {
			f.Err = err
			return m, nil
		}
	}
	var keyed Keyed
	m, keyed = m.mintKey(f.Keyed, cmd)
	f.Keyed, cmd.Key = keyed, keyed.Key
	f.Saving, f.Err = true, nil
	if f.UndoEntry != 0 {
		return submitUndoClose(m, cmd, quick)
	}
	return m, []Cmd{cmd}
}

func submitUndoClose(m Model, cmd CloseIssueCmd, quick bool) (Model, []Cmd) {
	f := &m.CloseForm
	entry, found := m.Undo.entry(f.UndoEntry)
	switch {
	case quick || cmd.Reason != ReasonDone:
		f.Saving, f.Err = false, errors.New("Undo completes the Issue as done, with evidence.")
	case !found:
		f.Saving, f.Err = false, errors.New("undo action is no longer available")
	default:
		return m.sendUndo(entry, &cmd)
	}
	return m, nil
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
		if e.Kind == EvidenceReviewed && len(ReviewedPaths(e.Value)) == 0 {
			return errors.New("Reviewed paths need at least one path.")
		}
	}
	if reason != ReasonDone && slices.ContainsFunc(cmd.Evidence, func(e CloseEvidence) bool { return e.Kind == EvidenceAttachment }) {
		return errors.New("Only a done close takes an attachment as evidence.")
	}
	kinds := map[CloseEvidenceKind]int{}
	for _, e := range cmd.Evidence {
		kinds[e.Kind]++
	}
	switch reason {
	case ReasonDone:
		if kinds[EvidenceCommit]+kinds[EvidencePR]+kinds[EvidenceTest]+kinds[EvidenceReviewed]+kinds[EvidenceExternal]+kinds[EvidenceAttachment] == 0 {
			return errors.New("Done requires commit, PR, test, reviewed paths, external or attachment evidence; use Quick done for owner-local interactive closing.")
		}
		if kinds[EvidenceDuplicateOf]+kinds[EvidenceAudit] > 0 {
			return errors.New("Done accepts commit, PR, test, reviewed paths, external or attachment evidence, not reference or audit evidence.")
		}
	case ReasonDuplicate, ReasonSuperseded:
		kind := referenceKind(reason)
		if len(cmd.Evidence) != 1 || kinds[kind] != 1 {
			return errors.New("Duplicate/superseded requires exactly one Issue reference and no other evidence.")
		}
	case ReasonAuditNoChange:
		if len(cmd.Evidence) != 1+kinds[EvidenceReviewed] || kinds[EvidenceAudit] != 1 {
			return errors.New("Audit requires an audit rationale, optionally with reviewed paths, and no other evidence.")
		}
	case "wontfix":
		if kinds[EvidenceDuplicateOf]+kinds[EvidenceAudit] > 0 {
			return errors.New("Wontfix does not accept reference or audit evidence.")
		}
	}
	return nil
}

func ReviewedPaths(value string) []string {
	var paths []string
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

func updateClose(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case CloseFieldMsg:
		if value := closeTable.textAt(&m.CloseForm, msg.Field); value != nil && m.CloseForm.Open && !m.CloseForm.Saving {
			*value = msg.Value
			m.CloseForm.Caret = Caret{}
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

func closeKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.CloseForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeCloseForm)
	switch {
	case bound && binding.Action == CloseCancel:
		f.Open = false
	case bound && binding.Action == CloseSave:
		return submitClose(m, false)
	case bound && binding.Action == CloseQuick:
		return submitClose(m, true)
	case bound && binding.Action == InputSpace && f.Field == CloseAttachments:
		if toggleCited(f, f.AttachmentAt) {
			f.Err = nil
		}
	default:
		closeTable.typeKey(&m, binding, bound, msg)
	}
	return m, nil
}
