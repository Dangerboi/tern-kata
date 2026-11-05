package main

import (
	"fmt"
	"slices"
	"strings"
)

// agentUsage is what `tern-kata agent` refuses a command line with.
const agentUsage = "usage: tern-kata agent [--as <actor>] [--ref <qualified ref>] [--address-prefix <actor>/<name>] [--home <dir>] [--db <file>] [--schema <name>] [--dsn-file <file>] [--env-file <file>] [--daemon <name> [--token-file <file>] [--auth-file <file>] | --server <url> [--token-file <file>] [--auth-file <file>] [--allow-insecure] [--trust-private-network]] -- <command> [args...]"

// agentFlag is one flag of `tern-kata agent`, its route flags included. The parser and both completion scripts read agentFlags,
// so a flag is one row there; agentUsage and the help text are prose and name it by hand.
type agentFlag struct {
	name       string // without its dashes
	takesValue bool
	what       string // the value a flag takes, or what a flag without one does
	set        func(plan *LaunchPlan, value string)
}

var agentFlags = []agentFlag{
	{"as", true, "an actor", func(p *LaunchPlan, v string) { p.fallbackAuthor = v }},
	{"ref", true, "a qualified Issue ref", func(p *LaunchPlan, v string) { p.AgentRef = v }},
	{"address-prefix", true, "an Agent address up to its block id", func(p *LaunchPlan, v string) { p.AgentAddressPrefix = v }},
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

func agentFlagNames() []string {
	names := make([]string, len(agentFlags))
	for i, f := range agentFlags {
		names[i] = f.name
	}
	return names
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
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		fi := slices.IndexFunc(agentFlags, func(f agentFlag) bool { return f.name == name })
		if fi < 0 {
			return LaunchPlan{}, fmt.Errorf("agent: unknown flag %s", arg)
		}
		flag := agentFlags[fi]
		if !flag.takesValue {
			if hasValue {
				return LaunchPlan{}, fmt.Errorf("agent: --%s takes no value (got %q); %s", name, value, agentUsage)
			}
			flag.set(&plan, "")
			continue
		}
		value, err := parseFlagValue(args[:end], &i, "--"+name, flag.what, false)
		if err != nil {
			return LaunchPlan{}, fmt.Errorf("agent: %w; %s", err, agentUsage)
		}
		flag.set(&plan, value)
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
