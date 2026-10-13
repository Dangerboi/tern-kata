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
	var plan LaunchPlan
	plan.FallbackArgs = make([]string, 0, len(args))
	issueSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--as" || arg == "--issue" {
			i++
			if i == len(args) || (arg == "--issue" && (args[i] == "--as" || strings.HasPrefix(args[i], "--as="))) {
				if arg == "--as" {
					return LaunchPlan{}, fmt.Errorf("--as requires an actor")
				}
				return LaunchPlan{}, fmt.Errorf("--issue requires <project>/<short_id>")
			}
			if arg == "--as" {
				plan.fallbackAuthor = args[i]
			} else {
				issueSet, plan.IssueRef = true, args[i]
			}
		} else if value, ok := strings.CutPrefix(arg, "--as="); ok {
			plan.fallbackAuthor = value
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
	plan.Actor = plan.fallbackAuthor
	for _, key := range []string{"KATA_AUTHOR", "USER", gitUserNameKey} {
		if plan.Actor != "" {
			break
		}
		if value, ok := lookup(key); ok {
			plan.Actor = value
		}
	}
	if plan.Actor == "" {
		plan.Actor = "anonymous"
	}
	return plan, nil
}

// usage is the --help text.
const usage = `tern-kata: browse and edit kata Issues in a native Tern Pane.

Usage:
  tern-kata [--issue <project>/<short_id>] [--as <actor>] [kata tui args...]
  tern-kata --version
  tern-kata --help

Flags:
  --issue <project>/<short_id>  open this Issue first
  --as <actor>                  author for edits (default: $KATA_AUTHOR, $USER,
                                git config user.name, then "anonymous")
  --version                     print the version and exit
  -h, --help                    print this help and exit
Any other argument is passed to ` + "`kata tui`" + ` when Tern is unavailable.

Inside Tern, it opens a Pane. Outside Tern, it runs ` + "`kata tui`" + ` instead.
In a directory bound to a kata Project (a .kata.toml in it or a parent), it opens
that Project's Issues; elsewhere it opens Inbox.

Environment: KATA_AUTHOR, USER (actor), EDITOR (external Draft editing), TZ (time zone).

Docs: https://github.com/Dangerboi/tern-kata#readme
`
