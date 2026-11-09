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
		{name: "docbank setup is two positionals, a usage error", args: []string{"docbank", "setup"}, err: "unexpected arguments: setup"},
		{name: "docbank supervise on names every extra word", args: []string{"docbank", "supervise", "on"}, err: "unexpected arguments: supervise on"},
		{name: "one positional ref falls back to kata tui", args: []string{"proj#ab12"}, fallbackArgs: []string{"proj#ab12"}, tuiArgs: []string{"proj#ab12"}},
		{name: "help after a double dash is kata tui's", args: []string{"--", "--help"}, fallbackArgs: []string{"--", "--help"}, tuiArgs: []string{"--", "--help"}},
		{name: "flags after a double dash are kata tui's", args: []string{"--", "--issue", "proj/abcd"}, fallbackArgs: []string{"--", "--issue", "proj/abcd"}, tuiArgs: []string{"--", "--issue", "proj/abcd"}},
		{name: "issue with a bare double dash keeps the flags before it", args: []string{"--issue=p/1", "--"}, issueRef: "p/1", fallbackArgs: []string{"--project", "p", "p#1", "--"}, tuiArgs: []string{"--"}},
		{name: "issue with a name after a double dash is refused", args: []string{"--issue=p/1", "--", "name"}, err: "--issue cannot be combined with another argument for kata tui"},
		{name: "issue with flags after a double dash is refused", args: []string{"--issue=p/1", "--", "--as", "x"}, err: "--issue cannot be combined with another argument for kata tui"},
		{name: "issue with a bare name before it is refused", args: []string{"foo", "--issue=p/1"}, err: "--issue cannot be combined with another argument for kata tui"},
		{name: "issue with a bare name before a double dash is refused", args: []string{"--issue=p/1", "foo", "--"}, err: "--issue cannot be combined with another argument for kata tui"},
		{name: "as takes a dash h value instead of help", args: []string{"--as", "-h"}, issueRef: "", fallbackArgs: []string{}, tuiArgs: nil},
		{name: "issue takes a dash h value instead of help", args: []string{"--issue", "-h"}, err: `Issue reference must be <project>/<short_id>: "-h"`},
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
		{name: "migrate", args: []string{"migrate-attachments"}, sub: "migrate-attachments", actor: "anonymous"},
		{name: "as before and after", args: []string{"migrate-attachments", "--as", "bob", "--as=amy"}, sub: "migrate-attachments", actor: "amy"},
		{name: "actor falls back like the Pane", args: []string{"migrate-attachments"}, env: map[string]string{"KATA_AUTHOR": "env-actor", "USER": "u"}, sub: "migrate-attachments", actor: "env-actor"},
		{name: "status as JSON", args: []string{"worker", "status", "--json"}, sub: "worker", operands: []string{"status"}, actor: "anonymous"},
		{name: "json elsewhere", args: []string{"worker", "doctor", "--json"}, err: "worker: --json applies to `tern-kata worker status` only"},
		{name: "views lists", args: []string{"views"}, sub: "views", actor: "anonymous"},
		{name: "views show as JSON", args: []string{"views", "show", "Work bugs", "--json"}, sub: "views", operands: []string{"show", "Work bugs"}, actor: "anonymous"},
		{name: "views add with flags anywhere", args: []string{"views", "add", "--search=a b", "x", "--replace"}, sub: "views", operands: []string{"add", "x"}, actor: "anonymous"},
		{name: "views show takes --as", args: []string{"views", "show", "x", "--as", "amy"}, sub: "views", operands: []string{"show", "x"}, actor: "amy"},
		{name: "views list ignores --as", args: []string{"views", "--as", "amy"}, err: "views: --as applies to `tern-kata views show` only"},
		{name: "views add ignores --as", args: []string{"views", "add", "x", "--as", "amy"}, err: "views: --as applies to `tern-kata views show` only"},
		{name: "views rm ignores --as", args: []string{"views", "rm", "x", "--as", "amy"}, err: "views: --as applies to `tern-kata views show` only"},
		{name: "worker init takes --as", args: []string{"worker", "init", "--as", "amy"}, sub: "worker", operands: []string{"init"}, actor: "amy"},
		{name: "worker doctor takes --as", args: []string{"worker", "doctor", "--as", "amy"}, sub: "worker", operands: []string{"doctor"}, actor: "amy"},
		{name: "foreground worker takes --as", args: []string{"worker", "--as", "amy"}, sub: "worker", actor: "amy"},
		{name: "worker upgrade ignores --as", args: []string{"worker", "upgrade", "--as", "amy"}, err: "worker: --as applies to `tern-kata worker` (foreground), init, doctor, resume and abandon only"},
		{name: "worker stop ignores --as", args: []string{"worker", "stop", "--as", "amy"}, err: "worker: --as applies to `tern-kata worker` (foreground), init, doctor, resume and abandon only"},
		{name: "worker edit ignores --as", args: []string{"worker", "edit", "--as", "amy"}, err: "worker: --as applies to `tern-kata worker` (foreground), init, doctor, resume and abandon only"},
		{name: "worker status ignores --as", args: []string{"worker", "status", "--as", "amy"}, err: "worker: --as applies to `tern-kata worker` (foreground), init, doctor, resume and abandon only"},
		{name: "worker add ignores --as", args: []string{"worker", "add", "--as", "amy"}, err: "worker: --as applies to `tern-kata worker` (foreground), init, doctor, resume and abandon only"},
		{name: "views rm", args: []string{"views", "rm", "x"}, sub: "views", operands: []string{"rm", "x"}, actor: "anonymous"},
		{name: "views show needs a name", args: []string{"views", "show"}, err: "usage: tern-kata views | tern-kata views show <name> [--json] | tern-kata views add <name> [--base <collection>|project:<name>] [--project <name>] [--status open|closed|all] [--owner <actor>|@me] [--label <label>] [--relationship <kind>] [--search <text>] [--replace] | tern-kata views rm <name>"},
		{name: "views with an unknown form", args: []string{"views", "edit", "x"}, err: "usage: tern-kata views | tern-kata views show <name> [--json] | tern-kata views add <name> [--base <collection>|project:<name>] [--project <name>] [--status open|closed|all] [--owner <actor>|@me] [--label <label>] [--relationship <kind>] [--search <text>] [--replace] | tern-kata views rm <name>"},
		{name: "views json elsewhere", args: []string{"views", "--json"}, err: "views: --json applies to `tern-kata views show` only"},
		{name: "views add flags elsewhere", args: []string{"views", "show", "x", "--replace"}, err: "views: --base, --project, --status, --owner, --label, --relationship, --search and --replace apply to `tern-kata views add` only"},
		{name: "views flag given twice", args: []string{"views", "add", "x", "--label", "a", "--label=b"}, err: "--label given twice"},
		{name: "views flag without a value", args: []string{"views", "add", "x", "--label"}, err: "--label requires a value"},
		{name: "completion", args: []string{"completion", "fish"}, sub: "completion", operands: []string{"fish"}, actor: "anonymous"},
		{name: "completion for an unknown shell", args: []string{"completion", "tcsh"}, err: "usage: tern-kata completion bash|zsh|fish"},
		{name: "completion takes no flags", args: []string{"completion", "bash", "--as", "bob"}, err: "completion: unknown flag --as"},
		{name: "migrate takes no operands", args: []string{"migrate-attachments", "ab12"}, err: "usage: tern-kata migrate-attachments [--as <actor>]"},
		{name: "as without an actor", args: []string{"migrate-attachments", "--as"}, err: "--as requires an actor"},
		{name: "unknown flag", args: []string{"migrate-attachments", "--issue", "x"}, err: "migrate-attachments: unknown flag --issue"},
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
		{"migrate-attachments"},
		{"recur", "ab12"},
		{"views", "show", "x"},
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

func TestPlanLaunchHelpIsJudgedByTheSubcommandParsers(t *testing.T) {
	for _, args := range [][]string{{"worker", "init", "--command", "-h"}, {"recur", "ab12", "--rule", "-h"}, {"views", "add", "x", "--search", "-h"}} {
		plan, err := PlanLaunch(args, lookupFrom(nil), "v")
		if err != nil || plan.Help {
			t.Errorf("PlanLaunch(%q) = %+v, %v; want the word taken as the flag's value", args, plan, err)
		}
	}
	for _, args := range [][]string{{"worker", "init", "--command", "--help"}, {"recur", "ab12", "--rule", "--help"}, {"views", "add", "x", "--search", "--help"}} {
		if plan, err := PlanLaunch(args, lookupFrom(nil), "v"); err != nil || !plan.Help {
			t.Errorf("PlanLaunch(%q) = %+v, %v; want help, as for --as --help", args, plan, err)
		}
	}
	_, err := PlanLaunch([]string{"migrate-attachments", "--issue", "-h"}, lookupFrom(nil), "v")
	if err == nil || !strings.Contains(err.Error(), "migrate-attachments: unknown flag --issue") {
		t.Errorf("err = %v, want the unknown-flag error before help", err)
	}
}

func TestSubcommandsAreOnlyTakenOverAsTheFirstArgument(t *testing.T) {
	plan, err := PlanLaunch([]string{"--as", "ann", "migrate-attachments"}, lookupFrom(nil), "v")
	if err != nil || plan.Subcommand != "" || !slices.Equal(plan.FallbackArgs, []string{"migrate-attachments"}) {
		t.Fatalf("got %+v, %v, want migrate-attachments passed through to kata tui", plan, err)
	}
	plan, err = PlanLaunch([]string{"migrate-attachments", "--help"}, lookupFrom(nil), "v")
	if err != nil || !plan.Help {
		t.Fatalf("got %+v, %v, want --help to win", plan, err)
	}
	if !strings.Contains(usage, "tern-kata migrate-attachments") {
		t.Fatal("--help does not document migrate-attachments")
	}
}

func TestSubcommandFlagsRefuseAFlagLikeWordAsTheirValue(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		err  string
	}{
		"worker init":      {[]string{"worker", "init", "--command", "--x"}, "--command requires a value"},
		"views add":        {[]string{"views", "add", "v", "--search", "--x"}, "--search requires a value"},
		"recur":            {[]string{"recur", "ab12", "--rule", "--stop"}, "--rule requires a value"},
		"agent route flag": {[]string{"agent", "--home", "--x", "--", "omp"}, "--home requires"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := planSubcommand(tc.args, lookupFrom(nil))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("error = %v, want one containing %q", err, tc.err)
			}
		})
	}
	plan, err := planSubcommand([]string{"views", "add", "v", "--search=--x"}, lookupFrom(nil))
	if err != nil || plan.ViewFlags["search"] != "--x" {
		t.Errorf("--search=--x: plan %+v, err %v; want the value passed", plan.ViewFlags, err)
	}
}

func TestPlanLaunchRunTakesNoAs(t *testing.T) {
	for _, args := range [][]string{{"run", "01M5J9AAAAAAAAAAAAAAAAAAAA", "--as", "ann"}, {"run", "--as=ann", "01M5J9AAAAAAAAAAAAAAAAAAAA"}} {
		if _, err := PlanLaunch(args, lookupFrom(nil), "v"); err == nil || !strings.HasPrefix(err.Error(), "run: unknown flag --as") {
			t.Errorf("%v: error = %v, want run: unknown flag --as", args, err)
		}
	}
}

func TestPlanAgentRefusesRoutingFlagsWithoutAnAddressPrefix(t *testing.T) {
	for _, flags := range [][]string{{"--message-from", "eve"}, {"--forward-comments"}, {"--coordinator", "dana/lead"}, {"--coordinating"}} {
		args := append(append([]string{"agent"}, flags...), "--", "omp")
		_, err := PlanLaunch(args, lookupFrom(nil), "v")
		if err == nil || !strings.Contains(err.Error(), flags[0]) || !strings.Contains(err.Error(), "--address-prefix") {
			t.Errorf("%v: error = %v, want a refusal naming %s and --address-prefix", args, err, flags[0])
		}
		if err != nil && exitStatus(err) != 2 {
			t.Errorf("%v: exits %d, want 2", args, exitStatus(err))
		}
		args = append([]string{"agent", "--address-prefix", "dana/omp"}, args[1:]...)
		if _, err := PlanLaunch(args, lookupFrom(nil), "v"); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestMessageReRefValueDoesNotHideTheNextArgumentFromTheHelpScan(t *testing.T) {
	plan, err := PlanLaunch([]string{"message", "g7bg", "--re", "c:x", "--help"}, lookupFrom(nil), "v")
	if err != nil || !plan.Help {
		t.Errorf("--help after --re's value = %+v, %v; want help", plan, err)
	}
	plan, err = PlanLaunch([]string{"message", "g7bg", "--re", "c:x", "-m", "--help"}, lookupFrom(nil), "v")
	if err != nil || plan.Help {
		t.Errorf("--help as the -m text after --re = %+v, %v; want the text, not help", plan, err)
	}
}

func TestAgentHelpAndVersionNeedNoCommandAfterTheDoubleDash(t *testing.T) {
	for _, args := range [][]string{{"agent", "--ref", "x", "--help"}, {"agent", "--ref", "x", "-h"}, {"agent", "--help"}} {
		if plan, err := PlanLaunch(args, lookupFrom(nil), "v"); err != nil || !plan.Help {
			t.Errorf("PlanLaunch(%q) = %+v, %v; want help", args, plan, err)
		}
	}
	if plan, err := PlanLaunch([]string{"agent", "--ref", "x", "--version"}, lookupFrom(nil), "1.2.3"); err != nil || plan.Version != "1.2.3" {
		t.Errorf("agent --version = %+v, %v; want the version", plan, err)
	}
}
