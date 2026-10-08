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
)

type Binding struct {
	Chord  string
	Key    string
	Action Action
	Help   string
}

var Keymap = []Binding{
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Issue"},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Issue"},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Issue"},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Issue"},
	{Chord: "space", Key: "space", Action: ToggleExpansion, Help: "expand/collapse"},
	{Chord: "right", Key: "right", Action: ExpandIssue, Help: "expand"},
	{Chord: "left", Key: "left", Action: CollapseIssue, Help: "collapse"},
	{Chord: "shift+e", Key: "E", Action: ToggleAllExpansion, Help: "expand/collapse all"},
	{Chord: "v", Key: "v", Action: ToggleIssueView, Help: "flat/nested"},
	{Chord: "q", Key: "q", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
}

func LookupBinding(chord string) (Binding, bool) {
	for _, binding := range Keymap {
		if binding.Chord == chord {
			return binding, true
		}
	}
	return Binding{}, false
}
