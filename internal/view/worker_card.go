package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// WorkerID is the Worker card's node.
const WorkerID = "layer.worker"

// WorkerFieldID is the id of the Worker card's field called name.
func WorkerFieldID(name model.Field) string { return WorkerID + "." + string(name) }

var workerPlaceholders = map[model.Field]string{
	model.WorkerFieldActor:    "who the runs are attributed to",
	model.WorkerFieldCommand:  "omp",
	model.WorkerFieldModel:    "blank: the agent's own default",
	model.WorkerFieldThinking: "blank: the agent's own default",
}

// workerLayer is the Worker card: the facts about this machine's worker, then what the machine still needs, as fields to fill in and a
// save, or the question how to restart the running worker once a save has been made.
func workerLayer(m model.Model) tern.Element {
	f := m.WorkerCard
	if !f.Open {
		return nil
	}
	setup, status := m.Worker.Setup, m.Worker
	line := func(key string, tone ui.Tone, text string) tern.Element {
		return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Text: text}
	}
	gap := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key}, Size: ui.GapMD} }
	var children []tern.Element

	switch m.WorkerCardKind() {
	case model.WorkerLoading:
		children = append(children, line("loading", ui.TonePending, "Reading this machine's worker setup…"))
	case model.WorkerUnreadable:
		children = append(children, line("unreadable", ui.ToneError, "Cannot read the worker setup: "+status.SetupErr.Error()))
	case model.WorkerUnbound:
		children = append(children, line("unbound", ui.ToneWarning, "This folder is not bound to a kata Project, so there is no Project to serve. Run `kata init` here, then open the card again."))
	default:
		children = append(children,
			line("project", ui.ToneNeutral, "Project: "+setup.Project+" · checkout "+setup.Checkout+" (the folder holding .kata.toml)"),
			line("file", fileTone(m), "worker.toml: "+fileWords(m)+" ("+setup.Path+")"))
	}
	if m.WorkerCardKind() >= model.WorkerSetUp {
		children = append(children, line("worker", workerTone(m), "Worker: "+workerWords(m)), line("supervision", ui.ToneMuted, "Supervision: "+supervisionWords(m)))
	}

	switch {
	case f.Saved != "":
		children = append(children, gap("gap-saved"), line("saved", ui.ToneSuccess, f.Saved), line("trust-note", ui.ToneWarning, model.WorkerTrustNote), line("next", ui.ToneNeutral, f.Next))
		if f.Restart {
			when, now := m.WorkerRestartPrompt()
			children = append(children, gap("gap-restart"), line("restart-idle", ui.ToneNeutral, when), line("restart-now", ui.ToneWarning, now))
		}
	case m.WorkerCardSaves():
		children = append(append(children, gap("gap-fields")), workerFields(m)...)
	}
	if f.Saving {
		children = append(children, line("pending", ui.TonePending, "Writing worker.toml…"))
	}
	if f.Stopping {
		children = append(children, line("pending", ui.TonePending, "Asking the worker to stop…"))
	}
	if f.Err != nil {
		children = append(children, line("error", ui.ToneError, f.Err.Error()))
	}
	children = append(children, gap("gap-actions"), workerActions(m))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "worker"}, Head: ui.T("Worker"), Modal: true, Size: ui.SizeLG, Children: children})
}

// workerFields are the card's inputs: the setup fields when worker.toml is to be made, the checkout and the trust note, and the trust choice.
func workerFields(m model.Model) []tern.Element {
	f := m.WorkerCard
	tone := func(name model.Field) ui.Tone {
		if f.Field == name {
			return ui.ToneAccent
		}
		return ui.ToneNeutral
	}
	var children []tern.Element
	if m.WorkerCardKind() == model.WorkerSetUp {
		for _, spec := range model.WorkerFields {
			if !spec.Text {
				continue
			}
			text := f.Value(spec.Name)
			children = append(children,
				ui.Text{Common: ui.Common{Key: string(spec.Name) + "-label", Tone: tone(spec.Name)}, Text: model.WorkerPrompt(spec.Name)},
				ui.Input{Common: ui.Common{Key: string(spec.Name), Tone: tone(spec.Name), Aria: model.WorkerPrompt(spec.Name)}, Field: ui.Field{Text: text, Cursor: caretAt(f.Caret, spec.Name, text), Mode: "insert", Placeholder: workerPlaceholders[spec.Name], Readonly: f.Saving}})
		}
	}
	verb := "Set up lists"
	if m.WorkerCardKind() == model.WorkerServe {
		verb = "Serving lists"
	}
	children = append(children,
		ui.Text{Common: ui.Common{Key: "checkout", Tone: ui.ToneInfo}, Text: verb + " " + m.Worker.Setup.Project + " with checkout " + m.Worker.Setup.Checkout + "."},
		ui.Text{Common: ui.Common{Key: "trust-note", Tone: ui.ToneWarning}, Text: model.WorkerTrustNote},
		choiceField(model.WorkerFieldTrust, tone(model.WorkerFieldTrust), "Trust", prefixIf(f.Field == model.WorkerFieldTrust, "Trust this Project's job authors: "), []string{"No", "Yes"}, func(i int) bool { return (i == 1) == f.Trusted }, -1))
	return children
}

func prefixIf(on bool, text string) string {
	if on {
		return "> " + text
	}
	return text
}

// workerActions is the card's button row. It holds the saves and the restart choice today; the edit (e) and the supervision actions
// that later tickets add join it here.
func workerActions(m model.Model) tern.Element {
	f := m.WorkerCard
	switch {
	case f.Saved != "" && f.Restart:
		return buttonRow(
			cardButton("restart-idle", model.ModeWorkerRestart, model.WorkerRestartIdle, "Restart when idle", true),
			cardButton("restart-now", model.ModeWorkerRestart, model.WorkerRestartNow, "Restart now", false),
			cardButton("leave", model.ModeWorkerRestart, model.WorkerLeave, "Leave it running", false))
	case m.WorkerCardSaves():
		save := "Set up worker"
		if m.WorkerCardKind() == model.WorkerServe {
			save = "Serve " + m.Worker.Setup.Project
		}
		return buttonRow(cardButton("save", model.ModeWorkerCard, model.WorkerSave, save, true), cardButton("cancel", model.ModeWorkerCard, model.WorkerCancel, "Cancel", false))
	}
	return buttonRow(cardButton("close", model.ModeWorkerCard, model.WorkerCancel, "Close", false))
}

func fileWords(m model.Model) string {
	s := m.Worker.Setup
	switch m.WorkerCardKind() {
	case model.WorkerSetUp:
		return "none yet; this machine has no worker config"
	case model.WorkerBroken:
		return "does not load: " + s.LoadErr
	case model.WorkerServe:
		return "loads, and does not list " + s.Project
	}
	return "loads, lists " + s.Project + " with checkout " + strings.Join(s.Checkouts, ", ")
}

func fileTone(m model.Model) ui.Tone {
	switch m.WorkerCardKind() {
	case model.WorkerBroken:
		return ui.ToneError
	case model.WorkerSetUp, model.WorkerServe:
		return ui.ToneWarning
	}
	return ui.ToneSuccess
}

func workerWords(m model.Model) string {
	w := m.Worker
	switch {
	case !w.Read:
		return "asking…"
	case w.Err != nil:
		return w.Err.Error()
	case !w.Running:
		return "not running"
	}
	text := "running as executor " + w.Executor + " (process " + strconv.Itoa(w.PID) + ", " + inFlightWords(m) + ")"
	switch w.Stopping {
	case "idle":
		text += ", draining toward an exit"
	case "now":
		text += ", ending now"
	}
	return text
}

func inFlightWords(m model.Model) string {
	n := max(len(m.WorkerInFlight()), m.Worker.InFlight)
	switch n {
	case 0:
		return "no run in flight"
	case 1:
		return "1 run in flight"
	}
	return strconv.Itoa(n) + " runs in flight"
}

func workerTone(m model.Model) ui.Tone {
	if m.Worker.Err != nil {
		return ui.ToneError
	}
	if m.Worker.Running {
		return ui.ToneSuccess
	}
	return ui.ToneWarning
}

func supervisionWords(m model.Model) string {
	w, s := m.Worker, m.Worker.Setup
	switch {
	case s.Manual:
		return "worker.manual is present: the plugin neither starts nor restarts the worker."
	case w.Running && w.Supervised:
		return "the plugin supervises it and starts it again when it exits."
	case w.Running:
		return "started by hand: the plugin does not restart it."
	case s.Exists && s.LoadErr == "":
		return "the plugin starts a worker within 10 seconds."
	}
	return "the plugin starts a worker once a worker.toml loads."
}
