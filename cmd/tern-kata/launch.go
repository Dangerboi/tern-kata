package main

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
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
	// Subcommand is "migrate-attachments", "recur", "run", "views", "worker", "update", "docbank", "agent", "agents", "team", "message" or "completion" when it was the first argument; main runs it
	// without Tern.
	Subcommand string
	// TargetRef is the Issue as recur and run take it (what `kata show` takes: a short id, <project>#<short id> or a ULID),
	// unlike IssueRef; Operands are the arguments after it.
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
	// Force and Pause are `update --force` and `update --pause`. Preflight is `update --preflight`, the check one release
	// runs of another; it is not for people, so usage and completion leave it out.
	Force, Pause, Preflight bool
	// MessageText is `message -m`: the text to send. Teammate is the sender's KATA_TEAMMATE, which the slot carries so the receiving
	// agent knows a peer wrote it; empty when a person sends. `agents --issue` and `message`'s address or ref are TargetRef.
	MessageText string
	Teammate    string
	// MessageRe is `message --re`: the handle of the comment the message asks the agent to answer; empty for none.
	MessageRe string
	// AgentRef is `agent --ref`: the qualified Issue ref the agent's KATA_REF is set to; empty leaves the agent without one.
	AgentRef string
	// AgentAddressPrefix is `agent --address-prefix`: the agent's Agent address up to its block id, <actor>/<name>. The wrapper sets
	// KATA_INBOX_USER to the address with the block id from TERN_PANE, and KATA_OMP_LIVE_INBOX=0; empty leaves both out.
	AgentAddressPrefix string
	// AgentForwardComments is `agent --forward-comments`: the wrapper sets TERN_KATA_FORWARD_COMMENTS=1, which turns on the omp
	// extension's delivery of a person's comment on the agent's Issue; it is read only with an address prefix.
	AgentForwardComments bool
	// AgentMessageFrom is every `agent --message-from`: the actors, besides the agent's own, whose messages it acts on. The wrapper sets
	// TERN_KATA_MESSAGE_FROM to them, comma-separated; it is read only with an address prefix.
	AgentMessageFrom []string
	// AgentVerifiedSenders is what the wrapper found on the agent's route at start: the daemon verifies the actor of every write
	// (data.VerifiedAuthKind). It sets TERN_KATA_VERIFIED_SENDERS=1; it is read only with an address prefix.
	AgentVerifiedSenders bool
	// AgentCoordinator is `agent --coordinator`: the Agent address of the coordinator of this child's team, which the wrapper sets as
	// TERN_KATA_COORDINATOR; AgentCoordinating is `agent --coordinating`, which sets TERN_KATA_COORDINATING=1 for the coordinator itself.
	// Both are read only with an address prefix, and an inherited value of either variable is dropped then.
	AgentCoordinator  string
	AgentCoordinating bool
	// AgentRoute is `agent`'s --home, --db, --schema, --dsn-file, --env-file, --daemon, --server, --token-file, --auth-file, --allow-insecure and --trust-private-network: the Pane's daemon, which the wrapper turns into
	// the agent's kata environment; zero leaves it as inherited.
	AgentRoute data.AgentRoute
	// Docbank is `docbank setup`'s own flags. The route flags it takes record into AgentRoute.
	Docbank DocbankSetup
	// docbankErr is the first refusal a `docbank setup` flag setter met (a setter cannot return one); planDocbank reports it.
	docbankErr error
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
var subcommands = []string{"migrate-attachments", "recur", "run", "views", "worker", "update", "docbank", "agent", "agents", "team", "message", "completion"}

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
		if subcommand == "message" && arg == "-m" { // the text after it is no flag, whatever it starts with
			i++
			continue
		}
		if subcommand == "message" && arg == "--re" && i+1 < len(flags) && !strings.HasPrefix(flags[i+1], "--") {
			i += 2
			continue
		}
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
		if subcommand == "update" && (arg == "--force" || arg == "--pause" || arg == "--preflight") {
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
		if subcommand == "agents" && (arg == "--issue" || strings.HasPrefix(arg, "--issue=")) {
			if arg == "--issue" && i+1 < len(flags) && !strings.HasPrefix(flags[i+1], "--") {
				i++
			}
			continue
		}
		if subcommand == "docbank" {
			if matched, _ := parseTableFlag(docbankFlags, "docbank", docbankUsage, flags, &i, &scratch); matched {
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
		if subcommand != "completion" && subcommand != "update" && subcommand != "docbank" && subcommand != "team" {
			if _, matched, _ := parseAs(flags, &i); matched {
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
		flags = args[:end] // after a lone --, --help and --version are refs, not flags
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
	for i := 0; i < len(args); i++ { //nolint:intrange // parseAs and parseFlagValue move i past a flag's value through &i
		arg := args[i]
		if arg == "--" { // flags end here: the rest is kata tui's, or a name
			plan.TUIArgs = append(plan.TUIArgs, args[i:]...)
			break
		}
		if actor, matched, err := parseAs(args, &i); err != nil {
			return LaunchPlan{}, err
		} else if matched {
			plan.fallbackAuthor = actor
		} else if arg == "--issue" || strings.HasPrefix(arg, "--issue=") {
			// An empty value is allowed through so ParseIssueRef names it in its own error.
			value, err := parseFlagValue(args, &i, "--issue", "<project>/<short_id>", true)
			if err != nil {
				return LaunchPlan{}, err
			}
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
	if name, _, _ := strings.Cut(args[*i], "="); name != "--as" {
		return "", false, nil
	}
	actor, err = parseFlagValue(args, i, "--as", "an actor", false)
	return actor, true, err
}

// parseFlagValue reads the value of flag, as `flag value` or `flag=value`, from args[*i], moving *i past a separate value; every
// flag of every subcommand goes through it. A missing value, or a separate one that starts with "--" (--flag=--x passes it), is
// refused as missing, leaving *i on flag so the caller judges that word on its own; what names the value in that refusal. An
// explicitly empty value is refused unless allowEmpty.
func parseFlagValue(args []string, i *int, flag, what string, allowEmpty bool) (string, error) {
	value, ok := strings.CutPrefix(args[*i], flag+"=")
	if !ok {
		if *i+1 == len(args) || strings.HasPrefix(args[*i+1], "--") {
			return "", fmt.Errorf("%s requires %s", flag, what)
		}
		*i++
		value = args[*i]
	}
	if value == "" && !allowEmpty {
		return "", fmt.Errorf("%s must be non-empty", flag)
	}
	return value, nil
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
// ref may be named like one, even --help.
func planSubcommand(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	if args[0] == "agent" {
		return planAgent(args)
	}
	if args[0] == "docbank" {
		return planDocbank(args)
	}
	if args[0] == "agents" {
		return planAgents(args, lookup)
	}
	if args[0] == "team" {
		return planTeam(args)
	}
	if args[0] == "message" {
		return planMessage(args, lookup)
	}
	plan := LaunchPlan{Subcommand: args[0]}
	operands, err := scanOperands(args, func(i *int) error {
		arg := args[*i]
		switch {
		case plan.Subcommand == "update" && arg == "--force":
			plan.Force = true
			return nil
		case plan.Subcommand == "update" && arg == "--pause":
			plan.Pause = true
			return nil
		case plan.Subcommand == "update" && arg == "--preflight":
			plan.Preflight = true
			return nil
		case plan.Subcommand == "worker" && arg == "--takeover":
			plan.Takeover = true
			return nil
		case plan.Subcommand == "worker" && arg == "--now":
			plan.Now = true
			return nil
		case plan.Subcommand == "worker" && arg == "--json":
			plan.JSON = true
			return nil
		}
		if plan.Subcommand == "worker" {
			if matched, err := parseInitFlag(args, i, &plan); err != nil || matched {
				return err
			}
		}
		if plan.Subcommand == "recur" {
			if matched, err := parseRecurFlag(args, i, &plan); err != nil || matched {
				return err
			}
		}
		if plan.Subcommand == "views" {
			if arg == "--json" {
				plan.JSON = true
				return nil
			}
			if arg == "--replace" {
				plan.Replace = true
				return nil
			}
			if matched, err := parseViewFlag(args, i, &plan); err != nil || matched {
				return err
			}
		}
		if plan.Subcommand == "completion" || plan.Subcommand == "update" {
			return fmt.Errorf("%s: unknown flag %s", plan.Subcommand, arg) // neither writes as an actor
		}
		actor, matched, err := parseAs(args, i)
		if err != nil {
			return err
		}
		if !matched {
			return fmt.Errorf("%s: unknown flag %s", plan.Subcommand, arg)
		}
		plan.fallbackAuthor = actor
		return nil
	})
	if err != nil {
		return LaunchPlan{}, err
	}
	switch {
	case plan.Subcommand == "migrate-attachments" && len(operands) != 0:
		return LaunchPlan{}, fmt.Errorf("usage: tern-kata migrate-attachments [--as <actor>]")
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
	if plan.Subcommand == "worker" || plan.Subcommand == "completion" || plan.Subcommand == "views" || plan.Subcommand == "update" || plan.Subcommand == "migrate-attachments" {
		plan.Operands = operands
		return plan, nil
	}
	plan.TargetRef, plan.Operands = operands[0], operands[1:]
	return plan, nil
}

// scanOperands splits args[1:] (args[0] is the subcommand) into operands and flags: a word not starting with "--", or any word
// after a lone --, is an operand; every other word goes to onFlag, which reads it at args[*i] and moves *i past a separate value.
func scanOperands(args []string, onFlag func(i *int) error) ([]string, error) {
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
		if err := onFlag(&i); err != nil {
			return nil, err
		}
	}
	return operands, nil
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
	return parseNamedFlag(args, i, viewFlagNames, &plan.ViewFlags, true)
}

// parseNamedFlag reads one of names as --name value or --name=value at args[*i], moving *i past a separate value, and records
// it in *given; matched is false for any other argument. A flag may be given once. An empty value is refused unless allowEmpty.
func parseNamedFlag(args []string, i *int, names []string, given *map[string]string, allowEmpty bool) (matched bool, err error) {
	name, _, _ := strings.Cut(strings.TrimPrefix(args[*i], "--"), "=")
	if !strings.HasPrefix(args[*i], "--") || !slices.Contains(names, name) {
		return false, nil
	}
	value, err := parseFlagValue(args, i, "--"+name, "a value", allowEmpty)
	if err != nil {
		return true, err
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
	return parseNamedFlag(args, i, initFlagNames, &plan.InitFlags, false)
}

// parseRecurFlag reads --stop, or one of recur's schedule flags as --name value or --name=value, at args[*i], moving *i past a
// separate value; matched is false for any other argument. A value may be empty (--on "") and is then refused by the schedule model.
func parseRecurFlag(args []string, i *int, plan *LaunchPlan) (matched bool, err error) {
	if args[*i] == "--stop" {
		plan.Stop = true
		return true, nil
	}
	return parseNamedFlag(args, i, model.RecurFlagNames, &plan.RecurFlags, true)
}

// usage is the --help text.
const usage = `tern-kata: browse and edit kata Issues in a native Tern Pane.

Usage:
  tern-kata [--issue <project>/<short_id>] [--as <actor>] [kata tui args...]
  tern-kata migrate-attachments [--as <actor>]
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
  tern-kata worker resume <job-uid>
  tern-kata worker abandon <run-uid> [--as <actor>]
  tern-kata update [--force] [--pause]
  tern-kata agents [--issue <ref>]
  tern-kata team <ref>
  tern-kata message <address|ref> -m <text> [--re <comment>] [--as <actor>]
  tern-kata docbank supervised [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]
  tern-kata docbank supervise on|off
  tern-kata docbank setup [--url <url> (--key-file <file>|- | --key-env <NAME>) | --port <port>] [--folder <path>] [--yes | --no-restart] [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]
  tern-kata agent [--as <actor>] [--ref <ref>] [--address-prefix <actor>/<name> [--forward-comments] [--message-from <actor>]... [--coordinator <address> | --coordinating]] [--home <dir>] [--db <file>] [--schema <name>] [--dsn-file <file>] [--env-file <file>] [--daemon <name> [--token-file <file>] [--auth-file <file>] | --server <url> [--token-file <file>] [--auth-file <file>] [--allow-insecure] [--trust-private-network]] -- <command> [args...]
  tern-kata completion bash|zsh|fish
  tern-kata --version
  tern-kata --help

Flags:
  --issue <project>/<short_id>  open this Issue first
  --as <actor>                  author for edits; an empty value is refused (default:
                                $KATA_AUTHOR, $USER, git config user.name, then "anonymous")
  --version                     print the version and exit
  -h, --help                    print this help and exit
  --                            end the flags: a ref after it may be named like one, even --help

Subcommands (only as the first argument; they never open a Pane or need Tern):
  migrate-attachments           one-off: upload the files that the workspace's Project's
                                old keys point to on this machine to
                                kata's Docbank attachments, clear the keys and delete the
                                local copies; files on other machines are listed. Run it
                                from a workspace bound to the Project.
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
                                (worker.toml exists and worker.manual does not) and none runs; exit 4
                                and print one line naming the worker when one already runs; else exit 1
                                with nothing on stdout: the plugin's host half asks it
  worker doctor                 check worker.toml, the checkouts, agent commands, secrets, the kata
                                daemon and each Project's lease, and say how to fix what is missing
  worker resume <run-uid>       continue a held run: the worker starts a new headless omp attempt on
                                the run's saved session (--resume) with a prompt that warns the
                                previous turn may have partly run; the run goes back to running
  worker resume <job-uid>       lift the pause the worker put on a job after repeated failed runs
                                (pause_after_failures in worker.toml); worker status names it
  worker abandon <run-uid>      give up a held run: kata records it cancelled ("abandoned by <actor>")
                                and the run's slot and overlap gate are freed. A running run is stopped
                                first (SIGTERM, SIGKILL after 10 s); a run this machine lost the state of
                                is recorded unknown ("local state lost"), a quarantined one acknowledged
  update [--force] [--pause]    bring this machine's tern-kata to the latest release: reads
                                version.txt, downloads this OS's archive and checks its SHA-256
                                against checksums.txt, then swaps the running binary (on Windows
                                the old file is renamed aside, so Panes and the worker may keep
                                running). Says "up to date" and leaves the binary alone when this
                                is the latest release, and never downgrades; either way it updates
                                this machine's installed tern-kata plugin copy to match, and after a
                                swap gives a supervised Worker an idle stop so it comes back as the
                                new binary. Only a release build updates itself: a go install or
                                source build prints what to run instead. TERN_KATA_BASE_URL points
                                it at another dist/ folder. --force installs even when the new
                                release's pre-flight refuses this machine (a kata daemon too old, a
                                worker.toml it rejects); --pause waits for Enter after a success,
                                so a Pane shows the result before it closes.
                                Exit 0 when the binary is at the latest version, else 1
  agents [--issue <ref>]        list the agents this machine's Panes started that have not
                                exited: Issue, agent name, state (from the Pane's latest relay)
                                and Agent address (- when no message reaches it); --issue cuts
                                the list to one Issue
  team <ref>                    print the team on that parent Issue: its state, limit and
                                coordinator's Agent address, then each child with its ref,
                                state (the agent's own, starting, queued, ran: closed, ran:
                                exited with the Issue open, skipped: other Project, not ready)
                                and Agent address. <ref> takes the forms agents --issue does.
                                No team on the Issue: says so, exit status 1
  message <address|ref> -m <text> [--re <comment>]
                                write a message to the agent at that Agent address, or to every
                                agent that can receive one on that Issue, as the --as actor
                                (else, inside an agent, the actor of its own address; else
                                KATA_AUTHOR) and, inside an agent, with its KATA_TEAMMATE
                                on it: the receiving agent is told it is from a peer, not the
                                person's instruction. --re names the comment (c:<handle>) the message
                                asks the agent to answer: its reply to that comment with
                                kata comment --reply settles the request. An address that holds a message its agent
                                has not cleared is not written over: that agent is named and
                                exit status 1 says so. Unknown address or ref, or no agent that
                                can receive it, is refused
  docbank setup [--url <url> (--key-file <file>|- | --key-env <NAME>) | --port <port>] [--folder <path>] [--yes | --no-restart] [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]
                                make attachments work. Without --url it sets Docbank up on this
                                machine: it gives Docbank a fixed port (--port, default 8080) and an
                                API key (kept in <KATA_HOME>/docbank.key, mode 0600, never printed) in
                                [server] of <DOCBANK_HOME>/config.toml (default ~/.docbank), restarts
                                Docbank with docbank daemon restart and checks it listens there; an
                                existing [server] with api_port and api_key is reused, a port in use
                                by another process is refused before anything is written, and it
                                prints that Docbank reports usage unless DOCBANK_TELEMETRY_ENABLED=0
                                is set in its daemon's environment. With --url it points kata at a
                                Docbank someone else runs: it checks that the --url answers with the
                                API key (read-only). Either way it then adds a
                                [docbank] section (url, the key's source, --folder, default /kata) to the
                                config.toml of the kata daemon the Pane would use (KATA_HOME or ~/.kata,
                                or a local profile chosen with --daemon; --home, --db and --schema
                                work as for agent), and asks before it restarts that daemon, then checks
                                that it has attachments on. --key-file names a file that holds the
                                key, or - reads it from standard input and stores it in <KATA_HOME>/docbank.key
                                (mode 0600); --key-env names a variable the kata daemon's own environment
                                holds. No flag takes the key itself. Plain http is for this machine
                                only. The rest of config.toml stays byte for byte; a [docbank] section
                                that is the same is "already set up", a different one is refused and shown.
                                --yes restarts without asking; --no-restart, or no terminal without
                                --yes, prints the restart command instead (for a --daemon profile it
                                runs and prints kata daemon restart with KATA_HOME set to the
                                profile's home). --key-file - needs a pipe, not a terminal. A remote kata daemon is
                                refused: its config is not on this machine. Never prints the key.
                                The plugin supervises only the default kata daemon's Docbank: a local
                                setup for it leaves the plugin keeping Docbank running (docbank
                                supervise); for a --home or --daemon profile it says how to keep Docbank running
  docbank supervised [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]
                                the plugin host half's question: does the plugin keep a local Docbank
                                running? Exit 0 and the line "on" when kata's [docbank] url is on this
                                machine, Docbank's [server] api_port in <DOCBANK_HOME>/config.toml is
                                that url's port and docbank.manual is not in tern-kata's config folder;
                                otherwise exit 1, nothing on stdout and the reason on stderr
  docbank supervise on|off      off creates docbank.manual in tern-kata's config folder (mode 0600), so
                                the plugin stops starting Docbank (a running one keeps running); on
                                removes it. Says what it did
  agent [--as <actor>] [--ref <ref>] [--address-prefix <actor>/<name> [--forward-comments] [--message-from <actor>]... [--coordinator <address> | --coordinating]] [--home <dir>] [--db <file>] [--schema <name>] [--dsn-file <file>] [--env-file <file>] [--daemon <name> [--token-file <file>] [--auth-file <file>] | --server <url> [--token-file <file>] [--auth-file <file>] [--allow-insecure] [--trust-private-network]] -- <command> [args...]
                                run the command with KATA_AUTHOR set to the actor (replacing
                                any value; without --as it is left as inherited) and KATA_REF
                                to the qualified Issue ref, with no KATA_INBOX_USER unless
                                --address-prefix gives <actor>/<name>: then KATA_INBOX_USER is
                                <actor>/<name>-<block id>, the block id from TERN_PANE, and
                                KATA_TEAMMATE is <name>-<block id> (left unset when kata would
                                refuse it); --forward-comments (with --address-prefix) sets
                                TERN_KATA_FORWARD_COMMENTS=1, so the agent is told of a person's
                                comment on its Issue; each --message-from (with --address-prefix) adds
                                an actor whose messages the agent acts on, joined with commas into
                                TERN_KATA_MESSAGE_FROM, and TERN_KATA_VERIFIED_SENDERS=1 is set when
                                the daemon on the agent's route verifies actors (identity mode);
                                --coordinator <address> (with --address-prefix) sets
                                TERN_KATA_COORDINATOR to the address of the coordinator of this
                                child's team, --coordinating TERN_KATA_COORDINATING=1 for the
                                coordinator itself; its
                                arguments passed on as typed and its exit status as ours;
                                -- is required. --home sets KATA_HOME; --daemon names a catalog entry
                                (its url and allow_insecure become KATA_SERVER and KATA_ALLOW_INSECURE,
                                a local one its home or the current one); --server and --allow-insecure
                                set KATA_SERVER and KATA_ALLOW_INSECURE; --trust-private-network sets
                                KATA_TRUST_PRIVATE_NETWORK; --token-file names a private file whose
                                token becomes KATA_AUTH_TOKEN and which is deleted once read;
                                --auth-file sets KATA_AUTH_TOKEN_FILE; --db and --schema set KATA_DB and
                                KATA_POSTGRES_SCHEMA, and --dsn-file names a private file whose content
                                becomes KATA_DSN and which is deleted once read; --env-file names a
                                private file of NAME=value lines (the Pane's token_env values) set
                                as they are, except names the route owns (KATA_HOME, KATA_SERVER ...),
                                deleted once read and ignored for a remote --daemon entry or a
                                switched profile with its own home. Any of them first drops
                                the inherited KATA_SERVER, KATA_AUTH_TOKEN, KATA_AUTH_TOKEN_FILE,
                                KATA_ALLOW_INSECURE, KATA_TRUST_PRIVATE_NETWORK, KATA_DSN, KATA_DB and
                                KATA_POSTGRES_SCHEMA. The Pane passes them so the agent's kata reaches
                                the Pane's daemon; a token or DSN is never an argument
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
