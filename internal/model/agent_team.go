package model

import "fmt"

// The Agent card's `c` starts a team on the card's Issue: it opens the start dialog, where the coordinator goes (split, tab or
// background, as `s`, `t` and `g` on the card) and how many children run at once (1 to 8).

// ModeAgentCardTeam is the Agent card showing the team start dialog.
const ModeAgentCardTeam Mode = "agent-card-team"

// The Agent card's `c` and the keys of the start dialog.
const (
	AgentCardTeam           Action = "agent-card-team"
	AgentCardTeamSplit      Action = "agent-card-team-split"
	AgentCardTeamTab        Action = "agent-card-team-tab"
	AgentCardTeamBackground Action = "agent-card-team-background"
	AgentCardTeamMore       Action = "agent-card-team-more"
	AgentCardTeamLess       Action = "agent-card-team-less"
	AgentCardTeamStart      Action = "agent-card-team-start"
	AgentCardTeamCancel     Action = "agent-card-team-cancel"
)

// Why `c` is off.
const (
	TeamExistsReason  = "This Issue already has a team."
	TeamNoChildReason = "This Issue has no open child to run."
)

// TeamDialog is the start dialog: Where the coordinator goes and how many children run at once.
type TeamDialog struct {
	Open  bool
	Where AgentPlacement
	Limit int
}

// teamRefusal is why a team cannot start on issue, "" when the Pane's data allows it: the closed-Issue rule, a team already on
// it (a stopping one counts until its record goes) and no open child. Only the gateway can tell the rest: the agent command and
// name, and whether an open child is in the Issue's own Project.
func (m Model) teamRefusal(issue Issue) string {
	if reason := IssueRefusal(issue, AgentCardTeam); reason != "" {
		return reason
	}
	if _, on := m.TeamOn(issue.UID); on {
		return TeamExistsReason
	}
	open := CountChildren(m.Snapshot.Links, issue.UID).Open
	if m.authority != nil && m.authority.Issue != nil && m.authority.Issue.UID == issue.UID {
		open = max(open, CountChildren(m.authority.Links, issue.UID).Open)
	}
	if open == 0 {
		return TeamNoChildReason
	}
	return ""
}

// openTeamDialog is `c`: the dialog, or the reason as a Notice with the card closed.
func openTeamDialog(m Model) (Model, []Cmd) {
	if reason := m.AgentCardUnavailable(AgentCardTeam); reason != "" {
		m.AgentCard = AgentCardState{}
		m.Notice = reason
		return m, nil
	}
	m.AgentCard.Team = TeamDialog{Open: true, Where: AgentSplit, Limit: TeamDefaultLimit}
	return m, nil
}

// updateTeamDialogKey takes a key while the start dialog is open.
func updateTeamDialogKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeAgentCardTeam)
	if !bound {
		return m, nil
	}
	d := m.AgentCard.Team
	switch binding.Action {
	case AgentCardTeamSplit:
		d.Where = AgentSplit
	case AgentCardTeamTab:
		d.Where = AgentTab
	case AgentCardTeamBackground:
		d.Where = AgentBackground
	case AgentCardTeamMore:
		d.Limit = min(d.Limit+1, TeamMaxLimit)
	case AgentCardTeamLess:
		d.Limit = max(d.Limit-1, TeamMinLimit)
	case AgentCardTeamCancel:
		d = TeamDialog{}
	case AgentCardTeamStart:
		return startTeam(m)
	}
	m.AgentCard.Team = d
	return m, nil
}

// startTeam sends the dialog's choice; the card closes, and a refusal of the card's rules is the Notice.
func startTeam(m Model) (Model, []Cmd) {
	d := m.AgentCard.Team
	reason := m.AgentCardUnavailable(AgentCardTeam)
	issue := m.issueByUID(m.AgentCard.IssueUID)
	m.AgentCard = AgentCardState{}
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	cmd := StartTeam{Placement: d.Where, IssueUID: issue.UID, IssueRef: issue.QualifiedID, IssueTitle: issue.Title, ProjectID: issue.ProjectID,
		ProjectName: issue.ProjectName, PaneBlock: m.PaneBlock, Actor: m.Actor, Limit: d.Limit}
	if m.Bound && issue.ProjectID == m.Project.ID {
		cmd.Workspace = m.Workspace
	}
	m.Notice = fmt.Sprintf("Starting a team on %s…", issue.QualifiedID)
	return m, []Cmd{cmd}
}
