package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var agentTones = map[model.AgentTone]ui.Tone{
	model.AgentTonePending: ui.TonePending,
	model.AgentToneWarning: ui.ToneWarning,
	model.AgentToneMuted:   ui.ToneMuted,
}

// agentBadge is the Agent property's value in the detail: the state word as a badge that focuses the agent it names, then the
// second "message unread" badge when an agent has a message waiting. The badge keeps the row's old key "v" when it stands alone.
func agentBadge(m model.Model, uid, key string) (tern.Element, bool) {
	badge, ok := m.AgentBadgeFor(uid)
	if !ok {
		return nil, false
	}
	state := agentBadgeElement(badge, key)
	state.Text = badge.Word
	unread, hasUnread := agentUnreadElement(badge, "agent-unread")
	if !hasUnread {
		return state, true
	}
	state.Key = "agent"
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapSM, Children: ui.Nodes(state, unread)}, true
}

// agentBadgeElement is the badge of an Issue's agents with its row/card words ("agent working"), keyed "agent" in a Board card;
// a click focuses the agent it names and nothing else, the Board card's own click included.
func agentBadgeElement(badge model.AgentBadge, key string) ui.Badge {
	common := ui.Common{Key: key, Tone: agentTones[badge.Tone]}
	if badge.Block != "" { // an Issue with only remote agents has no block to focus
		common.Actions = &ui.Actions{Click: AgentClick(badge.Block)}
	}
	return ui.Badge{Common: common, Text: badge.Text}
}

// agentUnreadElement is the second, small warning badge beside the agent badge, drawn only while a message waits.
func agentUnreadElement(badge model.AgentBadge, key string) (ui.Badge, bool) {
	if !badge.Unread {
		return ui.Badge{}, false
	}
	return ui.Badge{Common: ui.Common{Key: key, Tone: ui.ToneWarning}, Text: model.UnreadMessageWords}, true
}

func attentionTone(w model.WorkAttention) ui.Tone {
	if w.Stuck {
		return ui.ToneError
	}
	return ui.ToneWarning
}

// Rows and cards show only the work attention's word, so the Issue's title keeps its room at every width; the Issue detail shows
// the word with the whole message.
