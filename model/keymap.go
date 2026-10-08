package model

type Action string

const (
	MoveDown   Action = "down"
	MoveUp     Action = "up"
	MoveFirst  Action = "first"
	MoveLast   Action = "last"
	QuitAction Action = "quit"
	ToggleExpansion Action = "toggle-expansion"
	ExpandIssue Action = "expand"
	CollapseIssue Action = "collapse"
	ToggleAllExpansion Action = "toggle-all-expansion"
	ToggleIssueView Action = "toggle-issue-view"
	ToggleHelp Action = "help"
	CloseHelp  Action = "close-help"
	RetryAction Action = "retry"
)

type Binding struct {
	Chord        string
	Key          string
	Action       Action
	Help         string
	Mode         Mode
	ShowInFooter bool
}

type Mode string

const (
	ModeList  Mode = "list"
	ModeHelp  Mode = "help"
	ModeError Mode = "error"
)

// Chord dispatches Action; Key and Help label it. Empty Mode is global;
// otherwise the binding is active only in that mode. ShowInFooter selects hints.
var Keymap = []Binding{
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Issue", Mode: ModeList},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Issue", Mode: ModeList},
	{Chord: "space", Key: "space", Action: ToggleExpansion, Help: "expand/collapse", Mode: ModeList, ShowInFooter: true},
	{Chord: "right", Key: "right", Action: ExpandIssue, Help: "expand", Mode: ModeList},
	{Chord: "left", Key: "left", Action: CollapseIssue, Help: "collapse", Mode: ModeList},
	{Chord: "shift+e", Key: "E", Action: ToggleAllExpansion, Help: "expand/collapse all", Mode: ModeList},
	{Chord: "v", Key: "v", Action: ToggleIssueView, Help: "flat/nested", Mode: ModeList, ShowInFooter: true},
	{Chord: "q", Key: "q", Action: QuitAction, Help: "quit", ShowInFooter: true},
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: RetryAction, Help: "retry daemon", Mode: ModeError, ShowInFooter: true},
	{Chord: "?", Key: "?", Action: ToggleHelp, Help: "help", ShowInFooter: true},
	{Chord: "shift+?", Key: "?", Action: ToggleHelp, Help: "help"},
	{Chord: "escape", Key: "esc", Action: CloseHelp, Help: "close help", Mode: ModeHelp, ShowInFooter: true},
}
