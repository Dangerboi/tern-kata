package main

import (
	"fmt"
	"slices"
	"strings"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent [--as <actor>] [--ref <qualified ref>] [--address-prefix <actor>/<name> [--forward-comments] [--message-from <actor>]...] [--home <dir>] [--db <file>] [--schema <name>] [--dsn-file <file>] [--env-file <file>] [--daemon <name> [--token-file <file>] [--auth-file <file>] | --server <url> [--token-file <file>] [--auth-file <file>] [--allow-insecure] [--trust-private-network]] -- <command> [args...]"

// tableFlag is one flag of a subcommand with a flag table (`agent`, `docbank`): its parser and the completion scripts read the
// table, so a flag is one row there; the usage constant and the help text are prose and name it by hand.
type tableFlag struct {
	name       string // without its dashes
	takesValue bool
	what       string // the value a flag takes, or what a flag without one does
	set        func(plan *LaunchPlan, value string)
}

// routeFlags choose the kata daemon; `agent` takes them all and `docbank setup` those that pick the daemon (docbankRouteFlags).
var routeFlags = []tableFlag{
	{"home", true, "a kata home folder", func(p *LaunchPlan, v string) { p.AgentRoute.Home = v }},
	{"db", true, "the Pane's KATA_DB path", func(p *LaunchPlan, v string) { p.AgentRoute.DB = v }},
	{"schema", true, "the Pane's KATA_POSTGRES_SCHEMA", func(p *LaunchPlan, v string) { p.AgentRoute.Schema = v }},
	{"dsn-file", true, "the private DSN file the Pane wrote", func(p *LaunchPlan, v string) { p.AgentRoute.DSNFile = v }},
	{"env-file", true, "the private credential env file the Pane wrote", func(p *LaunchPlan, v string) { p.AgentRoute.EnvFile = v }},
	{"daemon", true, "a daemon catalog name", func(p *LaunchPlan, v string) { p.AgentRoute.Daemon = v }},
	{"server", true, "a kata server URL", func(p *LaunchPlan, v string) { p.AgentRoute.Server = v }},
	{"token-file", true, "the private token file the Pane wrote", func(p *LaunchPlan, v string) { p.AgentRoute.TokenFile = v }},
	{"auth-file", true, "the Pane's KATA_AUTH_TOKEN_FILE path", func(p *LaunchPlan, v string) { p.AgentRoute.AuthFile = v }},
	{"allow-insecure", false, "reach a server over plain http", func(p *LaunchPlan, _ string) { p.AgentRoute.AllowInsecure = true }},
	{"trust-private-network", false, "trust a server on a private network", func(p *LaunchPlan, _ string) { p.AgentRoute.TrustPrivate = true }},
}

var agentFlags = slices.Concat([]tableFlag{
	{"as", true, "an actor", func(p *LaunchPlan, v string) { p.fallbackAuthor = v }},
	{"ref", true, "a qualified Issue ref", func(p *LaunchPlan, v string) { p.AgentRef = v }},
	{"forward-comments", false, "tell the agent of a person's comment on its Issue", func(p *LaunchPlan, _ string) { p.AgentForwardComments = true }},
	{"message-from", true, "an actor whose messages the agent acts on, besides its own (repeatable)", func(p *LaunchPlan, v string) { p.AgentMessageFrom = append(p.AgentMessageFrom, v) }},
	{"address-prefix", true, "an Agent address up to its block id", func(p *LaunchPlan, v string) { p.AgentAddressPrefix = v }},
}, routeFlags)

func flagNames(table []tableFlag) []string {
	names := make([]string, len(table))
	for i, f := range table {
		names[i] = f.name
	}
	return names
}

func agentFlagNames() []string { return flagNames(agentFlags) }

// parseTableFlag reads the flag of table at args[*i] ("--name value", "--name=value" or a bare switch), moving *i past a separate
// value and recording it in plan; matched is false when the argument is no flag of table. command and usage head its refusals.
func parseTableFlag(table []tableFlag, command, usage string, args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	arg := args[*i]
	if !strings.HasPrefix(arg, "--") {
		return false, nil
	}
	name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
	fi := slices.IndexFunc(table, func(f tableFlag) bool { return f.name == name })
	if fi < 0 {
		return false, nil
	}
	flag := table[fi]
	if !flag.takesValue {
		if hasValue {
			return true, fmt.Errorf("%s: --%s takes no value (got %q); %s", command, name, value, usage)
		}
		flag.set(plan, "")
		return true, nil
	}
	value, err = parseFlagValue(args, i, "--"+name, flag.what, false)
	if err != nil {
		return true, fmt.Errorf("%s: %w; %s", command, err, usage)
	}
	flag.set(plan, value)
	return true, nil
}

// planAgent parses `tern-kata agent [flags] -- <command> [args...]` (args[0] is "agent"). The flags come before the lone --, and
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
		matched, err := parseTableFlag(agentFlags, "agent", agentUsage, args[:end], &i, &plan)
		if err != nil {
			return LaunchPlan{}, err
		}
		if !matched {
			return LaunchPlan{}, fmt.Errorf("agent: unknown flag %s", arg)
		}
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
