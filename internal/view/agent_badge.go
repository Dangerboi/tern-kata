package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var agentTones = map[model.AgentTone]ui.Tone{
	model.AgentToneAccent:  ui.ToneAccent,
	model.AgentToneWarning: ui.ToneWarning,
	model.AgentToneMuted:   ui.ToneMuted,
}

// agentBadge is the Issue's Agent badge as an element of its own, keyed "agent" in a Board card and "v" in the detail
// property row; a click focuses the agent it names and nothing else, the Board card's own click included.
func agentBadge(m model.Model, uid, key string) (ui.Badge, bool) {
	badge, ok := m.AgentBadgeFor(uid)
	if !ok {
		return ui.Badge{}, false
	}
	return agentBadgeElement(badge, key), true
}

// agentBadgeElement is badge as the element agentBadge draws.
func agentBadgeElement(badge model.AgentBadge, key string) ui.Badge {
	return ui.Badge{Common: ui.Common{Key: key, Tone: agentTones[badge.Tone], Actions: &ui.Actions{Click: AgentClick(badge.Block)}}, Text: badge.Text}
}

// attentionStyle is the colour word of an Issue's work attention: stuck is an error, needs-human a warning.
func attentionStyle(w model.WorkAttention) string {
	if w.Stuck {
		return string(ui.ToneError)
	}
	return string(ui.ToneWarning)
}

// attentionTone is attentionStyle as an element tone.
func attentionTone(w model.WorkAttention) ui.Tone {
	if w.Stuck {
		return ui.ToneError
	}
	return ui.ToneWarning
}

// attentionRowWidth is the most cells of the message a row or card shows; the detail shows it whole.
const attentionRowWidth = 60

// attentionShort is the work attention's text as a row or card shows it, the message cut to attentionRowWidth.
func attentionShort(w model.WorkAttention) string {
	w.Msg = cut(w.Msg, attentionRowWidth)
	return w.Text()
}
