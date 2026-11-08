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

type FederationQuarantine struct {
	ID                        int64
	Direction                 string
	FirstEventID, LastEventID int64
	CreatedAt                 time.Time
	Error                     string
}

// FederationSpoke is one spoke Project's federation status. A time the daemon did not report is the zero time.
type FederationSpoke struct {
	ProjectID             int64
	ProjectName           string
	HubURL, HubProjectUID string
	// SpokeName is the name the hub gives this instance, "" when it gives none or could not be read.
	SpokeName                            string
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
	// Embedding is the spoke's embedding status as kata words it, one field per line; empty when the daemon reports none.
	Embedding []string
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

type FederationScreen int

const (
	FederationList FederationScreen = iota
	FederationDetail
	FederationLeavePreview
	FederationLeaveDone
	// FederationPickLocal, FederationPickHub and FederationPickHubProject are the enrollment's three choices, in order.
	FederationPickLocal
	FederationPickHub
	FederationPickHubProject
	FederationBrowse
	FederationPreview
	// FederationRecovery is what a failure after the hub granted the enrollment leaves.
	FederationRecovery
	FederationEnrollDone
)

// FederationState is the Federation Page's state. It is not saved and is read again on every entry.
type FederationState struct {
	Screen                              FederationScreen
	Loading                             bool
	Err                                 error
	Daemon, Endpoint, InstanceUID, Auth string
	Spokes                              []FederationSpoke
	Cursor                              int
	// Leave is the leave being previewed or run; Leaving says it runs, LeaveErr is why the last run failed and Left is the result of
	// the last one that succeeded.
	Leave    FederationLeaveDraft
	Leaving  bool
	LeaveErr error
	Left     FederationLeaveOutcome
	Bindings []FederationBinding
	// Draft is the enrollment being chosen, previewed or run. EnrollErr is the last failure of a choice, a browse or a run;
	// Enrolling says a run is out.
	Draft                                    FederationEnrollDraft
	EnrollErr                                error
	Enrolling                                bool
	Enrolled                                 FederationEnrollOutcome
	Recovery                                 FederationEnrollRecovery
	LocalCursor, HubCursor, HubProjectCursor int
	Hubs                                     []FederationHub
	HubsLoading                              bool
	HubProjects                              []Project
	HubProjectsLoading                       bool
	Browsed                                  []FederationHubProjects
	BrowseCursor                             int
	Browsing                                 bool
	Adopting                                 bool
	AdoptInput                               string
	AdoptCaret                               Caret
	AdoptErr                                 error
	Name                                     FederationName
	// reads numbers the asks, attempts the leaves and enrollments, and hubReads the reads of the catalog and the hubs; an
	// answer to another is dropped.
	reads, attempts, hubReads Latest
}

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

func enterFederation(m Model) (Model, []Cmd) {
	f := m.Federation
	m.Federation = FederationState{reads: f.reads, attempts: f.attempts.Discarded(), hubReads: f.hubReads.Discarded()}
	return askFederation(m)
}

func clearFederation(m Model) Model {
	f := m.Federation
	m.Federation = FederationState{reads: f.reads.Discarded(), attempts: f.attempts.Discarded(), hubReads: f.hubReads.Discarded()}
	return m
}

func askFederation(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.Loading, f.Err = true, nil
	return m, []Cmd{LoadFederation{Workspace: m.Workspace, Request: f.reads.Ask()}}
}

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
	case FederationSpokeNamed:
		m, cmds := federationSpokeNamed(m, msg)
		return m, cmds, true
	case FederationLoaded:
		f := &m.Federation
		if !f.reads.IsCurrent(msg.Request) {
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

func moveFederation(m Model, action Action) Model {
	if m.Federation.Screen == FederationList {
		moveCursor(&m.Federation.Cursor, len(m.Federation.Spokes), action)
	}
	return m
}

func (m Model) SelectedSpoke() *FederationSpoke {
	return rowAt(m.Federation.Spokes, m.Federation.Cursor)
}

// Header is the line naming the active daemon, as kata tui words it.
func (f FederationState) Header() string {
	return "Federation for active daemon: " + oneLine(f.Daemon) + " " + oneLine(f.Endpoint) + " instance " + dashLine(f.InstanceUID) +
		" auth " + oneLine(f.Auth)
}

func (f FederationState) Summary() string { return strconv.Itoa(len(f.Spokes)) + " spoke federations" }

type FederationNotice struct {
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

func dashLine(s string) string { return dashIfEmpty(oneLine(s)) }

func dashIfEmpty(s string) string {
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
	return dashLine(s.HubURL)
}

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

func (s FederationSpoke) Lines(now time.Time) [2]string {
	return [2]string{
		oneLine(s.ProjectName) + s.nameWord() + "  Hub " + s.HubHost() + "  Actor " + dashLine(s.BoundActor),
		"Push " + s.Push() + "  Pending " + strconv.FormatInt(s.PendingPushCount, 10) + "  Sync " + s.Sync(now) + "  Badges " + s.Badges(),
	}
}

func (s FederationSpoke) nameWord() string {
	if s.SpokeName == "" {
		return ""
	}
	return "  Name " + oneLine(s.SpokeName)
}

func (s FederationSpoke) Footer() string {
	return "hub " + oneLine(s.HubURL) + "/project " + strconv.FormatInt(s.HubProjectID, 10) + " · actor " + dashLine(s.BoundActor) +
		" · credential " + dashLine(s.CredentialStatus)
}

func (s FederationSpoke) DetailLines() []string {
	var lines []string
	for _, field := range s.Embedding {
		lines = append(lines, oneLine(field))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines,
		oneLine(s.ProjectName),
		"spoke name: "+s.SpokeLabel(),
		"hub URL: "+oneLine(s.HubURL),
		"hub project ID: "+strconv.FormatInt(s.HubProjectID, 10),
		"hub project UID: "+dashLine(s.HubProjectUID),
		"actor: "+dashLine(s.BoundActor),
		"capabilities: "+oneLine(s.Capabilities),
		"push enabled: "+strconv.FormatBool(s.PushEnabled),
		"credential: "+dashLine(s.CredentialStatus),
		"allow_insecure: "+strconv.FormatBool(s.AllowInsecure),
		"",
		"pull cursor: "+strconv.FormatInt(s.PullCursorEventID, 10),
		"push cursor: "+strconv.FormatInt(s.PushCursorEventID, 10),
		"pending push: "+strconv.FormatInt(s.PendingPushCount, 10),
		"pending push high water: "+strconv.FormatInt(s.PendingPushHighWaterEventID, 10),
		"pending claims: "+strconv.FormatInt(s.PendingClaimCount, 10),
		"live claims: "+strconv.FormatInt(s.LiveClaimCount, 10),
		"quarantine count: "+strconv.FormatInt(s.ActiveQuarantineCount, 10),
	)
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
		"reset blocker: "+dashLine(s.ResetBlocker),
		fmt.Sprintf("claim violations: %d unresolved, %d recent", s.UnresolvedViolationCount, s.RecentViolationCount),
		"last pull success: "+federationTime(s.LastPullSuccessAt),
		"last push success: "+federationTime(s.LastPushSuccessAt),
		"last sync success: "+federationTime(s.LastSuccessfulSyncAt),
		"last error: "+lastError,
	)
}

var federationRows = []actionEntry{
	{action: OpenFederationDetail, modes: []Mode{ModeFederation}, keys: keys("enter", "↵"), help: "detail", footer: []Mode{ModeFederation}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 4, "show the spoke's detail"}}, handle: openFederationDetail},
	{action: StartFederationLeave, modes: []Mode{ModeFederation}, keys: keys("x", "x"), help: "leave", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 6, "leave the selected spoke's federation (preview first)"}}, handle: startFederationLeave},
	{action: RefreshFederation, modes: []Mode{ModeFederation}, keys: keys("r", "r"), help: "refresh", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 5, "read the federation status again"}}, handle: askFederation},
	{action: NameFederationSpoke, modes: []Mode{ModeFederation}, keys: keys("shift+n", "N"), help: "name spoke", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 32, "name the selected spoke, or clear its name with an empty one (kept by the hub)"}}, handle: openFederationNameForSpoke},
	{action: StartFederationEnroll, modes: []Mode{ModeFederation}, keys: keys("n", "n"), help: "enroll", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 30, "enroll a Project in a hub's federation (choose, preview, confirm)"}}, handle: startFederationEnroll},
	{action: StartFederationBrowse, modes: []Mode{ModeFederation}, keys: keys("b", "b"), help: "browse hubs", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 31, "browse the Projects of the catalog's hubs (read only)"}}, handle: startFederationBrowse},
	letteredBackRow(ModeFederation, federationBack),
}
