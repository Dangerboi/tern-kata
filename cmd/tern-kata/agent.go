package main

import (
	"fmt"
	"slices"
	"strings"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent --as <actor> -- <command> [args...]"

// planAgent parses `tern-kata agent --as <actor> -- <command> [args...]` (args[0] is "agent"). --as comes before the lone --, and
// everything after it is the command and its arguments, verbatim: Operands holds them.
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
