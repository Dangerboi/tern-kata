package model

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// The Config Page is a Lettered Page: it lists tern-kata's config files on this machine, each with its absolute path, whether it
// exists, whether it loads (the loader's own error when it does not) and a one-line summary of what it holds. It is read again on
// every entry and acts on no Project, so it has no reset rule.

// ConfigFileKey is the stable key of a row, which later actions and tests name a row by.
type ConfigFileKey string

// The rows of the Config Page, in the order the Page lists them.
const (
	ConfigLanes   ConfigFileKey = "lanes"
	ConfigFilters ConfigFileKey = "filters"
	ConfigViews   ConfigFileKey = "views"
	ConfigSeen    ConfigFileKey = "seen"
	ConfigWorker  ConfigFileKey = "worker"
	ConfigAgent   ConfigFileKey = "agent"
	ConfigKata    ConfigFileKey = "kata"
	// ConfigDocbank is the Docbank supervision row: its path is the marker docbank.manual, which exists when supervision is off.
	ConfigDocbank ConfigFileKey = "docbank"
)

// ConfigFileStatus is whether a config file exists and loads. The zero value is a file that does not exist.
type ConfigFileStatus uint8

const (
	ConfigMissing ConfigFileStatus = iota
	ConfigLoads
	ConfigBroken
)

// ConfigAction is something a row offers beyond being read: the Page's keys for it.
type ConfigAction string

const (
	// ConfigOpen is enter: the card or Page that already edits the file.
	ConfigOpen ConfigAction = "open"
	// ConfigEdit is e: the file in $EDITOR.
	ConfigEdit ConfigAction = "edit"
	// ConfigSwitch is s: the row's switch.
	ConfigSwitch ConfigAction = "switch"
)

// ConfigFile is one row of the Config Page.
type ConfigFile struct {
	Key   ConfigFileKey
	Title string
	// Path is the file's absolute path on this machine.
	Path   string
	Status ConfigFileStatus
	// Err is the loader's own error text when Status is ConfigBroken, "" otherwise.
	Err string
	// Summary is the one line of what the file holds, or what a missing file implies.
	Summary string
	// Actions are the keys the row offers.
	Actions []ConfigAction
	// Text is the file's text as read, "" when it is missing: the base an edit of the file must still find on disk. Only the
	// agent.toml row has it.
	Text string
	// Starter is the commented text an edit starts from when the file is missing or blank. Only the agent.toml row has it.
	Starter string
	// Supervised is the supervision rule's answer, set only on the Docbank row; the marker alone does not decide it.
	Supervised bool
}

// Has says the row offers action.
func (f ConfigFile) Has(action ConfigAction) bool { return slices.Contains(f.Actions, action) }

// StatusWords is the status as the row says it.
func (f ConfigFile) StatusWords() string {
	if f.Key == ConfigDocbank && f.Status != ConfigBroken {
		if f.Status == ConfigLoads {
			return "exists"
		}
		return "absent"
	}
	switch f.Status {
	case ConfigLoads:
		return "loads"
	case ConfigBroken:
		return "does not load"
	}
	return "missing"
}

// LoadConfigFiles asks for every row in one read. Request numbers the asks: an answer to an older one is dropped.
type LoadConfigFiles struct {
	Request int64
}

func (LoadConfigFiles) cmd() {}

// ConfigFilesLoaded answers LoadConfigFiles. Folder is tern-kata's config folder. Err says the folder could not be found, and the
// answer then holds no rows; a file that does not load is a row of its own with its error.
type ConfigFilesLoaded struct {
	Request int64
	Folder  string
	Files   []ConfigFile
	Err     error
}

func (ConfigFilesLoaded) msg() {}

// SetDocbankSupervision turns the plugin's supervision of Docbank on (On: docbank.manual is removed) or off (the marker is created).
type SetDocbankSupervision struct {
	On bool
}

func (SetDocbankSupervision) cmd() {}

// DocbankSupervisionSet answers SetDocbankSupervision. On echoes the ask; Err says the marker could not be changed.
type DocbankSupervisionSet struct {
	On  bool
	Err error
}

func (DocbankSupervisionSet) msg() {}

// AgentDraftUID is the key of the $EDITOR round trip of agent.toml.
const AgentDraftUID = "agent-config-draft"

// ErrAgentConfigChanged is SaveAgentConfig finding agent.toml no longer as the edit began: nothing was written.
var ErrAgentConfigChanged = errors.New("agent.toml changed while you edited it")

// SaveAgentConfig asks for Text to replace agent.toml: it is written only if it passes the checks an agent start applies, and only
// while the file's text is still Base, the text the edit began from ("" for a missing file).
type SaveAgentConfig struct{ Text, Base string }

func (SaveAgentConfig) cmd() {}

// AgentConfigSaved answers SaveAgentConfig with the text sent. Err is the refusal naming the problem, or ErrAgentConfigChanged;
// nothing was written then.
type AgentConfigSaved struct {
	Text string
	Err  error
}

func (AgentConfigSaved) msg() {}

// ConfigActMsg is a click on a row's action: the row's key, and the action it names.
type ConfigActMsg struct {
	Key    ConfigFileKey
	Action ConfigAction
}

func (ConfigActMsg) msg() {}

// ConfigPageState is the Config Page's state. The rows are read again on every entry.
type ConfigPageState struct {
	Loading bool
	Err     error
	Folder  string
	Files   []ConfigFile
	// Cursor indexes Files.
	Cursor int
	// Switching says a Docbank supervision switch is out; s waits for its answer.
	Switching bool
	// Saving says an agent.toml save is out; e waits for its answer.
	Saving bool
	// EditBase is agent.toml's text when the editor opened; EditText is the text a refused or unsaved edit left, which the next e
	// starts from, "" when there is none.
	EditBase, EditText string
	// Message is what the last switch said, shown under the list; MessageErr says it is a failure.
	Message    string
	MessageErr bool
	reads      Latest
}

// OnConfig says the Config Page is current.
func (m Model) OnConfig() bool { return m.Page == PageConfig }

// configMode is the Config Page's mode: the Menu's while the sidebar has the keyboard.
func configMode(m Model) Mode {
	if m.Focus == ColumnCollections {
		return ""
	}
	return ModeConfig
}

// enterConfig starts a fresh reading of the files.
func enterConfig(m Model) (Model, []Cmd) {
	m.ConfigPage = ConfigPageState{reads: m.ConfigPage.reads, Loading: true}
	return m, []Cmd{LoadConfigFiles{Request: m.ConfigPage.reads.Ask()}}
}

// clearConfig drops what the Page held and every read still out.
func clearConfig(m Model) Model {
	m.ConfigPage = ConfigPageState{reads: m.ConfigPage.reads.Discarded()}
	return m
}

// configBack is esc and backspace: the Page shown before.
func configBack(m Model) (Model, []Cmd) { return showPage(m.letteredFrom)(m) }

func updateConfig(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case DocbankSupervisionSet:
		m, cmds := docbankSupervisionSet(m, msg)
		return m, cmds, true
	case ConfigActMsg:
		m, cmds := configClicked(m, msg)
		return m, cmds, true
	case DraftEditorClosed:
		return agentEditorClosed(m, msg)
	case AgentConfigSaved:
		m, cmds := agentConfigSaved(m, msg)
		return m, cmds, true
	}
	loaded, ok := msg.(ConfigFilesLoaded)
	if !ok {
		return m, nil, false
	}
	c := &m.ConfigPage
	if !c.reads.IsCurrent(loaded.Request) {
		return m, nil, true
	}
	c.Loading, c.Err = false, loaded.Err
	if loaded.Err != nil {
		return m, nil, true
	}
	var kept ConfigFileKey
	if f := m.SelectedConfigFile(); f != nil {
		kept = f.Key
	}
	c.Folder, c.Files = loaded.Folder, slices.Clone(loaded.Files)
	c.Cursor = max(0, slices.IndexFunc(c.Files, func(f ConfigFile) bool { return f.Key == kept }))
	return m, nil, true
}

// moveConfig moves the cursor by action: one row, to either end, or a page.
func moveConfig(m Model, action Action) Model {
	n := len(m.ConfigPage.Files)
	if n == 0 {
		return m
	}
	m.ConfigPage.Cursor = movedIndex(m.ConfigPage.Cursor, n, listPage, action)
	return m
}

// SelectedConfigFile is the row under the cursor, nil when there is none.
func (m Model) SelectedConfigFile() *ConfigFile {
	c := m.ConfigPage
	if c.Cursor < 0 || c.Cursor >= len(c.Files) {
		return nil
	}
	return &c.Files[c.Cursor]
}

// Summary is the line under the title: how many files the Page lists and where they are.
func (c ConfigPageState) Summary() string {
	return strconv.Itoa(len(c.Files)) + " files in " + oneLine(c.Folder)
}

// ConfigNotice is what stands in place of the Config Page's list; Err says it is a failure, which the view draws as one.
type ConfigNotice struct {
	Text string
	Err  bool
}

// Notice is what stands in place of the list: loading, or the config folder that could not be found; false when the list is shown.
func (c ConfigPageState) Notice() (ConfigNotice, bool) {
	switch {
	case c.Loading:
		return ConfigNotice{Text: "loading config files..."}, true
	case c.Err != nil:
		return ConfigNotice{Text: "tern-kata's config folder could not be found: " + oneLine(c.Err.Error()), Err: true}, true
	}
	return ConfigNotice{}, false
}

// configOpenLabel is enter's label on the selected row: what it opens.
func configOpenLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigOpen)
	}
	return "open"
}

// ConfigActionLabel is the words of the selected row's action, as the footer and the row's button say them.
func ConfigActionLabel(f ConfigFile, action ConfigAction) string {
	if action == ConfigEdit {
		return "edit " + f.Title
	}
	if action == ConfigSwitch {
		if f.Status == ConfigLoads {
			return "turn supervision on"
		}
		return "turn supervision off"
	}
	switch f.Key {
	case ConfigLanes:
		return "open Lanes card"
	case ConfigFilters:
		return "open Filter card"
	case ConfigViews:
		return "open Views"
	case ConfigSeen:
		return "open Runs Page"
	case ConfigWorker:
		return "open Worker card"
	case ConfigKata:
		return "open Daemons Page"
	}
	return "open"
}

// selectedConfigOffers builds the condition of an entry that is on offer while the selected row has action.
func selectedConfigOffers(action ConfigAction) func(Model) bool {
	return func(m Model) bool {
		f := m.SelectedConfigFile()
		return f != nil && f.Has(action)
	}
}

// openConfigFile is enter: the card or Page that already edits the selected file, entered the way its own key would enter it. The
// Config Page is left first by the Page switch, so a refusal (no Project) is the target Page's own and no card opens.
func openConfigFile(m Model) (Model, []Cmd) {
	f := m.SelectedConfigFile()
	if f == nil || !f.Has(ConfigOpen) {
		return m, nil
	}
	switch f.Key {
	case ConfigLanes:
		m, cmds := showPage(PageBoard)(m)
		m.lanesWanted = m.OnBoard()
		return m, cmds
	case ConfigFilters:
		m, cmds := showPage(PageIssues)(m)
		if m.Page == PageIssues {
			m = openFilterForm(m)
		}
		return m, cmds
	case ConfigViews:
		m, cmds := configBack(m)
		if m.OnConfig() {
			return m, cmds
		}
		m, more := openMenuLevel(m, MenuViews)
		return m, append(cmds, more...)
	case ConfigSeen:
		return showPage(PageRuns)(m)
	case ConfigWorker:
		m, cmds := showPage(PageJobs)(m)
		if m.OnJobs() {
			var more []Cmd
			m, more = openWorkerCard(m)
			cmds = append(cmds, more...)
		}
		return m, cmds
	case ConfigKata:
		return showPage(PageDaemons)(m)
	}
	return m, nil
}

// configClicked is a click on a row's action: the cursor goes to the row and the action runs as its key would.
func configClicked(m Model, msg ConfigActMsg) (Model, []Cmd) {
	if !m.OnConfig() || m.CurrentMode() != ModeConfig {
		return m, nil
	}
	i := slices.IndexFunc(m.ConfigPage.Files, func(f ConfigFile) bool { return f.Key == msg.Key })
	if i < 0 || !m.ConfigPage.Files[i].Has(msg.Action) {
		return m, nil
	}
	m.ConfigPage.Cursor = i
	switch msg.Action {
	case ConfigSwitch:
		return switchDocbank(m)
	case ConfigEdit:
		return editAgentConfig(m)
	}
	return openConfigFile(m)
}

// switchDocbank is s on the Docbank row: supervision goes off when docbank.manual is absent and on when it is there. It is
// ignored while a switch is out.
func switchDocbank(m Model) (Model, []Cmd) {
	f := m.SelectedConfigFile()
	if f == nil || !f.Has(ConfigSwitch) || m.ConfigPage.Switching {
		return m, nil
	}
	m.ConfigPage.Switching, m.ConfigPage.Message, m.ConfigPage.MessageErr = true, "", false
	return m, []Cmd{SetDocbankSupervision{On: f.Status == ConfigLoads}}
}

// docbankSwitchMessage is what a switch that took effect says. Turning supervision on promises nothing beyond the rule: the row,
// read again, gives the reason when the rule still fails.
func docbankSwitchMessage(on bool) string {
	if on {
		return "Docbank supervision is on: the plugin starts a local Docbank within 10 seconds when the rule holds."
	}
	return "Docbank supervision is off: a Docbank that is running keeps running."
}

// docbankSupervisionSet reads the rows again after a switch took effect; a failed switch shows its error and leaves the rows.
func docbankSupervisionSet(m Model, msg DocbankSupervisionSet) (Model, []Cmd) {
	c := &m.ConfigPage
	if !m.OnConfig() || !c.Switching {
		return m, nil
	}
	c.Switching = false
	if msg.Err != nil {
		c.Message, c.MessageErr = "Docbank supervision not changed: "+oneLine(msg.Err.Error()), true
		return m, nil
	}
	c.Message, c.MessageErr = docbankSwitchMessage(msg.On), false
	return m, []Cmd{LoadConfigFiles{Request: c.reads.Ask()}}
}

// agentEditStart is the text the editor is given: the refused or unsaved edit when there is one, else the file, else the starter.
func (m Model) agentEditStart() string {
	if m.ConfigPage.EditText != "" {
		return m.ConfigPage.EditText
	}
	f := m.SelectedConfigFile()
	switch {
	case f == nil:
		return ""
	case strings.TrimSpace(f.Text) == "":
		return f.Starter
	}
	return f.Text
}

// editAgentConfig is e on the agent.toml row: the file opens in $EDITOR, from the starter when there is none. It is ignored while a
// save is out.
func editAgentConfig(m Model) (Model, []Cmd) {
	f := m.SelectedConfigFile()
	c := &m.ConfigPage
	if f == nil || !f.Has(ConfigEdit) || c.Saving {
		return m, nil
	}
	c.EditBase, c.Message, c.MessageErr = f.Text, "", false
	return m, []Cmd{OpenDraftEditor{UID: AgentDraftUID, Text: m.agentEditStart(), Suffix: ".toml"}}
}

// agentEditorClosed takes the text back from $EDITOR: an unchanged or blank result changes nothing, anything else is sent to be
// checked and written. An editor that could not open is the Page's message.
func agentEditorClosed(m Model, msg DraftEditorClosed) (Model, []Cmd, bool) {
	if msg.UID != AgentDraftUID {
		return m, nil, false
	}
	c := &m.ConfigPage
	if !m.OnConfig() {
		return m, nil, true
	}
	if msg.Err != nil {
		c.Message, c.MessageErr = "agent.toml was not opened: "+oneLine(msg.Err.Error()), true
		return m, nil, true
	}
	text := normalizeLineBreaks(msg.Text)
	if strings.TrimSpace(text) == "" || text == normalizeLineBreaks(m.agentEditStart()) {
		return m, nil, true
	}
	c.Saving, c.Message, c.MessageErr = true, "saving agent.toml...", false
	return m, []Cmd{SaveAgentConfig{Text: text, Base: c.EditBase}}, true
}

// agentConfigSaved shows what the save did and reads the rows again. A refused text and a text over a changed file are both kept
// for the next e.
func agentConfigSaved(m Model, msg AgentConfigSaved) (Model, []Cmd) {
	c := &m.ConfigPage
	if !m.OnConfig() || !c.Saving {
		return m, nil
	}
	c.Saving = false
	switch {
	case errors.Is(msg.Err, ErrAgentConfigChanged):
		c.EditText, c.MessageErr = msg.Text, true
		c.Message = "agent.toml changed while you edited it, so nothing was saved. Your text is kept but lacks that change; press e to edit it and save to replace the file with it."
		return m, []Cmd{LoadConfigFiles{Request: c.reads.Ask()}}
	case msg.Err != nil:
		c.EditText, c.MessageErr = msg.Text, true
		c.Message = "agent.toml was not saved and the file is as it was: " + oneLine(msg.Err.Error()) + ". Press e to keep editing; your text is kept."
		return m, nil
	}
	c.EditText, c.MessageErr = "", false
	c.Message = "agent.toml saved; agents started from now on use it."
	return m, []Cmd{LoadConfigFiles{Request: c.reads.Ask()}}
}

// configRows are the Config Page's own keys beyond its movement keys.
var configRows = []actionEntry{
	{action: OpenConfigFile, modes: []Mode{ModeConfig}, keys: keys("enter", "↵"), help: "open", label: configOpenLabel, footer: []Mode{ModeConfig}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 40, "open the card or Page that edits the selected file"}}, handle: openConfigFile, when: selectedConfigOffers(ConfigOpen)},
	{action: SwitchDocbank, modes: []Mode{ModeConfig}, keys: keys("s", "s"), help: "supervision", label: configSwitchLabel, footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 41, "turn the plugin's supervision of Docbank off or on"}}, handle: switchDocbank, when: selectedConfigOffers(ConfigSwitch)},
	{action: EditAgentConfig, modes: []Mode{ModeConfig}, keys: keys("e", "e"), help: "edit", label: configEditLabel, footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 42, "edit agent.toml in $EDITOR; it is checked before it is written"}}, handle: editAgentConfig, when: selectedConfigOffers(ConfigEdit)},
	{action: LetteredBack, modes: []Mode{ModeConfig}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: configBack},
}

func configSwitchLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigSwitch)
	}
	return "supervision"
}

func configEditLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigEdit)
	}
	return "edit"
}
