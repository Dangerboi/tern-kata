package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const RetryID = "main.retry"

func failureView(m model.Model) tern.Nodes {
	return ui.Nodes(
		plainText("error", ui.ToneError, m.Err.Error()),
		ui.Button{Common: ui.Common{Key: "retry", Actions: &ui.Actions{Click: RetryClick}}, Text: "Retry daemon"},
	)
}

func staleText(m model.Model) string {
	text := "Read-only: reconnecting to kata; showing stale Issues."
	if m.StreamErr != nil {
		text += "\n" + m.StreamErr.Error()
	}
	return text
}

// footerNodes is what the Pane draws under its columns: a question waiting for its answer, then the stale and notice lines.
func footerNodes(m model.Model) []tern.Element {
	var nodes []tern.Element
	if m.AskQuit {
		nodes = append(nodes, question("quit", model.QuitQuestion,
			cardButton("yes", model.ModeQuit, model.ConfirmQuitAction, "Quit", true),
			cardButton("no", model.ModeQuit, model.StayAction, "Stay", false)))
	}
	// With the Menu open the question is drawn in the Menu, which is modal: a copy here would sit behind it.
	if m.Views.Deleting != "" && !m.Picker.Open {
		nodes = append(nodes, deleteViewQuestion(m))
	}
	if m.ConfigPage.AskSwitch {
		nodes = append(nodes, question("switch-docbank", model.SwitchDocbankQuestion,
			cardButton("yes", model.ModeSwitchDocbank, model.ConfirmSwitchDocbankAction, "Switch", true),
			cardButton("no", model.ModeSwitchDocbank, model.KeepDocbankAction, "Keep", false)))
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		nodes = append(nodes, plainText("stale", ui.TonePending, staleText(m)))
	}
	if m.Notice != "" && !m.HelpOpen {
		nodes = append(nodes, plainText("notice", ui.ToneAccent, m.Notice))
	}
	return nodes
}

// question is a y/n question with a button for each answer, so a Pane on a phone, with no keyboard, can answer it. The
// buttons press the question's own keys, so a click and the key never disagree.
func question(key, text string, yes, no ui.Button) tern.Element {
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapMD, Align: ui.AlignCenter, Wrap: true, Children: ui.Nodes(
		plainText("text", ui.ToneAccent, text), yes, no)}
}

func deleteViewQuestion(m model.Model) tern.Element {
	return question("delete-view", model.DeleteViewQuestion(m.Views.Deleting),
		destructive(cardButton("yes", model.ModeDeleteView, model.ConfirmDeleteViewAction, "Delete", false)),
		cardButton("no", model.ModeDeleteView, model.KeepViewAction, "Keep", false))
}

// toasts is the model's toast, drawn from the step that raised it until ToastExpired for its Seq, so a late client never gets a stale
// button. The node is keyed on Seq so each one shows once; Tern ignores the button before it learned them. A toast without a
// label has no button: Tern would name one after its action.
func toasts(m model.Model) []tern.Element {
	t := m.Toast
	if t == nil {
		return nil
	}
	toast := ui.Toast{Common: ui.Common{Key: "toast-" + strconv.Itoa(t.Seq), Tone: ui.ToneAccent}, Text: t.Text}
	if t.Label != "" {
		toast.Action, toast.Label = ToastClick(t.Seq), t.Label
	}
	return []tern.Element{toast}
}
