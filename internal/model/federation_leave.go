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
	// ProjectID is the spoke Project on this daemon; HubURL and HubProjectID name what it replicates.
	ProjectID    int64
	ProjectName  string
	HubURL       string
	HubProjectID int64
	// InstanceUID is this daemon's instance, which the hub's enrollments for this spoke carry.
	InstanceUID   string
	AllowInsecure bool
	// Archive archives the replica instead of detaching it, which keeps the Project standalone with all its Issues.
	Archive bool
	Actor   string
	// LocalOnly skips the hub's revoke and leaves the enrollment token valid.
	LocalOnly bool
}

// Disposition is how kata names what becomes of the local Project: detach or archive.
func (d FederationLeaveDraft) Disposition() string {
	if d.Archive {
		return "archive"
	}
	return "detach"
}

// FederationLeaveOutcome is what a successful leave did.
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

// startFederationLeave is x on a spoke, from the list or its detail: the preview.
func startFederationLeave(m Model) (Model, []Cmd) {
	spoke := m.SelectedSpoke()
	if spoke == nil {
		return m, nil
	}
	f := &m.Federation
	f.attempt++
	f.Leaving, f.LeaveErr = false, nil
	f.Leave = FederationLeaveDraft{ProjectID: spoke.ProjectID, ProjectName: spoke.ProjectName, HubURL: spoke.HubURL, HubProjectID: spoke.HubProjectID,
		InstanceUID: strings.TrimSpace(f.InstanceUID), AllowInsecure: spoke.AllowInsecure, Actor: m.Actor}
	f.Screen = FederationLeavePreview
	return m, nil
}

// toggleFederationArchive is d: between detaching and archiving.
func toggleFederationArchive(m Model) (Model, []Cmd) {
	m.Federation.Leave.Archive = !m.Federation.Leave.Archive
	return m, nil
}

// toggleFederationLocalOnly is l.
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
	f.attempt++
	f.Leaving, f.LeaveErr = true, nil
	return m, []Cmd{LeaveFederation{Workspace: m.Workspace, Attempt: f.attempt, Draft: f.Leave}}
}

// cancelFederationLeave is esc and backspace in the preview: back to the list, dropping a run still out.
func cancelFederationLeave(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.attempt++
	f.Leaving, f.LeaveErr = false, nil
	f.Screen = FederationList
	return m, nil
}

// dismissFederationLeave is Enter, esc and backspace on the result: back to the list.
func dismissFederationLeave(m Model) (Model, []Cmd) {
	m.Federation.Screen = FederationList
	return m, nil
}

// federationLeft lands a run's answer. A failure keeps the preview with the error under it; a success shows the result and reads the
// status again, which no longer lists the Project.
func federationLeft(m Model, msg FederationLeft) (Model, []Cmd) {
	f := &m.Federation
	if msg.Attempt != f.attempt || !f.Leaving {
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

// PreviewLines are the leave preview's lines as kata tui words them; "" is a blank line. Warning is the index of the line that warns,
// -1 when none does. The error of a failed run is not among them.
func (d FederationLeaveDraft) PreviewLines() (lines []string, warning int) {
	hubProject := strconv.FormatInt(d.HubProjectID, 10)
	local := "keep (detach) — standalone with all issues"
	teardown := "detach locally (keep the project standalone)"
	if d.Archive {
		local = "archive — remove the replica (reversible via kata projects restore)"
		teardown = "archive the local replica"
	}
	lines = []string{
		"local spoke project: " + federationDash(oneLine(d.ProjectName)),
		"hub: " + federationHub(d.HubURL),
		"hub project ID: " + hubProject,
		"local project: " + local,
		"local-only: " + strconv.FormatBool(d.LocalOnly),
		"",
	}
	if d.LocalOnly {
		return append(lines, "local-only: skipping hub revoke; the enrollment token stays valid until manually revoked", "local teardown only: "+teardown), len(lines)
	}
	return append(lines, "will revoke the enrollment(s) on the hub, then "+teardown), -1
}

// ResultLines are the leave result's lines as kata tui words them. Warning is the index of the line that warns, -1 when none does.
func (o FederationLeaveOutcome) ResultLines() (lines []string, warning int) {
	status := "detached"
	if o.Archived {
		status = "archived"
	}
	revoke := "revoked " + strconv.Itoa(o.Revoked) + " enrollment(s) on the hub"
	if o.SkippedRevoke {
		revoke = "hub revoke skipped (local-only): the enrollment token remains valid until manually revoked"
	} else if o.ProviderReleased {
		revoke = "credential provider confirmed release"
	}
	lines = []string{"status: " + status, "local spoke project: " + federationDash(oneLine(o.Draft.ProjectName)), "hub: " + federationHub(o.Draft.HubURL), revoke}
	warning = -1
	if len(o.GlobalEnrollmentIDs) > 0 {
		ids := make([]string, len(o.GlobalEnrollmentIDs))
		for i, id := range o.GlobalEnrollmentIDs {
			ids[i] = "#" + strconv.FormatInt(id, 10)
		}
		warning = len(lines)
		lines = append(lines, "warning: global enrollment(s) "+strings.Join(ids, ", ")+" for this spoke remain active and still authorize this project; revoke manually if intended")
	}
	return lines, warning
}

// federationHub is a hub as the leave screens show it: the host of its URL, else the URL, "-" when there is none.
func federationHub(raw string) string { return FederationSpoke{HubURL: raw}.HubHost() }

// federationLeaveRows are the leave preview's and its result's keys.
var federationLeaveRows = []actionEntry{
	{action: RunFederationLeave, modes: []Mode{ModeFederationLeave}, keys: keys("enter", "↵"), help: "confirm", footer: []Mode{ModeFederationLeave}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 7, "run the leave in its preview"}}, handle: runFederationLeave},
	{action: ToggleFederationArchive, modes: []Mode{ModeFederationLeave}, keys: keys("d", "d"), help: "keep / archive", footer: []Mode{ModeFederationLeave}, sheet: []sheetLine{{sheetLettered, 8, "switch the leave between detaching and archiving"}}, handle: toggleFederationArchive},
	{action: ToggleFederationLocal, modes: []Mode{ModeFederationLeave}, keys: keys("l", "l"), help: "local-only", footer: []Mode{ModeFederationLeave}, sheet: []sheetLine{{sheetLettered, 9, "leave without revoking on the hub"}}, handle: toggleFederationLocalOnly},
	{action: CancelFederationLeave, modes: []Mode{ModeFederationLeave}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationLeave}, handle: cancelFederationLeave},
	{action: FederationLeaveDismiss, modes: []Mode{ModeFederationLeft}, keys: keys("enter", "↵", "escape", "esc", "backspace", "⌫"), help: "list", footer: []Mode{ModeFederationLeft}, footerKey: "↵", handle: dismissFederationLeave},
}
