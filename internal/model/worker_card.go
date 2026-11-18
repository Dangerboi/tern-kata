package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The Worker card is the Popup card `W` opens on the Jobs Page. It tells what this machine's worker is: whether worker.toml exists,
// loads and lists the Project, whether a worker runs and whether the plugin supervises it. Where something is missing it sets the
// worker up or serves the Project through the worker package's checked write, after a trust choice that must be yes, and
// afterwards asks how to restart a running worker so it reads the new file.

const (
	ModeWorkerCard    Mode = "worker-card"
	ModeWorkerRestart Mode = "worker-restart"
	ModeWorkerOff     Mode = "worker-supervision-off"
	ModeWorkerRead    Mode = "worker-card-read"
)

const (
	OpenWorkerCard    Action = "open-worker-card"
	WorkerSave        Action = "worker-save"
	WorkerCancel      Action = "worker-cancel"
	WorkerRestartIdle Action = "worker-restart-idle"
	WorkerRestartNow  Action = "worker-restart-now"
	WorkerLeave       Action = "worker-leave"
	WorkerEdit        Action = "worker-edit"
	WorkerSupervision Action = "worker-supervision"
	WorkerOffIdle     Action = "worker-off-idle"
	WorkerOffNow      Action = "worker-off-now"
	WorkerOffCancel   Action = "worker-off-cancel"
)

const WorkerDraftUID = "worker-draft"

const (
	WorkerFieldExecutor Field = "executor"
	WorkerFieldActor    Field = "actor"
	WorkerFieldCommand  Field = "command"
	WorkerFieldModel    Field = "model"
	WorkerFieldThinking Field = "thinking"
	WorkerFieldTrust    Field = "trust"
)

// workerTable declares the Worker card's fields in tab order, and its busy condition: it is saving, stopping the worker,
// switching supervision or asking a question. Which of the fields apply to the card as it stands is Model.WorkerCardShows;
// tab skips the others. Trust is a choice: 0 is No, 1 is Yes.
var workerTable = fieldTable[WorkerCardState]{
	form:   func(m *Model) *WorkerCardState { return &m.WorkerCard },
	busy:   func(f WorkerCardState) bool { return f.Saving || f.Stopping || f.Switching || f.Restart || f.AskOff },
	hidden: func(m Model, name Field) bool { return !m.WorkerCardShows(name) },
	fields: []formField[WorkerCardState]{
		textField(WorkerFieldExecutor, func(f *WorkerCardState) *string { return &f.Executor }),
		textField(WorkerFieldActor, func(f *WorkerCardState) *string { return &f.Actor }),
		textField(WorkerFieldCommand, func(f *WorkerCardState) *string { return &f.Command }),
		textField(WorkerFieldModel, func(f *WorkerCardState) *string { return &f.Model }),
		textField(WorkerFieldThinking, func(f *WorkerCardState) *string { return &f.Thinking }),
		yesNoField(WorkerFieldTrust, func(f *WorkerCardState) *bool { return &f.Trusted }),
	},
}

var WorkerFields = workerTable.specs()

var workerFieldTitles = map[Field]string{
	WorkerFieldExecutor: "Executor",
	WorkerFieldActor:    "Actor",
	WorkerFieldCommand:  "Command",
	WorkerFieldModel:    "Model",
	WorkerFieldThinking: "Thinking",
	WorkerFieldTrust:    "Trust",
}

func WorkerPrompt(name Field) string { return workerFieldTitles[name] }

var workerHints = map[Field]string{
	WorkerFieldActor:    "who the runs are attributed to",
	WorkerFieldCommand:  worker.DefaultCommand,
	WorkerFieldModel:    "agent default",
	WorkerFieldThinking: "agent default",
}

func WorkerHint(name Field) string { return workerHints[name] }

const WorkerTrustNote = worker.TrustNote

type WorkerSetup struct {
	// Project and Checkout are the kata Project the Pane's folder is bound to and the folder holding that Binding's .kata.toml;
	// both are empty for a folder bound to none.
	Project, Checkout string
	// Path is worker.toml. Exists says it is there; LoadErr is why the worker's own parser refuses it, empty when it loads.
	Path    string
	Exists  bool
	LoadErr string
	// Lists says a file that loads lists Project, and Checkouts are the folders it maps the Project's checkouts to.
	Lists     bool
	Checkouts []string
	Executor  string
	Text      string
	// Manual says worker.manual sits beside worker.toml: the plugin neither starts nor restarts the worker.
	Manual bool
}

type LoadWorkerSetup struct{ Workspace string }

func (LoadWorkerSetup) cmd() {}

// WorkerSetupLoaded answers LoadWorkerSetup. Err is a read that failed: a config folder that cannot be found, a .kata.toml that does
// not parse.
type WorkerSetupLoaded struct {
	Setup WorkerSetup
	Err   error
}

func (WorkerSetupLoaded) msg() {}

// SaveWorkerSetup asks for worker.toml to be written: Serve adds the folder's Project to the file that is there, otherwise a new file
// is made from the other fields. Trusted is the trust choice; nothing is written without it.
type SaveWorkerSetup struct {
	Workspace string
	Serve     bool
	Trusted   bool
	// Actor is empty when the card left the default; DefaultActor is that default.
	Executor, Actor, DefaultActor, Command, Model, Thinking string
}

func (SaveWorkerSetup) cmd() {}

// WorkerSetupSaved answers SaveWorkerSetup. Err is the refusal; Field is the card field it is about, "" when it names none.
type WorkerSetupSaved struct {
	Serve   bool
	Project string
	Key     string
	Field   Field
	Err     error
}

func (WorkerSetupSaved) msg() {}

// SaveWorkerEdit asks for Text to replace worker.toml: the worker package checks it first, and nothing is written when it would not load
// or when the file is no longer Base, the text the edit started from.
type SaveWorkerEdit struct{ Text, Base string }

func (SaveWorkerEdit) cmd() {}

// WorkerEdited answers SaveWorkerEdit with the text sent. Err is the refusal, naming the problem.
type WorkerEdited struct {
	Text string
	Err  error
}

func (WorkerEdited) msg() {}

// StopWorker sends the worker its stop request: Mode idle drains and exits once no run is in flight, now ends it at once.
type StopWorker struct{ Mode string }

func (StopWorker) cmd() {}

// WorkerStopped answers StopWorker: the mode in force, the runs in flight and whether the plugin starts a fresh worker afterwards.
// Gone says no worker answered.
type WorkerStopped struct {
	Mode     string
	InFlight int
	Restarts bool
	Gone     bool
	Err      error
}

func (WorkerStopped) msg() {}

// SetWorkerSupervision turns the plugin's supervision of the worker off (Manual: worker.manual is created beside worker.toml) or on
// (it is removed). Stop, when set, is a stop request sent after the marker is written: "idle" or "now".
type SetWorkerSupervision struct {
	Manual bool
	Stop   string
}

func (SetWorkerSupervision) cmd() {}

// WorkerSupervisionSet answers SetWorkerSupervision. Stop is the worker's answer to the stop request, nil when none was sent. Err is
// a marker that could not be written or removed; a refused stop is Stop.Err, with the marker already in place.
type WorkerSupervisionSet struct {
	Manual bool
	Stop   *WorkerStopped
	Err    error
}

func (WorkerSupervisionSet) msg() {}

// WorkerCardState is the Worker card. What it shows about the machine is Model.Worker; the card holds what was typed, and the step
// it is in after a save.
type WorkerCardState struct {
	formCard
	// Saving, Stopping and Switching say a write, a stop request or a supervision change has not answered; the card then takes no key.
	Stopping, Switching bool
	// AskOff says the card asks how to stop the running worker as supervision goes off; Supervision is what the last change did.
	AskOff      bool
	Supervision string
	// Executor, Actor, Command, Model and Thinking are the setup fields, prefilled as `worker init` would; Trusted is the trust choice.
	Executor, Actor, Command, Model, Thinking string
	Trusted                                   bool
	// Saved is what the card says after the file was written and Next what happens to the worker now. Restart says a worker was running then, so the card asks how to
	// restart it.
	Saved, Next string
	Restart     bool
	// EditText is the text of an edit that was refused; the next edit starts from it, not from the file.
	EditText string
	// EditBase is the file's text when the editor was opened; the save is refused when the file has since changed.
	EditBase string
}

func (f WorkerCardState) Value(name Field) string { return workerTable.value(f, name) }

type WorkerCardKind int

const (
	WorkerLoading WorkerCardKind = iota
	WorkerUnreadable
	WorkerUnbound
	WorkerSetUp
	// WorkerBroken: worker.toml exists and does not load; the card shows why and writes nothing over it.
	WorkerBroken
	WorkerServe
	WorkerServed
)

func (m Model) WorkerCardKind() WorkerCardKind {
	w := m.Worker
	switch s := w.Setup; {
	case w.SetupErr != nil:
		return WorkerUnreadable
	case !w.SetupRead:
		return WorkerLoading
	case s.Project == "":
		return WorkerUnbound
	case !s.Exists:
		return WorkerSetUp
	case s.LoadErr != "":
		return WorkerBroken
	case !s.Lists:
		return WorkerServe
	}
	return WorkerServed
}

func (m Model) WorkerCardSaves() bool {
	kind := m.WorkerCardKind()
	return (kind == WorkerSetUp || kind == WorkerServe) && m.WorkerCard.Saved == ""
}

// WorkerCardEdits says the card offers the edit: worker.toml is there, whether it loads or not, and no save or restart question is under way.
func (m Model) WorkerCardEdits() bool {
	f := m.WorkerCard
	return m.WorkerCardKind() >= WorkerBroken && f.Saved == "" && !f.Saving && !f.Stopping
}

func (m Model) WorkerCardShows(name Field) bool {
	if !m.WorkerCardSaves() {
		return false
	}
	return name == WorkerFieldTrust || m.WorkerCardKind() == WorkerSetUp
}

func (m Model) WorkerInFlight() []LocalRun {
	var runs []LocalRun
	for _, r := range m.Worker.Runs {
		if !r.Settled() && !r.Held {
			runs = append(runs, r)
		}
	}
	return runs
}

func (m Model) WorkerInFlightCount() int { return max(len(m.WorkerInFlight()), m.Worker.InFlight) }

// WorkerInFlightWords is the runs in flight in words: how many, or that none is.
func (m Model) WorkerInFlightWords() string {
	n := m.WorkerInFlightCount()
	if n == 0 {
		return "no run in flight"
	}
	return Count(n, "run", "runs") + " in flight"
}

// WorkerBusyWords is what the worker row says about its runs: the count in flight, or that it is idle.
func (m Model) WorkerBusyWords() string {
	if m.WorkerInFlightCount() == 0 {
		return "idle"
	}
	return m.WorkerInFlightWords()
}

// WorkerStopWords are what the two stops do to the running worker, as clauses that follow a button's name: when idle it
// finishes its runs in flight first, now it holds them to resume later.
func (m Model) WorkerStopWords() (idle, now string) {
	n := m.WorkerInFlightCount()
	if n == 0 {
		return "takes no new run and exits; no run is in flight", "exits at once; no run is in flight, so none is held"
	}
	return "takes no new run and exits after its " + m.WorkerInFlightWords(), "exits at once and holds the " + Noun(n, "run", "runs")
}

// WorkerStopLine is the one line under a stop question: what each of the two answers does.
func (m Model) WorkerStopLine(idleLabel, nowLabel string) string {
	idle, now := m.WorkerStopWords()
	return idleLabel + ": " + idle + ". " + nowLabel + ": " + now + "."
}

// tomlProblem is why a config text does not load, without the path of the file, the parser's "toml: " and its "(last key …)"
// tail, which only repeat what the card shows or mean nothing to a person.
func tomlProblem(problem, path string) string {
	problem = strings.TrimSpace(problem)
	for _, prefix := range []string{path + ": ", filepath.Base(path) + ": ", "toml: "} {
		if prefix != ": " {
			problem = strings.TrimPrefix(problem, prefix)
		}
	}
	problem = lastKeyTail.ReplaceAllString(problem, "")
	return strings.TrimSuffix(strings.TrimSpace(problem), ".")
}

var lastKeyTail = regexp.MustCompile(` ?\(last key "[^"]*"\)`)

// Problem is why worker.toml does not load, in the words the card shows; empty when it loads.
func (s WorkerSetup) Problem() string { return tomlProblem(s.LoadErr, s.Path) }

// workerFailure is the card's error for a write of worker.toml that was refused: the refusal without the file's path, which the
// card shows once.
func (m Model) workerFailure(err error) error {
	return errors.New(Cannot("save", "worker.toml", errors.New(tomlProblem(Reason(err), m.Worker.Setup.Path))))
}

// startSentence is what happens next when no worker runs: the plugin starts one, or nothing does under the supervision marker.
func (m Model) startSentence() string {
	if m.Worker.Setup.Manual {
		return manualFile + " is present, so the plugin does not start the worker. Run tern-kata worker to start one."
	}
	return "The plugin starts the worker within 10 seconds; tern-kata worker doctor checks the setup."
}

// openWorkerCard is `W` on the Jobs Page. The setup is read again and the status asked again; the Command field starts blank.
func openWorkerCard(m Model) (Model, []Cmd) {
	m.WorkerCard = WorkerCardState{
		Open: true, Field: WorkerFieldExecutor,
		Executor: worker.ExecutorName(m.Host), Actor: m.Actor,
	}
	m.Worker.SetupRead, m.Worker.SetupErr = false, nil
	m, ask := m.askWorker(0)
	return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}, ask}
}

func (m Model) firstWorkerField() Field {
	for _, f := range WorkerFields {
		if m.WorkerCardShows(f.Name) {
			return f.Name
		}
	}
	return WorkerFieldExecutor
}

func workerSetupLoaded(m Model, msg WorkerSetupLoaded) Model {
	w := &m.Worker
	w.Setup, w.SetupRead, w.SetupErr = msg.Setup, true, msg.Err
	if msg.Err != nil {
		w.Setup = WorkerSetup{}
	}
	if m.WorkerCard.Open && !m.WorkerCardShows(m.WorkerCard.Field) {
		m.WorkerCard.Field = m.firstWorkerField()
	}
	return m
}

type workerFieldError struct {
	field Field
	text  string
}

func (e workerFieldError) Error() string { return e.text }

func (m Model) workerSaveRefusal() error {
	f := m.WorkerCard
	if m.WorkerCardKind() == WorkerSetUp {
		switch executor := strings.TrimSpace(f.Executor); {
		case executor == "":
			return workerFieldError{WorkerFieldExecutor, Cannot("save", "worker.toml", errors.New("Executor must not be blank"))}
		case len(executor) > 256 || strings.ContainsAny(executor, " \t\r\n"):
			return workerFieldError{WorkerFieldExecutor, Cannot("save", "worker.toml", errors.New("Executor takes at most 256 bytes and no whitespace"))}
		}
	}
	if !f.Trusted {
		return workerFieldError{WorkerFieldTrust, Cannot("save", "worker.toml", errors.New("Trust must be Yes to list the Project"))}
	}
	return nil
}

func submitWorker(m Model) (Model, []Cmd) {
	f := &m.WorkerCard
	if !m.WorkerCardSaves() {
		f.Err = cardFailure("save", "worker.toml", errors.New("there is nothing to save"))
		return m, nil
	}
	if err := m.workerSaveRefusal(); err != nil {
		f.Err = err
		var named workerFieldError
		if errors.As(err, &named) {
			f.Field = named.field
		}
		return m, nil
	}
	f.Saving, f.Err = true, nil
	cmd := SaveWorkerSetup{Workspace: m.Workspace, Serve: m.WorkerCardKind() == WorkerServe, Trusted: f.Trusted}
	if !cmd.Serve {
		cmd.Executor, cmd.DefaultActor = strings.TrimSpace(f.Executor), m.Actor
		if actor := strings.TrimSpace(f.Actor); actor != m.Actor {
			cmd.Actor = actor
		}
		cmd.Command, cmd.Model, cmd.Thinking = strings.TrimSpace(f.Command), strings.TrimSpace(f.Model), strings.TrimSpace(f.Thinking)
	}
	return m, []Cmd{cmd}
}

func workerSetupSaved(m Model, msg WorkerSetupSaved) (Model, []Cmd) {
	f := &m.WorkerCard
	if !f.Open || !f.Saving {
		return m, nil
	}
	f.Saving = false
	if msg.Err != nil {
		f.Err = m.workerFailure(msg.Err)
		if msg.Field != "" && m.WorkerCardShows(msg.Field) {
			f.Field = msg.Field
		}
		return m, nil
	}
	f.Err, f.Trusted = nil, false
	return m.workerWritten("worker.toml written.")
}

// workerWritten is the card after worker.toml was written: what was saved, what happens to the worker, and the setup read again.
func (m Model) workerWritten(saved string) (Model, []Cmd) {
	f := &m.WorkerCard
	f.Saved = saved
	f.Restart = m.Worker.Running
	if f.Restart {
		f.Next = "The running worker reads worker.toml only at start. How should it restart?"
	} else {
		f.Next = m.startSentence()
	}
	m, ask := m.askWorker(0)
	return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}, ask}
}

// workerEditorClosed takes the text back from $EDITOR: an unchanged or blank result changes nothing, anything else is sent to be
// checked and written.
func workerEditorClosed(m Model, in Msg) (Model, []Cmd, bool) {
	msg, ok := in.(DraftEditorClosed)
	if !ok || msg.UID != WorkerDraftUID {
		return m, nil, false
	}
	f := &m.WorkerCard
	if !f.Open {
		return m, nil, true
	}
	if msg.Err != nil {
		f.Err = cardFailure("open", "the editor", msg.Err)
		return m, nil, true
	}
	text := normalizeLineBreaks(msg.Text)
	if strings.TrimSpace(text) == "" || text == normalizeLineBreaks(m.workerEditStart()) {
		return m, nil, true
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{SaveWorkerEdit{Text: text, Base: f.EditBase}}, true
}

func (m Model) workerEditStart() string {
	if m.WorkerCard.EditText != "" {
		return m.WorkerCard.EditText
	}
	return m.Worker.Setup.Text
}

func workerEdited(m Model, msg WorkerEdited) (Model, []Cmd) {
	f := &m.WorkerCard
	if !f.Open || !f.Saving {
		return m, nil
	}
	f.Saving = false
	if errors.Is(msg.Err, worker.ErrConfigChanged) {
		f.EditText = msg.Text
		f.Err = errors.New("Cannot save worker.toml: it changed since you opened it. Your text is kept.")
		return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}}
	}
	if msg.Err != nil {
		f.EditText = msg.Text
		f.Err = errors.New(m.workerFailure(msg.Err).Error() + " Your text is kept.")
		return m, nil
	}
	f.EditText, f.Err = "", nil
	return m.workerWritten("worker.toml written.")
}

func workerStopped(m Model, msg WorkerStopped) (Model, []Cmd) {
	f := &m.WorkerCard
	if !f.Open || !f.Stopping {
		return m, nil
	}
	f.Stopping = false
	if msg.Err != nil {
		f.Err = cardFailure("stop", "the worker", msg.Err)
		return m, nil
	}
	m.WorkerCard = WorkerCardState{}
	tail := "the plugin then starts a fresh one with the new worker.toml."
	switch {
	case msg.Gone:
		m = m.say("No worker is running now. " + m.startSentence())
	default:
		if !msg.Restarts {
			tail = "nothing starts it again: run tern-kata worker."
		}
		switch {
		case msg.Mode == worker.StopNow:
			m = m.say("The worker is ending now and holds its runs in flight to resume later; " + tail)
		case msg.InFlight > 0:
			m = m.say(fmt.Sprintf("The worker finishes its %s in flight, then exits; %s", Count(msg.InFlight, "run", "runs"), tail))
		default:
			m = m.say("The worker is exiting; " + tail)
		}
	}
	m, ask := m.askWorker(workerPollEvery)
	return m, []Cmd{ask}
}

// workerCardKey is one key for the Worker card on top. Left, right and space move through the trust choice, and in a text field
// space is typed.
func workerCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.WorkerCard
	if f.Saving || f.Stopping || f.Switching {
		return m, nil
	}
	if f.Restart {
		return workerRestartKey(m, msg)
	}
	if f.AskOff {
		return workerOffKey(m, msg)
	}
	binding, bound := LookupBindingForMode(msg.Chord, m.WorkerCardMode())
	if bound && binding.Action == WorkerEdit {
		if m.WorkerCardEdits() {
			f.EditBase = m.Worker.Setup.Text
			return m, []Cmd{OpenDraftEditor{UID: WorkerDraftUID, Text: m.workerEditStart(), Suffix: ".toml"}}
		}
		bound = false // no file to edit: the key is typed
	}
	field := workerTable.textAt(f, f.Field)
	if bound && binding.Action == WorkerSupervision && field != nil && m.WorkerCardShows(f.Field) {
		bound = false // in a text field, s is typed
	}
	switch {
	case bound && binding.Action == WorkerSupervision:
		return switchSupervision(m)
	case bound && binding.Action == WorkerCancel:
		m.WorkerCard = WorkerCardState{}
	case bound && binding.Action == WorkerSave:
		return submitWorker(m)
	case field == nil && bound && binding.Action == InputSpace:
		workerTable.stepFocused(&m, false)
	case field != nil && m.WorkerCardShows(f.Field):
		var edited bool
		if bound && binding.Action == InputSpace {
			bound = false // a space in a text field is typed
		}
		if *field, edited = editText(*field, &f.Caret, f.Field, bound, msg); edited {
			f.Err = nil
		}
	}
	return m, nil
}

// workerRestartKey is one key while the card asks how to restart the worker: when idle (the default), now, or not at all.
func workerRestartKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeWorkerRestart)
	if !bound {
		return m, nil
	}
	switch binding.Action {
	case WorkerRestartIdle, WorkerRestartNow:
		mode := worker.StopIdle
		if binding.Action == WorkerRestartNow {
			mode = worker.StopNow
		}
		m.WorkerCard.Stopping, m.WorkerCard.Err = true, nil
		return m, []Cmd{StopWorker{Mode: mode}}
	case WorkerLeave:
		m.WorkerCard = WorkerCardState{}
		m = m.say("Left the worker running; it reads the new worker.toml when it next starts.")
	}
	return m, nil
}

func (m Model) workerRunning() bool { return m.Worker.Read && m.Worker.Err == nil && m.Worker.Running }

// switchSupervision is s on the card. On removes the marker at once. Off creates it at once when no worker runs, and otherwise asks
// first how to stop the running one.
func switchSupervision(m Model) (Model, []Cmd) {
	f := &m.WorkerCard
	if m.WorkerCardKind() < WorkerSetUp {
		return m, nil
	}
	f.Err, f.Supervision = nil, ""
	if !m.Worker.Setup.Manual && m.workerRunning() {
		f.AskOff = true
		return m, nil
	}
	f.Switching = true
	return m, []Cmd{SetWorkerSupervision{Manual: !m.Worker.Setup.Manual}}
}

// WorkerOffQuestion is what the card asks when supervision goes off with a worker running.
const WorkerOffQuestion = "Turn supervision off: the plugin starts no new worker. How should the running one stop?"

// workerOffKey is one key while the card asks how to stop the worker: when idle (the default), now, or not at all.
func workerOffKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeWorkerOff)
	if !bound {
		return m, nil
	}
	f := &m.WorkerCard
	switch binding.Action {
	case WorkerOffIdle, WorkerOffNow:
		mode := worker.StopIdle
		if binding.Action == WorkerOffNow {
			mode = worker.StopNow
		}
		f.AskOff, f.Switching = false, true
		if m.WorkerInFlightCount() > 0 {
			f.Supervision = m.WorkerInFlightWords() // named in the answer: the status read after the stop no longer lists them
		}
		return m, []Cmd{SetWorkerSupervision{Manual: true, Stop: mode}}
	case WorkerOffCancel:
		f.AskOff = false
	}
	return m, nil
}

func workerSupervisionSet(m Model, msg WorkerSupervisionSet) (Model, []Cmd) {
	f := &m.WorkerCard
	flight := f.Supervision
	if !f.Open || !f.Switching {
		return m, nil
	}
	f.Switching, f.Supervision = false, ""
	if msg.Err != nil {
		f.Err = cardFailure("change", "supervision", msg.Err)
		return m, nil
	}
	m.Worker.Setup.Manual = msg.Manual
	switch stop := msg.Stop; {
	case !msg.Manual && m.workerRunning():
		f.Supervision = "Supervision is on: the running worker stays, and the plugin starts it again whenever it exits."
	case !msg.Manual:
		f.Supervision = "Supervision is on: the plugin starts the worker within 10 seconds."
	case stop == nil:
		f.Supervision = "Supervision is off: the plugin starts no worker. Run tern-kata worker to start one by hand."
	case stop.Err != nil:
		f.Supervision = "Supervision is off: the plugin starts no new worker."
		f.Err = cardFailure("stop", "the worker", stop.Err)
	case stop.Gone:
		f.Supervision = "Supervision is off: no worker is running now, and the plugin starts none."
	case stop.Mode == worker.StopNow && flight != "":
		f.Supervision = "Supervision is off: the worker is ending now (" + flight + ", held to resume later), and nothing starts it again."
	case stop.Mode == worker.StopNow:
		f.Supervision = "Supervision is off: the worker is ending now, and nothing starts it again."
	case stop.InFlight > 0:
		f.Supervision = fmt.Sprintf("Supervision is off: the worker finishes its %s in flight, then exits, and nothing starts it again.", Count(stop.InFlight, "run", "runs"))
	default:
		f.Supervision = "Supervision is off: the worker is exiting, and nothing starts it again."
	}
	m, ask := m.askWorker(workerPollEvery)
	return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}, ask}
}

// WorkerCardMode is the keymap the card answers to: the field keys while it draws fields, only e, s and esc when it does not.
func (m Model) WorkerCardMode() Mode {
	if m.WorkerCardSaves() {
		return ModeWorkerCard
	}
	return ModeWorkerRead
}

const manualFile = worker.ManualFileName

// ManualPresentWords is the Worker card's words for a present supervision marker; the view shows them as the model words them.
func ManualPresentWords() string {
	return manualFile + " is present, so the plugin neither starts nor restarts the worker"
}
