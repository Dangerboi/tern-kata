package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/el"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const RetryID = "main.retry"

func failureView(m model.Model) tern.Nodes {
	return ui.Nodes(
		ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Err.Error()},
		el.Button{Common: ui.Common{Key: "retry", Actions: &ui.Actions{Click: RetryClick}}, Text: "Retry daemon"},
	)
}

// staleText is the disconnected line: one text line, and the stream's error on the next.
func staleText(m model.Model) string {
	text := "Read-only: reconnecting to kata; showing stale Issues."
	if m.StreamErr != nil {
		text += "\n" + m.StreamErr.Error()
	}
	return text
}

// footerTexts are the texts drawn under the columns, in order: the disconnected line while it shows, then the Notice.
func footerTexts(m model.Model) []ui.Text {
	var texts []ui.Text
	if m.AskQuit {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "quit", Tone: ui.ToneAccent}, Text: model.QuitQuestion})
	}
	if m.Views.Deleting != "" {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "delete-view", Tone: ui.ToneAccent}, Text: model.DeleteViewQuestion(m.Views.Deleting)})
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "stale", Tone: ui.TonePending}, Text: staleText(m)})
	}
	if m.Notice != "" && !m.HelpOpen {
		notice := ui.Text{Common: ui.Common{Key: "notice", Tone: ui.ToneAccent}, Text: m.Notice}
		// The link is the Issue's name inside the sentence; a click anywhere on the Notice opens the Issue.
		if link := m.NoticeLink; link != nil {
			if before, after, found := strings.Cut(m.Notice, link.Text); found {
				notice.Actions = &ui.Actions{Click: NoticeLinkClick}
				notice.Spans = []ui.Span{{T: before}, {T: link.Text, S: "accent link"}, {T: after}}
			}
		}
		texts = append(texts, notice)
	}
	return texts
}
