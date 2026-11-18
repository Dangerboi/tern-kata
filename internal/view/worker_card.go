package view

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// workerMono is what the card's sentences name that is a command or a file, longest first so that "tern-kata worker doctor" is
// one span and not "tern-kata worker" followed by plain text.
var workerMono = []string{"tern-kata worker doctor", "tern-kata worker", "worker.toml", "worker.manual", "commands = true"}

// monoWords is text in style with each of words drawn mono, the earliest match first.
func monoWords(text, style string, words ...string) []ui.Span {
	var spans []ui.Span
	for text != "" {
		at, found := len(text), ""
		for _, word := range words {
			if i := strings.Index(text, word); i >= 0 && i < at {
				at, found = i, word
			}
		}
		if found == "" {
			return append(spans, ui.Span{T: text, S: style})
		}
		if at > 0 {
			spans = append(spans, ui.Span{T: text[:at], S: style})
		}
		spans = append(spans, monoSpan(found, style))
		text = text[at+len(found):]
	}
	return spans
}

// workerText is a line of the card's sentences in tone, with its commands and files drawn mono.
func workerText(key string, tone ui.Tone, text string) ui.Text {
	return ui.Text{Common: ui.Common{Key: key, Tone: tone}, Spans: monoWords(text, toneStyle(tone), workerMono...)}
}

func workerLayer(m model.Model) tern.Element {
	f := m.WorkerCard
	if !f.Open {
		return nil
	}
	gap := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key}, Size: ui.GapMD} }
	var children []tern.Element

	switch m.WorkerCardKind() {
	case model.WorkerLoading:
		children = append(children, toneText("loading", ui.TonePending, model.Progress("Reading", "the worker setup")))
	case model.WorkerUnreadable:
		children = append(children, toneText("unreadable", ui.ToneError, model.Cannot("read", "the worker setup", m.Worker.SetupErr)))
	case model.WorkerUnbound:
		children = append(children, ui.Text{Common: ui.Common{Key: "unbound", Tone: ui.ToneWarning}, Spans: []ui.Span{
			{T: "This folder is not bound to a kata Project, so there is nothing to serve. Run ", S: toneStyle(ui.ToneWarning)},
			monoSpan("kata init", toneStyle(ui.ToneWarning)),
			{T: " here, then open this card again.", S: toneStyle(ui.ToneWarning)}}})
	default:
		children = append(children, workerFacts(m))
	}

	switch {
	case f.Saved != "":
		children = append(children, gap("gap-saved"), toneText("saved", ui.ToneSuccess, f.Saved), workerText("trust-note", ui.ToneMuted, model.WorkerTrustNote))
		if f.Restart {
			children = append(children, gap("gap-restart"), stopQuestion(m, "restart", f.Next, "Restart when idle", "Restart now"))
		} else {
			children = append(children, workerText("next", ui.ToneMuted, f.Next))
		}
	case f.AskOff:
		children = append(children, gap("gap-off"), stopQuestion(m, "off", model.WorkerOffQuestion, "Stop when idle", "Stop now"))
	case m.WorkerCardSaves():
		children = append(append(children, gap("gap-fields")), workerFields(m)...)
	}
	if f.Supervision != "" {
		children = append(children, gap("gap-supervision"), workerText("supervision-done", ui.ToneSuccess, f.Supervision))
	}
	switch {
	case f.Saving:
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	case f.Switching || f.Stopping:
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Working", "")))
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	children = append(children, workerActions(m))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardWorker)}, Head: ui.T("Worker"), Icon: ui.IconCpu, Modal: true, Size: ui.SizeLG}, children))
}

// workerValue is the value cell of a fact row: it grows into the row and may shrink below its content, as every fact's does.
func workerValue(spans ...ui.Span) ui.Text {
	return ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: spans}
}

// workerLines stacks lines in one value cell: a state and the line that explains it.
func workerLines(lines ...tern.Element) ui.Col {
	return ui.Col{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Gap: ui.GapXS, Children: lines}
}

func workerFacts(m model.Model) tern.Element {
	s := m.Worker.Setup
	rows := []tern.Element{
		propRow("Project", workerValue(ui.Span{T: s.Project, S: "strong"})),
		propRow("Checkout", workerValue(monoSpan(s.Checkout, ""))),
	}
	if m.WorkerCardKind() == model.WorkerServed && !slices.Equal(s.Checkouts, []string{s.Checkout}) {
		rows = append(rows, propRow("Mapped to", workerValue(monoSpan(strings.Join(s.Checkouts, ", "), ""))))
	}
	return propGrid(append(rows,
		propRow("Config", configValue(m)),
		propRow("Worker", workerState(m)),
		propRow("Supervision", supervisionValue(m)))...)
}

// stopQuestion is a question about stopping the running worker: the question strong, then one muted line for what each answer
// does. The answers are the card's buttons.
func stopQuestion(m model.Model, key, question, idleLabel, nowLabel string) tern.Element {
	return ui.Col{Common: ui.Common{Key: key}, Gap: ui.GapXS, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "question"}, Spans: []ui.Span{{T: question, S: "strong"}}},
		toneText("consequences", ui.ToneMuted, m.WorkerStopLine(idleLabel, nowLabel)),
	)}
}

func workerFields(m model.Model) []tern.Element {
	f := m.WorkerCard
	tone := func(name model.Field) ui.Tone {
		if f.Field == name {
			return ui.ToneAccent
		}
		return ui.ToneMuted
	}
	var children []tern.Element
	if m.WorkerCardKind() == model.WorkerSetUp {
		for _, spec := range model.WorkerFields {
			if !spec.Text {
				continue
			}
			text := f.Value(spec.Name)
			children = append(children,
				toneText(string(spec.Name)+"-label", tone(spec.Name), model.WorkerPrompt(spec.Name)),
				ui.Input{Common: ui.Common{Key: string(spec.Name), Tone: tone(spec.Name), Aria: model.WorkerPrompt(spec.Name)}, Field: ui.Field{Text: text, Cursor: caretAt(f.Caret, spec.Name, text), Mode: "insert", Placeholder: ui.T(model.WorkerHint(spec.Name)), Readonly: f.Saving}})
		}
	}
	children = append(children,
		ui.Text{Common: ui.Common{Key: "checkout"}, Spans: []ui.Span{
			{T: "Will serve " + m.Worker.Setup.Project + " from ", S: "muted"},
			timeSpan(m.Worker.Setup.Checkout),
			{T: ".", S: "muted"}}},
		choiceControl(m, model.CardWorker, model.WorkerFieldTrust, "Trust", f.Field == model.WorkerFieldTrust, model.WorkerTrustNote, ""))
	return children
}

func workerActions(m model.Model) tern.Element {
	f := m.WorkerCard
	switch {
	case f.Saved != "" && f.Restart:
		return formFoot(
			cardButton("restart-idle", model.ModeWorkerRestart, model.WorkerRestartIdle, "Restart when idle", true),
			exitButton("leave", model.ModeWorkerRestart, model.WorkerLeave, "Leave it running"),
			cardButton("restart-now", model.ModeWorkerRestart, model.WorkerRestartNow, "Restart now", false))
	case f.AskOff:
		return formFoot(
			cardButton("off-idle", model.ModeWorkerOff, model.WorkerOffIdle, "Stop when idle", true),
			exitButton("off-cancel", model.ModeWorkerOff, model.WorkerOffCancel, "Keep supervision"),
			cardButton("off-now", model.ModeWorkerOff, model.WorkerOffNow, "Stop now", false))
	case m.WorkerCardSaves():
		save := "Set up worker"
		if m.WorkerCardKind() == model.WorkerServe {
			save = "Serve " + m.Worker.Setup.Project
		}
		var others []ui.Button
		if m.WorkerCardEdits() {
			others = append(others, cardButton("edit", model.ModeWorkerCard, model.WorkerEdit, "Edit worker.toml", false))
		}
		if m.WorkerCardKind() >= model.WorkerBroken {
			others = append(others, supervisionButton(m))
		}
		return formFoot(cardButton("save", model.ModeWorkerCard, model.WorkerSave, save, true),
			exitButton("cancel", model.ModeWorkerCard, model.WorkerCancel, "Cancel"), others...)
	}
	closeButton := exitButton("close", model.ModeWorkerRead, model.WorkerCancel, "Close")
	var others []ui.Button
	if m.WorkerCardKind() >= model.WorkerSetUp {
		others = append(others, supervisionButton(m))
	}
	if m.WorkerCardEdits() {
		return formFoot(cardButton("edit", model.ModeWorkerRead, model.WorkerEdit, "Edit worker.toml", true), closeButton, others...)
	}
	return buttonRow(buttonElements(append(others, closeButton))...)
}

func supervisionButton(m model.Model) ui.Button {
	label := "Turn supervision off"
	if m.Worker.Setup.Manual {
		label = "Turn supervision on"
	}
	return cardButton("supervision", m.WorkerCardMode(), model.WorkerSupervision, label, false)
}

// configValue is the Config row's value: worker.toml's state in words and its tone, the path of the file once as a muted second
// line, and, for a file that does not load, why in the error tone.
func configValue(m model.Model) tern.Element {
	s := m.Worker.Setup
	state, tone := "ok", ui.ToneSuccess
	switch m.WorkerCardKind() {
	case model.WorkerSetUp:
		state, tone = "not created", ui.ToneWarning
	case model.WorkerBroken:
		state, tone = "broken", ui.ToneError
	case model.WorkerServe:
		state, tone = "does not list "+s.Project, ui.ToneWarning
	}
	lines := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "state"}, Spans: []ui.Span{{T: state, S: toneStyle(tone)}}},
		ui.Text{Common: ui.Common{Key: "path"}, Spans: []ui.Span{timeSpan(s.Path)}})
	if m.WorkerCardKind() == model.WorkerBroken {
		lines = append(lines, toneText("problem", ui.ToneError, s.Problem()))
	}
	return workerLines(lines...)
}

// workerState is the Worker row's value: whether a worker runs here, as which executor, and what it is doing. The process id is
// in a Hovercard, where a person who needs it can read it.
func workerState(m model.Model) tern.Element {
	w := m.Worker
	switch {
	case !w.Read:
		return workerValue(ui.Span{T: model.Progress("Asking", ""), S: "muted"})
	case w.Err != nil:
		return workerValue(ui.Span{T: model.Cannot("read", "the worker", w.Err), S: toneStyle(ui.ToneError)})
	case !w.Running:
		return workerValue(ui.Span{T: "not running", S: toneStyle(ui.ToneWarning)})
	}
	spans := []ui.Span{
		{T: "running as " + w.Executor, S: toneStyle(ui.ToneSuccess)},
		{T: " · " + m.WorkerBusyWords(), S: "muted"},
	}
	switch w.Stopping {
	case "idle":
		spans = append(spans, ui.Span{T: " · exiting when idle", S: toneStyle(ui.ToneWarning)})
	case "now":
		spans = append(spans, ui.Span{T: " · ending now", S: toneStyle(ui.ToneWarning)})
	}
	value := workerValue(spans...)
	value.Hovercard = ui.Hovercard{Element: ui.Text{Common: ui.Common{Key: "process"}, Spans: []ui.Span{{T: "Process ", S: "muted"}, monoSpan(strconv.Itoa(w.PID), "")}}}
	return value
}

// supervisionValue is the Supervision row's value: on, off or a worker started by hand, with the sentence that explains it
// only for the two that need attention.
func supervisionValue(m model.Model) tern.Element {
	w, s := m.Worker, m.Worker.Setup
	state, tone, hint := "on", ui.ToneSuccess, ""
	switch {
	case s.Manual:
		state, tone, hint = "off", ui.ToneWarning, model.ManualPresentWords()
	case w.Running && w.Supervised:
	case w.Running:
		state, tone, hint = "started by hand", ui.ToneWarning, "the plugin does not restart it"
	case s.Exists && s.LoadErr == "":
	default:
		tone = ui.ToneNeutral
	}
	lines := ui.Nodes(ui.Text{Common: ui.Common{Key: "state"}, Spans: []ui.Span{{T: state, S: toneStyle(tone)}}})
	if hint != "" {
		lines = append(lines, ui.Text{Common: ui.Common{Key: "hint"}, Spans: monoWords(hint, "muted", workerMono...)})
	}
	return workerLines(lines...)
}
