package model

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// The Credentials Page is a Lettered Page: it lists the daemon's API tokens as kata redacts them (state, account, scope and the
// times kata holds), and acts on none. It reads the connection's auth capabilities first, because a daemon only lists tokens to
// an account with token audit read: an empty list must never be mistaken for no tokens.

// credentialsRefreshEvery is how often the list is read again while the Page is shown.
const credentialsRefreshEvery = 30 * time.Second

// Credential is one API token as the token list reports it. A time kata did not set is the zero time.
type Credential struct {
	ID                                  int64
	State, Name, Actor                  string
	ScopeKind, ProjectUID, RootIssueUID string
	CreatedAt, ExpiresAt                time.Time
	LastUsedAt, RevokedAt               time.Time
}

// LoadAuthCapabilities asks the daemon what the connection's account may do. Request numbers the asks: an answer to an older
// one is dropped.
type LoadAuthCapabilities struct {
	Workspace string
	Request   int64
	// Account says the ask is the Pane's own, for whether its connection is Issue-scoped, not the Credentials Page's.
	Account bool
}

func (LoadAuthCapabilities) cmd() {}

// AuthCapabilitiesLoaded answers LoadAuthCapabilities and echoes its Account. TokenAuditRead says the account may list tokens;
// IssueScoped that its credential is limited to one Issue; VerifiedSenders that the daemon verifies the actor of every write, and
// then Actor is the account it records as that actor ("" when it names none).
type AuthCapabilitiesLoaded struct {
	Request         int64
	Account         bool
	TokenAuditRead  bool
	IssueScoped     bool
	VerifiedSenders bool
	Actor           string
	Err             error
}

func (AuthCapabilitiesLoaded) msg() {}

// LoadCredentials asks for the token list, After a pause when After is set (the refresh). Request numbers the asks.
type LoadCredentials struct {
	Workspace string
	Request   int64
	After     time.Duration
}

func (LoadCredentials) cmd() {}

// CredentialsLoaded answers LoadCredentials. ObservedAt is the instant the daemon classified each token's state.
type CredentialsLoaded struct {
	Request    int64
	Tokens     []Credential
	ObservedAt time.Time
	Err        error
}

func (CredentialsLoaded) msg() {}

type CredentialsState struct {
	// Checking says the capability check is out; Available that the account may list tokens; Loading that a list is out.
	Checking, Available, Loading bool
	// Err is the last failure: of the capability check (Available is then false) or of a list read.
	Err        error
	Tokens     []Credential
	ObservedAt time.Time
	Cursor     int
	reads      Latest
}

func (m Model) OnCredentials() bool { return m.Page == PageCredentials }

var credentialsMode = sidebarMode(ModeCredentials)

func enterCredentials(m Model) (Model, []Cmd) {
	m.Credentials = CredentialsState{reads: m.Credentials.reads}
	return askCapabilities(m)
}

func clearCredentials(m Model) Model {
	m.Credentials = CredentialsState{reads: m.Credentials.reads.Discarded()}
	return m
}

func askAccount(m *Model) Cmd {
	return LoadAuthCapabilities{Workspace: m.Workspace, Request: m.accountReads.Ask(), Account: true}
}

// reaskAccount asks again when the stream opens and no read of the connection's capabilities has succeeded or is out: the daemon
// was unreachable when the Pane connected, and delete and restore stay offered until an answer says otherwise.
func reaskAccount(m *Model) []Cmd {
	if m.accountAnswered || m.accountReads.out {
		return nil
	}
	return []Cmd{askAccount(m)}
}

// accountCapabilities applies the answer to the Pane's own ask: an older one is dropped, and a failure leaves the Pane offering
// everything, as before the answer.
func accountCapabilities(m Model, msg AuthCapabilitiesLoaded) Model {
	if m.accountReads.IsCurrent(msg.Request) {
		m.IssueScoped = msg.Err == nil && msg.IssueScoped
		m.VerifiedSenders = msg.Err == nil && msg.VerifiedSenders
		m.accountAnswered = msg.Err == nil
	}
	return m
}

func askCapabilities(m Model) (Model, []Cmd) {
	c := &m.Credentials
	request := c.reads.Ask()
	c.Checking, c.Loading, c.Err = true, false, nil
	return m, []Cmd{LoadAuthCapabilities{Workspace: m.Workspace, Request: request}}
}

func askCredentials(m Model, after time.Duration) (Model, []Cmd) {
	c := &m.Credentials
	request := c.reads.Ask()
	c.Loading = after == 0
	return m, []Cmd{LoadCredentials{Workspace: m.Workspace, Request: request, After: after}}
}

// refreshCredentials is r: it reads the list again, or after a failed check asks for the capabilities again.
func refreshCredentials(m Model) (Model, []Cmd) {
	c := &m.Credentials
	switch {
	case c.Available:
		c.Err = nil
		return askCredentials(m, 0)
	case c.Err != nil:
		return askCapabilities(m)
	}
	return m, nil
}

func updateCredentials(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case AuthCapabilitiesLoaded:
		if msg.Account {
			return accountCapabilities(m, msg), nil, true
		}
		c := &m.Credentials
		if !c.reads.IsCurrent(msg.Request) {
			return m, nil, true
		}
		c.Checking, c.Err, c.Available = false, msg.Err, msg.Err == nil && msg.TokenAuditRead
		if !c.Available {
			return m, nil, true
		}
		next, cmds := askCredentials(m, 0)
		return next, cmds, true
	case CredentialsLoaded:
		c := &m.Credentials
		if !c.reads.IsCurrent(msg.Request) {
			return m, nil, true
		}
		c.Loading, c.Err = false, msg.Err
		if msg.Err == nil {
			c.Tokens = slices.Clone(msg.Tokens)
			slices.SortStableFunc(c.Tokens, func(a, b Credential) int {
				switch {
				case a.CreatedAt.After(b.CreatedAt):
					return -1
				case b.CreatedAt.After(a.CreatedAt):
					return 1
				}
				return int(b.ID - a.ID)
			})
			c.ObservedAt = msg.ObservedAt
			c.Cursor = min(c.Cursor, max(len(c.Tokens)-1, 0))
		}
		next, cmds := askCredentials(m, credentialsRefreshEvery)
		return next, cmds, true
	}
	return updateTokens(m, msg)
}

func moveCredential(m Model, action Action) Model {
	moveCursor(&m.Credentials.Cursor, len(m.Credentials.Tokens), action)
	return m
}

func (m Model) SelectedCredential() *Credential {
	return rowAt(m.Credentials.Tokens, m.Credentials.Cursor)
}

// CredentialsNotice is what the Page shows in place of the token list. Err says a failure, Empty that there are no credentials (the
// view draws an empty state) and Pending that the text is a line of progress; otherwise it is a plain statement.
type CredentialsNotice struct {
	Text                string
	Err, Empty, Pending bool
}

// Notice is what stands in place of the token list, or false when the list is shown.
func (c CredentialsState) Notice() (CredentialsNotice, bool) {
	switch {
	case c.Checking:
		return CredentialsNotice{Text: Progress("Loading", "credentials"), Pending: true}, true
	case !c.Available && c.Err != nil:
		return CredentialsNotice{Text: Cannot("read", "credentials", c.Err), Err: true}, true
	case !c.Available:
		return CredentialsNotice{Text: "Credential audit is not available on this connection."}, true
	case c.Loading && len(c.Tokens) == 0:
		return CredentialsNotice{Text: Progress("Loading", "credentials"), Pending: true}, true
	case c.Err != nil && len(c.Tokens) == 0:
		return CredentialsNotice{Text: Cannot("read", "credentials", c.Err), Err: true}, true
	case len(c.Tokens) == 0:
		return CredentialsNotice{Text: "No credentials", Empty: true}, true
	}
	return CredentialsNotice{}, false
}

// RefreshError is the line under the list when a later read failed and the list shown is the last one that succeeded.
func (c CredentialsState) RefreshError() string {
	if c.Err != nil && c.Available && len(c.Tokens) > 0 {
		return Cannot("refresh", "credentials", c.Err)
	}
	return ""
}

// AsOf is when the daemon classified the tokens' states, as the Page head words it ("as of 17:29"); "" before a list was read.
func (c CredentialsState) AsOf(cal PaneCalendar, now time.Time) string {
	if c.ObservedAt.IsZero() {
		return ""
	}
	return "as of " + cal.Read(c.ObservedAt, now)
}

// FactRow is one label→value row of a Page card: a credential, a spoke's detail or an enrollment step. A row with no Key is a
// sentence. Mono says the value is an id, an endpoint or a path; Tone is FactWarning or FactError for a sentence that needs it.
type FactRow struct {
	Key, Value string
	Mono       bool
	Tone       string
}

// Rows are the credential card's rows after its State, which the view draws toned. A time kata did not set reads as None.
// Kata's ids (the token's, the Project's and the root Issue's) are not rows: Title is the tooltip that holds them.
func (c Credential) Rows(cal PaneCalendar, now time.Time) []FactRow {
	name := None
	if c.Name != "" {
		name = oneLine(c.Name)
	}
	return []FactRow{
		{Key: "Name", Value: name},
		{Key: "Actor", Value: dashLine(c.Actor)},
		{Key: "Scope", Value: c.Scope()},
		{Key: "Created", Value: cal.Read(c.CreatedAt, now)},
		{Key: "Expires", Value: cal.Read(c.ExpiresAt, now)},
		{Key: "Last used", Value: cal.Read(c.LastUsedAt, now)},
		{Key: "Revoked", Value: cal.Read(c.RevokedAt, now)},
	}
}

// Scope is the token's scope in words: "Global", or "Issue subtree" for one limited to an Issue's subtree. A kind a newer kata
// sends comes back as it is.
func (c Credential) Scope() string {
	switch c.ScopeKind {
	case "":
		return "Global"
	case "issue_subtree":
		return "Issue subtree"
	}
	return oneLine(c.ScopeKind)
}

// Title is the card's tooltip: the ids kata knows the token by, which no row shows.
func (c Credential) Title() string {
	parts := []string{"Token " + strconv.FormatInt(c.ID, 10)}
	if c.ProjectUID != "" {
		parts = append(parts, "Project "+oneLine(c.ProjectUID))
	}
	if c.RootIssueUID != "" {
		parts = append(parts, "root Issue "+oneLine(c.RootIssueUID))
	}
	return strings.Join(parts, " · ")
}

var credentialsRows = []actionEntry{
	{action: RefreshCredentials, modes: []Mode{ModeCredentials}, keys: keys("r", "r"), help: "refresh", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 3, "Read again"}}, handle: refreshCredentials},
	{action: NewToken, modes: []Mode{ModeCredentials}, keys: keys("n", "n"), help: "new token", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 20, "New token"}}, handle: openTokenForm},
	{action: RevokeToken, modes: []Mode{ModeCredentials}, keys: keys("x", "x"), help: "revoke", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 21, "Revoke token"}}, handle: openRevoke},
	letteredBackRow(ModeCredentials, letteredBack),
}
