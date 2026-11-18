package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// keepsGoingWords tells that closing the card stops nothing: the setup job and the indexing run in the kata daemon.
const keepsGoingWords = "kata keeps going if you close this."

func embeddingsLayer(m model.Model) tern.Element {
	if !m.EmbeddingsCard.Open {
		return nil
	}
	body := m.EmbeddingsCardBody()
	var children []tern.Element
	if body.Question != "" {
		children = append(children, toneText("question", ui.ToneNeutral, body.Question))
	}
	if body.State != "" {
		tone := ui.ToneNeutral
		switch {
		case body.Failed:
			tone = ui.ToneError
		case body.Pending:
			tone = ui.TonePending
		}
		children = append(children, toneText("state", tone, body.State))
	}
	if body.Download != nil {
		children = append(children, progressBar("download", *body.Download))
	}
	if body.Indexing != nil {
		children = append(children, progressBar("indexing", *body.Indexing))
	}
	if body.KeepsGoing {
		children = append(children, toneText("keeps-going", ui.ToneMuted, keepsGoingWords))
	}
	if body.Err != "" {
		children = append(children, toneText("error", ui.ToneError, body.Err))
	}
	switch mode := m.EmbeddingsCardMode(); mode {
	case model.ModeEmbeddingsAsk:
		children = append(children, formFoot(
			cardButton("turn-on", mode, model.TurnOnEmbeddingsConfirm, "Turn on", true),
			exitButton("decline", mode, model.TurnOnEmbeddingsDecline, "Not now")))
	case model.ModeEmbeddingsRestart:
		children = append(children, formFoot(
			cardButton("restart", mode, model.RestartDaemonConfirm, "Restart now", true),
			exitButton("decline", mode, model.RestartDaemonDecline, "Not now")))
	case model.ModeEmbeddingsOff:
		children = append(children, formFoot(
			cardButton("turn-on", mode, model.TurnOnEmbeddingsAsk, "Turn on…", true),
			exitButton("close", mode, model.EmbeddingsCardClose, "Close")))
	default:
		children = append(children, buttonRow(exitButton("close", mode, model.EmbeddingsCardClose, "Close")))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardEmbeddings)}, Modal: true, Size: ui.SizeLG, Head: ui.T("Search by meaning"), Meta: ui.T(m.EmbeddingsCardTitle()), Icon: ui.IconSearch}, children))
}

// progressBar is kata's progress as a bar; a total kata does not give draws one that sweeps.
func progressBar(key string, p model.EmbeddingsProgress) ui.Progress {
	return ui.Progress{Common: ui.Common{Key: key}, Value: p.Fraction}
}
