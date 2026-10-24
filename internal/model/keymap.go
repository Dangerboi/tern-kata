package model

import "strings"

type Action string

const (
	MoveDown                Action = "down"
	MoveUp                  Action = "up"
	MoveFirst               Action = "first"
	MoveLast                Action = "last"
	PageUp                  Action = "page-up"
	PageDown                Action = "page-down"
	ScrollUp                Action = "scroll-up"
	ScrollDown              Action = "scroll-down"
	QuitAction              Action = "quit"
	DrillIn                 Action = "drill-in"
	DrillOut                Action = "drill-out"
	MenuBack                Action = "menu-back"
	ToggleDetail            Action = "toggle-detail"
	FocusDetail             Action = "focus-detail"
	NextActivityTab         Action = "next-activity-tab"
	PreviousActivityTab     Action = "previous-activity-tab"
	ToggleHelp              Action = "help"
	CloseHelp               Action = "close-help"
	ToggleExpansion         Action = "toggle-expansion"
	OpenRow                 Action = "open-row"
	ExpandIssue             Action = "expand"
	CollapseIssue           Action = "collapse"
	ToggleAllExpansion      Action = "toggle-all-expansion"
	ToggleIssueView         Action = "toggle-issue-view"
	ToggleGraphOrder        Action = "toggle-graph-order"
	RetryAction             Action = "retry"
	SelectIssueAction       Action = "select-issue"
	SearchIssues            Action = "search"
	SearchEnter             Action = "search-enter"
	SearchBack              Action = "search-back"
	SearchQuery             Action = "search-query"
	DeleteInput             Action = "delete-input"
	ClearInput              Action = "clear-input"
	CycleStatus             Action = "cycle-status"
	OpenFilterForm          Action = "filter-form"
	ApplyFilters            Action = "apply-filters"
	CancelFilters           Action = "cancel-filters"
	SaveFilterDefault       Action = "save-filter-default"
	NextField               Action = "next-field"
	PreviousField           Action = "previous-field"
	NextChoice              Action = "next-choice"
	PreviousChoice          Action = "previous-choice"
	ResetFilterForm         Action = "reset-filter-form"
	ClearFilters            Action = "clear-filters"
	InputSpace              Action = "input-space"
	ToggleBoard             Action = "toggle-board"
	ShowIssuesPage          Action = "show-issues-page"
	ShowBoardPage           Action = "show-board-page"
	ShowJobsPage            Action = "show-jobs-page"
	ToggleArchivedJobs      Action = "toggle-archived-jobs"
	ReloadJobs              Action = "reload-jobs"
	OpenJobParent           Action = "open-job-parent"
	FocusJobInspector       Action = "focus-job-inspector"
	JobInspectorBack        Action = "job-inspector-back"
	OpenLanes               Action = "open-lanes"
	PreviousLane            Action = "previous-lane"
	NextLane                Action = "next-lane"
	MoveCardLeft            Action = "move-card-left"
	MoveCardRight           Action = "move-card-right"
	OpenLane                Action = "open-lane"
	SelectParent            Action = "select-parent"
	ShowRunsPage            Action = "show-runs-page"
	ReloadRuns              Action = "reload-runs"
	OlderRuns               Action = "older-runs"
	FilterRunsByJob         Action = "filter-runs-by-job"
	OpenRunIssue            Action = "open-run-issue"
	RunsOfJob               Action = "runs-of-job"
	RunJobNowAction         Action = "run-job-now"
	OpenRunSessionAction    Action = "open-run-session"
	ResumeRunAction         Action = "resume-run"
	AbandonRunAction        Action = "abandon-run"
	RunInspectorBack        Action = "run-inspector-back"
	ShowWorkflowsPage       Action = "show-workflows-page"
	OpenWorkflow            Action = "open-workflow"
	NewWorkflow             Action = "new-workflow"
	EditWorkflow            Action = "edit-workflow"
	ArchiveWorkflowAction   Action = "archive-workflow"
	RestoreWorkflowAction   Action = "restore-workflow"
	ToggleArchivedWorkflows Action = "toggle-archived-workflows"
	ShowAttentionPage       Action = "show-attention-page"
	OpenAttentionRequest    Action = "open-attention-request"
	ClearAttentionRequest   Action = "clear-attention-request"
	ReloadAttention         Action = "reload-attention"
	ShowCredentialsPage     Action = "show-credentials-page"
	RefreshCredentials      Action = "refresh-credentials"
	LetteredBack            Action = "lettered-back"
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
	ModeRuns          Mode = "runs"
	ModeRunInspector  Mode = "run-inspector"
	ModeWorkflows     Mode = "workflows"
	ModeAttention     Mode = "attention"
	ModeCredentials   Mode = "credentials"
)

// cardBindings are the keys of the Popup cards. Each card's handler reads them by mode; the
// main-surface keys are registered in mainActions.
var cardBindings = []Binding{
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
	{Chord: "ctrl+d", Key: "ctrl+d", Action: SaveFilterDefault, Help: "save as default", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "meta+d", Key: "meta+d", Action: SaveFilterDefault, Help: "save as default" + headlessNote, Mode: ModeFilterForm},
	{Chord: "escape", Key: "esc", Action: CancelFilters, Help: "cancel", Mode: ModeFilterForm, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "next choice / space", Mode: ModeFilterForm},
	{Chord: "ctrl+r", Key: "ctrl+r", Action: ResetFilterForm, Help: "reset form", Mode: ModeFilterForm},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: CloseSave, Help: "close", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "ctrl+d", Key: "ctrl+d", Action: CloseQuick, Help: "quick done", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: CloseCancel, Help: "cancel", Mode: ModeCloseForm, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: DatesSubmit, Help: "save dates", Mode: ModeDatesForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: DatesSubmit, Help: "save dates", Mode: ModeDatesForm},
	{Chord: "escape", Key: "esc", Action: DatesCancel, Help: "cancel", Mode: ModeDatesForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: RecurrenceSave, Help: "save recurrence", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: RecurrenceSave, Help: "save recurrence" + headlessNote, Mode: ModeRecurrenceForm},
	{Chord: "ctrl+x", Key: "ctrl+x", Action: RecurrenceStop, Help: "stop recurring", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "meta+x", Key: "meta+x", Action: RecurrenceStop, Help: "stop recurring" + headlessNote, Mode: ModeRecurrenceForm},
	{Chord: "y", Key: "y", Action: RecurrenceStopConfirm, Help: "stop recurring", Mode: ModeRecurrenceStop, ShowInFooter: true},
	{Chord: "n", Key: "n", Action: RecurrenceStopKeep, Help: "keep editing", Mode: ModeRecurrenceStop, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: RecurrenceStopKeep, Help: "keep editing", Mode: ModeRecurrenceStop},
	{Chord: "escape", Key: "esc", Action: RecurrenceCancel, Help: "cancel", Mode: ModeRecurrenceForm, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "next choice / toggle / space", Mode: ModeRecurrenceForm},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: JobSave, Help: "save job", Mode: ModeJobForm, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: JobSave, Help: "save job" + headlessNote, Mode: ModeJobForm},
	{Chord: "escape", Key: "esc", Action: JobCancel, Help: "cancel", Mode: ModeJobForm, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "next choice / space", Mode: ModeJobForm},
	{Chord: "enter", Key: "enter", Action: JobNewline, Help: "new line (in the prompt)", Mode: ModeJobForm},
	{Chord: "o", Key: "o", Action: JobOverwrite, Help: "save over the newer job", Mode: ModeJobConflict, ShowInFooter: true},
	{Chord: "u", Key: "u", Action: JobTakeNewer, Help: "use the newer job", Mode: ModeJobConflict, ShowInFooter: true},
	{Chord: "d", Key: "d", Action: JobDiscard, Help: "discard draft", Mode: ModeJobConflict, ShowInFooter: true},
	{Chord: "c", Key: "c", Action: BackfillCreate, Help: "create the missed runs", Mode: ModeBackfill, ShowInFooter: true},
	{Chord: "l", Key: "l", Action: BackfillLatest, Help: "switch to latest", Mode: ModeBackfill, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: BackfillCancel, Help: "cancel", Mode: ModeBackfill, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: FieldSubmit, Help: "save", Mode: ModeFieldForm},
	{Chord: "escape", Key: "esc", Action: FieldCancel, Help: "cancel", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "tab", Key: "tab", Action: FieldComplete, Help: "complete name", Mode: ModeFieldForm, ShowInFooter: true},
	{Chord: "down", Key: "down", Action: FieldNext, Help: "next completion", Mode: ModeFieldForm},
	{Chord: "up", Key: "up", Action: FieldPrevious, Help: "previous completion", Mode: ModeFieldForm},
	{Chord: "down", Key: "↓", Action: MoveDown, Help: "next Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "up", Key: "↑", Action: MoveUp, Help: "previous Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "j", Key: "j", Action: MoveDown, Help: "next Collection (empty filter)", Mode: ModePicker},
	{Chord: "k", Key: "k", Action: MoveUp, Help: "previous Collection (empty filter)", Mode: ModePicker},
	{Chord: "home", Key: "home", Action: MoveFirst, Help: "first Collection", Mode: ModePicker},
	{Chord: "end", Key: "end", Action: MoveLast, Help: "last Collection", Mode: ModePicker},
	{Chord: "enter", Key: "enter", Action: PickCollection, Help: "open Collection", Mode: ModePicker, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: ClosePicker, Help: "close", Mode: ModePicker, ShowInFooter: true},
	// New Project form: one name field, so every other key is typed.
	{Chord: "enter", Key: "enter", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: ProjectSubmit, Help: "create Project", Mode: ModeProjectForm},
	{Chord: "escape", Key: "esc", Action: ProjectCancel, Help: "cancel", Mode: ModeProjectForm, ShowInFooter: true},
	// Lanes card: one field holding the Lane list, so every other key is typed.
	{Chord: "enter", Key: "enter", Action: LanesSubmit, Help: "save Lanes", Mode: ModeLanesForm, ShowInFooter: true},
	{Chord: "ctrl+s", Key: "ctrl+s", Action: LanesSubmit, Help: "save Lanes", Mode: ModeLanesForm},
	{Chord: "escape", Key: "esc", Action: LanesCancel, Help: "cancel", Mode: ModeLanesForm, ShowInFooter: true},
	// Worker card. Its edit (e) key is added here by the ticket that builds it. s (supervision) is typed while the card draws fields.
	{Chord: "s", Key: "s", Action: WorkerSupervision, Help: "supervision off / on", Mode: ModeWorkerCard, ShowInFooter: true},
	{Chord: "e", Key: "e", Action: WorkerEdit, Help: "edit worker.toml", Mode: ModeWorkerCard, ShowInFooter: true},
	{Chord: "ctrl+o", Key: "ctrl+o", Action: WorkerSave, Help: "save worker config", Mode: ModeWorkerCard, ShowInFooter: true},
	{Chord: "meta+o", Key: "meta+o", Action: WorkerSave, Help: "save worker config" + headlessNote, Mode: ModeWorkerCard},
	{Chord: "escape", Key: "esc", Action: WorkerCancel, Help: "close", Mode: ModeWorkerCard, ShowInFooter: true},
	{Chord: "space", Key: "space", Action: InputSpace, Help: "toggle choice / space", Mode: ModeWorkerCard},
	{Chord: "s", Key: "s", Action: WorkerSupervision, Help: "supervision off / on", Mode: ModeWorkerRead, ShowInFooter: true},
	{Chord: "e", Key: "e", Action: WorkerEdit, Help: "edit worker.toml", Mode: ModeWorkerRead, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: WorkerCancel, Help: "close", Mode: ModeWorkerRead, ShowInFooter: true},
	{Chord: "i", Key: "i", Action: WorkerRestartIdle, Help: "restart when idle", Mode: ModeWorkerRestart, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: WorkerRestartIdle, Help: "restart when idle", Mode: ModeWorkerRestart},
	{Chord: "n", Key: "n", Action: WorkerRestartNow, Help: "restart now", Mode: ModeWorkerRestart, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: WorkerLeave, Help: "leave it running", Mode: ModeWorkerRestart, ShowInFooter: true},
	{Chord: "i", Key: "i", Action: WorkerOffIdle, Help: "stop when idle", Mode: ModeWorkerOff, ShowInFooter: true},
	{Chord: "enter", Key: "enter", Action: WorkerOffIdle, Help: "stop when idle", Mode: ModeWorkerOff},
	{Chord: "n", Key: "n", Action: WorkerOffNow, Help: "stop now", Mode: ModeWorkerOff, ShowInFooter: true},
	{Chord: "escape", Key: "esc", Action: WorkerOffCancel, Help: "keep supervision", Mode: ModeWorkerOff, ShowInFooter: true},
}
