package model

import (
	"slices"
	"strconv"
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
	ConfigWorker  ConfigFileKey = "worker"
	ConfigAgent   ConfigFileKey = "agent"
	ConfigKata    ConfigFileKey = "kata"
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
}

// Has says the row offers action.
func (f ConfigFile) Has(action ConfigAction) bool { return slices.Contains(f.Actions, action) }

// StatusWords is the status as the row says it.
func (f ConfigFile) StatusWords() string {
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

// ConfigPageState is the Config Page's state. The rows are read again on every entry.
type ConfigPageState struct {
	Loading bool
	Err     error
	Folder  string
	Files   []ConfigFile
	// Cursor indexes Files.
	Cursor int
	reads  Latest
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
	c.Folder, c.Files = loaded.Folder, slices.Clone(loaded.Files)
	c.Cursor = 0
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

// configRows are the Config Page's own keys beyond its movement keys.
var configRows = []actionEntry{
	{action: LetteredBack, modes: []Mode{ModeConfig}, keys: keys("escape", "esc", "backspace", "⌫"), help: "back", footer: []Mode{ModeConfig}, sheet: []sheetLine{{sheetLettered, 1, "back to the Page shown before"}}, handle: configBack},
}
