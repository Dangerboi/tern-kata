package model

type Action string

const (
	MoveDown              Action = "down"
	MoveUp                Action = "up"
	MoveFirst             Action = "first"
	MoveLast              Action = "last"
	QuitAction            Action = "quit"
	DrillIn               Action = "drill-in"
	DrillOut              Action = "drill-out"
	ToggleDetail          Action = "toggle-detail"
	NextDetailSection     Action = "next-detail-section"
	PreviousDetailSection Action = "previous-detail-section"
	ToggleHelp            Action = "help"
	CloseHelp             Action = "close-help"
	ToggleExpansion       Action = "toggle-expansion"
	ExpandIssue           Action = "expand"
	CollapseIssue         Action = "collapse"
	ToggleAllExpansion    Action = "toggle-all-expansion"
	ToggleIssueView       Action = "toggle-issue-view"
	RetryAction           Action = "retry"
	SelectIssueAction     Action = "select-issue"
	SearchIssues          Action = "search"
	SearchEnter           Action = "search-enter"
	SearchBack            Action = "search-back"
	SearchQuery           Action = "search-query"
	DeleteInput           Action = "delete-input"
	ClearInput            Action = "clear-input"
	CycleStatus           Action = "cycle-status"
	OpenFilterForm        Action = "filter-form"
	ApplyFilters          Action = "apply-filters"
	CancelFilters         Action = "cancel-filters"
	NextFilterField       Action = "next-filter-field"
	PreviousFilterField   Action = "previous-filter-field"
	NextFilterChoice      Action = "next-filter-choice"
	PreviousFilterChoice  Action = "previous-filter-choice"
	ResetFilterForm       Action = "reset-filter-form"
	ClearFilters          Action = "clear-filters"
	InputSpace            Action = "input-space"
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
	ModeList          Mode = "list"
	ModeHelp          Mode = "help"
	ModeError         Mode = "error"
	ModeDetail        Mode = "detail"
	ModeCollections   Mode = "collections"
	ModeSearch        Mode = "search"
	ModeSearchResults Mode = "search-results"
	ModeFilterForm    Mode = "filter-form"
)

// Chord dispatches Action; Key and Help label it. Empty Mode is global;
// otherwise the binding is active only in that mode. ShowInFooter selects hints.
var Keymap = []Binding{
	{Chord: "n", Key: "n", Action: NewIssue, Help: "new Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "shift+n", Key: "N", Action: NewChild, Help: "new child", Mode: ModeList, ShowInFooter: true},
	{Chord: "n", Key: "n", Action: NewIssue, Help: "new Issue", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "shift+n", Key: "N", Action: NewChild, Help: "new child", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: CreateNext, Help: "next field", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: CreatePrevious, Help: "previous field", Mode: ModeCreate},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: SaveDraftAction, Help: "create Issue", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: SaveDraftAction, Help: "create Issue (Tern headless)", Mode: ModeCreate},
	{Chord: "escape", Key: "esc", Action: CancelDraft, Help: "discard?", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "ctrl+space", Key: "ctrl+space", Action: CreateComplete, Help: "complete label", Mode: ModeCreate, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "g", Key: "g", Action: MoveFirst, Help: "first Issue", Mode: ModeList},
	{Chord: "shift+g", Key: "G", Action: MoveLast, Help: "last Issue", Mode: ModeList},
	{Chord: "e", Key: "e", Action: EditBody, Help: "edit body", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "e", Key: "e", Action: EditBody, Help: "edit body", Mode: ModeList, ShowInFooter: true},
	{Chord: "c", Key: "c", Action: AddComment, Help: "add comment", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: SaveDraftAction, Help: "save Draft", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: SaveDraftAction, Help: "save Draft (Tern headless)", Mode: ModeEditor},
	{Chord: "ctrl+e", Key: "ctrl+e", Action: ExternalEditorAction, Help: "$EDITOR", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+e", Key: "meta+e", Action: ExternalEditorAction, Help: "$EDITOR (Tern headless)", Mode: ModeEditor},
	{Chord: "escape", Key: "esc", Action: CancelDraft, Help: "discard?", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "ctrl+z", Key: "ctrl+z", Action: EditorUndo, Help: "undo", Mode: ModeEditor, ShowInFooter: true},
	{Chord: "meta+z", Key: "meta+z", Action: EditorUndo, Help: "undo (Tern headless)", Mode: ModeEditor},
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
	{Chord: "meta+a", Key: "meta+a", Action: EditorSelectAll, Help: "select all (Tern headless)", Mode: ModeEditor},
	{Chord: "c", Key: "c", Action: ClearFilters, Help: "clear filters", Mode: ModeList, ShowInFooter: true},
	{Chord: "f", Key: "f", Action: OpenFilterForm, Help: "filters", Mode: ModeList, ShowInFooter: true},
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
	{Chord: "x", Key: "x", Action: OpenClose, Help: "close Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "x", Key: "x", Action: OpenClose, Help: "close Issue", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "r", Key: "r", Action: ReopenAction, Help: "reopen Issue", Mode: ModeList, ShowInFooter: true},
	{Chord: "r", Key: "r", Action: ReopenAction, Help: "reopen Issue", Mode: ModeDetail, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: CloseNext, Help: "next field", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "shift+tab", Key: "shift+tab", Action: ClosePrevious, Help: "previous field", Mode: ModeCloseForm},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: CloseSave, Help: "close", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "ctrl+d", Key: "ctrl+d", Action: CloseQuick, Help: "quick done", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: CloseCancel, Help: "cancel", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "backspace", Key: "backspace", Action: CloseErase, Help: "erase", Mode: ModeCloseForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: CloseClear, Help: "clear field", Mode: ModeCloseForm},
	{Chord: "q", Key: "q", Action: "close-text", Help: "", Mode: ModeCloseForm},
	{Chord: "+", Key: "+", Action: AddLabelAction, Help: "add label", Mode: ModeList},
	{Chord: "-", Key: "-", Action: RemoveLabelAction, Help: "remove label", Mode: ModeList},
	{Chord: "a", Key: "a", Action: AssignOwnerAction, Help: "assign owner", Mode: ModeList},
	{Chord: "shift+a", Key: "A", Action: ClearOwnerAction, Help: "clear owner", Mode: ModeList},
	{Chord: "!", Key: "!", Action: PriorityAction, Help: "priority", Mode: ModeList},
	{Chord: "+", Key: "+", Action: AddLabelAction, Help: "add label", Mode: ModeDetail},
	{Chord: "-", Key: "-", Action: RemoveLabelAction, Help: "remove label", Mode: ModeDetail},
	{Chord: "a", Key: "a", Action: AssignOwnerAction, Help: "assign owner", Mode: ModeDetail},
	{Chord: "shift+a", Key: "A", Action: ClearOwnerAction, Help: "clear owner", Mode: ModeDetail},
	{Chord: "!", Key: "!", Action: PriorityAction, Help: "priority", Mode: ModeDetail},
	{Chord: "enter", Key: "enter", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "escape", Key: "esc", Action: FieldCancel, Help: "cancel", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: FieldComplete, Help: "complete label", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "down", Key: "down", Action: FieldNext, Help: "next completion", Mode: ModeFieldForm},
	{Chord: "up", Key: "up", Action: FieldPrevious, Help: "previous completion", Mode: ModeFieldForm},
	{Chord: "backspace", Key: "backspace", Action: FieldErase, Help: "erase", Mode: ModeFieldForm},
	{Chord: "ctrl+u", Key: "ctrl+u", Action: FieldReset, Help: "clear input", Mode: ModeFieldForm},
}
