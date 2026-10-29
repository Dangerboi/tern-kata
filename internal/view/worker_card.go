package view

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// workerLayer is the Worker card: the facts about this machine's worker in a grid, then what the machine still needs, as fields
// to fill in and a save, or the question how to stop or restart the running worker, each choice beside what it does.
func workerLayer(m model.Model) tern.Element {
	f := m.WorkerCard
	if !f.Open {
		return nil
	}
	line := func(key string, tone ui.Tone, text string) tern.Element {
		return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Text: text}
	}
	gap := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key}, Size: ui.GapMD} }
	var children []tern.Element

	switch m.WorkerCardKind() {
	case model.WorkerLoading:
		children = append(children, line("loading", ui.TonePending, "Reading this machine's worker setup…"))
	case model.WorkerUnreadable:
		children = append(children, line("unreadable", ui.ToneError, "Cannot read the worker setup: "+m.Worker.SetupErr.Error()))
	case model.WorkerUnbound:
		children = append(children, line("unbound", ui.ToneWarning, "This folder is not bound to a kata Project, so there is no Project to serve. Run `kata init` here, then open the card again."))
	default:
		children = append(children, workerFacts(m))
	}

	switch {
	case f.Saved != "":
		children = append(children, gap("gap-saved"), line("saved", ui.ToneSuccess, f.Saved), line("trust-note", ui.ToneWarning, model.WorkerTrustNote))
		if f.Restart {
			children = append(children, gap("gap-restart"), stopChoices(m, "restart", f.Next, "Restart when idle", "Restart now"))
		} else {
			children = append(children, line("next", ui.ToneNeutral, f.Next))
		}
	case f.AskOff:
		children = append(children, gap("gap-off"), stopChoices(m, "off", model.WorkerOffQuestion, "Stop when idle", "Stop now"))
	case m.WorkerCardSaves():
		children = append(append(children, gap("gap-fields")), workerFields(m)...)
	}
	if f.Supervision != "" {
		children = append(children, gap("gap-supervision"), line("supervision-done", ui.ToneSuccess, f.Supervision))
	}
	if f.Switching {
		children = append(children, line("pending", ui.TonePending, "Changing supervision…"))
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
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardWorker)}, Head: ui.T("Worker"), Modal: true, Size: ui.SizeLG, Children: children})
}

// workerFacts is the card's fact grid: the Project and its checkout, worker.toml, the worker and its supervision, each value
// leading with its state in that state's colour.
func workerFacts(m model.Model) tern.Element {
	s := m.Worker.Setup
	muted := func(text string) ui.Span { return ui.Span{T: text, S: "muted"} }
	fact := func(key, label string, value ...ui.Span) tern.Element {
		return factRow(key, "", muted(label), value...)
	}
	return ui.Col{Common: ui.Common{Key: "facts"}, Gap: ui.GapXS, Children: ui.Nodes(
		fact("project", "Project", ui.Span{T: s.Project, S: "strong"}),
		fact("checkout", "Checkout", ui.Span{T: s.Checkout}),
		fact("file", "worker.toml", ui.Span{T: fileWords(m), S: toneStyle(fileTone(m))}, muted(" · "+s.Path)),
		fact("worker", "Worker", workerSpans(m)...),
		fact("supervision", "Supervision", supervisionSpans(m)...),
	)}
}

// stopChoices is the card's question how to stop the running worker, in bold, then the two ways, labelled as the card's buttons
// are, each beside what it does to the runs in flight.
func stopChoices(m model.Model, key, question, idleLabel, nowLabel string) tern.Element {
	idle, now := m.WorkerStopWords()
	nowStyle := ""
	if m.WorkerInFlightCount() > 0 {
		nowStyle = toneStyle(ui.ToneWarning)
	}
	return ui.Col{Common: ui.Common{Key: key}, Gap: ui.GapXS, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "question"}, Spans: []ui.Span{{T: question, S: "strong"}}},
		factRow("idle", "", ui.Span{T: idleLabel, S: "strong"}, ui.Span{T: idle}),
		factRow("now", "", ui.Span{T: nowLabel, S: "strong"}, ui.Span{T: now, S: nowStyle}),
	)}
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
				ui.Input{Common: ui.Common{Key: string(spec.Name), Tone: tone(spec.Name), Aria: model.WorkerPrompt(spec.Name)}, Field: ui.Field{Text: text, Cursor: caretAt(f.Caret, spec.Name, text), Mode: "insert", Placeholder: model.WorkerHint(spec.Name), Readonly: f.Saving}})
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

// workerActions is the card's button row. It holds the saves, the restart choice, the edit (e) and supervision.
func workerActions(m model.Model) tern.Element {
	f := m.WorkerCard
	switch {
	case f.Saved != "" && f.Restart:
		return buttonRow(
			cardButton("restart-idle", model.ModeWorkerRestart, model.WorkerRestartIdle, "Restart when idle", true),
			cardButton("restart-now", model.ModeWorkerRestart, model.WorkerRestartNow, "Restart now", false),
			cardButton("leave", model.ModeWorkerRestart, model.WorkerLeave, "Leave it running", false))
	case f.AskOff:
		return buttonRow(
			cardButton("off-idle", model.ModeWorkerOff, model.WorkerOffIdle, "Stop when idle", true),
			cardButton("off-now", model.ModeWorkerOff, model.WorkerOffNow, "Stop now", false),
			cardButton("off-cancel", model.ModeWorkerOff, model.WorkerOffCancel, "Keep supervision", false))
	case m.WorkerCardSaves():
		save := "Set up worker"
		if m.WorkerCardKind() == model.WorkerServe {
			save = "Serve " + m.Worker.Setup.Project
		}
		buttons := []tern.Element{cardButton("save", model.ModeWorkerCard, model.WorkerSave, save, true)}
		if m.WorkerCardEdits() {
			buttons = append(buttons, cardButton("edit", model.ModeWorkerCard, model.WorkerEdit, "Edit worker.toml", false))
		}
		if m.WorkerCardKind() >= model.WorkerBroken {
			buttons = append(buttons, supervisionButton(m))
		}
		return buttonRow(append(buttons, cardButton("cancel", model.ModeWorkerCard, model.WorkerCancel, "Cancel", false))...)
	}
	var buttons []tern.Element
	if m.WorkerCardEdits() {
		buttons = append(buttons, cardButton("edit", model.ModeWorkerRead, model.WorkerEdit, "Edit worker.toml", true))
	}
	if m.WorkerCardKind() >= model.WorkerSetUp {
		buttons = append(buttons, supervisionButton(m))
	}
	return buttonRow(append(buttons, cardButton("close", model.ModeWorkerRead, model.WorkerCancel, "Close", false))...)
}

func supervisionButton(m model.Model) tern.Element {
	label := "Turn supervision off"
	if m.Worker.Setup.Manual {
		label = "Turn supervision on"
	}
	return cardButton("supervision", m.WorkerCardMode(), model.WorkerSupervision, label, false)
}

// fileWords is the state of worker.toml. The checkouts it maps the Project to are named only when they are not just the
// Checkout row above.
func fileWords(m model.Model) string {
	s := m.Worker.Setup
	switch m.WorkerCardKind() {
	case model.WorkerSetUp:
		return "none yet"
	case model.WorkerBroken:
		return "does not load: " + s.LoadErr
	case model.WorkerServe:
		return "loads, and does not list " + s.Project
	}
	words := "loads, lists " + s.Project
	if !slices.Equal(s.Checkouts, []string{s.Checkout}) {
		words += " with checkout " + strings.Join(s.Checkouts, ", ")
	}
	return words
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

// workerSpans is whether a worker runs, in its state's colour, then the executor, process and runs in flight of a running one.
func workerSpans(m model.Model) []ui.Span {
	w := m.Worker
	switch {
	case !w.Read:
		return []ui.Span{{T: "asking…", S: "muted"}}
	case w.Err != nil:
		return []ui.Span{{T: w.Err.Error(), S: toneStyle(ui.ToneError)}}
	case !w.Running:
		return []ui.Span{{T: "not running", S: toneStyle(ui.ToneWarning)}}
	}
	spans := []ui.Span{
		{T: "running", S: toneStyle(ui.ToneSuccess)},
		{T: " · executor " + w.Executor + " · process " + strconv.Itoa(w.PID) + " · " + m.WorkerInFlightWords(), S: "muted"},
	}
	switch w.Stopping {
	case "idle":
		spans = append(spans, ui.Span{T: " · draining toward an exit", S: toneStyle(ui.ToneWarning)})
	case "now":
		spans = append(spans, ui.Span{T: " · ending now", S: toneStyle(ui.ToneWarning)})
	}
	return spans
}

// supervisionSpans is whether the plugin supervises the worker, in that state's colour, then what that means for it.
func supervisionSpans(m model.Model) []ui.Span {
	w, s := m.Worker, m.Worker.Setup
	state, style, means := "on", toneStyle(ui.ToneSuccess), "the plugin starts a worker once a worker.toml loads"
	switch {
	case s.Manual:
		state, style, means = "off", toneStyle(ui.ToneWarning), model.ManualPresentWords()
	case w.Running && w.Supervised:
		means = "the plugin starts it again when it exits"
	case w.Running:
		state, style, means = "started by hand", toneStyle(ui.ToneWarning), "the plugin does not restart it"
	case s.Exists && s.LoadErr == "":
		means = "the plugin starts a worker within 10 seconds"
	default:
		style = ""
	}
	return []ui.Span{{T: state, S: style}, {T: " · " + means, S: "muted"}}
}
