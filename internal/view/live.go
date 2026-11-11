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

func footerTexts(m model.Model) []ui.Text {
	var texts []ui.Text
	if m.AskQuit {
		texts = append(texts, plainText("quit", ui.ToneAccent, model.QuitQuestion))
	}
	if m.Views.Deleting != "" {
		texts = append(texts, plainText("delete-view", ui.ToneAccent, model.DeleteViewQuestion(m.Views.Deleting)))
	}
	if m.ConfigPage.AskSwitch {
		texts = append(texts, plainText("switch-docbank", ui.ToneAccent, model.SwitchDocbankQuestion))
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		texts = append(texts, plainText("stale", ui.TonePending, staleText(m)))
	}
	if m.Notice != "" && !m.HelpOpen {
		texts = append(texts, plainText("notice", ui.ToneAccent, m.Notice))
	}
	return texts
}

// toasts is the model's toast, drawn from the step that raised it until ToastExpired for its Seq, so a late client never gets a stale
// button. The node is keyed on Seq so each one shows once; Tern ignores the button before it learned them.
func toasts(m model.Model) []tern.Element {
	t := m.Toast
	if t == nil {
		return nil
	}
	return []tern.Element{ui.Toast{Common: ui.Common{Key: "toast-" + strconv.Itoa(t.Seq), Tone: ui.ToneAccent}, Text: t.Text, Action: ToastClick(t.Seq), Label: t.Label}}
}
