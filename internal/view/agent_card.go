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

// TeamHeadText is the value of the Team row of a parent's card: the team's state and how many children run at once.
func TeamHeadText(t model.Team) string {
	return model.TeamStateWords(t.State) + " · " + strconv.Itoa(t.Limit) + " at once"
}

// TeamChildText says an Issue's agent belongs to a team. The coordinator's address is behind a tooltip, not in the sentence.
func TeamChildText(t model.Team) string {
	return "Part of the team on " + t.ParentRef + "."
}

func teamSection(m model.Model) []tern.Element {
	if t, ok := m.TeamOn(m.AgentCard.IssueUID); ok {
		rows := m.TeamRows(t)
		items := make([]tern.Element, 0, len(rows))
		for _, r := range rows {
			items = append(items, ui.Item{Common: ui.Common{Key: r.UID}, Label: ui.T(TeamRowText(r))})
		}
		out := []tern.Element{propGrid(propRow("Team", toneText("team-head", ui.ToneNeutral, TeamHeadText(t))))}
		if len(items) > 0 {
			out = append(out, ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "team"}, Max: ui.MaxLines(8), Children: items})
		}
		return out
	}
	if t, ok := m.ChildTeam(m.AgentCard.IssueUID); ok {
		text := toneText("team-child", ui.ToneMuted, TeamChildText(t))
		text.Title = "Directed by the coordinator " + t.CoordinatorAddress
		return []tern.Element{text}
	}
	return nil
}

// agentStateStyle is the span style of an agent's state word: working is pending, waiting for you is warning, idle and exited are
// muted, and a state the relay has not given is plain text.
func agentStateStyle(state model.AgentStateWord) string {
	switch state {
	case model.AgentWorking:
		return string(ui.TonePending)
	case model.AgentWaitingInput:
		return string(ui.ToneWarning)
	case model.AgentIdle, model.AgentExited:
		return string(ui.ToneMuted)
	}
	return ""
}

// agentRowSpans is an agent's row: the state word in its tone, the start time, then where the agent is. The words an agent that
// published no address adds are the last part.
func agentRowSpans(m model.Model, a model.IssueAgent) []ui.Span {
	spans := []ui.Span{
		{T: model.AgentCardStateWords(a.State), S: agentStateStyle(a.State)},
		{T: " · ", S: "muted"},
		timeSpan(m.Calendar.Read(a.StartedAt, m.Now)),
		{T: " · ", S: "muted"},
		{T: model.AgentWhere(a.AgentEntry)},
	}
	if a.AddressUsedBy != "" {
		spans = append(spans, ui.Span{T: " · ", S: "muted"}, ui.Span{T: model.AddressUsedWords(a.AddressUsedBy), S: "muted"})
	}
	return spans
}

// AgentRowText is the text of an agent's row, as the spans of agentRowSpans read together.
func AgentRowText(m model.Model, a model.IssueAgent) string {
	return ui.Spans(agentRowSpans(m, a)...).String()
}

// unreadDetail is the muted line after a row that holds a message the agent has not cleared; none when there is no such message.
func unreadDetail(unread string) ui.Rich {
	if unread == "" {
		return ui.Rich{}
	}
	return ui.Spans(ui.Span{T: model.UnreadMessageWords + ": " + unread, S: "muted"})
}

// RemoteRowText has no state: the record is static.
func RemoteRowText(m model.Model, r model.RemoteAgent) string {
	return "on " + r.HostLabel() + " · " + m.Calendar.Read(r.StartedAt, m.Now)
}

func agentLayer(m model.Model) tern.Element {
	card := m.AgentCard
	if !card.Open {
		return nil
	}
	head := func(words string, size ui.Size, foot tern.Element) ui.Overlay {
		return ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T(words), Meta: refMeta(m.RefOf(card.IssueRef)), Icon: ui.IconUsers, Modal: true, Size: size, Foot: foot}
	}
	switch m.AgentCardMode() {
	case model.ModeAgentCardStop:
		return ui.Nodes(popup(head("Stop agent", ui.SizeMD, nil), []tern.Element{
			confirmQuestion("stop-ask", "Stop the agent on "+m.RefOf(card.IssueRef)+"?", "Its block closes and unsaved work is lost."),
			confirmFoot(cardButton("stop-yes", model.ModeAgentCardStop, model.AgentCardStopConfirm, "Stop", false), exitButton("stop-keep", model.ModeAgentCardStop, model.AgentCardStopKeep, "Cancel"), true),
		}))
	case model.ModeAgentCardForget:
		return ui.Nodes(popup(head("Forget agent", ui.SizeMD, nil), []tern.Element{
			confirmQuestion("forget-ask", agentForgetQuestion(m), "Its record and any message left for it are removed. The other host is not touched."),
			confirmFoot(cardButton("forget-yes", model.ModeAgentCardForget, model.AgentCardForgetConfirm, "Forget", false), exitButton("forget-keep", model.ModeAgentCardForget, model.AgentCardForgetKeep, "Cancel"), true),
		}))
	case model.ModeAgentCardTeam:
		return ui.Nodes(teamDialog(m))
	}
	rows := m.AgentCardRows()
	remote := m.AgentCardRemoteRows()
	items := make([]tern.Element, 0, len(rows)+len(remote))
	for _, a := range rows {
		items = append(items, ui.Item{Common: ui.Common{Key: a.Block}, Label: ui.Spans(agentRowSpans(m, a)...), Detail: unreadDetail(a.Unread)})
	}
	for _, r := range remote {
		items = append(items, ui.Item{Common: ui.Common{Key: r.Key()}, Label: ui.T(RemoteRowText(m, r)), Detail: unreadDetail(r.Unread)})
	}
	selected := ""
	if local, ok := m.AgentCardSelectedLocal(); ok {
		selected = AgentItemID(local.Block)
	} else if r, ok := m.AgentCardSelectedRemote(); ok {
		selected = AgentItemID(r.Key())
	}
	list := tern.Element(ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Max: ui.MaxLines(12), Children: items})
	if len(items) == 0 {
		list = noAgents(m)
	}
	children := append([]tern.Element{list}, teamSection(m)...)
	if local, ok := m.AgentCardSelectedLocal(); ok {
		var facts []tern.Element
		for _, fact := range m.AgentCardFacts(local.Block) {
			style := ""
			if fact.Muted {
				style = "muted"
			}
			facts = append(facts, propRow(fact.Label, ui.Text{Common: ui.Common{Key: "v"}, Spans: []ui.Span{{T: fact.Text, S: style}}}))
		}
		if len(facts) > 0 {
			children = append(children, propGrid(facts...))
		}
	}
	return ui.Nodes(popup(head("Agents", ui.SizeLG, agentFoot(m, len(items) > 0)), children))
}

// confirmQuestion is a confirm card's body: the question in the warning tone, then one muted line saying what follows.
func confirmQuestion(key, question, consequence string) tern.Element {
	return ui.Col{Common: ui.Common{Key: key}, Gap: ui.GapSM, Children: ui.Nodes(
		toneText("question", ui.ToneWarning, question),
		toneText("consequence", ui.ToneMuted, consequence),
	)}
}

// noAgents is the empty state of the card: it names the keys that start an agent as keycaps, taken from the card's own bindings so
// a rebind is followed.
func noAgents(m model.Model) tern.Element {
	key := func(action model.Action) string {
		if keys, _ := cardKeys(model.ModeAgentCard, action); len(keys) > 0 {
			return model.Key(keys[0])
		}
		return ""
	}
	e := emptyState("No agents", "")
	e.Key, e.Grow = "none", 0
	e.Text = ui.Spans(prose("Start one with "+key(model.AgentCardSplit)+", "+key(model.AgentCardTab)+" or "+key(model.AgentCardNewBackground)+".", "muted")...)
	return e
}

// agentFoot groups the Agent card's buttons by what they act on, one row each: a new agent on the Issue (and its team), then the
// selected agent, which is drawn only while the card lists an agent. The row's label names the object, so a button says only what
// happens to it. Forget acts only on an agent record from another host, so it is drawn only while the card lists one. Cancel ends
// the card on its own row.
func agentFoot(m model.Model, hasAgents bool) tern.Element {
	b := func(key string, action model.Action, words string, primary bool) tern.Element {
		return unavailable(cardButton(key, model.ModeAgentCard, action, words, primary), m.AgentCardUnavailable(action))
	}
	fresh := []tern.Element{
		b("split", model.AgentCardSplit, "Split", false),
		b("tab", model.AgentCardTab, "Tab", false),
		b("new-background", model.AgentCardNewBackground, "Background", false),
	}
	if _, onTeam := m.TeamOn(m.AgentCard.IssueUID); onTeam {
		fresh = append(fresh, b("pause", model.AgentCardPause, pauseLabel(m), false))
	} else {
		fresh = append(fresh, b("team", model.AgentCardTeam, "Start a team…", false))
	}
	groups := []tern.Element{footGroup("new", "New agent", fresh)}
	cancel := cardButton("cancel", model.ModeAgentCard, model.AgentCardCancel, "Cancel", false)
	var selected []tern.Element
	if hasAgents {
		selected = []tern.Element{
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
		groups = append(groups, footGroup("agent", "Selected", selected))
	}
	if m.NarrowerThan(model.FoldedLegendCols) {
		// On its own row it would wrap away from the buttons; last in the last group it wraps with them, in order.
		last := groups[len(groups)-1].(ui.Row)
		buttons := last.Children[1].(ui.Row)
		buttons.Children = append(buttons.Children, cancel)
		last.Children = ui.Nodes(last.Children[0], buttons)
		groups[len(groups)-1] = last
	} else {
		groups = append(groups, ui.Row{Common: ui.Common{Key: "end"}, Justify: ui.JustifyEnd, Children: ui.Nodes(cancel)})
	}
	return ui.Col{Common: ui.Common{Key: footKey}, Gap: ui.GapSM, Children: groups}
}

// footGroup is a labeled row of buttons.
func footGroup(key, label string, buttons []tern.Element) tern.Element {
	head := toneText("label", ui.ToneMuted, label)
	head.Shrink, head.Min, head.Wrap = new(float64), &ui.Bounds{W: ui.Ch(11)}, ui.WrapNone
	group := ui.Row{Common: ui.Common{Key: "buttons", Grow: 1}, Gap: ui.GapSM, Wrap: true, Children: buttons}
	return ui.Row{Common: ui.Common{Key: key}, Gap: ui.GapMD, Children: ui.Nodes(head, group)}
}

func pauseLabel(m model.Model) string {
	if t, ok := m.TeamOn(m.AgentCard.IssueUID); ok && t.State == model.TeamPaused {
		return "Resume the team"
	}
	return "Pause the team"
}

func agentForgetQuestion(m model.Model) string {
	r, ok := m.AgentCardForgetTarget()
	if !ok {
		return "Forget this agent?"
	}
	return "Forget the agent on " + r.HostLabel() + "?"
}

// teamDialog asks where the coordinator goes and how many children run at once. The coordinator's choices are clickable words that
// press the dialog's own keys, and the limit is a stepper whose two buttons press the keys for fewer and more.
func teamDialog(m model.Model) tern.Element {
	d := m.AgentCard.Team
	where := func(key string, action model.Action, words string, placement model.AgentPlacement) tern.Element {
		_, chord := cardKeys(model.ModeAgentCardTeam, action)
		style := ""
		if d.Where == placement {
			style = "accent strong"
		}
		return ui.Text{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: ButtonClick(chord)}}, Spans: []ui.Span{{T: words, S: style}}, Wrap: ui.WrapNone}
	}
	step := func(key string, action model.Action, words, why string) ui.Button {
		button := cardButton(key, model.ModeAgentCardTeam, action, words, false)
		button.Keys = nil
		return unavailable(button, why)
	}
	fewer, more := "", ""
	if d.Limit <= model.TeamMinLimit {
		fewer = "At least " + strconv.Itoa(model.TeamMinLimit) + "."
	}
	if d.Limit >= model.TeamMaxLimit {
		more = "At most " + strconv.Itoa(model.TeamMaxLimit) + "."
	}
	return popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgent)}, Head: ui.T("Start a team"), Meta: refMeta(m.RefOf(m.AgentCard.IssueRef)), Icon: ui.IconUsers, Modal: true, Size: ui.SizeMD}, []tern.Element{
		toneText("team-ask", ui.ToneNeutral, "A coordinator on this Issue runs background agents on its ready children."),
		propRow("Coordinator", ui.Row{Common: ui.Common{Key: "team-where"}, Gap: ui.GapMD, Children: ui.Nodes(
			where("team-split", model.AgentCardTeamSplit, "Split", model.AgentSplit),
			where("team-tab", model.AgentCardTeamTab, "Tab", model.AgentTab),
			where("team-background", model.AgentCardTeamBackground, "Background", model.AgentBackground))}),
		propRow("Children at once", ui.Row{Common: ui.Common{Key: "team-limit"}, Gap: ui.GapSM, Align: ui.AlignCenter, Children: ui.Nodes(
			step("team-less", model.AgentCardTeamLess, "−", fewer),
			ui.Text{Common: ui.Common{Key: "team-count"}, Text: strconv.Itoa(d.Limit)},
			step("team-more", model.AgentCardTeamMore, "+", more))}),
		formFoot(cardButton("team-start", model.ModeAgentCardTeam, model.AgentCardTeamStart, "Start team", true), exitButton("team-cancel", model.ModeAgentCardTeam, model.AgentCardTeamCancel, "Cancel")),
	})
}
