package model

import (
	"fmt"
	"slices"
	"strings"
)

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
	// AgentCardPause is `p` on the parent's card: it pauses the team's queue, or resumes a paused one.
	AgentCardPause Action = "agent-card-pause"
)

// Why `c` is off.
const (
	TeamExistsReason  = "This Issue already has a team."
	TeamNoChildReason = "This Issue has no open child to run."
	// TeamNoneReason is why `p` is off on an Issue without a team.
	TeamNoneReason = "This Issue has no team."
	// TeamStoppingReason and TeamEndingReason are why `p` is off on a team that no longer starts children.
	TeamStoppingReason = "The team is stopping: its coordinator exited."
	TeamEndingReason   = "The team is ending."
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

// teamPauseRefusal is why `p` is off on the Issue uid, "" when its team can be paused or resumed.
func (m Model) teamPauseRefusal(uid string) string {
	t, on := m.TeamOn(uid)
	switch {
	case !on:
		return TeamNoneReason
	case t.State == TeamStopping:
		return TeamStoppingReason
	case t.State == TeamEnding:
		return TeamEndingReason
	}
	return ""
}

// pauseTeam is `p`: a running team is paused and a paused one resumed. The card stays open, showing the new state once the
// gateway has written it; a refusal closes it with the reason as a Notice, as the card's other keys do.
func pauseTeam(m Model) (Model, []Cmd) {
	if reason := m.AgentCardUnavailable(AgentCardPause); reason != "" {
		m.AgentCard = AgentCardState{}
		m.Notice = reason
		return m, nil
	}
	t, _ := m.TeamOn(m.AgentCard.IssueUID)
	return m, []Cmd{SetTeamPaused{ParentUID: t.ParentUID, ParentRef: t.ParentRef, Paused: t.State == TeamRunning}}
}

// SetTeamPaused asks the gateway to pause the team on ParentUID (Paused) or resume it. The gateway writes it through
// UpdateTeams and refuses a team that is gone, stopping or ending.
type SetTeamPaused struct {
	ParentUID string
	ParentRef string
	Paused    bool
}

func (SetTeamPaused) cmd() {}

// TeamPausedSet answers SetTeamPaused: Teams is teams.json as it stands after the write.
type TeamPausedSet struct {
	ParentRef string
	Paused    bool
	Teams     []Team
	Err       error
}

func (TeamPausedSet) msg() {}

func teamPausedSet(m Model, msg TeamPausedSet) (Model, []Cmd) {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot %s the team on %s: %s", map[bool]string{true: "pause", false: "resume"}[msg.Paused], msg.ParentRef, msg.Err)
		return m, nil
	}
	m.Teams = msg.Teams
	if msg.Paused {
		m.Notice = fmt.Sprintf("Paused the team on %s: no child starts until you resume it.", msg.ParentRef)
		return m, nil
	}
	m.Notice = fmt.Sprintf("Resumed the team on %s.", msg.ParentRef)
	m, cmds := driveTeams(m, true)
	return restartAgentSessions(m, cmds)
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

// TeamStateWords is the team's state as the Agent card says it.
func TeamStateWords(s TeamState) string {
	switch s {
	case TeamPaused:
		return "paused"
	case TeamStopping:
		return "coordinator exited"
	case TeamEnding:
		return "ending"
	}
	return "running"
}

// TeamRows lists the team's children for the parent's card: the ones the team started, with their agent's state or how they
// ended, then the queued ones, then the rest of the open ones (another Project, a running agent, not ready). The children are
// the parent's Links as the Pane loaded them; a child the Pane has not loaded reads as open. The rows are the ones
// `tern-kata team` prints (see startedChildRow and otherChildRow).
func (m Model) TeamRows(t Team) []TeamRow {
	links := m.Snapshot.Links
	if m.authority != nil {
		links = append(slices.Clone(links), m.authority.Links...)
	}
	status := map[string]string{}
	ref := map[string]string{}
	var order []string
	blockers := map[string][]string{} // blocked uid -> its blockers' statuses
	for _, l := range links {
		switch {
		case l.Type == "parent" && l.ToIssueUID == t.ParentUID:
			if _, seen := status[l.FromIssueUID]; !seen {
				order = append(order, l.FromIssueUID)
			}
			status[l.FromIssueUID], ref[l.FromIssueUID] = l.FromStatus, l.FromQualifiedID
		case l.Type == "blocks":
			blockers[l.ToIssueUID] = append(blockers[l.ToIssueUID], l.FromStatus)
		case l.Type == "blocked_by":
			blockers[l.FromIssueUID] = append(blockers[l.FromIssueUID], l.ToStatus)
		}
	}
	var rows, queued, others []TeamRow
	for _, c := range t.Children {
		var agent *IssueAgent
		for _, a := range m.IssueAgents(c.UID) {
			if a.Block == c.Block && a.State != AgentExited {
				agent = &a
				break
			}
		}
		rows = append(rows, startedChildRow(c, agent, true, status[c.UID] == "closed"))
	}
	for _, uid := range order {
		if t.Started(uid) || status[uid] != "open" {
			continue
		}
		var agent *IssueAgent
		for _, a := range m.IssueAgents(uid) {
			if a.State != AgentExited {
				agent = &a
				break
			}
		}
		project, _, _ := strings.Cut(ref[uid], "#")
		remoteHost := ""
		if issue := m.issueByUID(uid); issue != nil {
			remoteHost = RemoteHostOn(issue.Metadata, m.HostID)
		}
		row := otherChildRow(uid, ref[uid], project == t.Project, agent, remoteHost, !slices.Contains(blockers[uid], "open"))
		if row.State == TeamChildQueued {
			queued = append(queued, row)
		} else {
			others = append(others, row)
		}
	}
	return slices.Concat(rows, queued, others)
}

// ChildTeam is the team that started the Issue uid, when uid is one of its children.
func (m Model) ChildTeam(uid string) (Team, bool) {
	for _, t := range m.Teams {
		if t.Started(uid) {
			return t, true
		}
	}
	return Team{}, false
}
