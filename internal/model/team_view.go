package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// `tern-kata team <ref>` prints a team the way the Agent card shows it, from the records on disk and the parent's children as
// kata holds them now.

// TeamPendingMaxAge is how long a child recorded without a block counts as being started. A Pane that died between the record
// and the start leaves such a child; after this it counts as neither running nor started.
const TeamPendingMaxAge = 2 * time.Minute

// TeamKid is a child of a team's parent Issue as kata holds it now: Closed is its status, Ready says it is among the Project's
// ready, unowned Issues (what the queue starts).
type TeamKid struct {
	UID       string
	Ref       string
	ProjectID int64
	Closed    bool
	Ready     bool
	// RemoteHost is the host of an agent another host started on the child (its Agent record); "" when there is none.
	RemoteHost string
}

// LoadTeamKids reads the children of a team's parent, with which of them are ready.
type LoadTeamKids struct {
	Workspace string
	ProjectID int64
	ParentUID string
}

func (LoadTeamKids) cmd() {}

// TeamKidsLoaded answers LoadTeamKids.
type TeamKidsLoaded struct {
	Kids []TeamKid
	Err  error
}

func (TeamKidsLoaded) msg() {}

// ResolveComment finds the comment on an Issue that a handle (c:<six characters>, or the bare six, or the whole ULID) names.
type ResolveComment struct {
	Workspace string
	ProjectID int64
	UID       string
	Handle    string
}

func (ResolveComment) cmd() {}

// CommentResolved answers ResolveComment: CommentUID is the comment's ULID.
type CommentResolved struct {
	CommentUID string
	Err        error
}

func (CommentResolved) msg() {}

// ErrNoSuchComment is a handle that names no comment on the Issue.
var ErrNoSuchComment = errors.New("no comment with that handle on the Issue")

// TeamRow is one child of a team: its Issue, its ref, where it stands and the Agent address of its agent ("" when none runs or
// it has none). The Agent card and `tern-kata team` build their rows through startedChildRow and otherChildRow, so both say
// the same words.
type TeamRow struct {
	UID     string
	Ref     string
	State   string
	Address string
}

// The words of a child's state besides a running agent's (AgentStateWords).
const (
	TeamChildQueued       = "queued"
	TeamChildStarting     = "starting"
	TeamChildRanClosed    = "ran: closed"
	TeamChildRanOpen      = "ran: exited with the Issue open"
	TeamChildRanExited    = "ran: exited"
	TeamChildOtherProject = "skipped: other Project"
	TeamChildNotReady     = "not ready"
)

// TeamChildOnHost is the state of an open child that has an agent on another host: the queue does not start a second one.
func TeamChildOnHost(host string) string { return "has an agent on " + host }

// TeamOnRef is the team whose parent ref names: its qualified ref (<project>#<short id>, also <project>/<short id>), its uid or
// its short id, the forms AgentEntry.OnIssue takes.
func TeamOnRef(teams []Team, ref string) (Team, bool) {
	for _, t := range teams {
		if (AgentEntry{IssueUID: t.ParentUID, IssueRef: t.ParentRef}).OnIssue(ref) {
			return t, true
		}
	}
	return Team{}, false
}

// TeamRows lists the children of t: those it started, then the parent's other open children (and any in another Project). A
// started child shows its agent's relay state while the agent runs and how it ended after; the others are skipped when in another
// Project, queued when ready, else not ready. Closed children the team never started are left out.
func TeamRows(t Team, entries []AgentEntry, states []AgentState, kids []TeamKid, now time.Time) []TeamRow {
	kid := func(uid string) (TeamKid, bool) {
		i := slices.IndexFunc(kids, func(k TeamKid) bool { return k.UID == uid })
		if i < 0 {
			return TeamKid{}, false
		}
		return kids[i], true
	}
	agentOn := func(pick func(AgentEntry) bool) (IssueAgent, bool) {
		for _, e := range entries {
			if !pick(e) {
				continue
			}
			if a := agentWithState(e, states); a.State != AgentExited {
				return a, true
			}
		}
		return IssueAgent{}, false
	}
	var rows []TeamRow
	started := make(map[string]bool, len(t.Children))
	for _, c := range t.Children {
		if c.Block == "" && now.Sub(c.StartedAt) >= TeamPendingMaxAge {
			continue // neither running nor started
		}
		started[c.UID] = true
		a, alive := agentOn(func(e AgentEntry) bool { return e.Block == c.Block })
		var agent *IssueAgent
		if alive {
			agent = &a
		}
		k, found := kid(c.UID)
		rows = append(rows, startedChildRow(c, agent, found, k.Closed))
	}
	for _, k := range kids {
		if started[k.UID] || k.Closed {
			continue
		}
		a, alive := agentOn(func(e AgentEntry) bool { return e.IssueUID == k.UID })
		var agent *IssueAgent
		if alive {
			agent = &a
		}
		rows = append(rows, otherChildRow(k.UID, k.Ref, k.ProjectID == t.ProjectID, agent, k.RemoteHost, k.Ready))
	}
	return rows
}

// teamAgentWords is a running agent's state as the team's rows say it: the badge's words.
func teamAgentWords(a IssueAgent) string {
	words, _ := AgentStateWords(a.State)
	return words
}

// startedChildRow is a child the team started. agent is its live agent (nil when none runs); known says the child's Issue is
// among the ones the caller could read, closed whether it is closed.
func startedChildRow(c TeamChild, agent *IssueAgent, known, closed bool) TeamRow {
	row := TeamRow{UID: c.UID, Ref: c.Ref}
	switch {
	case c.Block == "":
		row.State = TeamChildStarting
	case agent != nil:
		row.State, row.Address = teamAgentWords(*agent), agent.Address
	case !known:
		row.State = TeamChildRanExited
	case closed:
		row.State = TeamChildRanClosed
	default:
		row.State = TeamChildRanOpen
	}
	return row
}

// otherChildRow is an open child the team did not start. agent is its live agent (nil when none runs); remoteHost the host of an
// agent on another host ("" when none).
func otherChildRow(uid, ref string, sameProject bool, agent *IssueAgent, remoteHost string, ready bool) TeamRow {
	row := TeamRow{UID: uid, Ref: ref}
	switch {
	case !sameProject:
		row.State = TeamChildOtherProject
	case agent != nil:
		row.State, row.Address = teamAgentWords(*agent), agent.Address
	case remoteHost != "":
		row.State = TeamChildOnHost(remoteHost)
	case ready:
		row.State = TeamChildQueued
	default:
		row.State = TeamChildNotReady
	}
	return row
}

// TeamReport is the printed view of t: its state, limit and coordinator, then one line per row.
func TeamReport(t Team, rows []TeamRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Team on %s: %s, limit %d, coordinator %s\n", t.ParentRef, t.State, t.Limit, dashWord(t.CoordinatorAddress))
	if len(rows) == 0 {
		b.WriteString("No children.\n")
		return b.String()
	}
	width := len("CHILD")
	stateWidth := len("STATE")
	for _, r := range rows {
		width, stateWidth = max(width, len(r.Ref)), max(stateWidth, len(r.State))
	}
	fmt.Fprintf(&b, "%-*s  %-*s  %s\n", width, "CHILD", stateWidth, "STATE", "ADDRESS")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-*s  %-*s  %s\n", width, r.Ref, stateWidth, r.State, dashWord(r.Address))
	}
	return b.String()
}

func dashWord(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
