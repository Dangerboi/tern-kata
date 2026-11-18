package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// staleText is the one line for a Pane whose stream to kata is down: what is on screen, as of when, and what is off.
func staleText(m model.Model) string {
	text := "Disconnected from kata."
	if !m.StaleSince.IsZero() {
		text += " Showing Issues as of " + m.Calendar.Read(m.StaleSince, m.Now) + "; changes are off until it reconnects."
	} else {
		text += " Changes are off until it reconnects."
	}
	return text
}

// staleLine is the stale text in the pending tone. The error kata gave is in its Hovercard, not on the line.
func staleLine(m model.Model) tern.Element {
	line := plainText("stale", ui.TonePending, staleText(m))
	if m.StreamErr != nil {
		line.Hovercard = ui.Hovercard{Element: plainText("reason", ui.ToneNeutral, model.Reason(m.StreamErr))}
	}
	return line
}

// footerNodes is what the Pane draws under its columns: a question waiting for its answer, then the stale and notice lines.
func footerNodes(m model.Model) []tern.Element {
	var nodes []tern.Element
	// With the Menu open the question is drawn in the Menu, which is modal: a copy here would sit behind it.
	if q, ok := m.Question(); ok && !m.Picker.Open {
		nodes = append(nodes, question(q))
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		nodes = append(nodes, staleLine(m))
	}
	if m.Notice != "" && !m.HelpOpen {
		tone := ui.ToneNeutral
		if m.NoticeKind == model.NoticeFail {
			tone = ui.ToneError
		}
		nodes = append(nodes, ui.Text{Common: ui.Common{Key: "notice", Tone: tone}, Spans: prose(m.Notice, "")})
	}
	return nodes
}

// question is a y/n question with a button for each answer, so a Pane on a phone, with no keyboard, can answer it. The
// buttons press the question's own keys, so a click and the key never disagree.
func question(q model.Question) tern.Element {
	yes := cardButton("yes", q.Mode, q.Yes, q.YesWords, !q.Danger)
	if q.Danger {
		yes = destructive(yes)
	}
	return ui.Row{Common: ui.Common{Key: string(q.Mode)}, Gap: ui.GapMD, Align: ui.AlignCenter, Wrap: true, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "text", Tone: questionTone(q)}, Spans: prose(q.Text, "")}, yes, cardButton("no", q.Mode, q.No, q.NoWords, false))}
}

// questionTone is warning for a destructive question and default text otherwise: accent marks where you are, not a question.
func questionTone(q model.Question) ui.Tone {
	if q.Danger {
		return ui.ToneWarning
	}
	return ui.ToneNeutral
}

// toasts is the model's toast, drawn from the step that raised it until ToastExpired for its Seq, so a late client never gets a stale
// button. The node is keyed on Seq so each one shows once; Tern ignores the button before it learned them. A toast without a
// label has no button: Tern would name one after its action.
func toasts(m model.Model) []tern.Element {
	t := m.Toast
	if t == nil {
		return nil
	}
	toast := ui.Toast{Common: ui.Common{Key: "toast-" + strconv.Itoa(t.Seq), Tone: ui.ToneSuccess}, Text: t.Text}
	if t.Label != "" {
		toast.Action, toast.Label = ToastClick(t.Seq), t.Label
		if t.Key != "" {
			// Tern's toast has no keycap slot, so the key follows the label.
			toast.Label += " " + keycap(t.Chord, t.Key)
		}
	}
	return []tern.Element{toast}
}
