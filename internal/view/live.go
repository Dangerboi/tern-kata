package view

import (
	"strings"

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
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		texts = append(texts, plainText("stale", ui.TonePending, staleText(m)))
	}
	if m.Notice != "" && !m.HelpOpen {
		notice := plainText("notice", ui.ToneAccent, m.Notice)
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
