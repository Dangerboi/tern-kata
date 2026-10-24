package model

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Enrolling a Project in a hub's federation copies kata tui's enrollment: `n` on the Federation Page chooses a local Project (a new
// replica of a hub Project, or an existing Project to adopt), a hub from kata's daemon catalog and a hub Project, previews what
// would happen and what blocks it, asks to type the Project's name before an adoption, and runs it. `b` browses the hubs' Projects,
// read only. The Daemon gateway reads the catalog and makes the calls.

// FederationOperation is what an enrollment does to the local Project.
type FederationOperation string

const (
	// FederationAdoptSameName adopts the local Project into the hub Project of the same name, which the hub creates when it has none.
	FederationAdoptSameName FederationOperation = "adopt-same-name"
	// FederationAdoptSelectedHub adopts the local Project into the hub Project that was chosen.
	FederationAdoptSelectedHub FederationOperation = "adopt-selected-hub"
	// FederationCreateReplica makes a new local Project as a replica of the chosen hub Project.
	FederationCreateReplica FederationOperation = "create-replica"
	// FederationRejoin rebinds a local Project that left this hub Project's federation earlier; it still shares the hub Project's uid.
	FederationRejoin FederationOperation = "rejoin"
)

// Label is the operation as the preview words it.
func (o FederationOperation) Label() string {
	switch o {
	case FederationAdoptSameName:
		return "adopt existing local project"
	case FederationAdoptSelectedHub:
		return "adopt existing local project into selected hub project"
	case FederationCreateReplica:
		return "create new local replica from hub project"
	case FederationRejoin:
		return "rejoin: resume syncing a local project that previously left this federation"
	}
	return "-"
}

// FederationHub is a daemon of kata's catalog as the hub step lists it. Endpoint is its URL, "local" for a local entry.
type FederationHub struct {
	Name, Endpoint, Auth string
	AllowInsecure        bool
	// Active marks the daemon this Pane talks to, which cannot be its own hub; Local marks a local entry, which a spoke cannot reach.
	Active, Local bool
}

// Label is the hub's line in the hub step.
func (h FederationHub) Label() string {
	label := oneLine(h.Name) + " " + oneLine(h.Endpoint) + " auth " + oneLine(h.Auth)
	if h.AllowInsecure {
		label += " allow_insecure"
	}
	if h.Active {
		label += " (active spoke)"
	}
	return label
}

// FederationBinding is a local Project that has a federation role on this daemon, whatever the role.
type FederationBinding struct {
	ProjectID   int64
	ProjectName string
	Role        string
}

// FederationHubProjects is what one hub's browse read: its Projects, or why it could not be read. Auth is how the hub words its
// authentication of the read.
type FederationHubProjects struct {
	Hub      FederationHub
	Auth     string
	Projects []Project
	Err      error
}

// FederationEnrollDraft is one enrollment as the choices build it and the preview shows it.
type FederationEnrollDraft struct {
	Operation FederationOperation
	// SpokeProjectID and SpokeProjectName are the local Project; the id is 0 for a new replica until the hub Project names it.
	SpokeProjectID   int64
	SpokeProjectName string
	CreateReplica    bool
	// AdoptExisting says the local Project's history is rewritten as snapshots, which asks for a typed confirmation.
	AdoptExisting bool
	// Hub is the chosen hub, HubAuth how it words its authentication of the read.
	Hub                          FederationHub
	HubAuth                      string
	HubProjectID                 int64
	HubProjectName               string
	RequestedActor               string
	APICapabilities              string
	DisplayCapabilities          string
	PushEnabled, AllowInsecure   bool
	// BlockedReason is why the enrollment cannot run; the preview names it.
	BlockedReason string
}

func newFederationEnrollDraft(actor string) FederationEnrollDraft {
	if strings.TrimSpace(actor) == "" {
		actor = "anonymous"
	}
	return FederationEnrollDraft{RequestedActor: actor, APICapabilities: "claim,pull,push", DisplayCapabilities: "pull,push,lease", PushEnabled: true, AdoptExisting: true}
}

// HubProjectBehavior is what the preview says happens to the hub Project.
func (d FederationEnrollDraft) HubProjectBehavior() string {
	if d.Operation == FederationAdoptSameName {
		if d.HubProjectID != 0 {
			return fmt.Sprintf("use existing hub project %q; enable federation if needed", oneLine(d.HubProjectName))
		}
		return fmt.Sprintf("create hub project %q and enable federation", oneLine(d.SpokeProjectName))
	}
	if d.HubProjectName != "" {
		return oneLine(d.HubProjectName)
	}
	return "-"
}

// PreviewLines are the enrollment preview's lines as kata tui words them; "" is a blank line. The blocking reason is not among them.
func (d FederationEnrollDraft) PreviewLines() []string {
	auth := d.HubAuth
	if auth == "" {
		auth = d.Hub.Auth
	}
	lines := []string{
		"Operation: " + d.Operation.Label(),
		"local spoke project: " + federationDash(oneLine(d.SpokeProjectName)),
		"hub daemon: " + oneLine(d.Hub.Name) + " " + oneLine(d.Hub.Endpoint),
		"hub auth: " + oneLine(auth),
		"hub project: " + d.HubProjectBehavior(),
		"requested actor: " + federationDash(oneLine(d.RequestedActor)),
		"capabilities: " + oneLine(d.DisplayCapabilities),
		"push enabled: " + strconv.FormatBool(d.PushEnabled),
		"allow_insecure: " + strconv.FormatBool(d.AllowInsecure),
	}
	if d.AdoptExisting {
		lines = append(lines, "",
			fmt.Sprintf("this federates local project %q INTO hub project %q", federationDash(oneLine(d.SpokeProjectName)), federationDash(oneLine(d.HubProjectName))),
			"adoption warning: pre-adoption event history is replaced by snapshot events for federation")
	}
	if d.Operation == FederationRejoin {
		lines = append(lines, "",
			fmt.Sprintf("local project %q previously left this federation and still shares the hub project's identity;", oneLine(d.SpokeProjectName)),
			fmt.Sprintf("rejoining resumes syncing with hub project %q (edits made while standalone sync to the hub)", oneLine(d.HubProjectName)))
		if d.SpokeProjectName != d.HubProjectName {
			lines = append(lines, "the local project keeps its name; spoke and hub project names do not need to match")
		}
	}
	return lines
}

// FederationEnrollOutcome is what a successful enrollment did.
type FederationEnrollOutcome struct {
	Draft         FederationEnrollDraft
	HubURL        string
	Adopted       bool
	SnapshotCount int64
	// Actor is the enrollment's; HubProjectID and HubProjectUID are the hub Project's as it answered with the enrollment's token.
	Actor         string
	HubProjectID  int64
	HubProjectUID string
}

// ResultLines are the enrollment result's lines as kata tui words them.
func (o FederationEnrollOutcome) ResultLines() []string {
	status := "joined"
	if o.Adopted {
		status = "adopted"
	}
	return []string{
		"status: " + status,
		"actor: " + federationDash(oneLine(o.Actor)),
		"snapshot count: " + strconv.FormatInt(o.SnapshotCount, 10),
		"hub URL: " + oneLine(o.HubURL),
		"hub project ID: " + strconv.FormatInt(o.HubProjectID, 10),
		"hub project UID: " + federationDash(oneLine(o.HubProjectUID)),
	}
}

// FederationJoinCommand is what `kata federation join` needs to finish an enrollment by hand. It holds the enrollment's token, and
// the active daemon's when that one needs one: the Pane draws it only when the user asks.
type FederationJoinCommand struct {
	HubURL                                       string
	HubProjectID                                 int64
	HubProjectUID, ProjectName                   string
	ReplayHorizonEventID, BaselineThroughEventID int64
	Token, Actor, Capabilities                   string
	PushEnabled, AllowInsecure, AdoptExisting    bool
	// SpokeEndpoint is the active daemon's address ("local" for a local one), SpokeAllowInsecure its allow_insecure and SpokeToken its token.
	SpokeEndpoint      string
	SpokeAllowInsecure bool
	SpokeToken         string
}

// String is the command line, with the active daemon's settings in front when it is remote.
func (c FederationJoinCommand) String() string {
	var parts []string
	if c.SpokeEndpoint != "" && c.SpokeEndpoint != "local" {
		parts = append(parts, "KATA_SERVER="+shellWord(c.SpokeEndpoint))
		if c.SpokeAllowInsecure {
			parts = append(parts, "KATA_ALLOW_INSECURE=1")
		}
		if c.SpokeToken != "" {
			parts = append(parts, "KATA_AUTH_TOKEN="+shellWord(c.SpokeToken))
		}
	}
	parts = append(parts, "kata", "federation", "join",
		"--hub-url", shellWord(c.HubURL),
		"--hub-project-id", strconv.FormatInt(c.HubProjectID, 10),
		"--project", shellWord(c.ProjectName),
		"--token", shellWord(c.Token),
		"--actor", shellWord(c.Actor),
		"--capabilities", shellWord(c.Capabilities))
	if c.HubProjectUID != "" {
		parts = append(parts, "--hub-project-uid", shellWord(c.HubProjectUID))
	}
	if c.ReplayHorizonEventID != 0 {
		parts = append(parts, "--replay-horizon", strconv.FormatInt(c.ReplayHorizonEventID, 10))
	}
	if c.BaselineThroughEventID != 0 {
		parts = append(parts, "--baseline-through", strconv.FormatInt(c.BaselineThroughEventID, 10))
	}
	if c.PushEnabled {
		parts = append(parts, "--push")
	}
	if c.AllowInsecure {
		parts = append(parts, "--allow-insecure")
	}
	if c.AdoptExisting {
		parts = append(parts, "--adopt-existing")
	}
	return strings.Join(parts, " ")
}

// shellWord quotes s for a POSIX shell the way kata does: bare when it holds only safe characters.
func shellWord(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=,", r))
	}) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// FederationEnrollRecovery is what a failure after the hub granted the enrollment leaves: the grant exists on the hub, the spoke did
// not join. Stage is "metadata" when the hub Project's metadata could not be read with the token and "join" when the spoke refused
// the replica. Reveal is set by `R`, which is the only thing that shows Command.
type FederationEnrollRecovery struct {
	HubName, SpokeName, SpokeEndpoint, Stage string
	Err                                       error
	// TokenGuess says the token may be the cause (the failure was a refusal of it, or none was captured), which the screen then says.
	TokenGuess bool
	Command    FederationJoinCommand
	Reveal     bool
}

// Lines are the recovery screen's lines as kata tui words them, the revealed command not among them. The failure is lines[ErrLine].
func (r FederationEnrollRecovery) Lines() (lines []string, errLine int) {
	errLine = -1
	if r.Stage == "metadata" {
		lines = append(lines, fmt.Sprintf("hub %s: enrollment metadata fetch failed", oneLine(r.HubName)))
	} else {
		lines = append(lines, "hub: enrollment created", "spoke: join failed")
	}
	if r.Err != nil {
		errLine = len(lines)
		lines = append(lines, oneLine(r.Err.Error()))
	}
	lines = append(lines, "token: hidden")
	if r.TokenGuess {
		lines = append(lines, "the hub enrollment may be single-use, expired, revoked, or invalidated")
	}
	return lines, errLine
}

// RevealedLines are the lines drawn once `R` revealed the command: the warning, the target and the command itself.
func (r FederationEnrollRecovery) RevealedLines() []string {
	return []string{
		"single-use/secret-bearing recovery command",
		"works only while the hub enrollment remains valid and not revoked",
		"spoke target: " + oneLine(r.SpokeName) + " " + oneLine(r.SpokeEndpoint),
		r.Command.String(),
	}
}

// LoadFederationHubs reads kata's daemon catalog for the hub step. Request numbers the reads: an answer to an older one is dropped.
type LoadFederationHubs struct {
	Workspace string
	Request   int64
}

func (LoadFederationHubs) cmd() {}

// FederationHubsLoaded answers LoadFederationHubs: every daemon of the catalog in file order, the active one marked.
type FederationHubsLoaded struct {
	Request int64
	Hubs    []FederationHub
	Err     error
}

func (FederationHubsLoaded) msg() {}

// LoadFederationHubProjects reads the Projects of the catalog daemon called Hub, with that daemon's own token.
type LoadFederationHubProjects struct {
	Workspace string
	Request   int64
	Hub       string
}

func (LoadFederationHubProjects) cmd() {}

// FederationHubProjectsLoaded answers LoadFederationHubProjects. Hub is the daemon as it was reached and Auth how it words its
// authentication; Actor is the account it says the read acted as, which becomes the enrollment's actor. Early says nothing was
// sent to the hub: its token or its URL was refused, which the hub step shows.
type FederationHubProjectsLoaded struct {
	Request     int64
	Hub         FederationHub
	Auth, Actor string
	Projects    []Project
	Early       bool
	Err         error
}

func (FederationHubProjectsLoaded) msg() {}

// BrowseFederationHubs reads the Projects of every catalog daemon that is neither the active one nor local.
type BrowseFederationHubs struct {
	Workspace string
	Request   int64
}

func (BrowseFederationHubs) cmd() {}

// FederationBrowsed answers BrowseFederationHubs: one entry per hub read, each with its Projects or its failure.
type FederationBrowsed struct {
	Request int64
	Hubs    []FederationHubProjects
	Err     error
}

func (FederationBrowsed) msg() {}

// EnrollFederation runs an enrollment. Attempt numbers the runs: an answer to an older one is dropped. InstanceUID is the active
// daemon's, which the hub's enrollment is made for.
type EnrollFederation struct {
	Workspace   string
	Attempt     int64
	InstanceUID string
	Draft       FederationEnrollDraft
}

func (EnrollFederation) cmd() {}

// FederationEnrolled answers EnrollFederation: the outcome, or the failure. A failure after the hub granted the enrollment carries
// Recovery, which keeps what finishing it by hand needs.
type FederationEnrolled struct {
	Attempt  int64
	Outcome  FederationEnrollOutcome
	Recovery *FederationEnrollRecovery
	Err      error
}

func (FederationEnrolled) msg() {}

// FederationLocalChoice is a row of the local Project step: the new replica, or a Project to adopt.
type FederationLocalChoice struct {
	CreateReplica bool
	Project       Project
}

// Label is the row's text.
func (c FederationLocalChoice) Label() string {
	if c.CreateReplica {
		return "create new local replica from hub project"
	}
	return oneLine(c.Project.Name)
}

// FederationLocalChoices are the rows of the local Project step: the new replica, then the Projects of this daemon by name. The
// Pane's own Project is a row even before the catalog lists it.
func (m Model) FederationLocalChoices() []FederationLocalChoice {
	var projects []Project
	for _, entry := range m.Snapshot.Catalog {
		if !entry.Project.Archived {
			projects = append(projects, entry.Project)
		}
	}
	if m.Bound && m.Project.ID != 0 && !slices.ContainsFunc(projects, func(p Project) bool { return p.ID == m.Project.ID }) {
		projects = append(projects, m.Project)
	}
	sort.SliceStable(projects, func(i, j int) bool {
		li, lj := strings.ToLower(projects[i].Name), strings.ToLower(projects[j].Name)
		if li != lj {
			return li < lj
		}
		return projects[i].ID < projects[j].ID
	})
	rows := []FederationLocalChoice{{CreateReplica: true}}
	for _, p := range projects {
		rows = append(rows, FederationLocalChoice{Project: p})
	}
	return rows
}

// FederationHubProjectChoice is a row of the hub Project step. Sentinel is the row that adopts into the hub Project of the local
// Project's name, which the hub creates when it has none.
type FederationHubProjectChoice struct {
	Sentinel bool
	Project  Project
	Label    string
}

// FederationHubProjectChoices are the rows of the hub Project step. Creating a replica lists every hub Project. Adopting lists the
// same-name row first, then every hub Project of another name: the one of the same name is that first row.
func (m Model) FederationHubProjectChoices() []FederationHubProjectChoice {
	f := m.Federation
	if f.Draft.CreateReplica {
		rows := make([]FederationHubProjectChoice, len(f.HubProjects))
		for i, p := range f.HubProjects {
			rows[i] = FederationHubProjectChoice{Project: p, Label: oneLine(p.Name)}
		}
		return rows
	}
	label := fmt.Sprintf("create hub project %q and enable federation", oneLine(f.Draft.SpokeProjectName))
	if _, ok := hubProjectByName(f.HubProjects, f.Draft.SpokeProjectName); ok {
		label = fmt.Sprintf("use existing hub project %q; enable federation if needed", oneLine(f.Draft.SpokeProjectName))
	}
	rows := []FederationHubProjectChoice{{Sentinel: true, Label: label}}
	for _, p := range f.HubProjects {
		if p.Name != f.Draft.SpokeProjectName {
			rows = append(rows, FederationHubProjectChoice{Project: p, Label: oneLine(p.Name)})
		}
	}
	return rows
}

// FederationBrowseRow is a row of the browse: a hub's Project, Hub indexing Browsed.
type FederationBrowseRow struct {
	Hub     int
	Project Project
}

// Label is the row's text: the Project's id and name.
func (r FederationBrowseRow) Label() string { return strconv.FormatInt(r.Project.ID, 10) + " " + oneLine(r.Project.Name) }

// BrowseRows are the browse's rows across every hub read, in hub order.
func (f FederationState) BrowseRows() []FederationBrowseRow {
	var rows []FederationBrowseRow
	for i, hub := range f.Browsed {
		for _, p := range hub.Projects {
			rows = append(rows, FederationBrowseRow{Hub: i, Project: p})
		}
	}
	return rows
}

func hubProjectByName(projects []Project, name string) (Project, bool) {
	for _, p := range projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

func (m Model) localProjectByUID(uid string) (Project, bool) {
	if strings.TrimSpace(uid) == "" {
		return Project{}, false
	}
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.UID == uid {
			return entry.Project, true
		}
	}
	return Project{}, false
}

func (m Model) localProjectByID(id int64) (Project, bool) {
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.ID == id {
			return entry.Project, true
		}
	}
	return Project{}, false
}

func (m Model) localProjectNameExists(name string) bool {
	return slices.ContainsFunc(m.Snapshot.Catalog, func(c CatalogProject) bool { return c.Project.Name == name })
}

// binding is the local Project's federation role, matched by id or by name.
func (f FederationState) binding(projectID int64, name string) (FederationBinding, bool) {
	for _, b := range f.Bindings {
		if projectID != 0 && b.ProjectID == projectID || name != "" && b.ProjectName == name {
			return b, true
		}
	}
	return FederationBinding{}, false
}

// FederationSelectedProject is the line "selected project: ..." the enrollment screens carry: the Project being enrolled, else the
// Pane's own.
func (m Model) FederationSelectedProject() string {
	d := m.Federation.Draft
	switch {
	case d.Operation == FederationRejoin:
		return oneLine(d.SpokeProjectName) + " (rejoin)"
	case d.CreateReplica && d.SpokeProjectName != "":
		return oneLine(d.SpokeProjectName) + " (new local replica)"
	case d.CreateReplica:
		return "create new local replica from hub project"
	case d.SpokeProjectName != "":
		return oneLine(d.SpokeProjectName)
	case m.Bound && m.Project.Name != "":
		return oneLine(m.Project.Name)
	}
	return "none"
}

// federationCursorCount is the cursor of the screen shown and how many rows it lists: none while the read that fills the step is out,
// as the step draws none then.
func (m Model) federationCursorCount() (cursor, n int) {
	f := m.Federation
	switch f.Screen {
	case FederationPickLocal:
		return f.LocalCursor, len(m.FederationLocalChoices())
	case FederationPickHub:
		if f.HubsLoading {
			return f.HubCursor, 0
		}
		return f.HubCursor, len(f.Hubs)
	case FederationPickHubProject:
		if f.HubProjectsLoading {
			return f.HubProjectCursor, 0
		}
		return f.HubProjectCursor, len(m.FederationHubProjectChoices())
	case FederationBrowse:
		return f.BrowseCursor, len(f.BrowseRows())
	}
	return f.Cursor, len(f.Spokes)
}

// FederationChoiceCursor is the row the choice or browse screen has selected, clamped to its rows.
func (m Model) FederationChoiceCursor() int {
	cursor, n := m.federationCursorCount()
	return max(0, min(cursor, n-1))
}

// FederationChoosing says the Federation Page shows one of the enrollment's choices or the browse, whose rows are chosen from the
// cursor rather than being spokes.
func (m Model) FederationChoosing() bool {
	switch m.Federation.Screen {
	case FederationPickLocal, FederationPickHub, FederationPickHubProject, FederationBrowse:
		return true
	}
	return false
}

// moveFederationChoice moves the cursor of the choice or browse screen shown.
func moveFederationChoice(m Model, action Action) Model {
	cursor, n := m.federationCursorCount()
	if n == 0 {
		return m
	}
	cursor = movedIndex(cursor, n, listPage, action)
	f := &m.Federation
	switch f.Screen {
	case FederationPickLocal:
		f.LocalCursor = cursor
	case FederationPickHub:
		f.HubCursor = cursor
	case FederationPickHubProject:
		f.HubProjectCursor = cursor
	case FederationBrowse:
		f.BrowseCursor = cursor
	}
	return m
}

// federationChoiceMoves are the movement keys of the choice and browse screens.
func federationChoiceMoves(pageDef) []actionEntry {
	return slices.Concat(
		pageMoveActions(pageDef{moves: pageMoves{mode: ModeFederationPick, noun: "choice", move: moveFederationChoice}}),
		pageMoveActions(pageDef{moves: pageMoves{mode: ModeFederationBrowse, noun: "Project", move: moveFederationChoice}}))
}

// startFederationEnroll is n on the list: the local Project step, with the cursor on the Pane's own Project. It never skips the
// step, so adopting a Project stays an explicit choice.
func startFederationEnroll(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Screen != FederationList {
		return m, nil
	}
	f.attempt++
	f.hubRequest++
	f.Draft = newFederationEnrollDraft(m.Actor)
	f.EnrollErr, f.Enrolling = nil, false
	f.LocalCursor, f.HubCursor, f.HubProjectCursor = 0, 0, 0
	f.Hubs, f.HubProjects, f.HubsLoading, f.HubProjectsLoading = nil, nil, false, false
	f.Recovery = FederationEnrollRecovery{}
	if m.Bound {
		for i, choice := range m.FederationLocalChoices() {
			if !choice.CreateReplica && choice.Project.ID == m.Project.ID {
				f.LocalCursor = i
				break
			}
		}
	}
	f.Screen = FederationPickLocal
	return m, nil
}

// startFederationBrowse is b on the list: the hubs' Projects, read only.
func startFederationBrowse(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Screen != FederationList {
		return m, nil
	}
	f.attempt++
	f.hubRequest++
	f.Draft, f.Recovery = FederationEnrollDraft{}, FederationEnrollRecovery{}
	f.Enrolling = false
	f.Browsed, f.BrowseCursor, f.EnrollErr, f.Browsing = nil, 0, nil, true
	f.Screen = FederationBrowse
	return m, []Cmd{BrowseFederationHubs{Workspace: m.Workspace, Request: f.hubRequest}}
}

// chooseFederation is Enter on a choice screen.
func chooseFederation(m Model) (Model, []Cmd) {
	switch m.Federation.Screen {
	case FederationPickLocal:
		return chooseFederationLocal(m)
	case FederationPickHub:
		return chooseFederationHub(m)
	case FederationPickHubProject:
		return previewFederationEnroll(m)
	}
	return m, nil
}

func chooseFederationLocal(m Model) (Model, []Cmd) {
	f := &m.Federation
	choice := m.FederationLocalChoices()[m.FederationChoiceCursor()]
	d := &f.Draft
	if choice.CreateReplica {
		d.CreateReplica, d.AdoptExisting, d.SpokeProjectID, d.SpokeProjectName = true, false, 0, ""
	} else {
		d.CreateReplica, d.AdoptExisting, d.SpokeProjectID, d.SpokeProjectName = false, true, choice.Project.ID, choice.Project.Name
	}
	f.Screen = FederationPickHub
	f.HubCursor, f.EnrollErr = 0, nil
	f.Hubs, f.HubsLoading = nil, true
	f.hubRequest++
	return m, []Cmd{LoadFederationHubs{Workspace: m.Workspace, Request: f.hubRequest}}
}

func chooseFederationHub(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.HubsLoading || len(f.Hubs) == 0 {
		return m, nil
	}
	hub := f.Hubs[m.FederationChoiceCursor()]
	f.EnrollErr = nil
	switch {
	case hub.Active:
		f.EnrollErr = errors.New("active daemon cannot be selected as hub")
		return m, nil
	case hub.Local:
		f.EnrollErr = errors.New("local hub targets cannot be used for federation enrollment; select a hub daemon with a spoke-reachable URL")
		return m, nil
	}
	f.Draft.Hub, f.Draft.HubAuth, f.Draft.AllowInsecure = hub, "", hub.AllowInsecure
	f.Screen = FederationPickHubProject
	f.HubProjects, f.HubProjectsLoading, f.HubProjectCursor = nil, true, 0
	f.hubRequest++
	return m, []Cmd{LoadFederationHubProjects{Workspace: m.Workspace, Request: f.hubRequest, Hub: hub.Name}}
}

func federationHubsLoaded(m Model, msg FederationHubsLoaded) Model {
	f := &m.Federation
	if msg.Request != f.hubRequest || f.Screen != FederationPickHub {
		return m
	}
	f.HubsLoading, f.EnrollErr = false, msg.Err
	if msg.Err == nil {
		f.Hubs = slices.Clone(msg.Hubs)
		f.HubCursor = max(0, min(f.HubCursor, len(f.Hubs)-1))
	}
	return m
}

func federationHubProjectsLoaded(m Model, msg FederationHubProjectsLoaded) Model {
	f := &m.Federation
	if msg.Request != f.hubRequest || f.Screen != FederationPickHubProject {
		return m
	}
	f.HubProjectsLoading = false
	if msg.Err != nil && msg.Early {
		// Nothing was asked of the hub: its token or URL was refused, which the hub step shows.
		f.Screen, f.EnrollErr = FederationPickHub, msg.Err
		return m
	}
	f.EnrollErr = msg.Err
	if msg.Err != nil {
		return m
	}
	f.Draft.Hub = msg.Hub
	f.Draft.HubAuth = msg.Auth
	if actor := strings.TrimSpace(msg.Actor); actor != "" {
		f.Draft.RequestedActor = actor
	}
	f.HubProjects = slices.Clone(msg.Projects)
	f.HubProjectCursor = max(0, min(f.HubProjectCursor, len(m.FederationHubProjectChoices())-1))
	return m
}

func federationBrowsed(m Model, msg FederationBrowsed) Model {
	f := &m.Federation
	if msg.Request != f.hubRequest || f.Screen != FederationBrowse {
		return m
	}
	f.Browsing = false
	switch {
	case msg.Err != nil:
		f.EnrollErr = msg.Err
	case len(msg.Hubs) == 0:
		f.EnrollErr = errors.New("no catalog hub daemons configured")
	default:
		f.Browsed = slices.Clone(msg.Hubs)
	}
	return m
}

// federationChoiceBack is esc and backspace on a choice screen: one step back, the list from the first.
func federationChoiceBack(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.hubRequest++
	f.EnrollErr = nil
	switch f.Screen {
	case FederationPickLocal:
		f.attempt++
		f.Screen = FederationList
	case FederationPickHub:
		f.HubsLoading = false
		f.Screen = FederationPickLocal
	case FederationPickHubProject:
		f.HubProjectsLoading = false
		f.Screen = FederationPickHub
	}
	return m, nil
}

// federationBrowseBack is esc and backspace in the browse: back to the list.
func federationBrowseBack(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.hubRequest++
	f.Browsing, f.EnrollErr = false, nil
	f.Screen = FederationList
	return m, nil
}

// previewFederationEnroll is Enter on the hub Project step: it settles the operation and what blocks it, as kata tui does.
func previewFederationEnroll(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.HubProjectsLoading {
		return m, nil
	}
	f.EnrollErr = nil
	d := f.Draft
	d.Operation, d.HubProjectID, d.HubProjectName, d.BlockedReason = "", 0, "", ""
	var choice FederationHubProjectChoice
	choices := m.FederationHubProjectChoices()
	hasRow := len(choices) > 0
	if hasRow {
		choice = choices[m.FederationChoiceCursor()]
	}
	hasProject := hasRow && !choice.Sentinel
	if d.CreateReplica {
		if !hasProject {
			f.EnrollErr = errors.New("select an existing hub project to create a local replica")
			return m, nil
		}
		d.Operation = FederationCreateReplica
		d.HubProjectID, d.HubProjectName = choice.Project.ID, choice.Project.Name
		d.SpokeProjectName, d.AdoptExisting = choice.Project.Name, false
		if holder, ok := m.localProjectByUID(choice.Project.UID); ok {
			// The hub Project's identity exists locally already: this is a rejoin of that Project, which left this federation earlier.
			// The daemon would refuse it as a new replica.
			if b, bound := f.binding(holder.ID, holder.Name); bound {
				d.BlockedReason = fmt.Sprintf("local project %q already has federation binding as %s", holder.Name, b.Role)
			} else {
				d.Operation, d.SpokeProjectName = FederationRejoin, holder.Name
			}
		} else if m.localProjectNameExists(d.SpokeProjectName) {
			d.BlockedReason = fmt.Sprintf("local project %q already exists", d.SpokeProjectName)
		}
	} else {
		d.AdoptExisting = true
		switch {
		case hasRow && choice.Sentinel:
			d.Operation, d.HubProjectName = FederationAdoptSameName, d.SpokeProjectName
			if same, ok := hubProjectByName(f.HubProjects, d.SpokeProjectName); ok {
				d.HubProjectID = same.ID
			}
		case hasProject:
			d.Operation, d.HubProjectID, d.HubProjectName = FederationAdoptSelectedHub, choice.Project.ID, choice.Project.Name
		}
		if b, bound := f.binding(d.SpokeProjectID, d.SpokeProjectName); bound {
			name := d.SpokeProjectName
			if name == "" {
				name = b.ProjectName
			}
			d.BlockedReason = fmt.Sprintf("local project %q already has federation binding as %s", name, b.Role)
		}
		if d.BlockedReason == "" && d.HubProjectID != 0 {
			// The local Project may already share the hub Project's identity, the state after leaving: adopting would rewrite its
			// history a second time, so it rejoins instead. With either identity unknown it is blocked, not adopted.
			var hubUID, localUID string
			for _, p := range f.HubProjects {
				if p.ID == d.HubProjectID {
					hubUID = p.UID
				}
			}
			if local, ok := m.localProjectByID(d.SpokeProjectID); ok {
				localUID = local.UID
			}
			switch {
			case hubUID != "" && localUID == hubUID:
				d.Operation, d.AdoptExisting = FederationRejoin, false
			case localUID == "":
				d.BlockedReason = fmt.Sprintf("local project %q identity is still loading; press esc and retry", d.SpokeProjectName)
			case hubUID == "":
				d.BlockedReason = fmt.Sprintf("hub project %q identity is unknown; refresh the hub project list and retry", d.HubProjectName)
			}
		}
	}
	d.AllowInsecure = d.Hub.AllowInsecure
	f.Draft = d
	f.Screen = FederationPreview
	return m, nil
}

// federationPreviewBack is esc and backspace in the preview: back to the hub Project step. A run that is out keeps the preview, so
// its answer, the recovery of a grant above all, always has a screen to land on.
func federationPreviewBack(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Enrolling {
		return m, nil
	}
	f.EnrollErr = nil
	f.Screen = FederationPickHubProject
	return m, nil
}

// confirmFederationPreview is Enter in the preview: nothing while it is blocked or a run is out; an adoption asks to type the
// Project's name first, anything else runs.
func confirmFederationPreview(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Draft.BlockedReason != "" || f.Enrolling {
		return m, nil
	}
	if f.Draft.AdoptExisting {
		f.Adopting, f.AdoptInput, f.AdoptCaret, f.AdoptErr = true, "", Caret{}, nil
		f.EnrollErr = nil
		return m, nil
	}
	return runFederationEnroll(m)
}

func runFederationEnroll(m Model) (Model, []Cmd) {
	f := &m.Federation
	f.attempt++
	f.Enrolling, f.EnrollErr = true, nil
	return m, []Cmd{EnrollFederation{Workspace: m.Workspace, Attempt: f.attempt, InstanceUID: strings.TrimSpace(f.InstanceUID), Draft: f.Draft}}
}

// federationAdoptKey is one key for the card asking to type the local Project's name: Enter runs the adoption when the name is
// exact, esc goes back to the preview, and every other key is typed.
func federationAdoptKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.Federation
	binding, bound := LookupBindingForMode(msg.Chord, ModeFederationAdopt)
	switch {
	case bound && binding.Action == FederationAdoptConfirm:
		if f.AdoptInput != f.Draft.SpokeProjectName {
			f.AdoptErr = fmt.Errorf("type %q to confirm adoption", f.Draft.SpokeProjectName)
			return m, nil
		}
		f.Adopting, f.AdoptInput, f.AdoptCaret, f.AdoptErr = false, "", Caret{}, nil
		return runFederationEnroll(m)
	case bound && binding.Action == FederationAdoptCancel:
		f.Adopting, f.AdoptInput, f.AdoptCaret, f.AdoptErr = false, "", Caret{}, nil
	default:
		var edited bool
		if f.AdoptInput, edited = editText(f.AdoptInput, &f.AdoptCaret, "", bound, msg); edited {
			f.AdoptErr = nil
		}
	}
	return m, nil
}

// federationEnrolled lands a run's answer. A failure after the hub granted the enrollment keeps the recovery; any other keeps the
// preview with the failure under it. A success shows the result and reads the status again, which now lists the Project.
func federationEnrolled(m Model, msg FederationEnrolled) (Model, []Cmd) {
	f := &m.Federation
	if msg.Attempt != f.attempt || !f.Enrolling {
		return m, nil
	}
	f.Enrolling = false
	switch {
	case msg.Err != nil && msg.Recovery != nil:
		f.Recovery = *msg.Recovery
		f.Recovery.Err, f.Recovery.Reveal = msg.Err, false
		f.Screen = FederationRecovery
		return m, nil
	case msg.Err != nil:
		f.EnrollErr = msg.Err
		f.Screen = FederationPreview
		return m, nil
	}
	f.Enrolled = msg.Outcome
	f.Screen = FederationEnrollDone
	return askFederation(m)
}

// revealFederationJoin is R on the recovery: the one thing that shows the command and its token.
func revealFederationJoin(m Model) (Model, []Cmd) {
	m.Federation.Recovery.Reveal = true
	return m, nil
}

// federationRecoveryBack is esc and backspace on the recovery: back to the preview.
func federationRecoveryBack(m Model) (Model, []Cmd) {
	m.Federation.Recovery.Reveal = false
	m.Federation.Screen = FederationPreview
	return m, nil
}

// federationEnrollRows are the keys of the enrollment's choices, preview and recovery and of the browse.
var federationEnrollRows = []actionEntry{
	{action: FederationChoose, modes: []Mode{ModeFederationPick}, keys: keys("enter", "↵"), help: "select", footer: []Mode{ModeFederationPick}, footerKey: "↵", handle: chooseFederation},
	{action: FederationChoiceBack, modes: []Mode{ModeFederationPick}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationPick}, handle: federationChoiceBack},
	{action: FederationBrowseBack, modes: []Mode{ModeFederationBrowse}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationBrowse}, handle: federationBrowseBack},
	{action: RunFederationEnroll, modes: []Mode{ModeFederationPreview}, keys: keys("enter", "↵"), help: "confirm", footer: []Mode{ModeFederationPreview}, footerKey: "↵", handle: confirmFederationPreview},
	{action: FederationPreviewBack, modes: []Mode{ModeFederationPreview}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationPreview}, handle: federationPreviewBack},
	{action: RevealFederationJoin, modes: []Mode{ModeFederationRecovery}, keys: keys("shift+r", "R"), help: "reveal recovery command", footer: []Mode{ModeFederationRecovery}, handle: revealFederationJoin},
	{action: FederationRecoveryBack, modes: []Mode{ModeFederationRecovery}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeFederationRecovery}, handle: federationRecoveryBack},
}
