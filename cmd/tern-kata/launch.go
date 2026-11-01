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
	// InboxUser is KATA_INBOX_USER, trimmed: the address the Attention Page reads when not blank, else the actor's.
	InboxUser string
	// Version is non-empty when --version was given: main prints it and exits.
	Version string
	// Help is true when --help or -h was given: main prints usage and exits.
	Help bool
	// FallbackArgs are the arguments for `kata tui` when Tern is unsupported.
	FallbackArgs []string
	// TUIArgs are the arguments only `kata tui` takes (FallbackArgs without the --issue translation); inside Tern nothing reads
	// them, so main refuses them there instead of dropping them unseen.
	TUIArgs []string
	// Subcommand is "attach", "detach", "recur", "run", "views", "worker", "update", "agent" or "completion" when it was the first argument; main runs it
	// without Tern.
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
	// Now is `worker stop --now`: end the worker at once instead of letting its runs in flight finish.
	Now bool
	// JSON is `worker status --json` or `views show --json`: the status as the worker's control socket answers it, or the groups of
	// Issues, instead of the text.
	JSON bool
	// ViewFlags are `views add`'s --base, --project, --status, --owner, --label, --relationship and --search by name without dashes, as typed;
	// unset ones are absent. Replace is `views add --replace`.
	ViewFlags map[string]string
	Replace   bool
	// InitFlags are `worker init`'s --executor, --actor, --command, --model and --thinking by name without dashes, as typed; unset ones are absent.
	InitFlags map[string]string
	// Force and Pause are `update --force` and `update --pause`.
	Force, Pause bool
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

// usageError is a command line tern-kata refuses before doing anything: main exits 2, the Unix status for bad usage.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// subcommands are the first arguments that name a command run without Tern.
var subcommands = []string{"attach", "detach", "recur", "run", "views", "worker", "update", "agent", "completion"}

// completionShells are the shells `tern-kata completion` writes a script for.
var completionShells = []string{"bash", "zsh", "fish"}

// PlanLaunch parses args (without the program name). --version before a lone -- wins over everything else, then --help or -h
// before it. Unknown arguments are kept for `kata tui`. Every refusal is a usageError.
// The actor is the first non-empty of --as, KATA_AUTHOR, USER, the gitUserNameKey lookup, then "anonymous".
func PlanLaunch(args []string, lookup func(string) (string, bool), version string) (LaunchPlan, error) {
	plan, err := planLaunch(args, lookup, version)
	if err != nil {
		return LaunchPlan{}, usageError{err}
	}
	return plan, nil
}

// scanFlags reports whether flag appears in flags at a flag position, not as the value of a flag. Inside a subcommand it runs the
// subcommand's own flag parsers to find those values (so `worker init --command -h` is judged by --command), and stops at the
// first flag the subcommand does not know (--issue there), so the unknown-flag error wins over a later -h. At the top level it
// skips the value --as or --issue takes (a separate value not starting with --), so `--as -h` is judged by --as.
func scanFlags(flags []string, flag string) bool {
	subcommand := ""
	if len(flags) > 0 && slices.Contains(subcommands, flags[0]) {
		subcommand = flags[0]
	}
	var scratch LaunchPlan // the parsers record into it; only the position they stop at matters
	for i := 0; i < len(flags); i++ {
		arg := flags[i]
		if arg == flag {
			return true
		}
		if subcommand == "" {
			if (arg == "--as" || arg == "--issue") && i+1 < len(flags) && !strings.HasPrefix(flags[i+1], "--") {
				i++
			}
			continue
		}
		if i == 0 || !strings.HasPrefix(arg, "--") || arg == "--help" || arg == "--version" {
			continue
		}
		if subcommand == "update" && (arg == "--force" || arg == "--pause") {
			continue
		}
		if subcommand == "worker" {
			if arg == "--takeover" || arg == "--now" || arg == "--json" {
				continue
			}
			if matched, _ := parseInitFlag(flags, &i, &scratch); matched {
				continue
			}
		}
		if subcommand == "recur" {
			if matched, _ := parseRecurFlag(flags, &i, &scratch); matched {
				continue
			}
		}
		if subcommand == "views" {
			if arg == "--json" || arg == "--replace" {
				continue
			}
			if matched, _ := parseViewFlag(flags, &i, &scratch); matched {
				continue
			}
		}
		if subcommand != "completion" && subcommand != "update" {
			start := i
			if _, matched, err := parseAs(flags, &i); matched {
				if err != nil && i == start+1 && i < len(flags) && strings.HasPrefix(flags[i], "--") {
					i-- // parseAs refused a value starting with --: that word is judged on its own
				}
				continue
			}
		}
		return false // unknown flag: planSubcommand refuses it
	}
	return false
}

func planLaunch(args []string, lookup func(string) (string, bool), version string) (LaunchPlan, error) {
	flags := args
	if end := slices.Index(args, "--"); end >= 0 {
		flags = args[:end] // after a lone --, --help and --version are names, such as a file to attach
	}
	if scanFlags(flags, "--version") {
		return LaunchPlan{Version: version}, nil
	}
	if scanFlags(flags, "--help") || scanFlags(flags, "-h") {
		return LaunchPlan{Help: true}, nil
	}
	if len(args) > 0 && slices.Contains(subcommands, args[0]) {
		return planSubcommand(args, lookup)
	}
	var plan LaunchPlan
	plan.FallbackArgs = make([]string, 0, len(args))
	issueSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" { // flags end here: the rest is kata tui's, or a name
			plan.TUIArgs = append(plan.TUIArgs, args[i:]...)
			break
		}
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
			plan.TUIArgs = append(plan.TUIArgs, arg)
		}
	}
	plan.FallbackArgs = append(plan.FallbackArgs, plan.TUIArgs...)
	if issueSet {
		project, shortID, err := model.ParseIssueRef(plan.IssueRef)
		if err != nil {
			return LaunchPlan{}, err
		}
		// kata tui takes one positional: the ref, so a name after -- would be a second.
		dash := slices.Index(plan.FallbackArgs, "--")
		if dash >= 0 && dash < len(plan.FallbackArgs)-1 {
			return LaunchPlan{}, fmt.Errorf("--issue cannot be combined with another argument for kata tui")
		}
		// A bare word is a positional unless the flag before it may take it as a value (kata tui's flags are unknown here).
		for j, arg := range plan.TUIArgs {
			if arg == "--" {
				break
			}
			if !strings.HasPrefix(arg, "-") && (j == 0 || !strings.HasPrefix(plan.TUIArgs[j-1], "-")) {
				return LaunchPlan{}, fmt.Errorf("--issue cannot be combined with another argument for kata tui")
			}
		}
		translated := []string{"--project", project, project + "#" + shortID}
		if dash < 0 {
			dash = len(plan.FallbackArgs)
		}
		plan.FallbackArgs = slices.Insert(plan.FallbackArgs, dash, translated...)
	}
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	if inbox, ok := lookup("KATA_INBOX_USER"); ok {
		plan.InboxUser = strings.TrimSpace(inbox)
	}
	return plan, nil
}

// parseAs reads --as <actor> or --as=<actor> at args[*i], moving *i past a separate value; matched is false for any other argument.
// An explicitly empty actor is refused.
func parseAs(args []string, i *int) (actor string, matched bool, err error) {
	value, ok := strings.CutPrefix(args[*i], "--as=")
	if !ok {
		if args[*i] != "--as" {
			return "", false, nil
		}
		*i++
		if *i == len(args) || strings.HasPrefix(args[*i], "--") {
			return "", true, fmt.Errorf("--as requires an actor")
		}
		value = args[*i]
	}
	if value == "" {
		return "", true, fmt.Errorf("--as must be non-empty")
	}
	return value, true, nil
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

// planSubcommand parses the subcommands (args[0] is the subcommand). --as may stand anywhere; a lone -- ends the flags, so a
// file may be named like one, even --help.
func planSubcommand(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	if args[0] == "agent" {
		return planAgent(args)
	}
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
		if plan.Subcommand == "update" && arg == "--force" {
			plan.Force = true
			continue
		}
		if plan.Subcommand == "update" && arg == "--pause" {
			plan.Pause = true
			continue
		}
		if plan.Subcommand == "worker" && arg == "--takeover" {
			plan.Takeover = true
			continue
		}
		if plan.Subcommand == "worker" && arg == "--now" {
			plan.Now = true
			continue
		}
		if plan.Subcommand == "worker" && arg == "--json" {
			plan.JSON = true
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
		if plan.Subcommand == "views" {
			if arg == "--json" {
				plan.JSON = true
				continue
			}
			if arg == "--replace" {
				plan.Replace = true
				continue
			}
			if matched, err := parseViewFlag(args, &i, &plan); err != nil {
				return LaunchPlan{}, err
			} else if matched {
				continue
			}
		}
		if plan.Subcommand == "completion" || plan.Subcommand == "update" {
			return LaunchPlan{}, fmt.Errorf("%s: unknown flag %s", plan.Subcommand, arg) // neither writes as an actor
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
	case plan.Subcommand == "worker" && !settlesHeldRun(operands, plan.Takeover) && (len(operands) > 1 || len(operands) == 1 && (!slices.Contains(workerForms, operands[0]) || slices.Contains(heldRunForms, operands[0]))):
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata worker [--takeover] | tern-kata worker init | tern-kata worker add | tern-kata worker edit | tern-kata worker upgrade | tern-kata worker status [--json] | tern-kata worker stop [--now] | tern-kata worker supervised | tern-kata worker doctor | tern-kata worker resume <run-uid> | tern-kata worker abandon <run-uid>")
	case plan.Subcommand == "worker" && plan.Now && !slices.Equal(operands, []string{"stop"}):
		return LaunchPlan{}, fmt.Errorf("worker: --now applies to `tern-kata worker stop` only")
	case len(plan.InitFlags) > 0 && !slices.Equal(operands, []string{"init"}):
		return LaunchPlan{}, fmt.Errorf("worker: --executor, --actor, --command, --model and --thinking apply to `tern-kata worker init` only")
	case plan.Subcommand == "worker" && plan.JSON && !slices.Equal(operands, []string{"status"}):
		return LaunchPlan{}, fmt.Errorf("worker: --json applies to `tern-kata worker status` only")
	case plan.Subcommand == "worker" && len(operands) >= 1 && plan.Takeover:
		return LaunchPlan{}, fmt.Errorf("worker: --takeover applies to `tern-kata worker` only")
	case plan.Subcommand == "views" && !validViewsForm(operands):
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata views | tern-kata views show <name> [--json] | tern-kata views add <name> [--base <collection>|project:<name>] [--project <name>] [--status open|closed|all] [--owner <actor>|@me] [--label <label>] [--relationship <kind>] [--search <text>] [--replace] | tern-kata views rm <name>")
	case plan.Subcommand == "views" && plan.JSON && !(len(operands) > 0 && operands[0] == "show"):
		return LaunchPlan{}, fmt.Errorf("views: --json applies to `tern-kata views show` only")
	case plan.Subcommand == "views" && (plan.Replace || len(plan.ViewFlags) > 0) && !(len(operands) > 0 && operands[0] == "add"):
		return LaunchPlan{}, fmt.Errorf("views: --base, --project, --status, --owner, --label, --relationship, --search and --replace apply to `tern-kata views add` only")
	case plan.Subcommand == "update" && len(operands) > 0:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata update [--force] [--pause]")
	case plan.Subcommand == "completion" && (len(operands) != 1 || !slices.Contains(completionShells, operands[0])):
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata completion bash|zsh|fish")
	}
	plan.Actor = resolveActor(plan.fallbackAuthor, lookup)
	if plan.Subcommand == "worker" || plan.Subcommand == "completion" || plan.Subcommand == "views" || plan.Subcommand == "update" {
		plan.Operands = operands
		return plan, nil
	}
	plan.TargetRef, plan.Operands = operands[0], operands[1:]
	return plan, nil
}

// validViewsForm reports whether operands are one of the forms of `tern-kata views`: none, or show, add or rm with one name.
func validViewsForm(operands []string) bool {
	return len(operands) == 0 || len(operands) == 2 && slices.Contains(viewsForms, operands[0])
}

// viewFlagNames are the flags of `views add` that set a field of the saved view.
var viewFlagNames = []string{"base", "project", "status", "owner", "label", "relationship", "search"}

// parseViewFlag reads one of viewFlagNames as --name value or --name=value at args[*i], moving *i past a separate value;
// matched is false for any other argument. A value may be empty and is then judged by the view's checks.
func parseViewFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	return parseValueFlag(args, i, viewFlagNames, &plan.ViewFlags, false)
}

// parseValueFlag reads one of names as --name value or --name=value at args[*i], moving *i past a separate value, and records
// it in *given; matched is false for any other argument. A flag may be given once. An empty value is refused when nonEmpty.
func parseValueFlag(args []string, i *int, names []string, given *map[string]string, nonEmpty bool) (matched bool, err error) {
	name, value, hasValue := strings.Cut(strings.TrimPrefix(args[*i], "--"), "=")
	if !slices.Contains(names, name) {
		return false, nil
	}
	if !hasValue {
		if *i+1 == len(args) {
			return true, fmt.Errorf("--%s requires a value", name)
		}
		*i++
		value = args[*i]
	}
	if nonEmpty && value == "" {
		return true, fmt.Errorf("--%s requires a value", name)
	}
	if _, twice := (*given)[name]; twice {
		return true, fmt.Errorf("--%s given twice", name)
	}
	if *given == nil {
		*given = map[string]string{}
	}
	(*given)[name] = value
	return true, nil
}

// heldRunForms are the worker operands that take a run uid as a second operand; alone they are no command.
var heldRunForms = []string{"resume", "abandon"}

// settlesHeldRun reports whether operands are `resume <run-uid>` or `abandon <run-uid>`, a form that takes no --takeover.
func settlesHeldRun(operands []string, takeover bool) bool {
	return len(operands) == 2 && slices.Contains(heldRunForms, operands[0]) && !takeover
}

// initFlagNames are the flags of `worker init` that set a value of the written file.
var initFlagNames = []string{"executor", "actor", "command", "model", "thinking"}

// parseInitFlag reads one of initFlagNames as --name value or --name=value at args[*i], moving *i past a separate value;
// matched is false for any other argument. The value must not be empty.
func parseInitFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	return parseValueFlag(args, i, initFlagNames, &plan.InitFlags, true)
}

// parseRecurFlag reads --stop, or one of recur's schedule flags as --name value or --name=value, at args[*i], moving *i past a
// separate value; matched is false for any other argument. A value may be empty (--on "") and is then refused by the schedule model.
func parseRecurFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	if args[*i] == "--stop" {
		plan.Stop = true
		return true, nil
	}
	return parseValueFlag(args, i, model.RecurFlagNames, &plan.RecurFlags, false)
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
  tern-kata views [--as <actor>]
  tern-kata views show <name> [--json] [--as <actor>]
  tern-kata views add <name> [--base <collection>|project:<name>] [--project <name>]
                   [--status open|closed|all] [--owner <actor>|@me] [--label <label>]
                   [--relationship <kind>] [--search <text>] [--replace] [--as <actor>]
  tern-kata views rm <name>
  tern-kata worker [--takeover]
  tern-kata worker init [--executor <name>] [--actor <actor>] [--command <cmd>]
                        [--model <model>] [--thinking <level>]
  tern-kata worker add
  tern-kata worker edit
  tern-kata worker upgrade
  tern-kata worker status [--json]
  tern-kata worker stop [--now]
  tern-kata worker supervised
  tern-kata worker doctor
  tern-kata worker resume <run-uid> [--as <actor>]
  tern-kata worker abandon <run-uid> [--as <actor>]
  tern-kata update [--force] [--pause]
  tern-kata agent --as <actor> -- <command> [args...]
  tern-kata completion bash|zsh|fish
  tern-kata --version
  tern-kata --help

Flags:
  --issue <project>/<short_id>  open this Issue first
  --as <actor>                  author for edits; an empty value is refused (default:
                                $KATA_AUTHOR, $USER, git config user.name, then "anonymous")
  --version                     print the version and exit
  -h, --help                    print this help and exit
  --                            end the flags: a file after it may be named like one, even --help

Subcommands (only as the first argument; they never open a Pane or need Tern):
  attach <ref> <file>...        copy each file into the Project's Attachment store
                                (.kata-attachments in the primary checkout) and point
                                a tern-kata.attachment.<name> meta key of the Issue at it;
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
  views                         list the fixed Collections (inbox, today, upcoming, deadlines,
                                all-open, logbook), then the saved views with their base and the
                                filters they set; a view whose base Project is gone or archived says
                                so, and an unreadable views.toml is reported after the fixed Collections
  views show <name>             print the Issues of a fixed Collection (by key or title) or a saved
                                view, ignoring case, as the Pane groups them: "Overdue", "Today", one
                                heading per Project, and so on. One line per Issue: <project>#<id>,
                                status, priority, the dates of a dated Collection, title. @me in a
                                view's owner is the --as actor. Needs no Binding. An unknown name
                                exits 2 and lists the names
  views show <name> --json      print a JSON array with one object per group:
                                  {"id", "title", "issues": [{"ref": "<project>#<id>", "uid", "title",
                                  "status", "priority", "owner", "labels": [...], "scheduled_on",
                                  "deadline_on", "closed_on"}]}
                                priority, owner and the three dates are null when unset, labels is
                                [] when there are none; the dates are YYYY-MM-DD in this machine's zone
  views add <name> ...          save a view in views.toml (this machine only): --base is a Collection
                                key or title or project:<name> (default all-open); --project, --status
                                (open, closed or all), --owner (an actor or @me), --label,
                                --relationship (parent, child, blocks, blocked_by or related) and
                                --search are the Filter card's fields. A Project is looked up by name
                                in the daemon and saved by its uid. Refuses a name that is taken,
                                unless --replace saves over it, and a views.toml changed meanwhile
  views rm <name>               delete a saved view without asking; a fixed Collection or an
                                unknown name is refused
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
  worker edit                   edit worker.toml in $VISUAL or $EDITOR: the result must load before it
                                replaces the file, else the editor reopens with the problem as a comment
                                at the top; an unchanged or empty result cancels. Says whether a worker
                                runs and that ` + "`tern-kata worker stop`" + ` makes it reload
  worker upgrade                give the jobs that exist today what they had before a Project had to list
                                its secrets and allow command steps: add secrets = [...] (the secrets its
                                jobs use) and commands = true (a workflow has a command step) to each
                                [[projects]] table that lacks the key, leaving every other line and any
                                key you wrote as it is. Says what it added and what it could not, and
                                that a running worker reloads with ` + "`tern-kata worker stop`" + `
  worker status [--json]        say what the running worker serves, runs and refuses; --json prints
                                the status as the worker's control socket answers it
  worker stop [--now]           stop the running worker: it admits nothing new, lets its runs in flight
                                finish and report, then exits 0; --now ends it at once and the runs it
                                interrupts are held interrupted and resumable. Says whether the plugin
                                starts a fresh worker. Config changes take effect by restarting it
  worker supervised             exit 0 and print worker.toml's text when the plugin supervises the worker
                                (worker.toml exists and worker.manual does not), else exit 1 with
                                nothing on stdout: the plugin's host half asks it
  worker doctor                 check worker.toml, the checkouts, agent commands, secrets, the kata
                                daemon and each Project's lease, and say how to fix what is missing
  worker resume <run-uid>       continue a held run: the worker starts a new headless omp attempt on
                                the run's saved session (--resume) with a prompt that warns the
                                previous turn may have partly run; the run goes back to running
  worker abandon <run-uid>      give up a held run: kata records it cancelled ("abandoned by <actor>")
                                and the run's slot and overlap gate are freed. A running run is stopped
                                first (SIGTERM, SIGKILL after 10 s); a run this machine lost the state of
                                is recorded unknown ("local state lost"), a quarantined one acknowledged
  update [--force] [--pause]    bring this machine's tern-kata to the latest release: reads
                                version.txt, downloads this OS's archive and checks its SHA-256
                                against checksums.txt, then swaps the running binary (on Windows
                                the old file is renamed aside, so Panes and the worker may keep
                                running). Says "up to date" and changes nothing when this is the
                                latest release, and never downgrades. Only a release build updates
                                itself: a go install or source build prints what to run instead.
                                TERN_KATA_BASE_URL points it at another dist/ folder. --force is for
                                the checks that come with later releases; --pause waits for Enter
                                after a success, so a Pane shows the result before it closes.
                                Exit 0 when the binary is at the latest version, else 1
  agent --as <actor> -- <command> [args...]
                                run the command with KATA_AUTHOR set to the actor (replacing
                                any value), its arguments passed on as typed and its exit
                                status as ours; both --as and -- are required
  completion <shell>            print the completion script for bash, zsh or fish; load it with
                                source <(tern-kata completion bash) (or zsh), or
                                tern-kata completion fish | source
<ref> is a short id in this workspace's Project, <project>#<short id>, or a ULID.
The Issue's Project must be this workspace's Binding (views is the exception: it spans Projects).

Any other argument is passed to ` + "`kata tui`" + ` when Tern is unavailable; inside Tern it is refused.

Exit status: 0 on success, 1 when a subcommand fails, 2 for a command line it refuses, 3 when the Pane
fails, and 128 plus the signal's number when a signal stops a subcommand (130 for ctrl+c).

Inside Tern, it opens a Pane. Outside Tern, it runs ` + "`kata tui`" + ` instead.
In a directory bound to a kata Project (a .kata.toml in it or a parent), it opens
that Project's Issues; elsewhere it opens Inbox.

Environment: KATA_AUTHOR, USER (actor), KATA_INBOX_USER (the Inbox address the Attention Page reads, else the actor), EDITOR (external Draft editing), TZ (time zone).

Docs: https://github.com/Dangerboi/tern-kata#readme
`
