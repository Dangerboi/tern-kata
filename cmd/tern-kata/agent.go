package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent [--as <actor>] [--ref <qualified ref>] [--address-prefix <actor>/<name>] [--home <dir>] [--db <file>] [--schema <name>] [--dsn-file <file>] [--env-file <file>] [--daemon <name> [--token-file <file>] [--auth-file <file>] | --server <url> [--token-file <file>] [--auth-file <file>] [--allow-insecure] [--trust-private-network]] -- <command> [args...]"

// planAgent parses `tern-kata agent [--as <actor>] [--ref <qualified ref>] [--address-prefix <actor>/<name>] [route flags] -- <command>
// [args...]` (args[0] is "agent"). The flags come before the lone --, and everything after it is the command and its arguments,
// verbatim: Operands holds them.
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
		if arg == "--address-prefix" || strings.HasPrefix(arg, "--address-prefix=") {
			prefix, err := parseFlagValue(args[:end], &i, "--address-prefix", "an Agent address up to its block id")
			if err != nil {
				return LaunchPlan{}, fmt.Errorf("agent: %w; %s", err, agentUsage)
			}
			plan.AgentAddressPrefix = prefix
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
	if route := plan.AgentRoute; route.Daemon != "" && (route.Server != "" || route.AllowInsecure || route.TrustPrivate) {
		return LaunchPlan{}, fmt.Errorf("agent: --daemon takes its server and credential from the catalog entry, so it excludes --server, --allow-insecure and --trust-private-network; %s", agentUsage)
	}
	plan.Actor = plan.fallbackAuthor
	return plan, nil
}

// parseAgentRoute reads one of the route flags (--home, --db, --schema, --dsn-file, --env-file, --daemon, --server, --token-file, --auth-file, --allow-insecure, --trust-private-network) from args[*i] into route and advances *i
// past its value; matched is false for any other flag.
func parseAgentRoute(args []string, i *int, route *data.AgentRoute) (matched bool, err error) {
	name, value, hasValue := strings.Cut(args[*i], "=")
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
		field, what = &route.TokenFile, "the private token file the Pane wrote"
	case "--auth-file":
		field, what = &route.AuthFile, "the Pane's KATA_AUTH_TOKEN_FILE path"
	case "--db":
		field, what = &route.DB, "the Pane's KATA_DB path"
	case "--schema":
		field, what = &route.Schema, "the Pane's KATA_POSTGRES_SCHEMA"
	case "--env-file":
		field, what = &route.EnvFile, "the private credential env file the Pane wrote"
	case "--dsn-file":
		field, what = &route.DSNFile, "the private DSN file the Pane wrote"
	case "--allow-insecure", "--trust-private-network":
		if hasValue {
			return true, fmt.Errorf("%s takes no value (got %q)", name, value)
		}
		if name == "--allow-insecure" {
			route.AllowInsecure = true
		} else {
			route.TrustPrivate = true
		}
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
