package view

import (
	"strings"
	"unicode/utf8"

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

// footerTexts are the texts drawn under the columns, in order: the disconnected line while it shows, then the Notice. The view draws
// them and laneMax charges for them, so the two cannot disagree on which are there.
func footerTexts(m model.Model) []ui.Text {
	var texts []ui.Text
	if m.AskQuit {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "quit", Tone: ui.ToneAccent}, Text: model.QuitQuestion})
	}
	if m.Stale && m.Err == nil && !m.FirstLoad() && !m.HelpOpen {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "stale", Tone: ui.TonePending}, Text: staleText(m)})
	}
	if m.Notice != "" && !m.HelpOpen {
		texts = append(texts, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.ToneAccent}, Text: m.Notice})
	}
	return texts
}

// footerTextRows is the rows footerTexts take under the columns: for each text, the gap above it and every line of it, wrapped at
// the Pane's columns less the cell the 6px padding takes on each side.
func footerTextRows(m model.Model) int {
	usable := max(m.Cols-2, 1)
	rows := 0
	for _, text := range footerTexts(m) {
		rows++
		for _, line := range strings.Split(text.Text, "\n") {
			rows += max((utf8.RuneCountInString(line)+usable-1)/usable, 1)
		}
	}
	return rows
}
