package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var AgentListID = LayerID(model.CardAgent) + ".list"

func AgentItemID(block string) string { return itemID(AgentListID, block) }

var TeamListID = LayerID(model.CardAgent) + ".team"

func TeamRowText(r model.TeamRow) string { return r.Ref + " · " + r.State }

func TeamHeadText(t model.Team) string {
	return "Team: " + model.TeamStateWords(t.State) + " · " + strconv.Itoa(t.Limit) + " at once"
}

func TeamChildText(t model.Team) string {
	return "Team: a child of the team on " + t.ParentRef + ", directed by the coordinator " + t.CoordinatorAddress
}

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

func AgentRowText(m model.Model, a model.IssueAgent) string {
	state, _ := model.AgentStateWords(a.State)
	text := state + " · started " + m.DetailTime(a.StartedAt) + " · " + model.AgentWhere(a.AgentEntry)
	if a.AddressUsedBy != "" {
		text += " · " + model.AddressUsedWords(a.AddressUsedBy)
	}
	if a.Unread != "" {
		text += " · " + model.UnreadMessageWords + ": " + a.Unread
	}
	return text
}

// RemoteRowText has no state: the record is static.
func RemoteRowText(m model.Model, r model.RemoteAgent) string {
	text := "on " + r.HostLabel() + " · started " + m.DetailTime(r.StartedAt)
	if r.Unread != "" {
		text += " · " + model.UnreadMessageWords + ": " + r.Unread
	}
	return text
}

func agentLayer(m model.Model) tern.Element {
	card := m.AgentCard
	if !card.Open {
		return nil
	}
	rows := m.AgentCardRows()
	remote := m.AgentCardRemoteRows()
	items := make([]tern.Element, 0, len(rows)+len(remote))
	for _, a := range rows {
		items = append(items, ui.Item{Common: ui.Common{Key: a.Block}, Label: ui.T(AgentRowText(m, a))})
	}
	for _, r := range remote {
		items = append(items, ui.Item{Common: ui.Common{Key: r.Key()}, Label: ui.T(RemoteRowText(m, r))})
	}
	selected := ""
	if local, ok := m.AgentCardSelectedLocal(); ok {
		selected = AgentItemID(local.Block)
	} else if r, ok := m.AgentCardSelectedRemote(); ok {
		selected = AgentItemID(r.Key())
	}
	list := tern.Element(ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Max: ui.MaxLines(12), Children: items})
	if len(items) == 0 {
		list = plainText("none", ui.ToneMuted, "No agents on this Issue yet.")
	}
	if m.AgentCardMode() == model.ModeAgentCardStop {
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Stop agent"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeMD}, []tern.Element{
			ui.Text{Common: ui.Common{Key: "stop-ask"}, Text: agentStopQuestion(m)},
			buttonRow(destructive(cardButton("stop-yes", model.ModeAgentCardStop, model.AgentCardStopConfirm, "Stop agent", false)), cardButton("stop-keep", model.ModeAgentCardStop, model.AgentCardStopKeep, "Keep it", true)),
		}))
	}
	if m.AgentCardMode() == model.ModeAgentCardForget {
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Forget agent"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeMD}, []tern.Element{
			ui.Text{Common: ui.Common{Key: "forget-ask"}, Text: agentForgetQuestion(m)},
			buttonRow(destructive(cardButton("forget-yes", model.ModeAgentCardForget, model.AgentCardForgetConfirm, "Forget agent", false)), cardButton("forget-keep", model.ModeAgentCardForget, model.AgentCardForgetKeep, "Keep it", true)),
		}))
	}
	if m.AgentCardMode() == model.ModeAgentCardTeam {
		return ui.Nodes(teamDialog(m))
	}
	children := append([]tern.Element{list}, teamSection(m)...)
	if local, ok := m.AgentCardSelectedLocal(); ok {
		for i, fact := range m.AgentFacts(local.Block) {
			tone := ui.ToneNeutral
			if fact.Muted {
				tone = ui.ToneMuted
			}
			children = append(children, toneText("fact-"+strconv.Itoa(i), tone, fact.Text))
		}
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Agents"), Meta: refMeta(card.IssueRef), Icon: ui.IconUsers, Modal: true, Size: ui.SizeLG, Foot: agentFoot(m)}, children))
}

// agentFoot groups the Agent card's buttons by what they act on, one row each: a new agent on the Issue (and its team), then the
// selected agent. The row's label names the object, so a button says only what happens to it. Forget acts only on an agent
// record from another host, so it is drawn only while the card lists one.
func agentFoot(m model.Model) tern.Element {
	b := func(key string, action model.Action, words string, primary bool) tern.Element {
		return unavailable(cardButton(key, model.ModeAgentCard, action, words, primary), m.AgentCardUnavailable(action))
	}
	selected := []tern.Element{
		b("go", model.AgentCardGo, "Go to", true),
		b("background", model.AgentCardBackground, "Background", false),
		b("message", model.AgentCardMessage, "Message", false),
		b("quick-reply", model.AgentCardQuickReply, "Quick reply", false),
		b("interrupt", model.AgentCardInterrupt, "Interrupt", false),
		b("stop", model.AgentCardStop, "Stop", false),
	}
	if len(m.AgentCardRemoteRows()) > 0 {
		selected = append(selected, b("forget", model.AgentCardForget, "Forget", false))
	}
	cancel := cardButton("cancel", model.ModeAgentCard, model.AgentCardCancel, "Cancel", false)
	var end []tern.Element
	if m.NarrowerThan(model.FoldedLegendCols) {
		// Beside the first group it would wrap into the middle of the buttons; last in the grid it wraps with them, in order.
		selected = append(selected, cancel)
	} else {
		end = append(end, cancel)
	}
	return ui.Col{Common: ui.Common{Key: footKey}, Gap: ui.GapSM, Children: ui.Nodes(
		footGroup("new", "New agent", []tern.Element{
			b("split", model.AgentCardSplit, "Split", false),
			b("tab", model.AgentCardTab, "Tab", false),
			b("new-background", model.AgentCardNewBackground, "Background", false),
			b("team", model.AgentCardTeam, "Team", false),
			b("pause", model.AgentCardPause, pauseLabel(m), false),
		}, end...),
		footGroup("agent", "Selected", selected),
	)}
}

// footGroup is a labeled row of buttons; end is drawn at the row's end.
func footGroup(key, label string, buttons []tern.Element, end ...tern.Element) tern.Element {
	head := toneText("label", ui.ToneMuted, label)
	head.Shrink, head.Min, head.Wrap = new(float64), &ui.Bounds{W: ui.Ch(9)}, ui.WrapNone
	group := ui.Row{Common: ui.Common{Key: "buttons", Grow: 1}, Gap: ui.GapSM, Wrap: true, Children: buttons}
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapMD, Children: append(ui.Nodes(head, group), end...)}
}

func pauseLabel(m model.Model) string {
	if t, ok := m.TeamOn(m.AgentCard.IssueUID); ok && t.State == model.TeamPaused {
		return "Resume the team"
	}
	return "Pause the team"
}

func agentStopQuestion(m model.Model) string {
	name := "this agent"
	if e, ok := m.AgentCardStopTarget(); ok {
		name = model.AgentLabel(e)
	}
	return "Stop " + name + " on " + m.AgentCard.IssueRef + "? Its block closes and any unsaved work in it is dropped."
}

func agentForgetQuestion(m model.Model) string {
	r, ok := m.AgentCardForgetTarget()
	if !ok {
		return "Forget this agent?"
	}
	return "Forget the agent on " + r.HostLabel() + " that started " + m.DetailTime(r.StartedAt) + " on " + m.AgentCard.IssueRef +
		"? Its record and any message left at its address are removed; the other host is not touched."
}

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
