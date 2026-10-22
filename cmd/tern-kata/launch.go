package main

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// gitUserNameKey is the lookup key under which main supplies `git config user.name`; the launch module never execs.
const gitUserNameKey = "git config user.name"

// LaunchPlan is what main does after parsing the command line: run the Pane, or hand off to `kata tui`.
type LaunchPlan struct {
	// IssueRef is the Issue reference (<project>/<short_id>) the Pane opens first; empty for none.
	IssueRef string
	Actor    string
	// Version is non-empty when --version was given: main prints it and exits.
	Version string
	// Help is true when --help or -h was given: main prints usage and exits.
	Help bool
	// FallbackArgs are the arguments for `kata tui` when Tern is unsupported.
	FallbackArgs []string
	// Subcommand is "attach", "detach", "recur", "run" or "worker" when it was the first argument; main runs it without Tern.
	Subcommand string
	// TargetRef is the Issue as attach, detach and recur take it (what `kata show` takes: a short id, <project>#<short id> or a ULID),
	// unlike IssueRef; Operands are the files to attach, or the Attachment name to detach.
	TargetRef string
	Operands  []string
	// RecurFlags are recur's schedule flags by name without dashes (model.RecurFlagNames), as typed; Stop is --stop.
	RecurFlags map[string]string
	Stop       bool
	// Takeover is `worker --takeover`: claim the served Projects' leases even from another live worker.
	Takeover bool
	// InitFlags are `worker init`'s --executor, --actor, --command, --model and --thinking by name without dashes, as typed; unset ones are absent.
	InitFlags map[string]string
	// fallbackAuthor is the --as actor exported to `kata tui`; empty leaves the environment untouched.
	fallbackAuthor string
}

// version is the release build's version, set with -ldflags "-X main.version=...".
var version string

// pseudoVersion matches the VCS-stamped versions Go gives untagged builds, with optional +dirty/+incompatible.
var pseudoVersion = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}(\+.*)?$`)

// resolveVersion picks the linker value, else the main module version from build info unless empty, "(devel)" or a pseudo-version, else "dev".
func resolveVersion(linker string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if linker != "" {
		return linker
	}
	if info, ok := buildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" && !pseudoVersion.MatchString(info.Main.Version) {
		return info.Main.Version
	}
	return "dev"
}

// buildVersion is this binary's version.
func buildVersion() string { return resolveVersion(version, debug.ReadBuildInfo) }

// PlanLaunch parses args (without the program name). --version anywhere wins over everything else, then --help or -h. Unknown arguments pass through to `kata tui`.
// The actor is the first non-empty of --as, KATA_AUTHOR, USER, the gitUserNameKey lookup, then "anonymous".
func PlanLaunch(args []string, lookup func(string) (string, bool), version string) (LaunchPlan, error) {
	if slices.Contains(args, "--version") {
		return LaunchPlan{Version: version}, nil
	}
	if slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		return LaunchPlan{Help: true}, nil
	}
	if len(args) > 0 && (args[0] == "attach" || args[0] == "detach" || args[0] == "recur" || args[0] == "run" || args[0] == "worker") {
		plan, err := planSubcommand(args, lookup)
		if err != nil {
			err = subcommandError{err}
		}
		return plan, err
	}
	var plan LaunchPlan
	plan.FallbackArgs = make([]string, 0, len(args))
	issueSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if actor, matched, err := parseAs(args, &i); err != nil {
			return LaunchPlan{}, err
		} else if matched {
			plan.fallbackAuthor = actor
		} else if arg == "--issue" {
			i++
			if i == len(args) || strings.HasPrefix(args[i], "--") {
				return LaunchPlan{}, fmt.Errorf("--issue requires <project>/<short_id>")
			}
			issueSet, plan.IssueRef = true, args[i]
		} else if value, ok := strings.CutPrefix(arg, "--issue="); ok {
			issueSet, plan.IssueRef = true, value
		} else {
			plan.FallbackArgs = append(plan.FallbackArgs, arg)
		}
	}
	if issueSet {
		project, shortID, err := model.ParseIssueRef(plan.IssueRef)
		if err != nil {
			return LaunchPlan{}, err
		}
		plan.FallbackArgs = append(plan.FallbackArgs, "--project", project, project+"#"+shortID)
	}
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	return plan, nil
}

// parseAs reads --as <actor> or --as=<actor> at args[*i], moving *i past a separate value; matched is false for any other argument.
func parseAs(args []string, i *int) (actor string, matched bool, err error) {
	if value, ok := strings.CutPrefix(args[*i], "--as="); ok {
		return value, true, nil
	}
	if args[*i] != "--as" {
		return "", false, nil
	}
	*i++
	if *i == len(args) || strings.HasPrefix(args[*i], "--") {
		return "", true, fmt.Errorf("--as requires an actor")
	}
	return args[*i], true, nil
}

// resolveActor is flag, else the first set of KATA_AUTHOR, USER and the gitUserNameKey lookup, else "anonymous".
func resolveActor(flag string, lookup func(string) (string, bool)) string {
	actor := flag
	for _, key := range []string{"KATA_AUTHOR", "USER", gitUserNameKey} {
		if actor != "" {
			break
		}
		if value, ok := lookup(key); ok {
			actor = value
		}
	}
	if actor == "" {
		actor = "anonymous"
	}
	return actor
}

// planSubcommand parses `attach <ref> <file>...`, `detach <ref> <name>` and `recur <ref> [flags]` (args[0] is the subcommand). --as may stand anywhere;
// a lone -- ends the flags, so a file may be named like one. --version, --help and -h are caught anywhere first, so a file
// cannot be named exactly that.
func planSubcommand(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: args[0]}
	var operands []string
	flagsEnded := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if flagsEnded || !strings.HasPrefix(arg, "--") {
			operands = append(operands, arg)
			continue
		}
		if arg == "--" {
			flagsEnded = true
			continue
		}
		if plan.Subcommand == "worker" && arg == "--takeover" {
			plan.Takeover = true
			continue
		}
		if plan.Subcommand == "worker" {
			if matched, err := parseInitFlag(args, &i, &plan); err != nil {
				return LaunchPlan{}, err
			} else if matched {
				continue
			}
		}
		if plan.Subcommand == "recur" {
			if matched, err := parseRecurFlag(args, &i, &plan); err != nil {
				return LaunchPlan{}, err
			} else if matched {
				continue
			}
		}
		actor, matched, err := parseAs(args, &i)
		if err != nil {
			return LaunchPlan{}, err
		}
		if !matched {
			return LaunchPlan{}, fmt.Errorf("%s: unknown flag %s", plan.Subcommand, arg)
		}
		plan.fallbackAuthor = actor
	}
	switch {
	case plan.Subcommand == "attach" && len(operands) < 2:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata attach <ref> <file>...")
	case plan.Subcommand == "detach" && len(operands) != 2:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata detach <ref> <name>")
	case plan.Subcommand == "recur" && len(operands) != 1:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata recur <ref> [--frequency daily|weekly|monthly|yearly ... | --rule <rrule> | --stop]")
	case plan.Stop && len(plan.RecurFlags) > 0:
		return LaunchPlan{}, fmt.Errorf("recur: --stop takes no schedule flags")
	case plan.Subcommand == "run" && len(operands) != 1:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata run <job-uid>")
	case plan.Subcommand == "worker" && len(operands) == 2 && (operands[0] == "resume" || operands[0] == "abandon") && !plan.Takeover:
		// `worker resume <run-uid>` and `worker abandon <run-uid>`; validated below like the other forms.
	case plan.Subcommand == "worker" && (len(operands) > 1 || len(operands) == 1 && !slices.Contains([]string{"status", "doctor", "init", "add"}, operands[0])):
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata worker [--takeover] | tern-kata worker init | tern-kata worker add | tern-kata worker status | tern-kata worker doctor | tern-kata worker resume <run-uid> | tern-kata worker abandon <run-uid>")
	case len(plan.InitFlags) > 0 && !slices.Equal(operands, []string{"init"}):
		return LaunchPlan{}, fmt.Errorf("worker: --executor, --actor, --command, --model and --thinking apply to `tern-kata worker init` only")
	case plan.Subcommand == "worker" && len(operands) >= 1 && plan.Takeover:
		return LaunchPlan{}, fmt.Errorf("worker: --takeover applies to `tern-kata worker` only")
	}
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	if plan.Subcommand == "worker" {
		plan.Operands = operands
		return plan, nil
	}
	plan.TargetRef, plan.Operands = operands[0], operands[1:]
	return plan, nil
}

// initFlagNames are the flags of `worker init` that set a value of the written file.
var initFlagNames = []string{"executor", "actor", "command", "model", "thinking"}

// parseInitFlag reads one of initFlagNames as --name value or --name=value at args[*i], moving *i past a separate value;
// matched is false for any other argument. The value must not be empty.
func parseInitFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	name, value, hasValue := strings.Cut(strings.TrimPrefix(args[*i], "--"), "=")
	if !slices.Contains(initFlagNames, name) {
		return false, nil
	}
	if !hasValue {
		if *i+1 == len(args) {
			return true, fmt.Errorf("--%s requires a value", name)
		}
		*i++
		value = args[*i]
	}
	if value == "" {
		return true, fmt.Errorf("--%s requires a value", name)
	}
	if _, given := plan.InitFlags[name]; given {
		return true, fmt.Errorf("--%s given twice", name)
	}
	if plan.InitFlags == nil {
		plan.InitFlags = map[string]string{}
	}
	plan.InitFlags[name] = value
	return true, nil
}

// parseRecurFlag reads --stop, or one of recur's schedule flags as --name value or --name=value, at args[*i], moving *i past a
// separate value; matched is false for any other argument. A value may be empty (--on "") and is then refused by the schedule model.
func parseRecurFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	name, value, hasValue := strings.Cut(strings.TrimPrefix(args[*i], "--"), "=")
	if name == "stop" && !hasValue {
		plan.Stop = true
		return true, nil
	}
	if !slices.Contains(model.RecurFlagNames, name) {
		return false, nil
	}
	if !hasValue {
		if *i+1 == len(args) {
			return true, fmt.Errorf("--%s requires a value", name)
		}
		*i++
		value = args[*i]
	}
	if _, given := plan.RecurFlags[name]; given {
		return true, fmt.Errorf("--%s given twice", name)
	}
	if plan.RecurFlags == nil {
		plan.RecurFlags = map[string]string{}
	}
	plan.RecurFlags[name] = value
	return true, nil
}

// usage is the --help text.
const usage = `tern-kata: browse and edit kata Issues in a native Tern Pane.

Usage:
  tern-kata [--issue <project>/<short_id>] [--as <actor>] [kata tui args...]
  tern-kata attach <ref> <file>... [--as <actor>]
  tern-kata detach <ref> <name> [--as <actor>]
  tern-kata recur <ref> [--frequency daily|weekly|monthly|yearly [--every N] [--on mon,thu]
                   [--day 15|last|"2nd tue"] [--month 1-12] [--until DATE | --count N]
                   | --rule <rrule>] [--start DATE] [--zone <iana>] [--stop] [--as <actor>]
  tern-kata run <job-uid> [--as <actor>]
  tern-kata worker [--takeover]
  tern-kata worker init [--executor <name>] [--actor <actor>] [--command <cmd>]
                        [--model <model>] [--thinking <level>]
  tern-kata worker add
  tern-kata worker status
  tern-kata worker doctor
  tern-kata worker resume <run-uid> [--as <actor>]
  tern-kata worker abandon <run-uid> [--as <actor>]
  tern-kata --version
  tern-kata --help

Flags:
  --issue <project>/<short_id>  open this Issue first
  --as <actor>                  author for edits (default: $KATA_AUTHOR, $USER,
                                git config user.name, then "anonymous")
  --version                     print the version and exit
  -h, --help                    print this help and exit

Subcommands (only as the first argument; they never open a Pane or need Tern):
  attach <ref> <file>...        copy each file into the Project's Attachment store
                                (.kata-attachments in the primary checkout) and point
                                an attachment.<name> meta key of the Issue at it;
                                prints each stored absolute path, stops at the first failure
  detach <ref> <name>           clear that key and delete the copy if it is on this machine
  recur <ref>                   print the Issue's schedule in words and the next Occurrence's
                                date, or say it doesn't recur
  recur <ref> --frequency ...   make the Issue recur, or change its schedule when it already
                                recurs (a schedule changed meanwhile is printed, not overwritten);
                                --rule <rrule> replaces the frequency flags; the start date
                                defaults to the Issue's scheduled date or today, the zone to
                                this machine's IANA zone or UTC
  recur <ref> --stop            stop the Recurrence; existing Occurrences stay
  run <job-uid>                 ask this machine's worker to run an adapter prompt job now,
                                prints the new run's uid once the worker has saved it
  worker                        run the worker in the foreground: it runs the adapter jobs of the
                                Projects listed in worker.toml (tern-kata's config folder) through
                                omp and records each run in kata; --takeover claims the Projects'
                                leases from another machine's worker
  worker init                   write worker.toml for the Project bound to this directory: it serves
                                that Project from this checkout through the omp default profile;
                                refuses when the file exists or the directory is not bound;
                                --executor, --actor, --command, --model and --thinking set those
                                values in it (default: a name from this host, the --as actor
                                left commented, command omp, no model or thinking)
  worker add                    add the Project bound to this directory (and its checkout) to an
                                existing worker.toml, leaving every other line as it is
  worker status                 say what the running worker serves, runs and refuses
  worker doctor                 check worker.toml, the checkouts, agent commands, secrets, the kata
                                daemon and each Project's lease, and say how to fix what is missing
  worker resume <run-uid>       continue a held run: the worker starts a new headless omp attempt on
                                the run's saved session (--resume) with a prompt that warns the
                                previous turn may have partly run; the run goes back to running
  worker abandon <run-uid>      give up a held run: kata records it cancelled ("abandoned by <actor>")
                                and the run's slot and overlap gate are freed. A running run is stopped
                                first (SIGTERM, SIGKILL after 10 s); a run this machine lost the state of
                                is recorded unknown ("local state lost"), a quarantined one acknowledged
<ref> is a short id in this workspace's Project, <project>#<short id>, or a ULID.
The Issue's Project must be this workspace's Binding.

Any other argument is passed to ` + "`kata tui`" + ` when Tern is unavailable.

Inside Tern, it opens a Pane. Outside Tern, it runs ` + "`kata tui`" + ` instead.
In a directory bound to a kata Project (a .kata.toml in it or a parent), it opens
that Project's Issues; elsewhere it opens Inbox.

Environment: KATA_AUTHOR, USER (actor), EDITOR (external Draft editing), TZ (time zone).

Docs: https://github.com/Dangerboi/tern-kata#readme
`
