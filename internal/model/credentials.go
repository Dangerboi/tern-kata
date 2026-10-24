package model

import (
	"slices"
	"strconv"
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
}

func (LoadAuthCapabilities) cmd() {}

// AuthCapabilitiesLoaded answers LoadAuthCapabilities. TokenAuditRead says the account may list tokens.
type AuthCapabilitiesLoaded struct {
	Request        int64
	TokenAuditRead bool
	Err            error
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

// CredentialsState is the Credentials Page's state. It is not saved and is read again on every entry.
type CredentialsState struct {
	// Checking says the capability check is out; Available that the account may list tokens; Loading that a list is out.
	Checking, Available, Loading bool
	// Err is the last failure: of the capability check (Available is then false) or of a list read.
	Err        error
	Tokens     []Credential
	ObservedAt time.Time
	// Cursor indexes Tokens.
	Cursor int
	// request counts the asks; an answer to another is dropped.
	request int64
}

// OnCredentials says the Credentials Page is current.
func (m Model) OnCredentials() bool { return m.Page == PageCredentials }

// credentialsMode is the Credentials Page's mode: the Menu's while the sidebar has the keyboard.
func credentialsMode(m Model) Mode {
	if m.Focus == ColumnCollections {
		return ""
	}
	return ModeCredentials
}

// enterCredentials starts a fresh reading: the capability check first.
func enterCredentials(m Model) (Model, []Cmd) {
	m.Credentials = CredentialsState{request: m.Credentials.request}
	return askCapabilities(m)
}

// clearCredentials drops what the Page held and every ask still out.
func clearCredentials(m Model) Model {
	m.Credentials = CredentialsState{request: m.Credentials.request + 1}
	return m
}

func askCapabilities(m Model) (Model, []Cmd) {
	c := &m.Credentials
	c.request++
	c.Checking, c.Loading, c.Err = true, false, nil
	return m, []Cmd{LoadAuthCapabilities{Workspace: m.Workspace, Request: c.request}}
}

func askCredentials(m Model, after time.Duration) (Model, []Cmd) {
	c := &m.Credentials
	c.request++
	c.Loading = after == 0
	return m, []Cmd{LoadCredentials{Workspace: m.Workspace, Request: c.request, After: after}}
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

// credentialsBack is esc and backspace: the Page shown before.
func credentialsBack(m Model) (Model, []Cmd) { return showPage(m.letteredFrom)(m) }

func updateCredentials(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case AuthCapabilitiesLoaded:
		c := &m.Credentials
		if msg.Request != c.request {
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
		if msg.Request != c.request {
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

// moveCredential moves the cursor by action: one row, to either end, or a page.
func moveCredential(m Model, action Action) Model {
	n := len(m.Credentials.Tokens)
	if n == 0 {
		return m
	}
	m.Credentials.Cursor = movedIndex(m.Credentials.Cursor, n, listPage, action)
	return m
}

// SelectedCredential is the token under the cursor, nil when there is none.
func (m Model) SelectedCredential() *Credential {
	c := m.Credentials
	if c.Cursor < 0 || c.Cursor >= len(c.Tokens) {
		return nil
	}
	return &c.Tokens[c.Cursor]
}

// CredentialsNotice is what the Page shows in place of the token list, with a line under it when there is a reason to give.
type CredentialsNotice struct {
	// Text is the line; Err says it is a failure, which the view draws as one.
	Text, Detail string
	Err          bool
}

// Notice is what stands in place of the token list, or false when the list is shown.
func (c CredentialsState) Notice() (CredentialsNotice, bool) {
	switch {
	case c.Checking:
		return CredentialsNotice{Text: "loading credential inventory..."}, true
	case !c.Available && c.Err != nil:
		return CredentialsNotice{Text: "failed to determine credential audit access: " + oneLine(c.Err.Error()), Err: true}, true
	case !c.Available:
		return CredentialsNotice{Text: "credential audit unavailable for this connection", Detail: "the active principal does not have token audit read capability", Err: true}, true
	case c.Loading && len(c.Tokens) == 0:
		return CredentialsNotice{Text: "loading credential inventory..."}, true
	case c.Err != nil && len(c.Tokens) == 0:
		return CredentialsNotice{Text: "failed to load credentials: " + oneLine(c.Err.Error()), Err: true}, true
	case len(c.Tokens) == 0:
		return CredentialsNotice{Text: "no credentials"}, true
	}
	return CredentialsNotice{}, false
}

// RefreshError is the failure of a refresh that left the held list on screen, "" when there is none.
func (c CredentialsState) RefreshError() string {
	if c.Err != nil && c.Available && len(c.Tokens) > 0 {
		return "refresh failed: " + oneLine(c.Err.Error())
	}
	return ""
}

// Summary is the line under the title: the count and, once a list was read, when the daemon observed it.
func (c CredentialsState) Summary() string {
	count := strconv.Itoa(len(c.Tokens)) + " credentials"
	if c.ObservedAt.IsZero() {
		return count
	}
	return count + " · observed at " + credentialTime(c.ObservedAt)
}

// credentialTime words a time in UTC as kata tui does; "—" for none.
func credentialTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

// Lines are a token's four lines as kata tui words them, without the cursor marker.
func (c Credential) Lines() [4]string {
	name := "—"
	if c.Name != "" {
		name = oneLine(c.Name)
	}
	scope, project, root := "global", "—", "—"
	if c.ScopeKind != "" {
		scope, project, root = oneLine(c.ScopeKind), oneLine(c.ProjectUID), oneLine(c.RootIssueUID)
	}
	return [4]string{
		"state " + oneLine(c.State) + "  ID " + strconv.FormatInt(c.ID, 10) + "  name " + name + "  actor " + oneLine(c.Actor),
		"scope " + scope + "  project " + project + "  root " + root,
		"created " + credentialTime(c.CreatedAt) + "  expires " + credentialTime(c.ExpiresAt),
		"last observed use " + credentialTime(c.LastUsedAt) + "  revoked " + credentialTime(c.RevokedAt),
	}
}

// credentialsRows are the Credentials Page's own keys beyond its movement keys.
var credentialsRows = []actionEntry{
	{action: RefreshCredentials, modes: []Mode{ModeCredentials}, keys: keys("r", "r"), help: "refresh", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 3, "read the credentials again, or retry the check"}}, handle: refreshCredentials},
	{action: NewToken, modes: []Mode{ModeCredentials}, keys: keys("n", "n"), help: "new token", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 20, "create an API token (Token card)"}}, handle: openTokenForm},
	{action: RevokeToken, modes: []Mode{ModeCredentials}, keys: keys("x", "x"), help: "revoke", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 21, "revoke the selected token, after asking"}}, handle: openRevoke},
	{action: LetteredBack, modes: []Mode{ModeCredentials}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeCredentials}, sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: credentialsBack},
}
