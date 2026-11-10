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

// SwitchDocbank asks to move the default kata daemon's old pinned Docbank setup to kata's local one.
type SwitchDocbank struct{}

func (SwitchDocbank) cmd() {}

// DocbankSwitched answers SwitchDocbank. Err carries kata's error line, or why the file could not be changed, and says when
// config.toml was not restored; the rows are not read again then.
type DocbankSwitched struct{ Err error }

func (DocbankSwitched) msg() {}

type ConfigFileKey string

const (
	ConfigLanes   ConfigFileKey = "lanes"
	ConfigFilters ConfigFileKey = "filters"
	ConfigViews   ConfigFileKey = "views"
	ConfigSeen    ConfigFileKey = "seen"
	ConfigWorker  ConfigFileKey = "worker"
	ConfigAgent   ConfigFileKey = "agent"
	ConfigKata    ConfigFileKey = "kata"
	// ConfigDocbank is the Docbank row: the [docbank] section of the default kata daemon's config.toml, which is its path.
	ConfigDocbank ConfigFileKey = "docbank"
	// ConfigEmbeddings is the Search by meaning row: the connected daemon's embeddings status, not a file, so it has no path.
	ConfigEmbeddings ConfigFileKey = "embeddings"
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
	// ConfigSwitchDocbank is s: the old pinned Docbank setup becomes kata's local one. Only the Docbank row in that form has it.
	ConfigSwitchDocbank ConfigAction = "switch"
	// ConfigTurnOn is s: the Search by meaning card asks to turn search by meaning on. Only the Search by meaning row has it, and
	// only while kata would take a turn-on from this account.
	ConfigTurnOn ConfigAction = "turn-on"
	// ConfigRestart is r: the Search by meaning card asks to restart the kata daemon. Only the Search by meaning row has it, and only
	// while kata owes a restart and the daemon is local.
	ConfigRestart ConfigAction = "restart"
)

type ConfigFile struct {
	Key     ConfigFileKey
	Title   string
	Path    string
	Status  ConfigFileStatus
	Err     string
	Summary string
	Actions []ConfigAction
	// Text is the file's text as read, "" when it is missing: the base an edit of the file must still find on disk. Only the
	// agent.toml row has it.
	Text string
	// Starter is the commented text an edit starts from when the file is missing or blank. Only the agent.toml row has it.
	Starter string
}

func (f ConfigFile) Has(action ConfigAction) bool { return slices.Contains(f.Actions, action) }

func (f ConfigFile) StatusWords() string {
	if f.Key == ConfigEmbeddings {
		return ""
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

type ConfigActMsg struct {
	Key    ConfigFileKey
	Action ConfigAction
}

func (ConfigActMsg) msg() {}

type ConfigPageState struct {
	Loading bool
	Err     error
	Folder  string
	Files   []ConfigFile
	Cursor  int
	// AskSwitch says the question of the Docbank switch is shown; Switching says the switch is out, and s waits for its answer.
	AskSwitch, Switching bool
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

func (m Model) OnConfig() bool { return m.Page == PageConfig }

var configMode = sidebarMode(ModeConfig)

func enterConfig(m Model) (Model, []Cmd) {
	c := ConfigPageState{reads: m.ConfigPage.reads, Loading: true, Switching: m.ConfigPage.Switching}
	if c.Switching {
		c.Message = "switching Docbank..."
	}
	m.ConfigPage = c
	load := LoadConfigFiles{Request: m.ConfigPage.reads.Ask()}
	status := askEmbeddings(&m, 0)
	return m, []Cmd{load, status}
}

// clearConfig drops the Page's state but keeps Switching: the switch's answer still comes and is reported. The status read is the
// Pane's, not the Page's, so it stays; a busy answer that comes after the Page is left is not read again unless the card shows it.
func clearConfig(m Model) Model {
	m.ConfigPage = ConfigPageState{reads: m.ConfigPage.reads.Discarded(), Switching: m.ConfigPage.Switching}
	return m
}

func updateConfig(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case ConfigActMsg:
		m, cmds := configClicked(m, msg)
		return m, cmds, true
	case DraftEditorClosed:
		return agentEditorClosed(m, msg)
	case DocbankSwitched:
		m, cmds := docbankSwitched(m, msg)
		return m, cmds, true
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
	c.Folder = loaded.Folder
	c.setFiles(m.Daemon.Name, m.Embeddings, loaded.Files)
	return m, nil, true
}

func moveConfig(m Model, action Action) Model {
	moveCursor(&m.ConfigPage.Cursor, len(m.ConfigPage.Files), action)
	return m
}

func (m Model) SelectedConfigFile() *ConfigFile {
	return rowAt(m.ConfigPage.Files, m.ConfigPage.Cursor)
}

// setFiles lists files and, once the status has been read, the Search by meaning row after the Docbank row. The cursor stays on the
// row it was on.
func (c *ConfigPageState) setFiles(daemon string, search EmbeddingsSetup, files []ConfigFile) {
	var kept ConfigFileKey
	if f := rowAt(c.Files, c.Cursor); f != nil {
		kept = f.Key
	}
	c.Files = slices.Clone(files)
	if search.Read {
		c.Files = withEmbeddingsRow(c.Files, embeddingsRow(daemon, search))
	}
	c.Cursor = max(0, slices.IndexFunc(c.Files, func(f ConfigFile) bool { return f.Key == kept }))
}

func (c ConfigPageState) Summary() string {
	files := len(c.Files)
	if slices.ContainsFunc(c.Files, func(f ConfigFile) bool { return f.Key == ConfigEmbeddings }) {
		files--
	}
	return strconv.Itoa(files) + " files in " + oneLine(c.Folder)
}

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

func configOpenLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigOpen)
	}
	return "open"
}

// ConfigActionLabel is the words of the selected row's action, as the footer and the row's button say them.
func ConfigActionLabel(f ConfigFile, action ConfigAction) string {
	switch action {
	case ConfigEdit:
		return "edit " + f.Title
	case ConfigSwitchDocbank:
		return "switch to kata's local Docbank"
	case ConfigTurnOn:
		return "turn on search by meaning"
	case ConfigRestart:
		return "restart the kata daemon…"
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
	case ConfigEmbeddings:
		return "open Search by meaning card"
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
		m, cmds := letteredBack(m)
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
	case ConfigEmbeddings:
		return openEmbeddingsCard(m)
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
	case ConfigEdit:
		return editAgentConfig(m)
	case ConfigSwitchDocbank:
		return askSwitchDocbank(m)
	case ConfigTurnOn:
		return turnOnEmbeddings(m)
	case ConfigRestart:
		return restartEmbeddingsDaemon(m)
	}
	return openConfigFile(m)
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

var configRows = []actionEntry{
	{action: OpenConfigFile, modes: []Mode{ModeConfig}, keys: keys("enter", "↵"), help: "open", label: configOpenLabel, footer: []Mode{ModeConfig}, footerKey: "↵", sheet: []sheetLine{{sheetLettered, 40, "open the card or Page that edits the selected file"}}, handle: openConfigFile, when: selectedConfigOffers(ConfigOpen)},
	{action: EditAgentConfig, modes: []Mode{ModeConfig}, keys: keys("e", "e"), help: "edit", label: configEditLabel, footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 42, "edit agent.toml in $EDITOR; it is checked before it is written"}}, handle: editAgentConfig, when: selectedConfigOffers(ConfigEdit)},
	{action: ConfigSetUp, modes: []Mode{ModeConfig}, keys: keys("s", "s"), help: "set up", label: configSetUpLabel, footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 43, "set up the selected row: switch the old pinned Docbank to kata's local Docbank, or turn on search by meaning (both ask first)"}}, handle: configSetUp, when: selectedConfigSetsUp},
	{action: ConfigRestartDaemon, modes: []Mode{ModeConfig}, keys: keys("r", "r"), help: "restart", label: configRestartLabel, footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 44, "restart the kata daemon to finish setting up search by meaning (asks first; only while a restart is owed and the daemon is local)"}}, handle: restartEmbeddingsDaemon, when: selectedConfigOffers(ConfigRestart)},
	{action: ConfirmSwitchDocbankAction, modes: []Mode{ModeSwitchDocbank}, keys: keys("y", "y"), help: "switch", footer: []Mode{ModeSwitchDocbank}, handle: confirmSwitchDocbank},
	{action: KeepDocbankAction, modes: []Mode{ModeSwitchDocbank}, keys: keys("n", "n", "escape", "esc"), help: "keep", footer: []Mode{ModeSwitchDocbank}, handle: pure(func(m Model) Model { m.ConfigPage.AskSwitch = false; return m })},
	{action: QuitNowAction, modes: []Mode{ModeSwitchDocbank}, keys: keys("ctrl+c", "ctrl+c", "meta+c", ""), help: "quit at once", handle: pure(quitNow)},
	letteredBackRow(ModeConfig, letteredBack),
}

// selectedConfigSetsUp is the condition of s: the selected row offers the Docbank switch or turning search by meaning on.
func selectedConfigSetsUp(m Model) bool {
	return selectedConfigOffers(ConfigSwitchDocbank)(m) || selectedConfigOffers(ConfigTurnOn)(m)
}

func configSetUpLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		if f.Has(ConfigTurnOn) {
			return ConfigActionLabel(*f, ConfigTurnOn)
		}
		return ConfigActionLabel(*f, ConfigSwitchDocbank)
	}
	return "set up"
}

func configRestartLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigRestart)
	}
	return "restart"
}

// configSetUp is s: what the selected row sets up asks its question first.
func configSetUp(m Model) (Model, []Cmd) {
	if f := m.SelectedConfigFile(); f != nil && f.Has(ConfigTurnOn) {
		return turnOnEmbeddings(m)
	}
	return askSwitchDocbank(m)
}

// SwitchDocbankQuestion is what s on the old pinned Docbank row asks.
const SwitchDocbankQuestion = "Switch to kata's local Docbank? kata restarts its daemon. y/n"

// askSwitchDocbank is s on the Docbank row in its old pinned form. It is ignored while a switch is out.
func askSwitchDocbank(m Model) (Model, []Cmd) {
	f := m.SelectedConfigFile()
	if f == nil || !f.Has(ConfigSwitchDocbank) || m.ConfigPage.Switching {
		return m, nil
	}
	m.ConfigPage.AskSwitch, m.HelpOpen = true, false
	return m, nil
}

// confirmSwitchDocbank is y: the question is dismissed and the switch goes out; its answer follows as DocbankSwitched.
func confirmSwitchDocbank(m Model) (Model, []Cmd) {
	c := &m.ConfigPage
	c.AskSwitch = false
	if c.Switching {
		return m, nil
	}
	c.Switching, c.Message, c.MessageErr = true, "switching Docbank...", false
	return m, []Cmd{SwitchDocbank{}}
}

// DocbankSwitchQuitRefusal is why the Pane does not quit while the Docbank switch is out: quitting would leave config.toml
// without its [docbank] section when kata then fails, as the restore runs after kata's answer.
const DocbankSwitchQuitRefusal = "The Docbank switch is still running; wait for its answer before quitting."

// refuseQuitDuringSwitch keeps the Pane open and says why, in a Notice and on the Config Page's message line when it is shown.
func refuseQuitDuringSwitch(m Model) Model {
	m.AskQuit = false
	m.Notice = DocbankSwitchQuitRefusal
	if m.OnConfig() {
		m.ConfigPage.Message, m.ConfigPage.MessageErr = DocbankSwitchQuitRefusal, true
	}
	return m
}

// docbankSwitched shows what the switch did. Switching outlives leaving the Page, so the answer is always reported: on the Page's
// message line, or as a Notice when the Page is not shown. A switch reads the rows again; a failed one keeps them, as kata's own
// error is all there is to show.
func docbankSwitched(m Model, msg DocbankSwitched) (Model, []Cmd) {
	c := &m.ConfigPage
	if !c.Switching {
		return m, nil
	}
	c.Switching = false
	text, failed := "Switched to kata's local Docbank; kata restarted its daemon. docbank.key (in KATA_HOME) and docbank.manual (in tern-kata's config folder) are no longer used and can be deleted.", false
	if msg.Err != nil {
		text, failed = "Docbank was not switched: "+oneLine(msg.Err.Error()), true
	}
	if !m.OnConfig() {
		m.Notice = text
		return m, nil
	}
	c.Message, c.MessageErr = text, failed
	if failed {
		return m, nil
	}
	return m, []Cmd{LoadConfigFiles{Request: c.reads.Ask()}}
}

func configEditLabel(m Model) string {
	if f := m.SelectedConfigFile(); f != nil {
		return ConfigActionLabel(*f, ConfigEdit)
	}
	return "edit"
}
