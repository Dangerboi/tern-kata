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

// TeamListID is the id of the team section's list on the parent's Agent card.
var TeamListID = LayerID(model.CardAgent) + ".team"

// TeamItemID is the id of the team section's row for the child Issue uid.
func TeamItemID(uid string) string { return itemID(TeamListID, uid) }

// TeamRowText is one child of the team: its ref and where it stands.
func TeamRowText(r model.TeamRow) string { return r.Ref + " · " + r.State }

// TeamHeadText is the team section's heading: the state and the limit.
func TeamHeadText(t model.Team) string {
	return "Team: " + model.TeamStateWords(t.State) + " · " + strconv.Itoa(t.Limit) + " at once"
}

// TeamChildText is what a child's card says of its team: whose child it is and who directs it.
func TeamChildText(t model.Team) string {
	return "Team: a child of the team on " + t.ParentRef + ", directed by the coordinator " + t.CoordinatorAddress
}

// teamSection is the parent's team under the agent list, and a child's line naming its team; nil for an Issue in neither.
func teamSection(m model.Model) []tern.Element {
	if t, ok := m.TeamOn(m.AgentCard.IssueUID); ok {
		rows := m.TeamRows(t)
		items := make([]tern.Element, 0, len(rows))
		for _, r := range rows {
			items = append(items, ui.Item{Common: ui.Common{Key: r.UID}, Label: ui.T(TeamRowText(r))})
		}
		out := []tern.Element{toneText("team-head", ui.ToneNeutral, TeamHeadText(t))}
		if len(items) > 0 {
			out = append(out, ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "team"}, Max: ui.MaxLines(8), Children: items})
		}
		return out
	}
	if t, ok := m.ChildTeam(m.AgentCard.IssueUID); ok {
		return []tern.Element{toneText("team-child", ui.ToneNeutral, TeamChildText(t))}
	}
	return nil
}

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
	children := append([]tern.Element{list}, teamSection(m)...)
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
			unavailable(cardButton("pause", model.ModeAgentCard, model.AgentCardPause, pauseLabel(m), false), m.AgentCardUnavailable(model.AgentCardPause)),
			unavailable(cardButton("message", model.ModeAgentCard, model.AgentCardMessage, "Message", false), m.AgentCardUnavailable(model.AgentCardMessage)),
			unavailable(cardButton("interrupt", model.ModeAgentCard, model.AgentCardInterrupt, "Interrupt", false), m.AgentCardUnavailable(model.AgentCardInterrupt)),
			unavailable(cardButton("stop", model.ModeAgentCard, model.AgentCardStop, "Stop", false), m.AgentCardUnavailable(model.AgentCardStop)),
			unavailable(cardButton("quick-reply", model.ModeAgentCard, model.AgentCardQuickReply, "Quick reply", false), m.AgentCardUnavailable(model.AgentCardQuickReply)),
			cardButton("cancel", model.ModeAgentCard, model.AgentCardCancel, "Cancel", false)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Agents"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeLG}, children))
}

// pauseLabel is the pause button's words: it resumes a paused team and pauses any other.
func pauseLabel(m model.Model) string {
	if t, ok := m.TeamOn(m.AgentCard.IssueUID); ok && t.State == model.TeamPaused {
		return "Resume the team"
	}
	return "Pause the team"
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
