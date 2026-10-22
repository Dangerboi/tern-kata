package model

import (
	"errors"
	"fmt"
	"strconv"
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
)

const (
	OpenWorkerCard       Action = "open-worker-card"
	WorkerNext           Action = "worker-next-field"
	WorkerPrevious       Action = "worker-previous-field"
	WorkerNextChoice     Action = "worker-next-choice"
	WorkerPreviousChoice Action = "worker-previous-choice"
	WorkerSave           Action = "worker-save"
	WorkerCancel         Action = "worker-cancel"
	WorkerRestartIdle    Action = "worker-restart-idle"
	WorkerRestartNow     Action = "worker-restart-now"
	WorkerLeave          Action = "worker-leave"
	WorkerEdit           Action = "worker-edit"
)

// WorkerDraftUID is the key of the $EDITOR round trip of worker.toml.
const WorkerDraftUID = "worker-draft"

// The Worker card's fields.
const (
	WorkerFieldExecutor Field = "executor"
	WorkerFieldActor    Field = "actor"
	WorkerFieldCommand  Field = "command"
	WorkerFieldModel    Field = "model"
	WorkerFieldThinking Field = "thinking"
	WorkerFieldTrust    Field = "trust"
)

// WorkerFields is the Worker card's field list in tab order. Which of them apply to the card as it stands is Model.WorkerCardShows.
var WorkerFields = []FieldSpec{
	{WorkerFieldExecutor, true}, {WorkerFieldActor, true}, {WorkerFieldCommand, true}, {WorkerFieldModel, true}, {WorkerFieldThinking, true},
	{WorkerFieldTrust, false},
}

var workerFieldTitles = map[Field]string{
	WorkerFieldExecutor: "Executor",
	WorkerFieldActor:    "Actor",
	WorkerFieldCommand:  "Command",
	WorkerFieldModel:    "Model",
	WorkerFieldThinking: "Thinking",
	WorkerFieldTrust:    "Trust",
}

// WorkerPrompt is the title of the Worker card's field called name.
func WorkerPrompt(name Field) string { return workerFieldTitles[name] }

// WorkerTrustNote is the warning shown beside the trust choice and written into worker.toml.
const WorkerTrustNote = worker.TrustNote

// WorkerSetup is the one read of this machine's worker setup for the Pane's folder.
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
	// Executor is the file's executor label.
	Executor string
	// Text is the file's content as read; the edit opens it in $EDITOR.
	Text string
	// Manual says worker.manual sits beside worker.toml: the plugin neither starts nor restarts the worker.
	Manual bool
}

// LoadWorkerSetup asks for the setup read of the folder Workspace.
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

// WorkerSetupSaved answers SaveWorkerSetup. Err is the refusal, in words that name the field.
type WorkerSetupSaved struct {
	Serve   bool
	Project string
	Key     string
	Err     error
}

func (WorkerSetupSaved) msg() {}

// SaveWorkerEdit asks for Text to replace worker.toml: the worker package checks it first, and nothing is written when it would not load.
type SaveWorkerEdit struct{ Text string }

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

// WorkerCardState is the Worker card. What it shows about the machine is Model.Worker; the card holds what was typed, and the step
// it is in after a save.
type WorkerCardState struct {
	Open bool
	// Saving and Stopping say a write or a stop request has not answered; the card then takes no key.
	Saving, Stopping bool
	Field            Field
	Caret            Caret
	// Executor, Actor, Command, Model and Thinking are the setup fields, prefilled as `worker init` would; Trusted is the trust choice.
	Executor, Actor, Command, Model, Thinking string
	Trusted                                   bool
	Err                                       error
	// Saved is what the card says after the file was written and Next what happens to the worker now. Restart says a worker was running then, so the card asks how to
	// restart it.
	Saved, Next string
	Restart     bool
	// EditText is the text of an edit that was refused; the next edit starts from it, not from the file.
	EditText string
}

func (f *WorkerCardState) at(name Field) *string {
	switch name {
	case WorkerFieldExecutor:
		return &f.Executor
	case WorkerFieldActor:
		return &f.Actor
	case WorkerFieldCommand:
		return &f.Command
	case WorkerFieldModel:
		return &f.Model
	case WorkerFieldThinking:
		return &f.Thinking
	}
	return nil
}

// Value is the text the field called name holds.
func (f WorkerCardState) Value(name Field) string {
	if text := f.at(name); text != nil {
		return *text
	}
	return ""
}

// WorkerCardKind is what the card offers for the machine as it was read.
type WorkerCardKind int

const (
	// WorkerLoading: the setup read has not answered.
	WorkerLoading WorkerCardKind = iota
	// WorkerUnreadable: the read failed, Err says why.
	WorkerUnreadable
	// WorkerUnbound: the Pane's folder is bound to no Project.
	WorkerUnbound
	// WorkerSetUp: no worker.toml; the card offers to make one.
	WorkerSetUp
	// WorkerBroken: worker.toml exists and does not load; the card shows why and writes nothing over it.
	WorkerBroken
	// WorkerServe: worker.toml loads and does not list the Project; the card offers to add it.
	WorkerServe
	// WorkerServed: worker.toml loads and lists the Project.
	WorkerServed
)

// WorkerCardKind is read from the setup alone.
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

// WorkerCardSaves says the card offers a save: it sets the worker up or serves the Project, and has not saved yet.
func (m Model) WorkerCardSaves() bool {
	kind := m.WorkerCardKind()
	return (kind == WorkerSetUp || kind == WorkerServe) && m.WorkerCard.Saved == ""
}

// WorkerCardEdits says the card offers the edit: worker.toml is there, whether it loads or not, and no save or restart question is under way.
func (m Model) WorkerCardEdits() bool {
	f := m.WorkerCard
	return m.WorkerCardKind() >= WorkerBroken && f.Saved == "" && !f.Saving && !f.Stopping
}

// WorkerCardShows says the field applies to the card as it stands; the others are hidden and tab skips them.
func (m Model) WorkerCardShows(name Field) bool {
	if !m.WorkerCardSaves() {
		return false
	}
	return name == WorkerFieldTrust || m.WorkerCardKind() == WorkerSetUp
}

// WorkerInFlight are the runs the worker reports that a restart now would interrupt.
func (m Model) WorkerInFlight() []LocalRun {
	var runs []LocalRun
	for _, r := range m.Worker.Runs {
		if !r.Settled() && !r.Held {
			runs = append(runs, r)
		}
	}
	return runs
}

// workerInFlightWords is the runs in flight in words: how many, and which jobs.
func (m Model) workerInFlightWords() string {
	runs := m.WorkerInFlight()
	n := max(len(runs), m.Worker.InFlight)
	if n == 0 {
		return "no run in flight"
	}
	var names []string
	for _, r := range runs {
		name := r.Job
		if name == "" {
			name = r.UID
		}
		names = append(names, name)
	}
	text := strconv.Itoa(n) + " run"
	if n != 1 {
		text += "s"
	}
	text += " in flight"
	if len(names) > 0 {
		text += " (" + strings.Join(names, ", ") + ")"
	}
	return text
}

// WorkerRestartPrompt is what the card says when it asks how to restart the running worker.
func (m Model) WorkerRestartPrompt() (when, now string) {
	return "Restart when idle: the worker takes no new run, finishes " + m.workerInFlightWords() + " and exits.",
		"Restart now: the worker exits at once; " + m.workerInFlightWords() + " is interrupted and held `interrupted`, to resume later."
}

// startSentence is what happens next when no worker runs: the plugin starts one, or nothing does under worker.manual.
func (m Model) startSentence() string {
	if m.Worker.Setup.Manual {
		return "worker.manual is present, so the plugin does not start the worker: run `tern-kata worker`."
	}
	return "The plugin's host half starts the worker within 10 seconds (else run `tern-kata worker`); `tern-kata worker doctor` checks the setup."
}

// openWorkerCard is `W` on the Jobs Page. The setup is read again, the status asked again and the card prefilled as `worker init` would.
func openWorkerCard(m Model) (Model, []Cmd) {
	m.WorkerCard = WorkerCardState{
		Open: true, Field: WorkerFieldExecutor,
		Executor: worker.ExecutorName(m.Host), Actor: m.Actor, Command: "omp",
	}
	m.Worker.SetupRead, m.Worker.SetupErr = false, nil
	m, ask := m.askWorker(0)
	return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}, ask}
}

// firstWorkerField is the first field the card shows.
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

// workerFieldError is a refusal that names the field: the card puts focus there.
type workerFieldError struct {
	field Field
	text  string
}

func (e workerFieldError) Error() string { return e.text }

// refusedWorkerField is the field a refusal of the write is about, "" when it names none. The write's own words name the
// executor and the profile's command.
func refusedWorkerField(err error) Field {
	var named workerFieldError
	if errors.As(err, &named) {
		return named.field
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "executor"):
		return WorkerFieldExecutor
	case strings.Contains(text, "command"):
		return WorkerFieldCommand
	}
	return ""
}

func (m Model) workerSaveRefusal() error {
	f := m.WorkerCard
	if m.WorkerCardKind() == WorkerSetUp {
		switch executor := strings.TrimSpace(f.Executor); {
		case executor == "":
			return workerFieldError{WorkerFieldExecutor, "Executor: must not be blank."}
		case len(executor) > 256 || strings.ContainsAny(executor, " \t\r\n"):
			return workerFieldError{WorkerFieldExecutor, "Executor: at most 256 bytes with no whitespace."}
		}
	}
	if !f.Trusted {
		return workerFieldError{WorkerFieldTrust, "Trust: choose Yes to list the Project. " + WorkerTrustNote}
	}
	return nil
}

// submitWorker is the save key: it checks the card and asks for the write.
func submitWorker(m Model) (Model, []Cmd) {
	f := &m.WorkerCard
	if !m.WorkerCardSaves() {
		f.Err = errors.New("Nothing to save here.")
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
		if cmd.Command == "omp" {
			cmd.Command = ""
		}
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
		f.Err = msg.Err
		if field := refusedWorkerField(msg.Err); field != "" && m.WorkerCardShows(field) {
			f.Field = field
		}
		return m, nil
	}
	f.Err, f.Trusted = nil, false
	verb := "Set up the worker"
	if msg.Serve {
		verb = "Now serving " + msg.Project
	}
	return m.workerWritten(verb + ": worker.toml written (checkout " + msg.Key + ").")
}

// workerWritten is the card after worker.toml was written: what was saved, what happens to the worker, and the setup read again.
func (m Model) workerWritten(saved string) (Model, []Cmd) {
	f := &m.WorkerCard
	f.Saved = saved
	f.Restart = m.Worker.Running
	if f.Restart {
		f.Next = "The running worker reads worker.toml only at start."
	} else {
		f.Next = m.startSentence()
	}
	m, ask := m.askWorker(0)
	return m, []Cmd{LoadWorkerSetup{Workspace: m.Workspace}, ask}
}

// workerEditorClosed takes the text back from $EDITOR: an unchanged or blank result changes nothing, anything else is sent to be
// checked and written.
func workerEditorClosed(m Model, msg DraftEditorClosed) (Model, []Cmd, bool) {
	if msg.UID != WorkerDraftUID {
		return m, nil, false
	}
	f := &m.WorkerCard
	if !f.Open {
		return m, nil, true
	}
	if msg.Err != nil {
		f.Err = msg.Err
		return m, nil, true
	}
	text := normalizeLineBreaks(msg.Text)
	if strings.TrimSpace(text) == "" || text == m.workerEditStart() {
		return m, nil, true
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{SaveWorkerEdit{Text: text}}, true
}

// workerEditStart is the text the editor is given: the refused edit when there is one, else the file.
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
	if msg.Err != nil {
		f.EditText = msg.Text
		f.Err = fmt.Errorf("The edit was not saved and the file is as it was: %w. Press e to keep editing; your text is kept.", msg.Err)
		return m, nil
	}
	f.EditText, f.Err = "", nil
	return m.workerWritten("Edited the worker: worker.toml written.")
}

// workerStopped closes the card with what the stop did.
func workerStopped(m Model, msg WorkerStopped) (Model, []Cmd) {
	f := &m.WorkerCard
	if !f.Open || !f.Stopping {
		return m, nil
	}
	f.Stopping = false
	if msg.Err != nil {
		f.Err = fmt.Errorf("The stop was refused: %w", msg.Err)
		return m, nil
	}
	m.WorkerCard = WorkerCardState{}
	tail := "the plugin then starts a fresh one with the new worker.toml."
	switch {
	case msg.Gone:
		m.Notice = "No worker is running now. " + m.startSentence()
	default:
		if !msg.Restarts {
			tail = "nothing starts it again: run `tern-kata worker`."
		}
		switch {
		case msg.Mode == worker.StopNow:
			m.Notice = "The worker is ending now; runs in flight are held `interrupted`; " + tail
		case msg.InFlight > 0:
			m.Notice = fmt.Sprintf("The worker finishes its %d run%s in flight, then exits; %s", msg.InFlight, plural(msg.InFlight), tail)
		default:
			m.Notice = "The worker is exiting; " + tail
		}
	}
	m, ask := m.askWorker(workerPollEvery)
	return m, []Cmd{ask}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// workerCardKey is one key for the Worker card on top. Left, right and space move through the trust choice, and in a text field
// space is typed.
func workerCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.WorkerCard
	if f.Saving || f.Stopping {
		return m, nil
	}
	if f.Restart {
		return workerRestartKey(m, msg)
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeWorkerCard)
	if bound && binding.Action == WorkerEdit {
		if m.WorkerCardEdits() {
			return m, []Cmd{OpenDraftEditor{UID: WorkerDraftUID, Text: m.workerEditStart(), Suffix: ".toml"}}
		}
		bound = false // no file to edit: the key is typed
	}
	field := f.at(f.Field)
	switch {
	case bound && binding.Action == WorkerCancel:
		m.WorkerCard = WorkerCardState{}
	case bound && binding.Action == WorkerNext:
		cycleField(&m, 1)
	case bound && binding.Action == WorkerPrevious:
		cycleField(&m, -1)
	case bound && binding.Action == WorkerSave:
		return submitWorker(m)
	case field == nil && bound && (binding.Action == InputSpace || binding.Action == WorkerNextChoice || binding.Action == WorkerPreviousChoice):
		if m.WorkerCardShows(f.Field) {
			f.Trusted, f.Err = !f.Trusted, nil
		}
	case field != nil && m.WorkerCardShows(f.Field):
		var edited bool
		if bound && binding.Action == InputSpace {
			bound = false // a space in a text field is typed
		}
		if *field, edited = editText(*field, &f.Caret, f.Field, binding, bound, msg); edited {
			f.Err = nil
		}
	}
	return m, nil
}

// choose gives the trust field its choice at index i: 0 is No, 1 is Yes.
func (f *WorkerCardState) choose(i int) {
	if i == 0 || i == 1 {
		f.Trusted, f.Err = i == 1, nil
	}
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
		m.Notice = "Left the worker running; it reads the new worker.toml when it next starts."
	}
	return m, nil
}
