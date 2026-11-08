package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// AgentListID is the id of the Agent card's list of agents.
var AgentListID = LayerID(model.CardAgent) + ".list"

// AgentItemID is the id of the Agent card's row for the agent block.
func AgentItemID(block string) string { return itemID(AgentListID, block) }

// AgentRowText is one agent's row: its state in the badge's words, when it started, where it is and, while its address holds an uncleared message, that message's first line.
func AgentRowText(m model.Model, a model.IssueAgent) string {
	state, _ := model.AgentStateWords(a.State)
	text := state + " · started " + m.DetailTime(a.StartedAt) + " · " + model.AgentWhere(a.AgentEntry)
	if a.Unread != "" {
		text += " · " + model.UnreadMessageWords + ": " + a.Unread
	}
	return text
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
	list := tern.Element(ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Max: ui.MaxLines(12), Children: items})
	if len(rows) == 0 {
		list = ui.Text{Common: ui.Common{Key: "none", Tone: ui.ToneMuted}, Text: "No agents on this Issue yet."}
	}
	if m.AgentCardMode() == model.ModeAgentCardStop {
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Stop agent"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeMD}, []tern.Element{
			ui.Text{Common: ui.Common{Key: "stop-ask"}, Text: agentStopQuestion(m)},
			buttonRow(destructive(cardButton("stop-yes", model.ModeAgentCardStop, model.AgentCardStopConfirm, "Stop agent", false)), cardButton("stop-keep", model.ModeAgentCardStop, model.AgentCardStopKeep, "Keep it", true)),
		}))
	}
	if m.AgentCardMode() == model.ModeAgentCardTeam {
		return ui.Nodes(teamDialog(m))
	}
	children := []tern.Element{list}
	if len(rows) > 0 {
		for i, fact := range m.AgentFacts(rows[m.AgentCardCursor()].Block) {
			tone := ui.ToneNeutral
			if fact.Muted {
				tone = ui.ToneMuted
			}
			children = append(children, toneText("fact-"+strconv.Itoa(i), tone, fact.Text))
		}
	}
	children = append(children,
		buttonRow(
			unavailable(cardButton("go", model.ModeAgentCard, model.AgentCardGo, "Go to agent", true), m.AgentCardUnavailable(model.AgentCardGo)),
			unavailable(cardButton("split", model.ModeAgentCard, model.AgentCardSplit, "New in a split", false), m.AgentCardUnavailable(model.AgentCardSplit)),
			unavailable(cardButton("tab", model.ModeAgentCard, model.AgentCardTab, "New in a tab", false), m.AgentCardUnavailable(model.AgentCardTab)),
			unavailable(cardButton("new-background", model.ModeAgentCard, model.AgentCardNewBackground, "New in the background", false), m.AgentCardUnavailable(model.AgentCardNewBackground)),
			unavailable(cardButton("background", model.ModeAgentCard, model.AgentCardBackground, "Send to background", false), m.AgentCardUnavailable(model.AgentCardBackground)),
			unavailable(cardButton("team", model.ModeAgentCard, model.AgentCardTeam, "Start a team", false), m.AgentCardUnavailable(model.AgentCardTeam)),
			unavailable(cardButton("message", model.ModeAgentCard, model.AgentCardMessage, "Message", false), m.AgentCardUnavailable(model.AgentCardMessage)),
			unavailable(cardButton("interrupt", model.ModeAgentCard, model.AgentCardInterrupt, "Interrupt", false), m.AgentCardUnavailable(model.AgentCardInterrupt)),
			unavailable(cardButton("stop", model.ModeAgentCard, model.AgentCardStop, "Stop", false), m.AgentCardUnavailable(model.AgentCardStop)),
			unavailable(cardButton("quick-reply", model.ModeAgentCard, model.AgentCardQuickReply, "Quick reply", false), m.AgentCardUnavailable(model.AgentCardQuickReply)),
			cardButton("cancel", model.ModeAgentCard, model.AgentCardCancel, "Cancel", false)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Agents"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeLG}, children))
}

// agentStopQuestion is the stop prompt: it names the agent and says what stopping drops.
func agentStopQuestion(m model.Model) string {
	name := "this agent"
	if e, ok := m.AgentCardStopTarget(); ok {
		name = model.AgentLabel(e)
	}
	return "Stop " + name + " on " + m.AgentCard.IssueRef + "? Its block closes and any unsaved work in it is dropped."
}

// teamDialog is the start dialog of the Agent card's `c`: where the coordinator goes and how many children run at once.
func teamDialog(m model.Model) tern.Element {
	d := m.AgentCard.Team
	return popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Start a team"), Meta: refMeta(m.AgentCard.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeMD}, []tern.Element{
		ui.Text{Common: ui.Common{Key: "team-ask"}, Text: "A coordinator agent on this Issue and a queue of background agents on its ready children."},
		ui.Text{Common: ui.Common{Key: "team-where"}, Text: "Coordinator: " + string(d.Where)},
		ui.Text{Common: ui.Common{Key: "team-limit"}, Text: "Children at once: " + strconv.Itoa(d.Limit) + " (" + strconv.Itoa(model.TeamMinLimit) + " to " + strconv.Itoa(model.TeamMaxLimit) + ")"},
		buttonRow(cardButton("team-split", model.ModeAgentCardTeam, model.AgentCardTeamSplit, "Split", false),
			cardButton("team-tab", model.ModeAgentCardTeam, model.AgentCardTeamTab, "Tab", false),
			cardButton("team-background", model.ModeAgentCardTeam, model.AgentCardTeamBackground, "Background", false),
			cardButton("team-less", model.ModeAgentCardTeam, model.AgentCardTeamLess, "Fewer", false),
			cardButton("team-more", model.ModeAgentCardTeam, model.AgentCardTeamMore, "More", false),
			cardButton("team-start", model.ModeAgentCardTeam, model.AgentCardTeamStart, "Start the team", true),
			cardButton("team-cancel", model.ModeAgentCardTeam, model.AgentCardTeamCancel, "Cancel", false)),
	})
}
