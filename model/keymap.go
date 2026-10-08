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
	ToggleHelp Action = "help"
	CloseHelp Action = "close-help"
	ToggleExpansion Action = "toggle-expansion"
	ExpandIssue Action = "expand"
	CollapseIssue Action = "collapse"
	ToggleAllExpansion Action = "toggle-all-expansion"
	ToggleIssueView Action = "toggle-issue-view"
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
	ModeList Mode = "list"
	ModeHelp Mode = "help"
	ModeError Mode = "error"
	ModeDetail Mode = "detail"
	ModeCollections Mode = "collections"
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
	{Chord: "escape", Key: "esc", Action: DrillOut, Help: "Collections", Mode: ModeList, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Collection", Mode: ModeCollections},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Collection", Mode: ModeCollections},
	{Chord: "enter", Key: "enter", Action: DrillIn, Help: "open Collection", Mode: ModeCollections, ShowInFooter: true},
	{Chord: "q", Key: "q", Action: QuitAction, Help: "quit", ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: DrillIn, Help: "open", Mode: ModeList, ShowInFooter: true},
	{Chord: "shift+l", Key: "L", Action: ToggleDetail, Help: "toggle detail", Mode: ModeList, ShowInFooter: true},
	{Chord: "shift+l", Key: "L", Action: ToggleDetail, Help: "toggle detail", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next reference", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous reference", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first reference", Mode: ModeDetail},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last reference", Mode: ModeDetail},
	{Chord: "enter", Key: "enter", Action: DrillIn, Help: "open reference", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: DrillOut, Help: "back", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: NextDetailSection, Help: "next section", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: PreviousDetailSection, Help: "previous section", Mode: ModeDetail},
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: RetryAction, Help: "retry daemon", Mode: ModeError, ShowInFooter: true},
	{Chord: "?", Key: "?", Action: ToggleHelp, Help: "help", ShowInFooter: true},
	{Chord: "shift+?", Key: "?", Action: ToggleHelp, Help: "help"},
	{Chord: "escape", Key: "esc", Action: CloseHelp, Help: "close help", Mode: ModeHelp, ShowInFooter: true},
	{Chord:"x", Key:"x", Action:OpenClose, Help:"close Issue", Mode:ModeList, ShowInFooter:true},
	{Chord:"x", Key:"x", Action:OpenClose, Help:"close Issue", Mode:ModeDetail, ShowInFooter:true},
	{Chord:"r", Key:"r", Action:ReopenAction, Help:"reopen Issue", Mode:ModeList, ShowInFooter:true},
	{Chord:"r", Key:"r", Action:ReopenAction, Help:"reopen Issue", Mode:ModeDetail, ShowInFooter:true},
	{Chord:"tab", Key:"tab", Action:CloseNext, Help:"next field", Mode:ModeCloseForm, ShowInFooter:true},
	{Chord:"shift+tab", Key:"shift+tab", Action:ClosePrevious, Help:"previous field", Mode:ModeCloseForm},
	{Chord:"ctrl+o", Key:"ctrl+o", Action:CloseSave, Help:"close", Mode:ModeCloseForm, ShowInFooter:true},
	{Chord:"ctrl+d", Key:"ctrl+d", Action:CloseQuick, Help:"quick done", Mode:ModeCloseForm, ShowInFooter:true},
	{Chord:"escape", Key:"esc", Action:CloseCancel, Help:"cancel", Mode:ModeCloseForm, ShowInFooter:true},
	{Chord:"backspace", Key:"backspace", Action:CloseErase, Help:"erase", Mode:ModeCloseForm},
	{Chord:"ctrl+u", Key:"ctrl+u", Action:CloseClear, Help:"clear field", Mode:ModeCloseForm},
	{Chord:"q", Key:"q", Action:"close-text", Help:"", Mode:ModeCloseForm},
	{Chord:"+", Key:"+", Action:AddLabelAction, Help:"add label", Mode:ModeList},
	{Chord:"-", Key:"-", Action:RemoveLabelAction, Help:"remove label", Mode:ModeList},
	{Chord:"a", Key:"a", Action:AssignOwnerAction, Help:"assign owner", Mode:ModeList},
	{Chord:"shift+a", Key:"A", Action:ClearOwnerAction, Help:"clear owner", Mode:ModeList},
	{Chord:"!", Key:"!", Action:PriorityAction, Help:"priority", Mode:ModeList},
	{Chord:"+", Key:"+", Action:AddLabelAction, Help:"add label", Mode:ModeDetail},
	{Chord:"-", Key:"-", Action:RemoveLabelAction, Help:"remove label", Mode:ModeDetail},
	{Chord:"a", Key:"a", Action:AssignOwnerAction, Help:"assign owner", Mode:ModeDetail},
	{Chord:"shift+a", Key:"A", Action:ClearOwnerAction, Help:"clear owner", Mode:ModeDetail},
	{Chord:"!", Key:"!", Action:PriorityAction, Help:"priority", Mode:ModeDetail},
	{Chord:"enter", Key:"enter", Action:FieldSubmit, Help:"save", Mode:ModeFieldForm, ShowInFooter:true},
	{Chord:"ctrl+o", Key:"ctrl+o", Action:FieldSubmit, Help:"save", Mode:ModeFieldForm},
	{Chord:"ctrl+s", Key:"ctrl+s", Action:FieldSubmit, Help:"save", Mode:ModeFieldForm},
	{Chord:"escape", Key:"esc", Action:FieldCancel, Help:"cancel", Mode:ModeFieldForm, ShowInFooter:true},
	{Chord:"tab", Key:"tab", Action:FieldComplete, Help:"complete label", Mode:ModeFieldForm, ShowInFooter:true},
	{Chord:"down", Key:"down", Action:FieldNext, Help:"next completion", Mode:ModeFieldForm},
	{Chord:"up", Key:"up", Action:FieldPrevious, Help:"previous completion", Mode:ModeFieldForm},
	{Chord:"backspace", Key:"backspace", Action:FieldErase, Help:"erase", Mode:ModeFieldForm},
	{Chord:"ctrl+u", Key:"ctrl+u", Action:FieldReset, Help:"clear input", Mode:ModeFieldForm},
}
