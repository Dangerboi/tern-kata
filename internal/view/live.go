package view

import (
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

func staleNotice(m model.Model) tern.Element {
	return ui.Text{Common: ui.Common{Key: "stale", Tone: ui.TonePending}, Text: staleText(m)}
}

// staleText is the disconnected line: one text row, and the stream's error on the next.
func staleText(m model.Model) string {
	text := "Read-only: reconnecting to kata; showing stale Issues."
	if m.StreamErr != nil {
		text += "\n" + m.StreamErr.Error()
	}
	return text
}
