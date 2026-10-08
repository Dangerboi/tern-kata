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
	DrillIn Action = "drill-in"
	DrillOut Action = "drill-out"
	RetryAction Action = "retry"
	SearchIssues Action = "search"
	SearchEnter Action = "search-enter"
	SearchBack Action = "search-back"
	SearchQuery Action = "search-query"
	DeleteInput Action = "delete-input"
	ClearInput Action = "clear-input"
	CycleStatus Action = "cycle-status"
	OpenFilterForm Action = "filter-form"
	ApplyFilters Action = "apply-filters"
	CancelFilters Action = "cancel-filters"
	NextFilterField Action = "next-filter-field"
	PreviousFilterField Action = "previous-filter-field"
	NextFilterChoice Action = "next-filter-choice"
	PreviousFilterChoice Action = "previous-filter-choice"
	ResetFilterForm Action = "reset-filter-form"
	ClearFilters Action = "clear-filters"
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
	ModeSearch Mode = "search"
	ModeSearchResults Mode = "search-results"
	ModeFilterForm Mode = "filter-form"
)

// Chord dispatches Action; Key and Help label it. Empty Mode is global;
// otherwise the binding is active only in that mode. ShowInFooter selects hints.
var Keymap = []Binding{
	{Chord: "c", Key: "c", Action: ClearFilters, Help: "clear filters", Mode: ModeList, ShowInFooter: true},
	{Chord: "f", Key: "f", Action: OpenFilterForm, Help: "filters", Mode: ModeList, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: ApplyFilters, Help: "apply filters", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: ApplyFilters, Help: "apply filters", Mode: ModeFilterForm},
	{Chord: "escape", Key: "esc", Action: CancelFilters, Help: "cancel", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: NextFilterField, Help: "next field", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: NextFilterField, Help: "next field", Mode: ModeFilterForm},
	{Chord: "shift+tab", Key: "shift+tab", Action: PreviousFilterField, Help: "previous field", Mode: ModeFilterForm},
	{Chord: "right", Key: "right", Action: NextFilterChoice, Help: "next choice", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: NextFilterChoice, Help: "next choice", Mode: ModeFilterForm},
	{Chord: "left", Key: "left", Action: PreviousFilterChoice, Help: "previous choice", Mode: ModeFilterForm},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeFilterForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeFilterForm},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: ResetFilterForm, Help: "reset form", Mode: ModeFilterForm},
	{Chord: "s", Key: "s", Action: CycleStatus, Help: "cycle status", Mode: ModeList, ShowInFooter: true},
	{Chord: "/", Key: "/", Action: SearchIssues, Help: "search", Mode: ModeList, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: SearchEnter, Help: "results", Mode: ModeSearch, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: SearchBack, Help: "cancel search", Mode: ModeSearch, ShowInFooter: true},
	{Chord: "up", Key: "up", Action: MoveUp, Help: "previous result", Mode: ModeSearch},
	{Chord: "down", Key: "down", Action: MoveDown, Help: "next result", Mode: ModeSearch},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeSearch},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear search", Mode: ModeSearch},
	{Chord: "up", Key: "up", Action: MoveUp, Help: "previous result", Mode: ModeSearchResults, ShowInFooter: true},
	{Chord: "down", Key: "down", Action: MoveDown, Help: "next result", Mode: ModeSearchResults, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: SearchEnter, Help: "commit search", Mode: ModeSearchResults, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: SearchBack, Help: "edit query", Mode: ModeSearchResults, ShowInFooter: true},
	{Chord: "/", Key: "/", Action: SearchQuery, Help: "edit query", Mode: ModeSearchResults},
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
	{Chord: "ctrl+c", Key: "ctrl+c", Action: QuitAction, Help: "quit"},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: RetryAction, Help: "retry daemon", Mode: ModeError, ShowInFooter: true},
	{Chord: "?", Key: "?", Action: ToggleHelp, Help: "help", ShowInFooter: true},
	{Chord: "shift+?", Key: "?", Action: ToggleHelp, Help: "help"},
	{Chord: "escape", Key: "esc", Action: CloseHelp, Help: "close help", Mode: ModeHelp, ShowInFooter: true},
}
