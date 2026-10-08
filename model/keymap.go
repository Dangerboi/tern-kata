package model

type Action string

const (
	MoveDown   Action = "down"
	MoveUp     Action = "up"
	MoveFirst  Action = "first"
	MoveLast   Action = "last"
	QuitAction Action = "quit"
	RetryAction Action = "retry"
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
	{Chord: "q", Key: "q", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: RetryAction, Help: "retry daemon"},
}

func LookupBinding(chord string) (Binding, bool) {
	for _, binding := range Keymap {
		if binding.Chord == chord {
			return binding, true
		}
	}
	return Binding{}, false
}
