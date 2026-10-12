package main

import (
	"fmt"
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
	// FallbackArgs are the arguments for `kata tui` when Tern is unsupported.
	FallbackArgs []string
	// fallbackAuthor is the --as actor exported to `kata tui`; empty leaves the environment untouched.
	fallbackAuthor string
}

// FallbackEnv returns environ for `kata tui`, with KATA_AUTHOR replaced by the --as actor when one was given.
func (p LaunchPlan) FallbackEnv(environ []string) []string {
	if p.fallbackAuthor == "" {
		return environ
	}
	env := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, "KATA_AUTHOR=") {
			env = append(env, entry)
		}
	}
	return append(env, "KATA_AUTHOR="+p.fallbackAuthor)
}

// PlanLaunch parses args (without the program name). Unknown arguments pass through to `kata tui`.
// The actor is the first non-empty of --as, KATA_AUTHOR, USER, the gitUserNameKey lookup, then "anonymous".
func PlanLaunch(args []string, lookup func(string) (string, bool)) (LaunchPlan, error) {
	var plan LaunchPlan
	plan.FallbackArgs = make([]string, 0, len(args))
	issueSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--as" || arg == "--issue" {
			i++
			if i == len(args) {
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
