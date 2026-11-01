package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const agentHelperEnv = "TERN_KATA_AGENT_HELPER"

// TestAgentHelper is the child process of runAgent: it runs tern-kata's main with the arguments after the -- of its own.
func TestAgentHelper(t *testing.T) {
	if os.Getenv(agentHelperEnv) == "" {
		t.Skip("child of the agent tests")
	}
	os.Args = append([]string{"tern-kata"}, os.Args[slices.Index(os.Args, "--")+1:]...)
	main()
}

// runAgent runs `tern-kata <args>` in a child process and returns its output, exit status and stderr.
func runAgent(t *testing.T, env []string, args ...string) (stdout string, status int, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestAgentHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), append([]string{agentHelperEnv + "=1"}, env...)...)
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		status = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), status, errOut.String()
}

func shArgs(script string, args ...string) []string {
	return append([]string{"agent", "--as", "claude", "--", "sh", "-c", script, "sh"}, args...)
}

func TestAgentSetsTheActorAndPassesArgumentsThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	out, status, _ := runAgent(t, []string{"KATA_AUTHOR=someone-else"}, shArgs(`printf '%s|' "$KATA_AUTHOR" "$@"`, "-x", "--as", "a b", "--", "")...)
	if status != 0 || out != "claude|-x|--as|a b|--||" {
		t.Errorf("status %d, output %q", status, out)
	}
}

func TestAgentPassesTheExitStatusThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	if _, status, _ := runAgent(t, nil, shArgs("exit 7")...); status != 7 {
		t.Errorf("status %d, want 7", status)
	}
}

func TestAgentFailsOnACommandItCannotFind(t *testing.T) {
	_, status, stderr := runAgent(t, nil, "agent", "--as", "claude", "--", "tern-kata-no-such-command")
	if status != 1 || !strings.Contains(stderr, "tern-kata-no-such-command") {
		t.Errorf("unfindable command: status %d, stderr %q", status, stderr)
	}
}

func TestPlanAgent(t *testing.T) {
	for name, tc := range map[string]struct {
		args    []string
		wantErr string
		actor   string
		command []string
	}{
		"ok":                 {[]string{"agent", "--as", "bot", "--", "omp", "--model", "x"}, "", "bot", []string{"omp", "--model", "x"}},
		"ok with ref":        {[]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--", "omp"}, "", "bot", []string{"omp"}},
		"ok with equals":     {[]string{"agent", "--as=bot", "--ref=demo#ab12", "--", "omp"}, "", "bot", []string{"omp"}},
		"missing ref value":  {[]string{"agent", "--as", "bot", "--ref", "--", "omp"}, "--ref requires", "", nil},
		"empty ref":          {[]string{"agent", "--as", "bot", "--ref=", "--", "omp"}, "--ref must be non-empty", "", nil},
		"ok with equals as":  {[]string{"agent", "--as=bot", "--", "omp"}, "", "bot", []string{"omp"}},
		"command takes --as": {[]string{"agent", "--as", "bot", "--", "omp", "--as", "other"}, "", "bot", []string{"omp", "--as", "other"}},
		"without --as":       {[]string{"agent", "--ref", "demo#ab12", "--", "omp"}, "", "", []string{"omp"}},
		"empty actor":        {[]string{"agent", "--as=", "--", "omp"}, "non-empty", "", nil},
		"missing actor":      {[]string{"agent", "--as", "--", "omp"}, "--as requires an actor", "", nil},
		"missing --":         {[]string{"agent", "--as", "bot", "omp"}, "must follow --", "", nil},
		"missing command":    {[]string{"agent", "--as", "bot", "--"}, "no command", "", nil},
		"stray operand":      {[]string{"agent", "--as", "bot", "omp", "--", "omp"}, "unexpected argument omp", "", nil},
		"unknown flag":       {[]string{"agent", "--as", "bot", "--json", "--", "omp"}, "unknown flag --json", "", nil},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := PlanLaunch(tc.args, func(string) (string, bool) { return "", false }, "v")
			if tc.wantErr != "" {
				var usage usageError
				if !errors.As(err, &usage) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want usage error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Subcommand != "agent" || plan.Actor != tc.actor || !slices.Equal(plan.Operands, tc.command) {
				t.Errorf("plan = %+v", plan)
			}
		})
	}
}

func TestPlanAgentCarriesTheRef(t *testing.T) {
	plan, err := PlanLaunch([]string{"agent", "--as", "bot", "--ref", "demo#ab12", "--", "omp"}, func(string) (string, bool) { return "", false }, "v")
	if err != nil || plan.AgentRef != "demo#ab12" {
		t.Errorf("AgentRef = %q, err %v", plan.AgentRef, err)
	}
}

func TestAgentEnvironmentHasTheIssueRefAndNoInbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	args := []string{"agent", "--as", "claude", "--ref", "demo#ab12", "--", "sh", "-c", `printf '%s|%s|%s|%s' "$KATA_REF" "${KATA_INBOX_USER-unset}" "$KATA_AUTHOR" "$KATA_SESSION_ID"`}
	out, status, _ := runAgent(t, []string{"KATA_REF=other#zz99", "KATA_INBOX_USER=person", "KATA_SESSION_ID=s1"}, args...)
	if status != 0 || out != "demo#ab12|unset|claude|s1" {
		t.Errorf("status %d, output %q", status, out)
	}
	out, _, _ = runAgent(t, []string{"KATA_REF=other#zz99"}, shArgs(`printf '%s' "${KATA_REF-unset}"`)...)
	if out != "unset" {
		t.Errorf("without --ref KATA_REF = %q, want unset", out)
	}
}

func TestAgentWithoutAnActorLeavesTheAuthorAsInherited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	args := []string{"agent", "--ref", "demo#ab12", "--", "sh", "-c", `printf '%s|%s|%s' "$KATA_REF" "${KATA_INBOX_USER-unset}" "$KATA_AUTHOR"`}
	out, status, _ := runAgent(t, []string{"KATA_AUTHOR=person", "KATA_INBOX_USER=person"}, args...)
	if status != 0 || out != "demo#ab12|unset|person" {
		t.Errorf("status %d, output %q", status, out)
	}
}

func TestAgentHelpAfterTheCommandIsTheCommands(t *testing.T) {
	plan, err := PlanLaunch([]string{"agent", "--as", "bot", "--", "omp", "--help"}, func(string) (string, bool) { return "", false }, "v")
	if err != nil || plan.Help || !slices.Equal(plan.Operands, []string{"omp", "--help"}) {
		t.Errorf("plan = %+v, err = %v", plan, err)
	}
}
