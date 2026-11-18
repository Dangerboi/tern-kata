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

// Facts are the Page head's facts after its title: the spoke count (on the list only), the active daemon and how it is
// authenticated. The daemon's endpoint is named only when it differs from its name, and its instance uid not at all.
func (f FederationState) Facts() []string {
	var facts []string
	if f.Screen == FederationList && len(f.Spokes) > 0 {
		facts = append(facts, strconv.Itoa(len(f.Spokes)))
	}
	daemon, endpoint := oneLine(f.Daemon), oneLine(f.Endpoint)
	if daemon != "" {
		facts = append(facts, daemon)
	}
	if endpoint != "" && endpoint != daemon {
		facts = append(facts, endpoint)
	}
	if auth := oneLine(f.Auth); auth != "" {
		facts = append(facts, "auth "+auth)
	}
	return facts
}

// FederationNotice is what stands in place of the list. Err says a failure, Empty that there is no spoke (the view draws an empty
// state) and Pending that the text is a line of progress.
type FederationNotice struct {
	Text                string
	Err, Empty, Pending bool
}

// Notice is what stands in place of the list: loading comes first, then a failure, then no spokes; false when the list is shown.
func (f FederationState) Notice() (FederationNotice, bool) {
	switch {
	case f.Loading:
		return FederationNotice{Text: Progress("Loading", "federation status"), Pending: true}, true
	case f.Err != nil:
		return FederationNotice{Text: Cannot("read", "federation status", f.Err), Err: true}, true
	case len(f.Spokes) == 0:
		return FederationNotice{Text: "No spoke federations", Empty: true}, true
	}
	return FederationNotice{}, false
}

// LeaveFailure is the sentence for the last leave that failed, "" when none did.
func (f FederationState) LeaveFailure() string {
	if f.LeaveErr == nil {
		return ""
	}
	return Cannot("leave", "the federation", f.LeaveErr)
}

// dashLine is s on one line, None when it is empty.
func dashLine(s string) string {
	if line := oneLine(s); line != "" {
		return line
	}
	return None
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// HubHost is the hub as the list shows it: the host of its URL, else the URL, None when there is none.
func (s FederationSpoke) HubHost() string {
	if u, err := url.Parse(s.HubURL); err == nil && u.Host != "" {
		return oneLine(u.Host)
	}
	return dashLine(s.HubURL)
}

// Push is whether the spoke pushes its changes to the hub, as the card words it.
func (s FederationSpoke) Push() string { return onOff(s.PushEnabled) }

// SyncWords is when the Project last synced with its hub: "never" when it never did, "error" when it never did and a failure is
// recorded, else how long ago.
func (s FederationSpoke) SyncWords(now time.Time) string {
	switch {
	case !s.LastSuccessfulSyncAt.IsZero():
		return RelativeTime(s.LastSuccessfulSyncAt, now)
	case s.HasLastError:
		return "error"
	}
	return "never"
}

// Warnings are the spoke's conditions that need attention, in words: none for a healthy spoke.
func (s FederationSpoke) Warnings() []string {
	var words []string
	if s.AllowInsecure {
		words = append(words, "insecure connection")
	}
	if s.ActiveQuarantineCount > 0 {
		words = append(words, "quarantined")
	}
	if s.ResetBlocker != "" {
		words = append(words, "reset blocked")
	}
	if s.UnresolvedViolationCount > 0 {
		words = append(words, "claim violations")
	}
	return words
}

// CardFacts is the muted line under a spoke's name in the list: the push state, the events waiting to push and the last sync.
func (s FederationSpoke) CardFacts(now time.Time) string {
	return "Push " + s.Push() + " · " + strconv.FormatInt(s.PendingPushCount, 10) + " pending · Synced " + s.SyncWords(now)
}

// HubURLFact is the hub's URL as the line under the list shows it.
func (s FederationSpoke) HubURLFact() string { return dashLine(s.HubURL) }

// CredentialFact is the spoke's credential status as the line under the list words it.
func (s FederationSpoke) CredentialFact() string {
	return "Credential " + strings.ToLower(dashLine(s.CredentialStatus))
}

// InsecureWords says whether an insecure connection to the hub is allowed.
func InsecureWords(allowed bool) string {
	if allowed {
		return "allowed"
	}
	return "not allowed"
}

// onOff says whether a setting is on.
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// A FactRow's Tone: a sentence that needs attention, or one that reports a failure.
const (
	FactWarning = "warning"
	FactError   = "error"
)

// DetailRows are the spoke's detail as label→value rows. A row with no Key is a sentence, a row with neither Key nor Value ends a
// group. Times read in the Pane's zone. The hub Project's uid is not a row: it names no one.
func (s FederationSpoke) DetailRows(cal PaneCalendar, now time.Time) []FactRow {
	var rows []FactRow
	for _, field := range s.Embedding {
		rows = append(rows, FactRow{Value: oneLine(field)})
	}
	if len(rows) > 0 {
		rows = append(rows, FactRow{})
	}
	rows = append(rows,
		FactRow{Key: "Project", Value: oneLine(s.ProjectName)},
		FactRow{Key: "Spoke name", Value: s.SpokeLabel()},
		FactRow{Key: "Hub", Value: dashLine(s.HubURL), Mono: true},
		FactRow{Key: "Hub Project", Value: strconv.FormatInt(s.HubProjectID, 10), Mono: true},
		FactRow{Key: "Actor", Value: dashLine(s.BoundActor)},
		FactRow{Key: "Capabilities", Value: dashLine(s.Capabilities)},
		FactRow{Key: "Push", Value: s.Push()},
		FactRow{Key: "Credential", Value: dashLine(s.CredentialStatus)},
		FactRow{Key: "Insecure connection", Value: InsecureWords(s.AllowInsecure)},
		FactRow{},
		FactRow{Key: "Pull cursor", Value: strconv.FormatInt(s.PullCursorEventID, 10), Mono: true},
		FactRow{Key: "Push cursor", Value: strconv.FormatInt(s.PushCursorEventID, 10), Mono: true},
		FactRow{Key: "Pending push", Value: strconv.FormatInt(s.PendingPushCount, 10), Mono: true},
		FactRow{Key: "Push high water", Value: strconv.FormatInt(s.PendingPushHighWaterEventID, 10), Mono: true},
		FactRow{Key: "Pending claims", Value: strconv.FormatInt(s.PendingClaimCount, 10), Mono: true},
		FactRow{Key: "Live claims", Value: strconv.FormatInt(s.LiveClaimCount, 10), Mono: true},
		FactRow{Key: "Quarantines", Value: strconv.FormatInt(s.ActiveQuarantineCount, 10), Mono: true},
	)
	for _, q := range s.ActiveQuarantines {
		rows = append(rows, FactRow{Value: fmt.Sprintf("Quarantine #%d: %s events %d to %d at %s: %s", q.ID, oneLine(q.Direction), q.FirstEventID, q.LastEventID,
			cal.Stamp(q.CreatedAt), oneLine(q.Error)), Tone: FactWarning})
	}
	lastError := None
	if s.HasLastError {
		lastError = oneLine(s.LastError)
		if !s.LastErrorAt.IsZero() {
			lastError = cal.Read(s.LastErrorAt, now) + " " + lastError
		}
	}
	return append(rows,
		FactRow{},
		FactRow{Key: "Reset blocker", Value: dashLine(s.ResetBlocker)},
		FactRow{Key: "Claim violations", Value: fmt.Sprintf("%d unresolved, %d recent", s.UnresolvedViolationCount, s.RecentViolationCount)},
		FactRow{Key: "Last pull", Value: cal.Read(s.LastPullSuccessAt, now)},
		FactRow{Key: "Last push", Value: cal.Read(s.LastPushSuccessAt, now)},
		FactRow{Key: "Last sync", Value: cal.Read(s.LastSuccessfulSyncAt, now)},
		FactRow{Key: "Last error", Value: lastError},
	)
}

var federationRows = []actionEntry{
	{action: OpenFederationDetail, modes: []Mode{ModeFederation}, keys: keys("enter", "enter"), help: "detail", footer: []Mode{ModeFederation}, footerKey: "enter", sheet: []sheetLine{{sheetLettered, 4, "Show spoke detail"}}, handle: openFederationDetail},
	{action: StartFederationLeave, modes: []Mode{ModeFederation}, keys: keys("x", "x"), help: "leave", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 6, "Leave federation"}}, handle: startFederationLeave},
	{action: RefreshFederation, modes: []Mode{ModeFederation}, keys: keys("r", "r"), help: "refresh", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 5, "Read again"}}, handle: askFederation},
	{action: NameFederationSpoke, modes: []Mode{ModeFederation}, keys: keys("shift+n", "⇧N"), help: "name spoke", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 32, "Name spoke"}}, handle: openFederationNameForSpoke},
	{action: StartFederationEnroll, modes: []Mode{ModeFederation}, keys: keys("n", "n"), help: "enroll", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 30, "Enroll a Project"}}, handle: startFederationEnroll},
	{action: StartFederationBrowse, modes: []Mode{ModeFederation}, keys: keys("b", "b"), help: "browse hubs", footer: []Mode{ModeFederation}, sheet: []sheetLine{{sheetLettered, 31, "Browse hub Projects"}}, handle: startFederationBrowse},
	letteredBackRow(ModeFederation, federationBack),
}
