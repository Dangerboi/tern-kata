package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// AgentListID is the id of the Agent card's list of agents.
var AgentListID = LayerID(model.CardAgent) + ".list"

// AgentItemID is the id of the Agent card's row for the agent block.
func AgentItemID(block string) string { return itemID(AgentListID, block) }

// AgentRowText is one agent's row: its state in the badge's words, when it started and where it is.
func AgentRowText(m model.Model, a model.IssueAgent) string {
	state, _ := model.AgentStateWords(a.State)
	return state + " · started " + m.DetailTime(a.StartedAt) + " · " + model.AgentWhere(a.AgentEntry)
}

func agentLayer(m model.Model) tern.Element {
	card := m.AgentCard
	if !card.Open {
		return nil
	}
	rows := m.AgentCardRows()
	items := make([]tern.Element, 0, len(rows))
	for _, a := range rows {
		items = append(items, ui.Item{Common: ui.Common{Key: a.Block}, Label: ui.T(AgentRowText(m, a))})
	}
	selected := ""
	if len(rows) > 0 {
		selected = AgentItemID(rows[m.AgentCardCursor()].Block)
	}
	children := []tern.Element{
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T("No agents on this Issue."), Max: ui.MaxLines(12), Children: items},
		buttonRow(
			cardButton("go", model.ModeAgentCard, model.AgentCardGo, "Go to agent", true),
			cardButton("start", model.ModeAgentCard, model.AgentCardStart, "Start another", false),
			cardButton("cancel", model.ModeAgentCard, model.AgentCardCancel, "Cancel", false)),
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Agents on " + card.IssueRef), Modal: true, Size: ui.SizeMD, Children: children})
}
