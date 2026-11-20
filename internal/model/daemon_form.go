package model

import (
	"errors"
	"net/url"
	"slices"
	"strings"
)

// The Daemons Page manages the entries of kata's client config: `n` adds one, `e` edits the selected one and `x` removes it after
// asking. The Daemon gateway writes <KATA_HOME>/config.toml; the Pane keeps no copy of an entry's secret, so editing an entry
// with an inline token shows that field empty and an empty field keeps the token it has.

const (
	ModeDaemonForm   Mode = "daemon-form"
	ModeDaemonRemove Mode = "daemon-remove"

	NewDaemon           Action = "new-daemon"
	EditDaemon          Action = "edit-daemon"
	RemoveDaemonAction  Action = "remove-daemon"
	DaemonSubmit        Action = "daemon-submit"
	DaemonCancel        Action = "daemon-cancel"
	DaemonRemoveConfirm Action = "daemon-remove-confirm"
	DaemonRemoveCancel  Action = "daemon-remove-cancel"
)

const (
	DaemonName     Field = "name"
	DaemonWhere    Field = "where"
	DaemonURL      Field = "url"
	DaemonAuth     Field = "auth"
	DaemonSecret   Field = "secret"
	DaemonInsecure Field = "insecure"
)

// The values of the Where and Auth choices, in the order the controls show them.
const (
	WhereRemote = "remote"
	WhereLocal  = "local"
	AuthNone    = "none"
	AuthEnv     = "env"
	AuthInline  = "inline"
)

var (
	daemonWheres = []string{WhereRemote, WhereLocal}
	daemonAuths  = []string{AuthNone, AuthEnv, AuthInline}
)

// DaemonValues are the card's fields. Secret is the token's environment variable when Auth is AuthEnv and the token itself when it
// is AuthInline.
type DaemonValues struct {
	Name, Where, URL, Auth, Secret string
	Insecure                       bool
}

// DaemonFormState is the card that adds or edits an entry. Old is the name of the entry being edited, "" for a new one; HadToken
// is the kind of token that entry has, so an empty inline token field can mean "keep it".
type DaemonFormState struct {
	formCard
	Values   DaemonValues
	Old      string
	Current  bool
	HadToken string
}

// Editing says the card changes an entry that exists.
func (f DaemonFormState) Editing() bool { return f.Old != "" }

// Shows says whether the card draws the field for the values it holds: a local entry has no URL, token or insecure switch, and a
// token of kind none has no value.
func (f DaemonFormState) Shows(name Field) bool {
	switch name {
	case DaemonURL, DaemonAuth, DaemonInsecure:
		return f.Values.Where != WhereLocal
	case DaemonSecret:
		return f.Values.Where != WhereLocal && f.Values.Auth != AuthNone
	}
	return true
}

var daemonTable = fieldTable[DaemonFormState]{
	form:   func(m *Model) *DaemonFormState { return &m.DaemonForm },
	busy:   func(f DaemonFormState) bool { return f.Saving },
	hidden: func(m Model, name Field) bool { return !m.DaemonForm.Shows(name) },
	fields: []formField[DaemonFormState]{
		textField(DaemonName, func(f *DaemonFormState) *string { return &f.Values.Name }),
		daemonChoiceField(DaemonWhere, daemonWheres, func(f *DaemonFormState) *string { return &f.Values.Where }),
		textField(DaemonURL, func(f *DaemonFormState) *string { return &f.Values.URL }),
		daemonChoiceField(DaemonAuth, daemonAuths, func(f *DaemonFormState) *string { return &f.Values.Auth }),
		textField(DaemonSecret, func(f *DaemonFormState) *string { return &f.Values.Secret }),
		yesNoField(DaemonInsecure, func(f *DaemonFormState) *bool { return &f.Values.Insecure }),
	},
}

var DaemonFields = daemonTable.specs()

// daemonChoiceField is a choice that steps and picks among options and holds the chosen one as the value's text.
func daemonChoiceField(name Field, options []string, held func(*DaemonFormState) *string) formField[DaemonFormState] {
	return formField[DaemonFormState]{name: name,
		step: func(_ *Model, f *DaemonFormState, back bool) {
			value := held(f)
			*value = options[cycleIndex(max(slices.Index(options, *value), 0), len(options), back)]
		},
		choose: func(_ *Model, f *DaemonFormState, index int) bool {
			if index < 0 || index >= len(options) {
				return false
			}
			*held(f) = options[index]
			return true
		},
		held: held,
	}
}

func daemonChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	v := m.DaemonForm.Values
	switch name {
	case DaemonWhere:
		return newChoices(ControlTabs, []string{"Remote", "Local"}, slices.Index(daemonWheres, v.Where)), true
	case DaemonAuth:
		return newChoices(ControlTabs, []string{"None", "Environment variable", "Inline"}, slices.Index(daemonAuths, v.Auth)), true
	case DaemonInsecure:
		return yesNoSwitch(v.Insecure), true
	}
	return ChoiceSet{}, false
}

// Label is the word above a field of the card; the secret's word follows the kind of token.
func (f DaemonFormState) Label(field Field) string {
	switch field {
	case DaemonName:
		return "Name"
	case DaemonWhere:
		return "Runs"
	case DaemonURL:
		return "URL"
	case DaemonAuth:
		return "Token"
	case DaemonSecret:
		if f.Values.Auth == AuthEnv {
			return "Variable"
		}
		return "Secret"
	case DaemonInsecure:
		return "Insecure URL"
	}
	return string(field)
}

// Hint is the one muted line under a field, "" for a field that needs none.
func (f DaemonFormState) Hint(field Field) string {
	switch field {
	case DaemonName:
		return "A name for this daemon in the Menu and in kata --daemon."
	case DaemonURL:
		return "For example https://kata.example:7474."
	case DaemonSecret:
		if f.Values.Auth == AuthEnv {
			return "The name of the variable that holds the token."
		}
		if f.Old != "" && f.HadToken == AuthInline {
			return "Leave empty to keep the token it has."
		}
		return "Stored in config.toml in plain text."
	case DaemonInsecure:
		return "Allow http to a host that is not this machine."
	}
	return ""
}

// DaemonRemoveState is the question that asks before an entry is removed.
type DaemonRemoveState struct {
	Open, Saving bool
	Entry        DaemonEntry
	Err          error
}

func (r DaemonRemoveState) Question() string { return "Remove " + r.Entry.Label() + "?" }

// Consequence says what a removal does and does not do.
func (r DaemonRemoveState) Consequence() string {
	return "Its entry leaves config.toml. kata and its data are not touched."
}

// SaveDaemonCmd writes an entry to kata's config. Old is the entry it replaces, "" for a new one. KeepToken leaves the entry's token
// as the file has it. Request numbers the writes.
type SaveDaemonCmd struct {
	Request              int64
	Old, Name, URL       string
	Local, AllowInsecure bool
	TokenEnv, Token      string
	KeepToken            bool
}

func (SaveDaemonCmd) cmd() {}

// DaemonSaved answers SaveDaemonCmd.
type DaemonSaved struct {
	Request int64
	Name    string
	Err     error
}

func (DaemonSaved) msg() {}

// RemoveDaemonCmd deletes an entry from kata's config.
type RemoveDaemonCmd struct {
	Request int64
	Name    string
}

func (RemoveDaemonCmd) cmd() {}

// DaemonRemoved answers RemoveDaemonCmd.
type DaemonRemoved struct {
	Request int64
	Name    string
	Err     error
}

func (DaemonRemoved) msg() {}

func (m Model) daemonsReady() bool {
	return m.OnDaemons() && !m.Daemons.Loading && m.Daemons.Err == nil
}

func newDaemonForm(m Model) (Model, []Cmd) {
	if !m.daemonsReady() || m.DaemonForm.Open {
		return m, nil
	}
	m.DaemonForm = DaemonFormState{formCard: formCard{Open: true, Field: DaemonName}, Values: DaemonValues{Where: WhereRemote, Auth: AuthNone}}
	return m, nil
}

func editDaemonForm(m Model) (Model, []Cmd) {
	entry := m.SelectedDaemon()
	if !m.daemonsReady() || entry == nil || m.DaemonForm.Open {
		return m, nil
	}
	v := DaemonValues{Name: entry.Name, Where: WhereRemote, URL: entry.URL, Auth: AuthNone, Insecure: entry.AllowInsecure}
	had := AuthNone
	switch {
	case entry.Local:
		v.Where, v.URL = WhereLocal, ""
	}
	switch {
	case entry.TokenEnv != "":
		v.Auth, v.Secret, had = AuthEnv, entry.TokenEnv, AuthEnv
	case entry.InlineToken:
		v.Auth, had = AuthInline, AuthInline
	}
	m.DaemonForm = DaemonFormState{formCard: formCard{Open: true, Field: DaemonName}, Values: v, Old: entry.Name, Current: entry.Current, HadToken: had}
	return m, nil
}

func askRemoveDaemon(m Model) (Model, []Cmd) {
	entry := m.SelectedDaemon()
	if !m.daemonsReady() || entry == nil || m.DaemonRemove.Open {
		return m, nil
	}
	if entry.Current {
		return m.fail("Cannot remove " + entry.Label() + ": this Pane uses it."), nil
	}
	m.DaemonRemove = DaemonRemoveState{Open: true, Entry: *entry}
	return m, nil
}

// request is the write the form asks for, or why it cannot: the same checks kata's own config validation makes, so a refusal
// comes before the file is read.
func (m Model) daemonRequest() (SaveDaemonCmd, error) {
	f := m.DaemonForm
	v := f.Values
	cmd := SaveDaemonCmd{Old: f.Old, Name: strings.TrimSpace(v.Name), Local: v.Where == WhereLocal}
	if cmd.Name == "" {
		return cmd, errors.New("the name is required")
	}
	if f.Current && cmd.Name != f.Old {
		return cmd, errors.New("this Pane uses it, so it keeps its name")
	}
	for _, e := range m.Daemons.Entries {
		if e.Name == cmd.Name && e.Name != f.Old {
			return cmd, errors.New("another daemon has that name")
		}
	}
	if cmd.Local {
		cmd.KeepToken = f.Editing() && f.HadToken != AuthNone
		return cmd, nil
	}
	cmd.URL = strings.TrimSpace(v.URL)
	u, err := url.Parse(cmd.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return cmd, errors.New("the URL must start with http:// or https:// and name a host")
	}
	cmd.AllowInsecure = v.Insecure
	secret := strings.TrimSpace(v.Secret)
	switch v.Auth {
	case AuthEnv:
		if secret == "" {
			return cmd, errors.New("name the variable that holds the token")
		}
		cmd.TokenEnv = secret
	case AuthInline:
		switch {
		case secret != "":
			cmd.Token = secret
		case f.Editing() && f.HadToken == AuthInline:
			cmd.KeepToken = true
		default:
			return cmd, errors.New("the token is required")
		}
	}
	return cmd, nil
}

func submitDaemon(m Model) (Model, []Cmd) {
	f := &m.DaemonForm
	if f.Saving {
		return m, nil
	}
	cmd, err := m.daemonRequest()
	if err != nil {
		f.Err = cardFailure("save", "the daemon", err)
		return m, nil
	}
	cmd.Request = m.Daemons.writes.Ask()
	f.Saving, f.Err = true, nil
	return m, []Cmd{cmd}
}

func updateDaemonFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.DaemonForm
	if form.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeDaemonForm)
	switch {
	case bound && binding.Action == DaemonSubmit:
		return submitDaemon(m)
	case bound && binding.Action == DaemonCancel:
		m.DaemonForm = DaemonFormState{}
	case bound && binding.Action == InputSpace && daemonTable.textAt(form, form.Field) == nil:
		daemonTable.stepFocused(&m, false)
	default:
		daemonTable.typeKey(&m, binding, bound, msg)
	}
	return m, nil
}

func updateDaemonRemoveKey(m Model, msg KeyMsg) (Model, []Cmd) {
	r := &m.DaemonRemove
	if r.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeDaemonRemove)
	switch {
	case bound && binding.Action == DaemonRemoveCancel:
		m.DaemonRemove = DaemonRemoveState{}
	case bound && binding.Action == DaemonRemoveConfirm:
		r.Saving, r.Err = true, nil
		return m, []Cmd{RemoveDaemonCmd{Request: m.Daemons.writes.Ask(), Name: r.Entry.Name}}
	}
	return m, nil
}

// reloadDaemons reads the catalog again after a write, without the Loading state, with the cursor on name (or kept in range).
func reloadDaemons(m Model, focus string) (Model, []Cmd) {
	d := &m.Daemons
	d.focusName = focus
	return m, []Cmd{LoadDaemons{Request: d.reads.Ask(), Workspace: m.Workspace}}
}

func updateDaemonWrites(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case DaemonSaved:
		f := &m.DaemonForm
		if !f.Open || !m.Daemons.writes.IsCurrent(msg.Request) {
			return m, nil, true
		}
		if msg.Err != nil {
			f.Saving, f.Err = false, cardFailure("save", "the daemon", msg.Err)
			return m, nil, true
		}
		m.DaemonForm = DaemonFormState{}
		next, cmds := reloadDaemons(m.say("Saved "+msg.Name+"."), msg.Name)
		return next, cmds, true
	case DaemonRemoved:
		r := &m.DaemonRemove
		if !r.Open || !m.Daemons.writes.IsCurrent(msg.Request) {
			return m, nil, true
		}
		if msg.Err != nil {
			r.Saving, r.Err = false, cardFailure("remove", msg.Name, msg.Err)
			return m, nil, true
		}
		m.DaemonRemove = DaemonRemoveState{}
		next, cmds := reloadDaemons(m.say("Removed "+msg.Name+"."), "")
		return next, cmds, true
	}
	return m, nil, false
}
