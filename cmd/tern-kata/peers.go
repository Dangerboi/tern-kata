package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const (
	agentsUsage  = "usage: tern-kata agents [--issue <ref>]"
	messageUsage = "usage: tern-kata message <address|ref> -m <text> [--re <comment>] [--as <actor>]"
	teamUsage    = "usage: tern-kata team <ref>"
)

// planAgents parses `tern-kata agents [--issue <ref>]`: TargetRef is the Issue the list is cut to, empty for every Issue.
func planAgents(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: "agents"}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || !strings.HasPrefix(arg, "--") {
			return LaunchPlan{}, fmt.Errorf("agents: unexpected argument %s; %s", arg, agentsUsage)
		}
		if arg == "--issue" || strings.HasPrefix(arg, "--issue=") {
			value, err := parseFlagValue(args, &i, "--issue", "an Issue ref", false)
			if err != nil {
				return LaunchPlan{}, fmt.Errorf("agents: %w; %s", err, agentsUsage)
			}
			plan.TargetRef = value
			continue
		}
		return LaunchPlan{}, fmt.Errorf("agents: unknown flag %s", arg)
	}
	return plan, nil
}

// planTeam parses `tern-kata team <ref>`: TargetRef is the parent Issue, in the forms `agents --issue` takes.
func planTeam(args []string) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: "team"}
	for _, arg := range args[1:] {
		switch {
		case strings.HasPrefix(arg, "--"):
			return LaunchPlan{}, fmt.Errorf("team: unknown flag %s", arg)
		case plan.TargetRef != "":
			return LaunchPlan{}, fmt.Errorf("team: unexpected argument %s; %s", arg, teamUsage)
		default:
			plan.TargetRef = arg
		}
	}
	if plan.TargetRef == "" {
		return LaunchPlan{}, fmt.Errorf("%s", teamUsage)
	}
	return plan, nil
}

// planMessage parses `tern-kata message <address|ref> -m <text>`: TargetRef is the address or Issue ref, MessageText the text,
// which may start with dashes. The sender is an agent only when the environment `tern-kata agent` set is present, KATA_INBOX_USER
// in address form (<actor>/<name>) next to KATA_REF: KATA_TEAMMATE then names the sending agent and KATA_INBOX_USER is its own
// address. A person's own KATA_INBOX_USER (the Attention Page address) does not make them one.
func planMessage(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: "message"}
	var operands []string
	flagsEnded, texted := false, false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case flagsEnded || !strings.HasPrefix(arg, "-") || arg != "-m" && !strings.HasPrefix(arg, "--"):
			operands = append(operands, arg)
		case arg == "--":
			flagsEnded = true
		case arg == "--re" || strings.HasPrefix(arg, "--re="):
			value, err := parseFlagValue(args, &i, "--re", "a comment handle", false)
			if err != nil {
				return LaunchPlan{}, fmt.Errorf("message: %w; %s", err, messageUsage)
			}
			if plan.MessageRe != "" {
				return LaunchPlan{}, fmt.Errorf("message: --re given twice; %s", messageUsage)
			}
			plan.MessageRe = value
		case arg == "-m":
			if i+1 >= len(args) {
				return LaunchPlan{}, fmt.Errorf("message: -m needs the text; %s", messageUsage)
			}
			if texted {
				return LaunchPlan{}, fmt.Errorf("message: -m given twice; %s", messageUsage)
			}
			i++
			plan.MessageText, texted = args[i], true
		default:
			actor, matched, err := parseAs(args, &i)
			if err != nil {
				return LaunchPlan{}, err
			}
			if !matched {
				return LaunchPlan{}, fmt.Errorf("message: unknown flag %s", arg)
			}
			plan.fallbackAuthor = actor
		}
	}
	if len(operands) != 1 || !texted {
		return LaunchPlan{}, fmt.Errorf("%s", messageUsage)
	}
	plan.TargetRef = operands[0]
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	inbox, _ := lookup("KATA_INBOX_USER")
	ref, _ := lookup("KATA_REF")
	if inbox = strings.TrimSpace(inbox); strings.Contains(inbox, "/") && strings.TrimSpace(ref) != "" {
		plan.InboxUser = inbox
		if plan.fallbackAuthor == "" {
			plan.Actor = model.AgentAddressActor(inbox)
		}
	}
	if v, ok := lookup("KATA_TEAMMATE"); ok {
		plan.Teammate = strings.TrimSpace(v)
	}
	return plan, nil
}

// runAgents is `agents`: one line per running agent, cut to plan.TargetRef's Issue when given.
func runAgents(plan LaunchPlan, running []model.IssueAgent, out io.Writer) error {
	if plan.TargetRef != "" {
		running = model.AgentsOnIssue(running, plan.TargetRef)
	}
	if len(running) == 0 {
		_, err := fmt.Fprintln(out, "No running agents.")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ISSUE\tAGENT\tSTATE\tADDRESS")
	for _, a := range running {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", a.IssueRef, dash(a.Teammate), dash(string(a.State)), dash(a.Address))
	}
	return table.Flush()
}

// runTeam is `team`: the team whose parent plan.TargetRef names, from teams.json, agents.json and the latest relay, with the
// parent's children as kata holds them now. No team on the Issue is an error, so a script can tell.
func runTeam(plan LaunchPlan, workspace string, files func() ([]model.Team, []model.AgentEntry, []model.AgentState, error), now time.Time, send func(model.Cmd) model.Msg, out io.Writer) error {
	teams, entries, states, err := files()
	if err != nil {
		return subcommandError{err}
	}
	t, found := model.TeamOnRef(teams, plan.TargetRef)
	if !found {
		return subcommandError{fmt.Errorf("no team on %s; `tern-kata agents` lists the running agents", plan.TargetRef)}
	}
	loaded := send(model.LoadTeamKids{Workspace: workspace, ProjectID: t.ProjectID, ParentUID: t.ParentUID}).(model.TeamKidsLoaded)
	if loaded.Err != nil {
		return subcommandError{fmt.Errorf("cannot read the children of %s: %w", t.ParentRef, loaded.Err)}
	}
	_, err = io.WriteString(out, model.TeamReport(t, model.TeamRows(t, entries, states, loaded.Kids, now)))
	return err
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// runMessage is `message`: the text goes to the agent at plan.TargetRef's address, or to each agent that can receive it on the
// Issue it names, through the same SendAgentMessage the Message card sends, written as plan.Actor with plan.Teammate on the slot.
// An address that holds a message its agent has not cleared is not written over. Every target is tried; the error says how many
// were not written.
func runMessage(plan LaunchPlan, workspace string, running []model.IssueAgent, send func(model.Cmd) model.Msg, out io.Writer) error {
	if plan.InboxUser != "" && plan.Teammate == "" {
		return subcommandError{fmt.Errorf("this agent has no teammate name, so its message would read as the person's own; a shorter agent.toml name gives it one")}
	}
	text, err := model.CheckAgentMessage(plan.MessageText)
	if err != nil {
		return subcommandError{err}
	}
	targets, err := model.MessageTargets(running, plan.TargetRef, plan.InboxUser)
	if err != nil {
		return subcommandError{err}
	}
	verified := daemonVerified(workspace, send)
	missed := 0
	for _, a := range targets {
		line, written := writeSlot(plan, workspace, a, text, verified, send)
		if !written {
			missed++
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	if missed > 0 {
		return subcommandError{fmt.Errorf("the message was not written to %d of %d agents", missed, len(targets))}
	}
	return nil
}

// daemonVerified reads, once and only when a target needs it, whether the daemon verifies the actor of every write: the Pane's own
// capability read, through send. A failed read is not verified.
func daemonVerified(workspace string, send func(model.Cmd) model.Msg) func() bool {
	var answered, verified bool
	return func() bool {
		if !answered {
			read := send(model.LoadAuthCapabilities{Workspace: workspace, Account: true}).(model.AuthCapabilitiesLoaded)
			answered, verified = true, read.Err == nil && read.VerifiedSenders
		}
		return verified
	}
}

// writeSlot writes text to agent a's slot and says what came of it, and whether it was written. The sender rule
// (model.MessageSenderRefusal) decides first; verified asks the daemon, which happens only when its answer can change the rule's
// result: a listed sender who is not the agent's own actor.
func writeSlot(plan LaunchPlan, workspace string, a model.IssueAgent, text string, verified func() bool, send func(model.Cmd) model.Msg) (line string, written bool) {
	owner := model.AgentAddressActor(a.Address)
	listed := a.MessageFromActors()
	known := plan.Actor == owner || !model.MayMessage(plan.Actor, owner, listed) || verified()
	if refusal := model.MessageSenderRefusal(plan.Actor, owner, listed, false, known); refusal != "" {
		return fmt.Sprintf("Cannot message %s on %s: %s (you are %s).", a.Address, a.IssueRef, refusal, plan.Actor), false
	}
	if a.ProjectID == 0 {
		return fmt.Sprintf("Cannot message %s on %s: its entry does not name the Issue's Project (it started with an older tern-kata).", a.Address, a.IssueRef), false
	}
	re := ""
	if plan.MessageRe != "" {
		resolved := send(model.ResolveComment{Workspace: workspace, ProjectID: a.ProjectID, UID: a.IssueUID, Handle: plan.MessageRe}).(model.CommentResolved)
		if resolved.Err != nil {
			return fmt.Sprintf("Cannot message %s on %s: --re %s: %s", a.Address, a.IssueRef, plan.MessageRe, resolved.Err), false
		}
		re = resolved.CommentUID
	}
	sent := send(model.SendAgentMessage{Workspace: workspace, ProjectID: a.ProjectID, UID: a.IssueUID, Actor: plan.Actor, Teammate: plan.Teammate, Address: a.Address, Message: text, Re: re}).(model.AgentMessageSent)
	switch {
	case sent.Err != nil:
		return fmt.Sprintf("Cannot message %s on %s: %s", a.Address, a.IssueRef, sent.Err), false
	case sent.Held != "":
		unread, _, _ := strings.Cut(strings.TrimSpace(sent.Unread), "\n")
		return fmt.Sprintf("%s on %s holds a message it has not cleared (%s); nothing was written there.", a.Address, a.IssueRef, unread), false
	}
	return fmt.Sprintf("Sent the message to %s on %s.", a.Address, a.IssueRef), true
}
