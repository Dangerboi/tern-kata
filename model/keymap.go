package model

type Action string

const (
	MoveDown   Action = "down"
	MoveUp     Action = "up"
	MoveFirst  Action = "first"
	MoveLast   Action = "last"
	QuitAction Action = "quit"
	DrillIn      Action = "drill-in"
	DrillOut     Action = "drill-out"
	ToggleDetail Action = "toggle-detail"
	NextDetailSection Action = "next-detail-section"
	PreviousDetailSection Action = "previous-detail-section"
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
	{Chord: "enter", Key: "enter", Action: DrillIn, Help: "open"},
	{Chord: "escape", Key: "esc", Action: DrillOut, Help: "back"},
	{Chord: "shift+l", Key: "L", Action: ToggleDetail, Help: "toggle detail"},
	{Chord: "tab", Key: "tab", Action: NextDetailSection, Help: "next section"},
	{Chord: "shift+tab", Key: "shift+tab", Action: PreviousDetailSection, Help: "previous section"},
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
