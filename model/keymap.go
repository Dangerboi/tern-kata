package model

type Action string

const (
	MoveDown    Action = "down"
	MoveUp      Action = "up"
	MoveFirst   Action = "first"
	MoveLast    Action = "last"
	QuitAction  Action = "quit"
	ToggleHelp  Action = "help"
	CloseHelp   Action = "close-help"
	DrillIn     Action = "drill-in"
	DrillOut    Action = "drill-out"
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
	ModeList        Mode = "list"
	ModeHelp        Mode = "help"
	ModeCollections Mode = "collections"
	ModeError       Mode = "error"
)

// Chord dispatches Action; Key and Help label it. Empty Mode is global;
// otherwise the binding is active only in that mode. ShowInFooter selects hints.
var Keymap = []Binding{
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Issue", Mode: ModeList},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Issue", Mode: ModeList},
	{Chord: "escape", Key: "esc", Action: DrillOut, Help: "Collections", Mode: ModeList, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Collection", Mode: ModeCollections},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Collection", Mode: ModeCollections},
	{Chord: "enter", Key: "enter", Action: DrillIn, Help: "open Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "q", Key: "q", Action: QuitAction, Help: "quit", ShowInFooter: true},
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: RetryAction, Help: "retry daemon", Mode: ModeError, ShowInFooter: true},
	{Chord: "?", Key: "?", Action: ToggleHelp, Help: "help", ShowInFooter: true},
	{Chord: "shift+?", Key: "?", Action: ToggleHelp, Help: "help"},
	{Chord: "escape", Key: "esc", Action: CloseHelp, Help: "close help", Mode: ModeHelp, ShowInFooter: true},
}
