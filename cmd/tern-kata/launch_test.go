package main

import (
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func TestPlanLaunch(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          map[string]string
		issueRef     string
		fallbackArgs []string
		tuiArgs      []string
		err          string
	}{
		{name: "no arguments", fallbackArgs: []string{}},
		{name: "issue separate value", args: []string{"--issue", "proj/ab12"}, issueRef: "proj/ab12", fallbackArgs: []string{"--project", "proj", "proj#ab12"}},
		{name: "issue equals value", args: []string{"--issue=proj/ab12"}, issueRef: "proj/ab12", fallbackArgs: []string{"--project", "proj", "proj#ab12"}},
		{name: "issue missing value", args: []string{"--issue"}, err: "--issue requires <project>/<short_id>"},
		{name: "issue followed by as", args: []string{"--issue", "--as", "bob"}, err: "--issue requires <project>/<short_id>"},
		{name: "as followed by issue", args: []string{"--as", "--issue", "x"}, err: "--as requires an actor"},
		{name: "issue followed by as equals", args: []string{"--issue", "--as=bob"}, err: "--issue requires <project>/<short_id>"},
		{name: "as missing value", args: []string{"--as"}, err: "--as requires an actor"},
		{name: "as equals empty", args: []string{"--as="}, err: "--as must be non-empty"},
		{name: "as empty separate value", args: []string{"--as", ""}, err: "--as must be non-empty"},
		{name: "as empty after issue", args: []string{"--issue=p/1", "--as="}, err: "--as must be non-empty"},
		{name: "malformed issue ref", args: []string{"--issue", "noslash"}, err: `Issue reference must be <project>/<short_id>: "noslash"`},
		{name: "empty issue equals value", args: []string{"--issue="}, err: `Issue reference must be <project>/<short_id>: ""`},
		{name: "unknown arguments pass through", args: []string{"--as", "ann", "--foo", "bar", "--issue=p/1", "-x"}, issueRef: "p/1", fallbackArgs: []string{"--foo", "bar", "-x", "--project", "p", "p#1"}, tuiArgs: []string{"--foo", "bar", "-x"}},
		{name: "help after a double dash is kata tui's", args: []string{"--", "--help"}, fallbackArgs: []string{"--", "--help"}, tuiArgs: []string{"--", "--help"}},
		{name: "flags after a double dash are kata tui's", args: []string{"--", "--issue", "proj/abcd"}, fallbackArgs: []string{"--", "--issue", "proj/abcd"}, tuiArgs: []string{"--", "--issue", "proj/abcd"}},
		{name: "flags before a double dash still parse", args: []string{"--issue=p/1", "--", "--as", "x"}, issueRef: "p/1", fallbackArgs: []string{"--", "--as", "x", "--project", "p", "p#1"}, tuiArgs: []string{"--", "--as", "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env), "v")
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("error = %v, want %q", err, tt.err)
				}
				if exitStatus(err) != 2 {
					t.Fatalf("refused command line exits %d, want 2", exitStatus(err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.IssueRef != tt.issueRef {
				t.Errorf("IssueRef = %q, want %q", plan.IssueRef, tt.issueRef)
			}
			if !reflect.DeepEqual(plan.FallbackArgs, tt.fallbackArgs) {
				t.Errorf("FallbackArgs = %q, want %q", plan.FallbackArgs, tt.fallbackArgs)
			}
			if !slices.Equal(plan.TUIArgs, tt.tuiArgs) {
				t.Errorf("TUIArgs = %q, want %q (what Tern refuses)", plan.TUIArgs, tt.tuiArgs)
			}
		})
	}
}

func TestPlanLaunchActorPrecedence(t *testing.T) {
	all := map[string]string{"KATA_AUTHOR": "author", "USER": "user", gitUserNameKey: "git"}
	without := func(keys ...string) map[string]string {
		env := map[string]string{}
		for k, v := range all {
			if !slices.Contains(keys, k) {
				env[k] = v
			}
		}
		return env
	}
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"as flag wins", []string{"--as", "flag"}, all, "flag"},
		{"as equals form wins", []string{"--as=flag"}, all, "flag"},
		{"KATA_AUTHOR next", nil, all, "author"},
		{"USER next", nil, without("KATA_AUTHOR"), "user"},
		{"git user.name next", nil, without("KATA_AUTHOR", "USER"), "git"},
		{"anonymous last", nil, nil, "anonymous"},
		{"empty environment values are unset", nil, map[string]string{"KATA_AUTHOR": "", "USER": "user"}, "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env), "v")
			if err != nil {
				t.Fatal(err)
			}
			if plan.Actor != tt.want {
				t.Errorf("Actor = %q, want %q", plan.Actor, tt.want)
			}
		})
	}
}

func TestPlanLaunchReadsTheInboxUserNextToTheActor(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"unset", nil, map[string]string{"KATA_AUTHOR": "author"}, ""},
		{"set", nil, map[string]string{"KATA_AUTHOR": "author", "KATA_INBOX_USER": "coordinator/pip"}, "coordinator/pip"},
		{"trimmed", nil, map[string]string{"KATA_INBOX_USER": "  pip \t"}, "pip"},
		{"blank is as unset", nil, map[string]string{"KATA_INBOX_USER": "   "}, ""},
		{"independent of --as", []string{"--as", "flag"}, map[string]string{"KATA_INBOX_USER": "pip"}, "pip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env), "v")
			if err != nil {
				t.Fatal(err)
			}
			if plan.InboxUser != tt.want {
				t.Errorf("InboxUser = %q, want %q", plan.InboxUser, tt.want)
			}
		})
	}
}

func TestPlanLaunchFallbackAuthor(t *testing.T) {
	env := map[string]string{"KATA_AUTHOR": "author", "USER": "user"}
	for args, want := range map[string]string{"--as ann": "ann", "--as=ann": "ann", "": ""} {
		plan, err := PlanLaunch(strings.Fields(args), lookupFrom(env), "v")
		if err != nil {
			t.Fatal(err)
		}
		if plan.fallbackAuthor != want {
			t.Errorf("fallbackAuthor for %q = %q, want %q", args, plan.fallbackAuthor, want)
		}
	}
}

func TestPlanLaunchGitLookupOnlyWhenNeeded(t *testing.T) {
	asked := []string{}
	lookup := func(key string) (string, bool) {
		if key == gitUserNameKey {
			asked = append(asked, key)
		}
		return "", false
	}
	if _, err := PlanLaunch([]string{"--as", "flag"}, lookup, "v"); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Errorf("lookups with --as = %q, want none", asked)
	}
}

func TestPlanLaunchVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--as", "--version"}, {"--issue", "bad", "--version"}, {"--bogus", "--version", "--as"}} {
		plan, err := PlanLaunch(args, lookupFrom(nil), "1.2.3")
		if err != nil || plan.Version != "1.2.3" || plan.FallbackArgs != nil {
			t.Errorf("PlanLaunch(%q) = %+v, %v; want version plan", args, plan, err)
		}
	}
}

func TestPlanLaunchHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--as", "--help"}, {"--issue", "bad", "-h"}, {"--bogus", "--help", "--as"}} {
		plan, err := PlanLaunch(args, lookupFrom(nil), "1.2.3")
		if err != nil || !plan.Help || plan.FallbackArgs != nil {
			t.Errorf("PlanLaunch(%q) = %+v, %v; want help plan", args, plan, err)
		}
	}
	if plan, _ := PlanLaunch([]string{"--help", "--version"}, lookupFrom(nil), "1.2.3"); plan.Help || plan.Version != "1.2.3" {
		t.Errorf("--version must win over --help, got %+v", plan)
	}
}

func TestResolveVersion(t *testing.T) {
	info := func(v string, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, ok }
	}
	tests := []struct {
		name   string
		linker string
		info   func() (*debug.BuildInfo, bool)
		want   string
	}{
		{"linker wins", "1.0.0", info("v2.0.0", true), "1.0.0"},
		{"build info", "", info("v2.0.0", true), "v2.0.0"},
		{"devel", "", info("(devel)", true), "dev"},
		{"empty build info version", "", info("", true), "dev"},
		{"tagged", "", info("v1.2.3", true), "v1.2.3"},
		{"tagged dirty", "", info("v1.2.3+dirty", true), "v1.2.3+dirty"},
		{"pseudo from zero", "", info("v0.0.0-20261012164748-6b737607699e", true), "dev"},
		{"pseudo after prerelease", "", info("v1.2.3-pre.0.20261012164748-6b737607699e", true), "dev"},
		{"pseudo after tag", "", info("v1.2.4-0.20261012164748-6b737607699e+dirty", true), "dev"},
		{"pseudo incompatible", "", info("v2.0.0-20261012164748-6b737607699e+incompatible", true), "dev"},
		{"no build info", "", info("v2.0.0", false), "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.linker, tt.info); got != tt.want {
				t.Errorf("resolveVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanLaunchSubcommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		sub, ref string
		operands []string
		actor    string
		err      string
	}{
		{name: "attach several files", args: []string{"attach", "ab12", "a.png", "b.png"}, sub: "attach", ref: "ab12", operands: []string{"a.png", "b.png"}, actor: "anonymous"},
		{name: "detach", args: []string{"detach", "proj#ab12", "a.png"}, sub: "detach", ref: "proj#ab12", operands: []string{"a.png"}, actor: "anonymous"},
		{name: "as before and after", args: []string{"attach", "--as", "bob", "ab12", "a.png", "--as=amy"}, sub: "attach", ref: "ab12", operands: []string{"a.png"}, actor: "amy"},
		{name: "actor falls back like the Pane", args: []string{"detach", "ab12", "a.png"}, env: map[string]string{"KATA_AUTHOR": "env-actor", "USER": "u"}, sub: "detach", ref: "ab12", operands: []string{"a.png"}, actor: "env-actor"},
		{name: "double dash ends flags", args: []string{"attach", "ab12", "--", "--odd.png"}, sub: "attach", ref: "ab12", operands: []string{"--odd.png"}, actor: "anonymous"},
		{name: "help after a double dash is a file", args: []string{"attach", "ab12", "--", "--help", "--version"}, sub: "attach", ref: "ab12", operands: []string{"--help", "--version"}, actor: "anonymous"},
		{name: "status as JSON", args: []string{"worker", "status", "--json"}, sub: "worker", operands: []string{"status"}, actor: "anonymous"},
		{name: "json elsewhere", args: []string{"worker", "doctor", "--json"}, err: "worker: --json applies to `tern-kata worker status` only"},
		{name: "completion", args: []string{"completion", "fish"}, sub: "completion", operands: []string{"fish"}, actor: "anonymous"},
		{name: "completion for an unknown shell", args: []string{"completion", "tcsh"}, err: "usage: tern-kata completion bash|zsh|fish"},
		{name: "completion takes no flags", args: []string{"completion", "bash", "--as", "bob"}, err: "completion: unknown flag --as"},
		{name: "attach without a file", args: []string{"attach", "ab12"}, err: "usage: tern-kata attach <ref> <file>..."},
		{name: "attach without anything", args: []string{"attach"}, err: "usage: tern-kata attach <ref> <file>..."},
		{name: "detach without a name", args: []string{"detach", "ab12"}, err: "usage: tern-kata detach <ref> <name>"},
		{name: "detach with two names", args: []string{"detach", "ab12", "a", "b"}, err: "usage: tern-kata detach <ref> <name>"},
		{name: "as without an actor", args: []string{"attach", "ab12", "a", "--as"}, err: "--as requires an actor"},
		{name: "unknown flag", args: []string{"attach", "--issue", "x", "ab12", "a"}, err: "attach: unknown flag --issue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env), "v")
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("error = %v, want %q", err, tt.err)
				}
				if exitStatus(err) != 2 {
					t.Fatalf("error %T exits %d, want 2 like every refused command line", err, exitStatus(err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Subcommand != tt.sub || plan.TargetRef != tt.ref || !slices.Equal(plan.Operands, tt.operands) || plan.Actor != tt.actor {
				t.Errorf("plan = %+v, want %s %s %q as %s", plan, tt.sub, tt.ref, tt.operands, tt.actor)
			}
		})
	}
}

func TestPlanLaunchRefusesEmptyAsForEverySubcommand(t *testing.T) {
	commands := [][]string{
		{"attach", "ab12", "a.png"},
		{"detach", "ab12", "a.png"},
		{"recur", "ab12"},
		{"run", "01M5J9AAAAAAAAAAAAAAAAAAAA"},
		{"worker", "resume", "01M5J9AAAAAAAAAAAAAAAAAAAA"},
		{"worker", "abandon", "01M5J9AAAAAAAAAAAAAAAAAAAA"},
	}
	for _, command := range commands {
		for _, as := range [][]string{{"--as="}, {"--as", ""}} {
			t.Run(strings.Join(append(slices.Clone(command), as...), " "), func(t *testing.T) {
				_, err := PlanLaunch(append(slices.Clone(command), as...), lookupFrom(map[string]string{"KATA_AUTHOR": "env"}), "v")
				if err == nil || err.Error() != "--as must be non-empty" {
					t.Fatalf("error = %v, want %q", err, "--as must be non-empty")
				}
				if exitStatus(err) != 2 {
					t.Fatalf("error %T exits %d, want 2 like --as requires an actor", err, exitStatus(err))
				}
			})
		}
	}
}

func TestSubcommandsAreOnlyTakenOverAsTheFirstArgument(t *testing.T) {
	plan, err := PlanLaunch([]string{"--as", "ann", "attach", "x"}, lookupFrom(nil), "v")
	if err != nil || plan.Subcommand != "" || !slices.Equal(plan.FallbackArgs, []string{"attach", "x"}) {
		t.Fatalf("got %+v, %v, want attach passed through to kata tui", plan, err)
	}
	plan, err = PlanLaunch([]string{"detach", "--help"}, lookupFrom(nil), "v")
	if err != nil || !plan.Help {
		t.Fatalf("got %+v, %v, want --help to win", plan, err)
	}
	if !strings.Contains(usage, "tern-kata attach <ref> <file>...") || !strings.Contains(usage, "tern-kata detach <ref> <name>") {
		t.Fatal("--help does not document attach and detach")
	}
}
