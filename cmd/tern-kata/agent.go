package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent [--as <actor>] [--ref <qualified ref>] [--home <dir>] [--daemon <name> | --server <url> [--token-file <file>] [--allow-insecure]] -- <command> [args...]"

// planAgent parses `tern-kata agent [--as <actor>] [--ref <qualified ref>] -- <command> [args...]` (args[0] is "agent"). --as and --ref
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
		if matched, err := parseAgentRoute(args[:end], &i, &plan.AgentRoute); matched {
			if err != nil {
				return LaunchPlan{}, fmt.Errorf("agent: %w; %s", err, agentUsage)
			}
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
	if len(plan.Operands) == 0 {
		return LaunchPlan{}, fmt.Errorf("agent: no command after --; %s", agentUsage)
	}
	if route := plan.AgentRoute; route.Daemon != "" && (route.Server != "" || route.TokenFile != "" || route.AllowInsecure) {
		return LaunchPlan{}, fmt.Errorf("agent: --daemon takes its server and credential from the catalog entry, so it excludes --server, --token-file and --allow-insecure; %s", agentUsage)
	}
	plan.Actor = plan.fallbackAuthor
	return plan, nil
}

// parseAgentRoute reads one of --home, --daemon, --server, --token-file or --allow-insecure from args[*i] into route and advances *i
// past its value; matched is false for any other flag.
func parseAgentRoute(args []string, i *int, route *data.AgentRoute) (matched bool, err error) {
	name, _, _ := strings.Cut(args[*i], "=")
	var field *string
	var what string
	switch name {
	case "--home":
		field, what = &route.Home, "a kata home folder"
	case "--daemon":
		field, what = &route.Daemon, "a daemon catalog name"
	case "--server":
		field, what = &route.Server, "a kata server URL"
	case "--token-file":
		field, what = &route.TokenFile, "a credential file path"
	case "--allow-insecure":
		route.AllowInsecure = true
		return true, nil
	default:
		return false, nil
	}
	*field, err = parseFlagValue(args, i, name, what)
	return true, err
}

// parseAgentRef reads --ref's value, as `--ref <ref>` or `--ref=<ref>`, from args[*i] and advances *i past it.
func parseAgentRef(args []string, i *int) (string, error) {
	value, err := parseFlagValue(args, i, "--ref", "a qualified Issue ref")
	if err != nil {
		return "", fmt.Errorf("agent: %w; %s", err, agentUsage)
	}
	return value, nil
}
