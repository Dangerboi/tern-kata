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

func agentBadgeElement(badge model.AgentBadge, key string) ui.Badge {
	common := ui.Common{Key: key, Tone: agentTones[badge.Tone]}
	if badge.Block != "" { // an Issue with only remote agents has no block to focus
		common.Actions = &ui.Actions{Click: AgentClick(badge.Block)}
	}
	return ui.Badge{Common: common, Text: badge.Text}
}

func attentionTone(w model.WorkAttention) ui.Tone {
	if w.Stuck {
		return ui.ToneError
	}
	return ui.ToneWarning
}

// Rows and cards show only the work attention's word, so the Issue's title keeps its room at every width; the Issue detail shows
// the word with the whole message.
