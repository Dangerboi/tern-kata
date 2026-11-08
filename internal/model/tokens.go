package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The Credentials Page creates and revokes API tokens as `kata tokens create` and `kata tokens revoke` do. A new token's secret
// is written by the Daemon gateway straight into a new owner-only file: it never reaches the model, so no Pane state, view or
// message holds it.

const (
	ModeTokenForm   Mode = "token-form"
	ModeRevokeToken Mode = "revoke-token"

	NewToken      Action = "new-token"
	RevokeToken   Action = "revoke-token"
	TokenSubmit   Action = "token-submit"
	TokenCancel   Action = "token-cancel"
	RevokeConfirm Action = "revoke-confirm"
	RevokeCancel  Action = "revoke-cancel"
)

const (
	TokenActor   Field = "actor"
	TokenName    Field = "name"
	TokenExpires Field = "expires"
	TokenScope   Field = "scope"
	TokenIssue   Field = "issue"
	TokenTeams   Field = "teams"
	TokenFile    Field = "file"
)

// tokenTable declares the Token card's fields, in tab order, and its busy condition: the card is busy while it saves and
// once the token is made.
var tokenTable = fieldTable[TokenFormState]{
	form: func(m *Model) *TokenFormState { return &m.TokenForm },
	busy: func(f TokenFormState) bool { return f.Saving || f.Done },
	fields: []formField[TokenFormState]{
		textField(TokenActor, func(f *TokenFormState) *string { return &f.Values.Actor }),
		textField(TokenName, func(f *TokenFormState) *string { return &f.Values.Name }),
		textField(TokenExpires, func(f *TokenFormState) *string { return &f.Values.Expires }),
		textField(TokenScope, func(f *TokenFormState) *string { return &f.Values.Scope }),
		textField(TokenIssue, func(f *TokenFormState) *string { return &f.Values.Issue }),
		textField(TokenTeams, func(f *TokenFormState) *string { return &f.Values.Teams }),
		textField(TokenFile, func(f *TokenFormState) *string { return &f.Values.File }),
	},
}

var TokenFields = tokenTable.specs()

const (
	TokenScopeAccount = "account"
	TokenScopeIssue   = "issue"
)

const maxTokenTeams = 256

func TokenLabel(field Field) string {
	switch field {
	case TokenActor:
		return "Actor"
	case TokenName:
		return "Name (optional)"
	case TokenExpires:
		return "Expires in (e.g. 720h; required for an Issue subtree)"
	case TokenScope:
		return "Scope (account or issue)"
	case TokenIssue:
		return "Issue (for an Issue subtree: short id, #id or project#id)"
	case TokenTeams:
		return "Teams (optional, comma-separated names or uids)"
	case TokenFile:
		return "Token file (a new file for the secret)"
	}
	return string(field)
}

type TokenValues struct{ Actor, Name, Expires, Scope, Issue, Teams, File string }

func (v TokenValues) Value(name Field) string {
	return tokenTable.value(TokenFormState{Values: v}, name)
}

// TokenFormState is the Token card. Once a token is made DoneID and DoneFile name it; the secret is in the file alone.
type TokenFormState struct {
	formCard
	Values   TokenValues
	Done     bool
	DoneID   int64
	DoneFile string
}

type RevokeState struct {
	Open, Saving bool
	Token        Credential
	Err          error
}

func (r RevokeState) Question() string {
	name := "—"
	if r.Token.Name != "" {
		name = oneLine(r.Token.Name)
	}
	return "Revoke token " + strconv.FormatInt(r.Token.ID, 10) + " (" + name + ", " + oneLine(r.Token.Actor) + ")? This cannot be undone."
}

// CreateTokenCmd describes a token to make in plain values; the Daemon gateway turns it into kata's request. A subtree token's
// Issue is ShortID in the Project ProjectID; the gateway resolves it to the Project uid and root Issue uid. File is where the
// secret goes, a path that must not exist yet.
type CreateTokenCmd struct {
	Connection             int
	Workspace, Actor, Name string
	// ExpiresInSeconds is 0 for a token that does not expire.
	ExpiresInSeconds int64
	Subtree          bool
	ProjectID        int64
	ShortID          string
	Teams            []string
	File             string
}

func (CreateTokenCmd) cmd() {}

// TokenCreated answers CreateTokenCmd. It never carries the secret: File holds it.
type TokenCreated struct {
	Connection int
	ID         int64
	File       string
	Err        error
}

func (TokenCreated) msg() {}

type RevokeTokenCmd struct {
	Workspace string
	ID        int64
}

func (RevokeTokenCmd) cmd() {}

type TokenRevoked struct {
	ID  int64
	Err error
}

func (TokenRevoked) msg() {}

func openTokenForm(m Model) (Model, []Cmd) {
	if !m.Credentials.Available || m.TokenForm.Open {
		return m, nil
	}
	m.TokenForm = TokenFormState{Open: true, Field: TokenActor, Values: TokenValues{Actor: m.Actor, Scope: TokenScopeAccount}}
	return m, nil
}

// openRevoke is x on a token: the confirmation. A token already revoked has nothing to revoke.
func openRevoke(m Model) (Model, []Cmd) {
	token := m.SelectedCredential()
	if !m.Credentials.Available || token == nil || m.RevokeCard.Open {
		return m, nil
	}
	if token.State == "revoked" {
		m.Notice = "token " + strconv.FormatInt(token.ID, 10) + " is already revoked"
		return m, nil
	}
	m.RevokeCard = RevokeState{Open: true, Token: *token}
	return m, nil
}

func tokenSeconds(text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(text)
	if err != nil || d < time.Second || d%time.Second != 0 {
		return 0, errors.New("expires in must be a positive whole-second duration")
	}
	return int64(d / time.Second), nil
}

func tokenTeams(text string) ([]string, error) {
	var teams []string
	for _, team := range strings.Split(text, ",") {
		if team = strings.TrimSpace(team); team != "" {
			teams = append(teams, team)
		}
	}
	if len(teams) > maxTokenTeams {
		return nil, fmt.Errorf("a token takes at most %d teams, not %d", maxTokenTeams, len(teams))
	}
	return teams, nil
}

// tokenIssue finds the Project and short id of the Issue a subtree token covers. A bare short id or one with `#` is in the
// Pane's Project; `project#id` and `project/id` name another one the Collection snapshot lists.
func (m Model) tokenIssue(ref string) (projectID int64, shortID string, err error) {
	ref = strings.TrimSpace(ref)
	project, shortID := "", ref
	if i := strings.IndexAny(ref, "#/"); i >= 0 {
		project, shortID = ref[:i], ref[i+1:]
	}
	if shortID == "" || strings.ContainsAny(shortID, "#/?%\\ \t\r\n\x00") || strings.ContainsAny(project, "?%\\\x00\r\n") {
		return 0, "", fmt.Errorf("Issue reference must be a short id, #id or project#id: %q", ref)
	}
	if project == "" || (m.Project.ID != 0 && project == m.Project.Name) {
		if m.Project.ID == 0 {
			return 0, "", errors.New("name the Issue's Project as project#id: this Pane has no Project")
		}
		return m.Project.ID, shortID, nil
	}
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.Name == project {
			return entry.Project.ID, shortID, nil
		}
	}
	return 0, "", fmt.Errorf("kata Project %q not found", project)
}

func (m Model) tokenRequest() (CreateTokenCmd, error) {
	v := m.TokenForm.Values
	cmd := CreateTokenCmd{Connection: m.Connection, Workspace: m.Workspace, Actor: strings.TrimSpace(v.Actor), Name: strings.TrimSpace(v.Name), File: strings.TrimSpace(v.File)}
	if cmd.Actor == "" {
		return cmd, errors.New("actor is required")
	}
	var err error
	if cmd.ExpiresInSeconds, err = tokenSeconds(v.Expires); err != nil {
		return cmd, err
	}
	switch strings.ToLower(strings.TrimSpace(v.Scope)) {
	case TokenScopeAccount, "":
		if strings.TrimSpace(v.Issue) != "" {
			return cmd, errors.New("an account token covers no Issue: set the scope to issue, or clear the Issue")
		}
	case TokenScopeIssue:
		cmd.Subtree = true
		if strings.TrimSpace(v.Issue) == "" {
			return cmd, errors.New("an Issue-subtree token needs an Issue")
		}
		if cmd.ExpiresInSeconds == 0 {
			return cmd, errors.New("an Issue-subtree token needs an expiry")
		}
		if cmd.ProjectID, cmd.ShortID, err = m.tokenIssue(v.Issue); err != nil {
			return cmd, err
		}
	default:
		return cmd, errors.New("scope must be account or issue")
	}
	if cmd.Teams, err = tokenTeams(v.Teams); err != nil {
		return cmd, err
	}
	if cmd.File == "" {
		return cmd, errors.New("token file is required: the secret is written there and never shown")
	}
	return cmd, nil
}

func submitToken(m Model) (Model, []Cmd) {
	f := &m.TokenForm
	if f.Saving {
		return m, nil
	}
	if f.Done {
		m.TokenForm = TokenFormState{}
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before creating a token.")
		return m, nil
	}
	cmd, err := m.tokenRequest()
	if err != nil {
		f.Err = err
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{cmd}
}

func updateTokenFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.TokenForm
	if form.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeTokenForm)
	switch {
	case bound && binding.Action == TokenSubmit:
		return submitToken(m)
	case bound && binding.Action == TokenCancel:
		m.TokenForm = TokenFormState{}
	case form.Done:
	default:
		text := tokenTable.textAt(form, form.Field)
		var edited bool
		if *text, edited = editText(*text, &form.Caret, form.Field, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}

func updateRevokeKey(m Model, msg KeyMsg) (Model, []Cmd) {
	r := &m.RevokeCard
	if r.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeRevokeToken)
	switch {
	case bound && binding.Action == RevokeCancel:
		m.RevokeCard = RevokeState{}
	case bound && binding.Action == RevokeConfirm:
		if m.Stale {
			r.Err = errors.New("Daemon disconnected; reconnect before revoking a token.")
			return m, nil
		}
		r.Saving, r.Err = true, nil
		return m, []Cmd{RevokeTokenCmd{Workspace: m.Workspace, ID: r.Token.ID}}
	}
	return m, nil
}

func readAgain(m Model) (Model, []Cmd) {
	if m.OnCredentials() && m.Credentials.Available {
		return askCredentials(m, 0)
	}
	return m, nil
}

func updateTokens(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case TokenCreated:
		f := &m.TokenForm
		if !f.Open {
			return m, nil, true
		}
		f.Saving = false
		if msg.Err != nil {
			f.Err = msg.Err
		} else {
			f.Done, f.DoneID, f.DoneFile, f.Err = true, msg.ID, msg.File, nil
		}
		next, cmds := readAgain(m)
		return next, cmds, true
	case TokenRevoked:
		r := &m.RevokeCard
		if !r.Open || r.Token.ID != msg.ID {
			return m, nil, true
		}
		r.Saving = false
		if msg.Err != nil {
			r.Err = msg.Err
			return m, nil, true
		}
		m.RevokeCard = RevokeState{}
		m.Notice = "Revoked token " + strconv.FormatInt(msg.ID, 10)
		next, cmds := readAgain(m)
		return next, cmds, true
	}
	return m, nil, false
}
