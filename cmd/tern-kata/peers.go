package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const (
	agentsUsage  = "usage: tern-kata agents [--issue <ref>] [--as <actor>]"
	messageUsage = "usage: tern-kata message <address|ref> -m <text> [--as <actor>]"
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
		actor, matched, err := parseAs(args, &i)
		if err != nil {
			return LaunchPlan{}, err
		}
		if !matched {
			return LaunchPlan{}, fmt.Errorf("agents: unknown flag %s", arg)
		}
		plan.fallbackAuthor = actor
	}
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	return plan, nil
}

// planMessage parses `tern-kata message <address|ref> -m <text>`: TargetRef is the address or Issue ref, MessageText the text,
// which may start with dashes. The sender's identity comes from the environment `tern-kata agent` set: KATA_TEAMMATE names the
// sending agent, KATA_INBOX_USER is its own address.
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
	if v, ok := lookup("KATA_INBOX_USER"); ok {
		plan.InboxUser = strings.TrimSpace(v)
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
	text, err := model.CheckAgentMessage(plan.MessageText)
	if err != nil {
		return subcommandError{err}
	}
	targets, err := model.MessageTargets(running, plan.TargetRef, plan.InboxUser)
	if err != nil {
		return subcommandError{err}
	}
	missed := 0
	for _, a := range targets {
		line, written := deliver(plan, workspace, a, text, send)
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

// deliver writes text to agent a and says what came of it, and whether it was written.
func deliver(plan LaunchPlan, workspace string, a model.IssueAgent, text string, send func(model.Cmd) model.Msg) (line string, written bool) {
	if owner := model.AgentAddressActor(a.Address); owner != plan.Actor {
		return fmt.Sprintf("Cannot message %s on %s: it acts for %s, and only that actor's messages reach it (you are %s).", a.Address, a.IssueRef, owner, plan.Actor), false
	}
	if a.ProjectID == 0 {
		return fmt.Sprintf("Cannot message %s on %s: its entry does not name the Issue's Project (it started with an older tern-kata).", a.Address, a.IssueRef), false
	}
	sent := send(model.SendAgentMessage{Workspace: workspace, ProjectID: a.ProjectID, UID: a.IssueUID, Actor: plan.Actor, Teammate: plan.Teammate, Address: a.Address, Message: text}).(model.AgentMessageSent)
	switch {
	case sent.Err != nil:
		return fmt.Sprintf("Cannot message %s on %s: %s", a.Address, a.IssueRef, sent.Err), false
	case sent.Held != "":
		unread, _, _ := strings.Cut(strings.TrimSpace(sent.Unread), "\n")
		return fmt.Sprintf("%s on %s holds a message it has not cleared (%s); nothing was written there.", a.Address, a.IssueRef, unread), false
	}
	return fmt.Sprintf("Sent the message to %s on %s.", a.Address, a.IssueRef), true
}
