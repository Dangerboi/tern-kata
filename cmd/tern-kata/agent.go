package main

import (
	"fmt"
	"slices"
	"strings"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent --as <actor> [--ref <qualified ref>] -- <command> [args...]"

// planAgent parses `tern-kata agent --as <actor> [--ref <qualified ref>] -- <command> [args...]` (args[0] is "agent"). --as and --ref
// come before the lone --, and everything after it is the command and its arguments, verbatim: Operands holds them.
func planAgent(args []string) (LaunchPlan, error) {
	end := slices.Index(args, "--")
	if end < 0 {
		return LaunchPlan{}, fmt.Errorf("agent: the command must follow --; %s", agentUsage)
	}
	plan := LaunchPlan{Subcommand: "agent", Operands: args[end+1:]}
	for i := 1; i < end; i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return LaunchPlan{}, fmt.Errorf("agent: unexpected argument %s; %s", arg, agentUsage)
		}
		if arg == "--ref" || strings.HasPrefix(arg, "--ref=") {
			ref, err := parseAgentRef(args[:end], &i)
			if err != nil {
				return LaunchPlan{}, err
			}
			plan.AgentRef = ref
			continue
		}
		actor, matched, err := parseAs(args[:end], &i)
		if err != nil {
			return LaunchPlan{}, err
		}
		if !matched {
			return LaunchPlan{}, fmt.Errorf("agent: unknown flag %s", arg)
		}
		plan.fallbackAuthor = actor
	}
	if plan.fallbackAuthor == "" {
		return LaunchPlan{}, fmt.Errorf("agent: --as <actor> is required; %s", agentUsage)
	}
	if len(plan.Operands) == 0 {
		return LaunchPlan{}, fmt.Errorf("agent: no command after --; %s", agentUsage)
	}
	plan.Actor = plan.fallbackAuthor
	return plan, nil
}

// parseAgentRef reads --ref's value, as `--ref <ref>` or `--ref=<ref>`, from args[*i] and advances *i past it.
func parseAgentRef(args []string, i *int) (string, error) {
	value, ok := strings.CutPrefix(args[*i], "--ref=")
	if !ok {
		*i++
		if *i == len(args) || strings.HasPrefix(args[*i], "--") {
			return "", fmt.Errorf("agent: --ref requires a qualified Issue ref; %s", agentUsage)
		}
		value = args[*i]
	}
	if value == "" {
		return "", fmt.Errorf("agent: --ref must be non-empty; %s", agentUsage)
	}
	return value, nil
}
