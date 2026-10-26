package model

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The Daemons Page is a Lettered Page: it lists the daemons named in kata's client configuration, as kata tui lists them, and Enter
// switches this Pane to the one under the cursor. A switch lives in this Pane only and persists nothing.

// DaemonEntry is one daemon of kata's catalog as the Page shows it.
type DaemonEntry struct {
	Name string
	// Endpoint is "local" for a local entry, else the host of the entry's url.
	Endpoint string
	// Token says the entry has a token (inline or by environment variable) and AllowInsecure that it allows an unencrypted url.
	Token, AllowInsecure bool
	// Current marks the daemon this Pane talks to.
	Current bool
}

// LoadDaemons asks for the catalog. Request numbers the asks: an answer to an older one is dropped. Workspace is the one whose
// connection the Page marks Current.
type LoadDaemons struct {
	Request   int64
	Workspace string
}

func (LoadDaemons) cmd() {}

// DaemonsLoaded answers LoadDaemons. ConfigPath is the file the catalog is read from, which the empty Page names.
type DaemonsLoaded struct {
	Request    int64
	Entries    []DaemonEntry
	ConfigPath string
	Err        error
}

func (DaemonsLoaded) msg() {}

// SwitchDaemon asks the Daemon gateway to drop its connection and connect to the catalog entry called Name, starting a stopped
// local daemon first. Request numbers the asks.
type SwitchDaemon struct {
	Workspace string
	Request   int64
	Name      string
}

func (SwitchDaemon) cmd() {}

// DaemonSwitched answers SwitchDaemon. Daemon is the connection the Pane now has. A failed switch (Err) leaves the old connection
// in place.
type DaemonSwitched struct {
	Request int64
	Name    string
	Daemon  Daemon
	Err     error
}

func (DaemonSwitched) msg() {}

// DaemonsState is the Daemons Page's state. The list is read again on every entry.
type DaemonsState struct {
	Loading bool
	Err     error
	Entries []DaemonEntry
	// ConfigPath is where the catalog is read from.
	ConfigPath string
	// Cursor indexes Entries.
	Cursor int
	// Switching is the daemon being connected to, "" when no switch is out. It outlives leaving the Page, because the answer still
	// changes the Pane's connection.
	Switching string
	// request counts the list reads and switchRequest the switches; an answer to another is dropped.
	request, switchRequest int64
}

// OnDaemons says the Daemons Page is current.
func (m Model) OnDaemons() bool { return m.Page == PageDaemons }

// daemonsMode is the Daemons Page's mode: the Menu's while the sidebar has the keyboard.
func daemonsMode(m Model) Mode {
	if m.Focus == ColumnCollections {
		return ""
	}
	return ModeDaemons
}

// enterDaemons starts a fresh reading of the catalog.
func enterDaemons(m Model) (Model, []Cmd) {
	d := m.Daemons
	m.Daemons = DaemonsState{request: d.request + 1, Loading: true, Switching: d.Switching, switchRequest: d.switchRequest}
	return m, []Cmd{LoadDaemons{Request: m.Daemons.request, Workspace: m.Workspace}}
}

// clearDaemons drops what the Page held and every list read still out; a switch still out is kept.
func clearDaemons(m Model) Model {
	d := m.Daemons
	m.Daemons = DaemonsState{request: d.request + 1, Switching: d.Switching, switchRequest: d.switchRequest}
	return m
}

// daemonsBack is esc and backspace: the Page shown before.
func daemonsBack(m Model) (Model, []Cmd) { return showPage(m.letteredFrom)(m) }

// switchSelectedDaemon is Enter: it connects this Pane to the daemon under the cursor. A switch already out is waited for.
func switchSelectedDaemon(m Model) (Model, []Cmd) {
	d := &m.Daemons
	entry := m.SelectedDaemon()
	if entry == nil || d.Switching != "" {
		return m, nil
	}
	d.switchRequest++
	d.Switching = entry.Name
	return m, []Cmd{SwitchDaemon{Workspace: m.Workspace, Request: d.switchRequest, Name: entry.Name}}
}

func updateDaemons(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case DaemonsLoaded:
		d := &m.Daemons
		if msg.Request != d.request {
			return m, nil, true
		}
		d.Loading, d.Err = false, msg.Err
		if msg.Err != nil {
			return m, nil, true
		}
		d.Entries, d.ConfigPath = slices.Clone(msg.Entries), msg.ConfigPath
		d.Cursor = max(slices.IndexFunc(d.Entries, func(e DaemonEntry) bool { return e.Current }), 0)
		return m, nil, true
	case DaemonSwitched:
		next, cmds := daemonSwitched(m, msg)
		return next, cmds, true
	}
	return m, nil, false
}

// daemonSwitched ends a switch. A failure says why and changes nothing else. A success starts the Pane over on the new daemon: what
// it held of the old one (the Collection and Pages, every cache, every card and form, the Drafts, the Undo history, the event
// stream) goes, and the Project is read again, which reads the first snapshot and follows the new daemon's events. Every request
// counter steps, so an answer asked of the old daemon is dropped when it arrives. Drafts live in memory only and are keyed by an
// Issue's uid, which a replica shares with its hub, so one kept would send the old daemon's text to the new one: they are dropped,
// and the Notice says how many.
func daemonSwitched(m Model, msg DaemonSwitched) (Model, []Cmd) {
	if msg.Request != m.Daemons.switchRequest {
		return m, nil
	}
	m.Daemons.Switching = ""
	if msg.Err != nil {
		m.Notice = `daemon "` + msg.Name + `": ` + oneLine(msg.Err.Error())
		return m, nil
	}
	m.Connection++
	m = m.leavePage()
	m.Create, m.pending, m.etag, m.etagQuery = CreateState{}, "", "", SnapshotQuery{}
	m.CloseForm, m.DatesForm, m.RecurrenceForm, m.JobForm, m.Backfill = CloseFormState{}, DatesFormState{}, RecurrenceFormState{}, JobFormState{}, BackfillState{}
	m.FieldForm, m.MoveProject, m.afterMove, m.Moving = FieldFormState{}, MoveProjectState{}, nil, ""
	m.DeleteForm, m.TokenForm, m.RevokeCard = DeleteFormState{}, TokenFormState{}, RevokeState{}
	dropped := len(m.Drafts)
	m.Drafts = nil
	m.Project, m.Bound, m.Daemon = Project{}, false, msg.Daemon
	m.Snapshot, m.Relationships, m.Issues, m.SelectedUID, m.Groups, m.ExpandedUIDs = Snapshot{}, nil, nil, "", nil, nil
	m.Loading, m.Err = true, nil
	m.Collection, m.Collections, m.CollectionCursor, m.SidebarProjects = InboxCollection(), nil, 0, false
	m.Picker, m.ProjectForm, m.LanesForm = PickerState{}, ProjectFormState{}, LanesFormState{}
	m.Focus, m.HelpOpen, m.EditorUID = ColumnList, false, ""
	m.Filters, m.DefaultFilters, m.Search, m.IDSearch, m.FilterForm = Filters{}, Filters{}, SearchState{}, IDSearch{}, FilterFormState{}
	m.IssueDetail = m.IssueDetail.cleared()
	m.Board = BoardState{}
	for _, def := range pageDefs {
		if def.reset != nil {
			m = def.reset(m, "")
		}
	}
	m.Attention = AttentionState{request: m.Attention.request + 1}
	m = clearCredentials(clearFederation(m))
	m.Undo = UndoHistory{nextID: m.Undo.nextID}.cleared("connected to a different daemon")
	m.LiveState = LiveState{
		StreamGeneration: m.StreamGeneration + 1, RefreshRequest: m.RefreshRequest + 1,
		wakeGeneration: m.wakeGeneration, resubscribe: true,
	}
	m.Notice = `daemon "` + msg.Name + `": connected`
	switch dropped {
	case 0:
	case 1:
		m.Notice += "; 1 unsent draft dropped"
	default:
		m.Notice += "; " + strconv.Itoa(dropped) + " unsent drafts dropped"
	}
	return m, []Cmd{LoadProject{Workspace: m.Workspace, Actor: m.Actor, Switched: true, Connection: m.Connection}}
}

// moveDaemon moves the cursor by action: one row, to either end, or a page.
func moveDaemon(m Model, action Action) Model {
	n := len(m.Daemons.Entries)
	if n == 0 {
		return m
	}
	m.Daemons.Cursor = movedIndex(m.Daemons.Cursor, n, listPage, action)
	return m
}

// SelectedDaemon is the daemon under the cursor, nil when there is none.
func (m Model) SelectedDaemon() *DaemonEntry {
	d := m.Daemons
	if d.Cursor < 0 || d.Cursor >= len(d.Entries) {
		return nil
	}
	return &d.Entries[d.Cursor]
}

// Summary is the line under the title: how many daemons the catalog names.
func (d DaemonsState) Summary() string { return strconv.Itoa(len(d.Entries)) + " daemons" }

// DaemonsNotice is what stands in place of the list, with a line under it when there is something to add.
type DaemonsNotice struct {
	Text, Detail string
	// Err says the notice is a failure, which the view draws as one.
	Err bool
}

// Notice is what stands in place of the list: loading, a failed read, or the empty catalog; false when the list is shown.
func (d DaemonsState) Notice() (DaemonsNotice, bool) {
	switch {
	case d.Loading:
		return DaemonsNotice{Text: "loading daemons..."}, true
	case d.Err != nil:
		return DaemonsNotice{Text: "failed to read the daemon catalog: " + oneLine(d.Err.Error()), Err: true}, true
	case len(d.Entries) == 0:
		return DaemonsNotice{
			Text:   "no named daemons",
			Detail: "add a [[daemon]] entry to " + oneLine(d.ConfigPath) + ": a name, and either url (a remote daemon) or local = true",
		}, true
	}
	return DaemonsNotice{}, false
}

// SwitchingLine is what the Page says while a switch is out, "" when none is.
func (d DaemonsState) SwitchingLine() string {
	if d.Switching == "" {
		return ""
	}
	return "connecting to daemon " + oneLine(d.Switching) + "..."
}

// Footer is the line under the list about the selected daemon: its name and endpoint, and allow_insecure when it is set.
func (e DaemonEntry) Footer() string {
	text := oneLine(e.Name) + " · " + oneLine(e.Endpoint)
	if e.AllowInsecure {
		text += " · allow_insecure"
	}
	return text
}

// Auth is "token" when the entry has a token, else "no token".
func (e DaemonEntry) Auth() string {
	if e.Token {
		return "token"
	}
	return "no token"
}

// State is "current" for the daemon this Pane talks to, else "".
func (e DaemonEntry) State() string {
	if e.Current {
		return "current"
	}
	return ""
}

// DaemonsHeader is the list's header in kata tui's columns.
func DaemonsHeader() string { return daemonRow("Daemon", "Endpoint", "Auth", "State") }

// Row is the entry on kata tui's columns: Daemon, Endpoint, Auth and State.
func (e DaemonEntry) Row() string {
	return daemonRow(oneLine(e.Name), oneLine(e.Endpoint), e.Auth(), e.State())
}

func daemonRow(name, endpoint, auth, state string) string {
	return strings.TrimRight(padRunes(name, 22)+"  "+padRunes(endpoint, 28)+"  "+padRunes(auth, 8)+"  "+state, " ")
}

func padRunes(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// daemonsRows are the Daemons Page's own keys beyond its movement keys.
var daemonsRows = []actionEntry{
	{action: SwitchDaemonAction, modes: []Mode{ModeDaemons}, keys: keys("enter", "↵"), help: "switch", footer: []Mode{ModeDaemons}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 10, "switch this Pane to the selected daemon"}}, handle: switchSelectedDaemon},
	{action: LetteredBack, modes: []Mode{ModeDaemons}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeDaemons}, sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: daemonsBack},
}
