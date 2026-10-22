package model

import "strings"

type Action string

const (
	MoveDown             Action = "down"
	MoveUp               Action = "up"
	MoveFirst            Action = "first"
	MoveLast             Action = "last"
	PageUp               Action = "page-up"
	PageDown             Action = "page-down"
	ScrollUp             Action = "scroll-up"
	ScrollDown           Action = "scroll-down"
	QuitAction           Action = "quit"
	DrillIn              Action = "drill-in"
	DrillOut             Action = "drill-out"
	MenuBack             Action = "menu-back"
	ToggleDetail         Action = "toggle-detail"
	FocusDetail          Action = "focus-detail"
	NextActivityTab      Action = "next-activity-tab"
	PreviousActivityTab  Action = "previous-activity-tab"
	ToggleHelp           Action = "help"
	CloseHelp            Action = "close-help"
	ToggleExpansion      Action = "toggle-expansion"
	OpenRow              Action = "open-row"
	ExpandIssue          Action = "expand"
	CollapseIssue        Action = "collapse"
	ToggleAllExpansion   Action = "toggle-all-expansion"
	ToggleIssueView      Action = "toggle-issue-view"
	RetryAction          Action = "retry"
	SelectIssueAction    Action = "select-issue"
	SearchIssues         Action = "search"
	SearchEnter          Action = "search-enter"
	SearchBack           Action = "search-back"
	SearchQuery          Action = "search-query"
	DeleteInput          Action = "delete-input"
	ClearInput           Action = "clear-input"
	CycleStatus          Action = "cycle-status"
	OpenFilterForm       Action = "filter-form"
	ApplyFilters         Action = "apply-filters"
	CancelFilters        Action = "cancel-filters"
	NextFilterField      Action = "next-filter-field"
	PreviousFilterField  Action = "previous-filter-field"
	NextFilterChoice     Action = "next-filter-choice"
	PreviousFilterChoice Action = "previous-filter-choice"
	ResetFilterForm      Action = "reset-filter-form"
	ClearFilters         Action = "clear-filters"
	InputSpace           Action = "input-space"
	ToggleBoard          Action = "toggle-board"
	ShowIssuesPage       Action = "show-issues-page"
	ShowBoardPage        Action = "show-board-page"
	ShowJobsPage         Action = "show-jobs-page"
	ToggleArchivedJobs   Action = "toggle-archived-jobs"
	ReloadJobs           Action = "reload-jobs"
	OpenJobParent        Action = "open-job-parent"
	FocusJobInspector    Action = "focus-job-inspector"
	JobInspectorBack     Action = "job-inspector-back"
	OpenLanes            Action = "open-lanes"
	PreviousLane         Action = "previous-lane"
	NextLane             Action = "next-lane"
	MoveCardLeft         Action = "move-card-left"
	MoveCardRight        Action = "move-card-right"
	OpenLane             Action = "open-lane"
	SelectParent         Action = "select-parent"
)

// Binding is one chord in one mode, as the lookup, the footer and the help sheet read it. A binding of
// a main-surface action carries its entry; a Popup card's binding has none, the card handles its keys.
type Binding struct {
	Chord        string
	Key          string
	Action       Action
	Help         string
	Mode         Mode
	ShowInFooter bool
	entry        *actionEntry
}

// CardKeys are the non-headless bindings of action in mode, in keymap order: what a card button names and sends.
func CardKeys(mode Mode, action Action) []Binding {
	var keys []Binding
	for _, b := range Keymap {
		if b.Mode == mode && b.Action == action && !b.Headless() {
			keys = append(keys, b)
		}
	}
	return keys
}

// headlessNote ends the help of a chord only the headless Tern delivers; Headless tells such bindings apart.
const headlessNote = " (Tern headless)"

// Headless says the binding is a chord only the headless Tern delivers, which no footer or help sheet shows.
func (b Binding) Headless() bool { return strings.HasSuffix(b.Help, headlessNote) }

type Mode string

const (
	ModeList          Mode = "list"
	ModeHelp          Mode = "help"
	ModeError         Mode = "error"
	ModeDetail        Mode = "detail"
	ModeCollections   Mode = "collections"
	ModeSearch        Mode = "search"
	ModeSearchResults Mode = "search-results"
	ModeFilterForm    Mode = "filter-form"
	ModeBoard         Mode = "board"
	ModeLane          Mode = "lane"
	ModeJobs          Mode = "jobs"
	ModeJobInspector  Mode = "job-inspector"
)

// cardBindings are the keys of the Popup cards. Each card's handler reads them by mode; the
// main-surface keys are registered in mainActions.
var cardBindings = []Binding{
	{Chord: "tab", Key: "tab", Action: CreateNext, Help: "next field", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: CreatePrevious, Help: "previous field", Mode: ModeCreate},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: SaveDraftAction, Help: "create Issue", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: SaveDraftAction, Help: "create Issue" + headlessNote, Mode: ModeCreate},
	{Chord: "escape", Key: "esc", Action: CancelDraft, Help: "discard?", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "ctrl+space", Key: "ctrl+space", Action: CreateComplete, Help: "complete label", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: SaveDraftAction, Help: "save Draft", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: SaveDraftAction, Help: "save Draft" + headlessNote, Mode: ModeEditor},
	{Chord: "ctrl+e", Key: "ctrl+e", Action: ExternalEditorAction, Help: "$EDITOR", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+e", Key: "meta+e", Action: ExternalEditorAction, Help: "$EDITOR" + headlessNote, Mode: ModeEditor},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: CycleReplyKind, Help: "reply kind", Mode: ModeEditor},
	{Chord: "meta+r", Key: "meta+r", Action: CycleReplyKind, Help: "reply kind" + headlessNote, Mode: ModeEditor},
	{Chord: "ctrl+f", Key: "ctrl+f", Action: SendReplyAnyway, Help: "send reply anyway", Mode: ModeEditor},
	{Chord: "meta+f", Key: "meta+f", Action: SendReplyAnyway, Help: "send reply anyway" + headlessNote, Mode: ModeEditor},
	{Chord: "escape", Key: "esc", Action: CancelDraft, Help: "discard?", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "ctrl+z", Key: "ctrl+z", Action: EditorUndo, Help: "undo", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+z", Key: "meta+z", Action: EditorUndo, Help: "undo" + headlessNote, Mode: ModeEditor},
	{Chord: "y", Key: "y", Action: DiscardDraft, Help: "discard Draft", Mode: ModeDiscard, ShowInFooter: true},
	{Chord: "n", Key: "n", Action: KeepDraft, Help: "keep editing", Mode: ModeDiscard, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: KeepDraft, Help: "keep editing", Mode: ModeDiscard},
	{Chord: "o", Key: "o", Action: OverwriteDraft, Help: "overwrite new version", Mode: ModeConflict, ShowInFooter: true},
	{Chord: "d", Key: "d", Action: DiscardDraft, Help: "discard Draft", Mode: ModeConflict, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: EditorBackspace, Help: "delete before caret", Mode: ModeEditor},
	{Chord: "delete", Key: "delete", Action: EditorDelete, Help: "delete after caret", Mode: ModeEditor},
	{Chord: "enter", Key: "enter", Action: EditorNewline, Help: "newline", Mode: ModeEditor},
	{Chord: "left", Key: "left", Action: EditorLeft, Help: "caret left", Mode: ModeEditor},
	{Chord: "right", Key: "right", Action: EditorRight, Help: "caret right", Mode: ModeEditor},
	{Chord: "up", Key: "up", Action: EditorUp, Help: "caret up", Mode: ModeEditor},
	{Chord: "down", Key: "down", Action: EditorDown, Help: "caret down", Mode: ModeEditor},
	{Chord: "home", Key: "home", Action: EditorHome, Help: "line start", Mode: ModeEditor},
	{Chord: "end", Key: "end", Action: EditorEnd, Help: "line end", Mode: ModeEditor},
	{Chord: "shift+left", Key: "shift+left", Action: EditorLeft, Help: "select left", Mode: ModeEditor},
	{Chord: "shift+right", Key: "shift+right", Action: EditorRight, Help: "select right", Mode: ModeEditor},
	{Chord: "shift+up", Key: "shift+up", Action: EditorUp, Help: "select up", Mode: ModeEditor},
	{Chord: "shift+down", Key: "shift+down", Action: EditorDown, Help: "select down", Mode: ModeEditor},
	{Chord: "shift+home", Key: "shift+home", Action: EditorHome, Help: "select to line start", Mode: ModeEditor},
	{Chord: "shift+end", Key: "shift+end", Action: EditorEnd, Help: "select to line end", Mode: ModeEditor},
	{Chord: "ctrl+a", Key: "ctrl+a", Action: EditorSelectAll, Help: "select all", Mode: ModeEditor},
	{Chord: "meta+a", Key: "meta+a", Action: EditorSelectAll, Help: "select all" + headlessNote, Mode: ModeEditor},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: ApplyFilters, Help: "apply filters", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: ApplyFilters, Help: "apply filters", Mode: ModeFilterForm},
	{Chord: "escape", Key: "esc", Action: CancelFilters, Help: "cancel", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: NextFilterField, Help: "next field", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: NextFilterField, Help: "next field", Mode: ModeFilterForm},
	{Chord: "shift+tab", Key: "shift+tab", Action: PreviousFilterField, Help: "previous field", Mode: ModeFilterForm},
	{Chord: "right", Key: "right", Action: NextFilterChoice, Help: "next choice", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "next choice / space", Mode: ModeFilterForm},
	{Chord: "left", Key: "left", Action: PreviousFilterChoice, Help: "previous choice", Mode: ModeFilterForm},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeFilterForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeFilterForm},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: ResetFilterForm, Help: "reset form", Mode: ModeFilterForm},
	{Chord: "tab", Key: "tab", Action: CloseNext, Help: "next field", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: ClosePrevious, Help: "previous field", Mode: ModeCloseForm},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: CloseSave, Help: "close", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "ctrl+d", Key: "ctrl+d", Action: CloseQuick, Help: "quick done", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: CloseCancel, Help: "cancel", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "erase", Mode: ModeCloseForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeCloseForm},
	{Chord: "tab", Key: "tab", Action: DatesNext, Help: "next field", Mode: ModeDatesForm, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: DatesPrevious, Help: "previous field", Mode: ModeDatesForm},
	{Chord: "enter", Key: "enter", Action: DatesSubmit, Help: "save dates", Mode: ModeDatesForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: DatesSubmit, Help: "save dates", Mode: ModeDatesForm},
	{Chord: "escape", Key: "esc", Action: DatesCancel, Help: "cancel", Mode: ModeDatesForm, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeDatesForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeDatesForm},
	{Chord: "tab", Key: "tab", Action: RecurrenceNext, Help: "next field", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: RecurrencePrevious, Help: "previous field", Mode: ModeRecurrenceForm},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: RecurrenceSave, Help: "save recurrence", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: RecurrenceSave, Help: "save recurrence" + headlessNote, Mode: ModeRecurrenceForm},
	{Chord: "ctrl+x", Key: "ctrl+x", Action: RecurrenceStop, Help: "stop recurring", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "meta+x", Key: "meta+x", Action: RecurrenceStop, Help: "stop recurring" + headlessNote, Mode: ModeRecurrenceForm},
	{Chord: "y", Key: "y", Action: RecurrenceStopConfirm, Help: "stop recurring", Mode: ModeRecurrenceStop, ShowInFooter: true},
	{Chord: "n", Key: "n", Action: RecurrenceStopKeep, Help: "keep editing", Mode: ModeRecurrenceStop, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: RecurrenceStopKeep, Help: "keep editing", Mode: ModeRecurrenceStop},
	{Chord: "escape", Key: "esc", Action: RecurrenceCancel, Help: "cancel", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "right", Key: "right", Action: RecurrenceNextChoice, Help: "next choice", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "left", Key: "left", Action: RecurrencePreviousChoice, Help: "previous choice", Mode: ModeRecurrenceForm},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "next choice / toggle / space", Mode: ModeRecurrenceForm},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "erase", Mode: ModeRecurrenceForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeRecurrenceForm},
	{Chord: "enter", Key: "enter", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "escape", Key: "esc", Action: FieldCancel, Help: "cancel", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: FieldComplete, Help: "complete name", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "down", Key: "down", Action: FieldNext, Help: "next completion", Mode: ModeFieldForm},
	{Chord: "up", Key: "up", Action: FieldPrevious, Help: "previous completion", Mode: ModeFieldForm},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "erase", Mode: ModeFieldForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear input", Mode: ModeFieldForm},
	{Chord: "down", Key: "↓", Action: MoveDown, Help: "next Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "up", Key: "↑", Action: MoveUp, Help: "previous Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Collection (empty filter)", Mode: ModePicker},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Collection (empty filter)", Mode: ModePicker},
	{Chord: "home", Key: "home", Action: MoveFirst, Help: "first Collection", Mode: ModePicker},
	{Chord: "end", Key: "end", Action: MoveLast, Help: "last Collection", Mode: ModePicker},
	{Chord: "enter", Key: "enter", Action: PickCollection, Help: "open Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: ClosePicker, Help: "close", Mode: ModePicker, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModePicker},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear filter", Mode: ModePicker},
	// New Project form: one name field, so every other key is typed.
	{Chord: "enter", Key: "enter", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm},
	{Chord: "escape", Key: "esc", Action: ProjectCancel, Help: "cancel", Mode: ModeProjectForm, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeProjectForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear name", Mode: ModeProjectForm},
	// Lanes card: one field holding the Lane list, so every other key is typed.
	{Chord: "tab", Key: "tab", Action: LanesNext, Help: "next field", Mode: ModeLanesForm, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: LanesPrevious, Help: "previous field", Mode: ModeLanesForm},
	{Chord: "enter", Key: "enter", Action: LanesSubmit, Help: "save Lanes", Mode: ModeLanesForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: LanesSubmit, Help: "save Lanes", Mode: ModeLanesForm},
	{Chord: "escape", Key: "esc", Action: LanesCancel, Help: "cancel", Mode: ModeLanesForm, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: DeleteInput, Help: "delete", Mode: ModeLanesForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: ClearInput, Help: "clear field", Mode: ModeLanesForm},
}
