package model

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The Federation Page is a Lettered Page: it lists this daemon's spoke federations, the Projects that are replicas of a Project
// on a hub, as the daemon's federation status reports them, with a detail for one. It copies kata tui's federation view.

// FederationQuarantine is one active quarantine on a spoke Project.
type FederationQuarantine struct {
	ID                        int64
	Direction                 string
	FirstEventID, LastEventID int64
	CreatedAt                 time.Time
	Error                     string
}

// FederationSpoke is one spoke Project's federation status. A time the daemon did not report is the zero time.
type FederationSpoke struct {
	ProjectID                            int64
	ProjectName                          string
	HubURL, HubProjectUID                string
	HubProjectID                         int64
	BoundActor, Capabilities             string
	PushEnabled, AllowInsecure           bool
	CredentialStatus                     string
	PendingPushCount                     int64
	PendingPushHighWaterEventID          int64
	PullCursorEventID, PushCursorEventID int64
	PendingClaimCount, LiveClaimCount    int64
	ActiveQuarantineCount                int64
	ActiveQuarantines                    []FederationQuarantine
	ResetBlocker                         string
	UnresolvedViolationCount             int64
	RecentViolationCount                 int64
	LastPullSuccessAt, LastPushSuccessAt time.Time
	LastSuccessfulSyncAt, LastErrorAt    time.Time
	LastError                            string
	HasLastError                         bool
}

// LoadFederation asks for the daemon's instance and federation status. Request numbers the asks: an answer to an older one is
// dropped.
type LoadFederation struct {
	Workspace string
	Request   int64
}

func (LoadFederation) cmd() {}

// FederationLoaded answers LoadFederation. Daemon, Endpoint, InstanceUID and Auth are the active daemon as the header names it;
// Spokes are only the status rows whose role is spoke and Bindings every Project that has a federation role, which an enrollment
// reads to refuse a Project that is bound already.
type FederationLoaded struct {
	Request                             int64
	Daemon, Endpoint, InstanceUID, Auth string
	Spokes                              []FederationSpoke
	Bindings                            []FederationBinding
	Err                                 error
}

func (FederationLoaded) msg() {}

// FederationScreen is which of the Federation Page's screens is shown.
type FederationScreen int

const (
	// FederationList is the list of spokes.
	FederationList FederationScreen = iota
	// FederationDetail is one spoke's detail.
	FederationDetail
	// FederationLeavePreview is the preview of leaving one spoke's federation.
	FederationLeavePreview
	// FederationLeaveDone is the result of a leave.
	FederationLeaveDone
	// FederationPickLocal, FederationPickHub and FederationPickHubProject are the enrollment's three choices, in order.
	FederationPickLocal
	FederationPickHub
	FederationPickHubProject
	// FederationBrowse is the read-only list of the hubs' Projects.
	FederationBrowse
	// FederationPreview is the enrollment preview; the typed confirmation of an adoption is a card over it.
	FederationPreview
	// FederationRecovery is what a failure after the hub granted the enrollment leaves.
	FederationRecovery
	// FederationEnrollDone is the result of an enrollment.
	FederationEnrollDone
)

// FederationState is the Federation Page's state. It is not saved and is read again on every entry.
type FederationState struct {
	Screen FederationScreen
	// Loading says a read is out; Err is the last read's failure.
	Loading bool
	Err     error
	// Daemon, Endpoint, InstanceUID and Auth are what the last read said of the active daemon.
	Daemon, Endpoint, InstanceUID, Auth string
	Spokes                              []FederationSpoke
	// Cursor indexes Spokes.
	Cursor int
	// Leave is the leave being previewed or run; Leaving says it runs, LeaveErr is why the last run failed and Left is the result of
	// the last one that succeeded.
	Leave    FederationLeaveDraft
	Leaving  bool
	LeaveErr error
	Left     FederationLeaveOutcome
	// Bindings are the Projects with a federation role, of any role, as the last read said.
	Bindings []FederationBinding
	// Draft is the enrollment being chosen, previewed or run. EnrollErr is the last failure of a choice, a browse or a run;
	// Enrolling says a run is out.
	Draft      FederationEnrollDraft
	EnrollErr  error
	Enrolling  bool
	Enrolled   FederationEnrollOutcome
	Recovery   FederationEnrollRecovery
	LocalCursor, HubCursor, HubProjectCursor int
	// Hubs are the daemons of kata's catalog, read when the hub step opens; HubProjects are the chosen hub's Projects.
	Hubs               []FederationHub
	HubsLoading        bool
	HubProjects        []Project
	HubProjectsLoading bool
	// Browsed are the hubs' Projects of the browse and BrowseCursor indexes their rows; Browsing says the read is out.
	Browsed      []FederationHubProjects
	BrowseCursor int
	Browsing     bool
	// Adopting says the card asking to type the local Project's name is open; AdoptErr is why the last try was refused.
	Adopting   bool
	AdoptInput string
	AdoptCaret Caret
	AdoptErr   error
	// request counts the asks, attempt the leaves and enrollments, and hubRequest the reads of the catalog and the hubs; an
	// answer to another is dropped.
	request, attempt, hubRequest int64
}

// OnFederation says the Federation Page is current.
func (m Model) OnFederation() bool { return m.Page == PageFederation }

// federationMode is the Federation Page's mode: the Menu's while the sidebar has the keyboard.
func federationMode(m Model) Mode {
	if m.Focus == ColumnCollections {
		return ""
	}
	switch m.Federation.Screen {
	case FederationLeavePreview:
		return ModeFederationLeave
	case FederationLeaveDone, FederationEnrollDone:
		return ModeFederationResult
	case FederationPickLocal, FederationPickHub, FederationPickHubProject:
		return ModeFederationPick
	case FederationBrowse:
		return ModeFederationBrowse
	case FederationPreview:
		return ModeFederationPreview
	case FederationRecovery:
		return ModeFederationRecovery
	}
	return ModeFederation
}

// enterFederation starts a fresh reading.
func enterFederation(m Model) (Model, []Cmd) {
	m.Federation = FederationState{request: m.Federation.request, attempt: m.Federation.attempt + 1, hubRequest: m.Federation.hubRequest + 1}
	return askFederation(m)
}

// clearFederation drops what the Page held and every ask still out.
func clearFederation(m Model) Model {
	m.Federation = FederationState{request: m.Federation.request + 1, attempt: m.Federation.attempt + 1, hubRequest: m.Federation.hubRequest + 1}
	return m
}

func askFederation(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.request++
	f.Loading, f.Err = true, nil
	return m, []Cmd{LoadFederation{Workspace: m.Workspace, Request: f.request}}
}

// refreshFederation is r on either screen.
func refreshFederation(m Model) (Model, []Cmd) { return askFederation(m) }

// openFederationDetail is Enter on a spoke.
func openFederationDetail(m Model) (Model, []Cmd) {
	if m.Federation.Screen == FederationList && m.SelectedSpoke() != nil {
		m.Federation.Screen = FederationDetail
	}
	return m, nil
}

// federationBack is esc and backspace: the list from a detail, else the Page shown before.
func federationBack(m Model) (Model, []Cmd) {
	if m.Federation.Screen != FederationList {
		m.Federation.Screen = FederationList
		return m, nil
	}
	return showPage(m.letteredFrom)(m)
}

func updateFederation(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case FederationLeft:
		m, cmds := federationLeft(m, msg)
		return m, cmds, true
	case FederationHubsLoaded:
		return federationHubsLoaded(m, msg), nil, true
	case FederationHubProjectsLoaded:
		return federationHubProjectsLoaded(m, msg), nil, true
	case FederationBrowsed:
		return federationBrowsed(m, msg), nil, true
	case FederationEnrolled:
		m, cmds := federationEnrolled(m, msg)
		return m, cmds, true
	case FederationLoaded:
		f := &m.Federation
		if msg.Request != f.request {
			return m, nil, true
		}
		f.Loading, f.Err = false, msg.Err
		if msg.Err != nil {
			return m, nil, true
		}
		f.Daemon, f.Endpoint, f.InstanceUID, f.Auth = msg.Daemon, msg.Endpoint, msg.InstanceUID, msg.Auth
		f.Spokes = slices.Clone(msg.Spokes)
		f.Bindings = slices.Clone(msg.Bindings)
		f.Cursor = min(f.Cursor, max(len(f.Spokes)-1, 0))
		return m, nil, true
	}
	return m, nil, false
}

// moveFederation moves the cursor by action: one row, to either end, or a page. A detail has no cursor to move.
func moveFederation(m Model, action Action) Model {
	n := len(m.Federation.Spokes)
	if n == 0 || m.Federation.Screen != FederationList {
		return m
	}
	m.Federation.Cursor = movedIndex(m.Federation.Cursor, n, listPage, action)
	return m
}

// SelectedSpoke is the spoke under the cursor, nil when there is none.
func (m Model) SelectedSpoke() *FederationSpoke {
	f := m.Federation
	if f.Cursor < 0 || f.Cursor >= len(f.Spokes) {
		return nil
	}
	return &f.Spokes[f.Cursor]
}

// Header is the line naming the active daemon, as kata tui words it.
func (f FederationState) Header() string {
	return "Federation for active daemon: " + oneLine(f.Daemon) + " " + oneLine(f.Endpoint) + " instance " + federationDash(oneLine(f.InstanceUID)) +
		" auth " + oneLine(f.Auth)
}

// Summary is the line under the title: how many spoke federations the daemon has.
func (f FederationState) Summary() string { return strconv.Itoa(len(f.Spokes)) + " spoke federations" }

// FederationNotice is what stands in place of the list.
type FederationNotice struct {
	// Text is the line; Err says it is a failure, which the view draws as one.
	Text string
	Err  bool
}

// Notice is what stands in place of the list: loading comes first, then a failure, then no spokes; false when the list is shown.
func (f FederationState) Notice() (FederationNotice, bool) {
	switch {
	case f.Loading:
		return FederationNotice{Text: "loading federation status..."}, true
	case f.Err != nil:
		return FederationNotice{Text: "failed to load federation: " + oneLine(f.Err.Error()), Err: true}, true
	case len(f.Spokes) == 0:
		return FederationNotice{Text: "no spoke federation enrollments"}, true
	}
	return FederationNotice{}, false
}

func federationDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func federationTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

// HubHost is the hub as the list shows it: the host of its URL, else the URL, "-" when there is none.
func (s FederationSpoke) HubHost() string {
	if u, err := url.Parse(s.HubURL); err == nil && u.Host != "" {
		return oneLine(u.Host)
	}
	return federationDash(oneLine(s.HubURL))
}

// Push is "push" when the Project pushes to the hub, else "off".
func (s FederationSpoke) Push() string {
	if s.PushEnabled {
		return "push"
	}
	return "off"
}

// Sync is when the Project last synced with its hub: "never", "error" when it never did and a failure is recorded, else how long ago.
func (s FederationSpoke) Sync(now time.Time) string {
	switch {
	case !s.LastSuccessfulSyncAt.IsZero():
		return RelativeTime(s.LastSuccessfulSyncAt, now)
	case s.HasLastError:
		return "error"
	}
	return "never"
}

// Badges are the Project's warnings, comma separated, "-" when it has none.
func (s FederationSpoke) Badges() string {
	var badges []string
	if s.AllowInsecure {
		badges = append(badges, "insecure")
	}
	if s.ActiveQuarantineCount > 0 {
		badges = append(badges, "quarantine")
	}
	if s.ResetBlocker != "" {
		badges = append(badges, "reset")
	}
	if s.UnresolvedViolationCount > 0 {
		badges = append(badges, "violations")
	}
	if len(badges) == 0 {
		return "-"
	}
	return strings.Join(badges, ",")
}

// Lines are a spoke's two lines in the list: the Project and its hub and actor, then push, pending, sync and badges.
func (s FederationSpoke) Lines(now time.Time) [2]string {
	return [2]string{
		oneLine(s.ProjectName) + "  Hub " + s.HubHost() + "  Actor " + federationDash(oneLine(s.BoundActor)),
		"Push " + s.Push() + "  Pending " + strconv.FormatInt(s.PendingPushCount, 10) + "  Sync " + s.Sync(now) + "  Badges " + s.Badges(),
	}
}

// Footer is the line under the list about the selected spoke: its hub URL and Project id, actor and credential.
func (s FederationSpoke) Footer() string {
	return "hub " + oneLine(s.HubURL) + "/project " + strconv.FormatInt(s.HubProjectID, 10) + " · actor " + federationDash(oneLine(s.BoundActor)) +
		" · credential " + federationDash(oneLine(s.CredentialStatus))
}

// DetailLines are a spoke's detail as kata tui words it, one line each; "" is a blank line.
func (s FederationSpoke) DetailLines() []string {
	lines := []string{
		oneLine(s.ProjectName),
		"hub URL: " + oneLine(s.HubURL),
		"hub project ID: " + strconv.FormatInt(s.HubProjectID, 10),
		"hub project UID: " + federationDash(oneLine(s.HubProjectUID)),
		"actor: " + federationDash(oneLine(s.BoundActor)),
		"capabilities: " + oneLine(s.Capabilities),
		"push enabled: " + strconv.FormatBool(s.PushEnabled),
		"credential: " + federationDash(oneLine(s.CredentialStatus)),
		"allow_insecure: " + strconv.FormatBool(s.AllowInsecure),
		"",
		"pull cursor: " + strconv.FormatInt(s.PullCursorEventID, 10),
		"push cursor: " + strconv.FormatInt(s.PushCursorEventID, 10),
		"pending push: " + strconv.FormatInt(s.PendingPushCount, 10),
		"pending push high water: " + strconv.FormatInt(s.PendingPushHighWaterEventID, 10),
		"pending claims: " + strconv.FormatInt(s.PendingClaimCount, 10),
		"live claims: " + strconv.FormatInt(s.LiveClaimCount, 10),
		"quarantine count: " + strconv.FormatInt(s.ActiveQuarantineCount, 10),
	}
	for _, q := range s.ActiveQuarantines {
		lines = append(lines, fmt.Sprintf("quarantine #%d: %s events %d-%d at %s: %s", q.ID, oneLine(q.Direction), q.FirstEventID, q.LastEventID,
			q.CreatedAt.UTC().Format("2006-01-02 15:04:05Z"), oneLine(q.Error)))
	}
	lastError := "-"
	if s.HasLastError {
		lastError = oneLine(s.LastError)
		if !s.LastErrorAt.IsZero() {
			lastError = federationTime(s.LastErrorAt) + " " + lastError
		}
	}
	return append(lines,
		"reset blocker: "+federationDash(oneLine(s.ResetBlocker)),
		fmt.Sprintf("claim violations: %d unresolved, %d recent", s.UnresolvedViolationCount, s.RecentViolationCount),
		"last pull success: "+federationTime(s.LastPullSuccessAt),
		"last push success: "+federationTime(s.LastPushSuccessAt),
		"last sync success: "+federationTime(s.LastSuccessfulSyncAt),
		"last error: "+lastError,
	)
}

// federationRows are the Federation Page's own keys beyond its movement keys.
var federationRows = []actionEntry{
	{action: OpenFederationDetail, modes: []Mode{ModeFederation}, keys: keys("enter", "↵"), help: "detail", footer: []Mode{ModeFederation}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 4, "show the spoke's detail"}}, handle: openFederationDetail},
	{action: StartFederationLeave, modes: []Mode{ModeFederation}, keys: keys("x", "x"), help: "leave", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 6, "leave the selected spoke's federation (preview first)"}}, handle: startFederationLeave},
	{action: RefreshFederation, modes: []Mode{ModeFederation}, keys: keys("r", "r"), help: "refresh", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 5, "read the federation status again"}}, handle: refreshFederation},
	{action: StartFederationEnroll, modes: []Mode{ModeFederation}, keys: keys("n", "n"), help: "enroll", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 20, "enroll a Project in a hub's federation (choose, preview, confirm)"}}, handle: startFederationEnroll},
	{action: StartFederationBrowse, modes: []Mode{ModeFederation}, keys: keys("b", "b"), help: "browse hubs", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 21, "browse the Projects of the catalog's hubs (read only)"}}, handle: startFederationBrowse},
	{action: LetteredBack, modes: []Mode{ModeFederation}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: federationBack},
}
