package model

import (
	"slices"
	"strconv"
	"strings"
)

// Leaving a federation copies kata tui's leave: `x` on a spoke opens a preview, `d` and `l` choose what the leave does, Enter runs it
// and a result follows. The Daemon gateway does the three steps (leave preflight, the hub's revoke, the leave itself).

// FederationLeaveDraft is one spoke's leave as the preview shows it, set when `x` is pressed.
type FederationLeaveDraft struct {
	ProjectID    int64
	ProjectName  string
	HubURL       string
	HubProjectID int64
	// InstanceUID is this daemon's instance, which the hub's enrollments for this spoke carry.
	InstanceUID   string
	AllowInsecure bool
	Archive       bool
	Actor         string
	LocalOnly     bool
}

func (d FederationLeaveDraft) Disposition() string {
	if d.Archive {
		return "archive"
	}
	return "detach"
}

type FederationLeaveOutcome struct {
	Draft    FederationLeaveDraft
	Archived bool
	// Revoked counts the enrollments revoked on the hub; SkippedRevoke says a local-only leave revoked none, ProviderReleased that
	// the hub's credential provider confirmed the release in their place.
	Revoked          int
	SkippedRevoke    bool
	ProviderReleased bool
	// GlobalEnrollmentIDs are this spoke's hub enrollments that are not limited to a Project: they still authorize the left Project
	// and are not revoked, since they may serve other Projects.
	GlobalEnrollmentIDs []int64
}

// LeaveFederation runs a leave. Attempt numbers the runs: an answer to an older one is dropped.
type LeaveFederation struct {
	Workspace string
	Attempt   int64
	Draft     FederationLeaveDraft
}

func (LeaveFederation) cmd() {}

// FederationLeft answers LeaveFederation: the outcome, or kata tui's error for the step that failed.
type FederationLeft struct {
	Attempt int64
	Outcome FederationLeaveOutcome
	Err     error
}

func (FederationLeft) msg() {}

func startFederationLeave(m Model) (Model, []Cmd) {
	spoke := m.SelectedSpoke()
	if spoke == nil {
		return m, nil
	}
	f := &m.Federation
	f.attempts.Discard()
	f.Leaving, f.LeaveErr = false, nil
	f.Leave = FederationLeaveDraft{ProjectID: spoke.ProjectID, ProjectName: spoke.ProjectName, HubURL: spoke.HubURL, HubProjectID: spoke.HubProjectID,
		InstanceUID: strings.TrimSpace(f.InstanceUID), AllowInsecure: spoke.AllowInsecure, Actor: m.Actor}
	f.Screen = FederationLeavePreview
	return m, nil
}

func toggleFederationArchive(m Model) (Model, []Cmd) {
	m.Federation.Leave.Archive = !m.Federation.Leave.Archive
	return m, nil
}

func toggleFederationLocalOnly(m Model) (Model, []Cmd) {
	m.Federation.Leave.LocalOnly = !m.Federation.Leave.LocalOnly
	return m, nil
}

// runFederationLeave is Enter in the preview. One run is out at a time.
func runFederationLeave(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Leaving {
		return m, nil
	}
	if m.Daemons.Switching != "" {
		f.LeaveErr = errDaemonSwitching
		return m, nil
	}
	f.Leaving, f.LeaveErr = true, nil
	return m, []Cmd{LeaveFederation{Workspace: m.Workspace, Attempt: f.attempts.Ask(), Draft: f.Leave}}
}

// cancelFederationLeave is esc and backspace in the preview: back to the list, dropping a run still out.
func cancelFederationLeave(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.attempts.Discard()
	f.Leaving, f.LeaveErr = false, nil
	f.Screen = FederationList
	return m, nil
}

func dismissFederationLeave(m Model) (Model, []Cmd) {
	m.Federation.Screen = FederationList
	return m, nil
}

// federationLeft lands a run's answer. A failure keeps the preview with the error under it; a success shows the result and reads the
// status again, which no longer lists the Project.
func federationLeft(m Model, msg FederationLeft) (Model, []Cmd) {
	f := &m.Federation
	if !f.attempts.IsCurrent(msg.Attempt) || !f.Leaving {
		// A leave dropped by esc still ran on the daemon: a success changes no screen but reads the status, so the left spoke is not listed.
		// A newer run in progress reads when it answers.
		if msg.Err == nil && m.OnFederation() && !f.Leaving {
			return askFederation(m)
		}
		return m, nil
	}
	f.Leaving = false
	if msg.Err != nil {
		f.LeaveErr = msg.Err
		return m, nil
	}
	f.Left = msg.Outcome
	f.Left.GlobalEnrollmentIDs = slices.Clone(msg.Outcome.GlobalEnrollmentIDs)
	f.Screen = FederationLeaveDone
	return askFederation(m)
}

// PreviewRows are the leave preview as label→value rows, then the sentence that says what running it does, in the warning tone when
// the hub revoke is skipped. The error of a failed run is not among them.
func (d FederationLeaveDraft) PreviewRows() []FactRow {
	local := "Stays as a standalone Project with all its Issues"
	teardown := "detaches the Project locally"
	if d.Archive {
		local = "Archived; restore it with kata projects restore"
		teardown = "archives the local replica"
	}
	revoke := "Revoked on the hub"
	if d.LocalOnly {
		revoke = "Skipped (local only)"
	}
	rows := []FactRow{
		{Key: "Project", Value: dashLine(d.ProjectName)},
		{Key: "Hub", Value: FederationSpoke{HubURL: d.HubURL}.HubHost(), Mono: true},
		{Key: "Hub Project", Value: strconv.FormatInt(d.HubProjectID, 10), Mono: true},
		{Key: "Local copy", Value: local},
		{Key: "Hub revoke", Value: revoke},
		{},
	}
	if d.LocalOnly {
		return append(rows, FactRow{Value: "The hub revoke is skipped: the enrollment token stays valid until you revoke it by hand, and the leave only " + teardown + ".", Tone: FactWarning})
	}
	return append(rows, FactRow{Value: "Revokes the enrollment on the hub, then " + teardown + "."})
}

// ResultRows are the leave result as label→value rows, then a warning sentence when a global enrollment still authorizes the left
// Project.
func (o FederationLeaveOutcome) ResultRows() []FactRow {
	status := "Detached"
	if o.Archived {
		status = "Archived"
	}
	revoke := "Revoked " + Count(o.Revoked, "enrollment", "") + " on the hub"
	if o.SkippedRevoke {
		revoke = "Skipped (local only); the enrollment token stays valid until you revoke it"
	} else if o.ProviderReleased {
		revoke = "The credential provider confirmed the release"
	}
	rows := []FactRow{
		{Key: "Result", Value: status},
		{Key: "Project", Value: dashLine(o.Draft.ProjectName)},
		{Key: "Hub", Value: FederationSpoke{HubURL: o.Draft.HubURL}.HubHost(), Mono: true},
		{Key: "Hub revoke", Value: revoke},
	}
	if len(o.GlobalEnrollmentIDs) > 0 {
		ids := make([]string, len(o.GlobalEnrollmentIDs))
		for i, id := range o.GlobalEnrollmentIDs {
			ids[i] = "#" + strconv.FormatInt(id, 10)
		}
		warning := "Global enrollments " + strings.Join(ids, ", ") + " for this spoke are still active and still authorize this Project; revoke them by hand if you mean to."
		if len(ids) == 1 {
			warning = "Global enrollment " + ids[0] + " for this spoke is still active and still authorizes this Project; revoke it by hand if you mean to."
		}
		rows = append(rows, FactRow{Value: warning, Tone: FactWarning})
	}
	return rows
}

var federationLeaveRows = []actionEntry{
	{action: RunFederationLeave, modes: []Mode{ModeFederationLeave}, keys: keys("enter", "enter"), help: "confirm", footer: []Mode{ModeFederationLeave}, footerKey: "enter", sheet: []sheetLine{{sheetLettered, 7, "Run the leave"}}, handle: runFederationLeave},
	{action: ToggleFederationArchive, modes: []Mode{ModeFederationLeave}, keys: keys("d", "d"), help: "keep / archive", footer: []Mode{ModeFederationLeave}, sheet: []sheetLine{{sheetLettered, 8, "Switch between keeping and archiving"}}, handle: toggleFederationArchive},
	{action: ToggleFederationLocal, modes: []Mode{ModeFederationLeave}, keys: keys("l", "l"), help: "local-only", footer: []Mode{ModeFederationLeave}, sheet: []sheetLine{{sheetLettered, 9, "Leave without revoking on the hub"}}, handle: toggleFederationLocalOnly},
	{action: CancelFederationLeave, modes: []Mode{ModeFederationLeave}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationLeave}, handle: cancelFederationLeave},
	{action: FederationLeaveDismiss, modes: []Mode{ModeFederationResult}, keys: keys("enter", "enter", "escape", "esc", "backspace", "⌫"), help: "list", footer: []Mode{ModeFederationResult}, footerKey: "enter", handle: dismissFederationLeave},
}
