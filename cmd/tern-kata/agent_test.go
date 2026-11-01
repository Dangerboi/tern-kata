package main

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
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

// agentEnvOutput runs the real wrapper with route flags around a command that prints the kata variables the agent would see.
func agentEnvOutput(t *testing.T, env []string, flags ...string) (string, int, string) {
	t.Helper()
	args := append(append([]string{"agent", "--as", "claude"}, flags...), "--ref", "demo#ab12", "--", "sh", "-c",
		`printf 'HOME=%s SERVER=%s TOKEN=%s FILE=%s INSECURE=%s REF=%s' "${KATA_HOME-unset}" "${KATA_SERVER-unset}" "${KATA_AUTH_TOKEN-unset}" "${KATA_AUTH_TOKEN_FILE-unset}" "${KATA_ALLOW_INSECURE-unset}" "$KATA_REF"`)
	return runAgent(t, env, args...)
}

func TestAgentWrapperRoutesKataToThePanesDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	if err := os.WriteFile(home+"/config.toml", []byte("[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\nallow_insecure = true\n[[daemon]]\nname = \"mine\"\nlocal = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The environment the agent's block inherits (the test child sets its own KATA_HOME, KATA_SERVER and KATA_AUTH_TOKEN): a route
	// changes only what it names and leaves the rest as it was.
	block := []string{"WORK_TOKEN=work-token"}
	base, status, stderr := agentEnvOutput(t, block)
	childHome := regexp.MustCompile(`/tmp/workertest\d+`) // each test child gets its own throwaway KATA_HOME
	norm := func(out string) string { return childHome.ReplaceAllString(out, "/tmp/workertest") }
	m := regexp.MustCompile(`^HOME=(\S*) SERVER=(\S*) TOKEN=(\S*) `).FindStringSubmatch(norm(base))
	if status != 0 || m == nil {
		t.Fatalf("baseline %q: status %d, stderr %q", base, status, stderr)
	}
	baseHome, baseServer, baseToken := m[1], m[2], m[3]
	want := func(home, server, token, file, insecure string) string {
		return "HOME=" + home + " SERVER=" + server + " TOKEN=" + token + " FILE=" + file + " INSECURE=" + insecure + " REF=demo#ab12"
	}
	for name, tc := range map[string]struct {
		flags []string
		want  string
	}{
		"no route leaves the block as it is":  {nil, want(baseHome, baseServer, baseToken, "unset", "unset")},
		"by KATA_HOME":                        {[]string{"--home", home}, want(home, baseServer, baseToken, "unset", "unset")},
		"by KATA_SERVER":                      {[]string{"--server", "https://pane.example", "--token-file", "/run/pane-token", "--allow-insecure"}, want(baseHome, "https://pane.example", "unset", "/run/pane-token", "1")},
		"after a switch to a remote entry":    {[]string{"--home", home, "--daemon", "work"}, want(home, "https://kata.example", "work-token", "unset", "1")},
		"after a switch to the local default": {[]string{"--home", home, "--daemon", "mine"}, want(home, "unset", "unset", "unset", "unset")},
	} {
		t.Run(name, func(t *testing.T) {
			out, status, stderr := agentEnvOutput(t, block, tc.flags...)
			if status != 0 || norm(out) != tc.want {
				t.Errorf("status %d, output %q, stderr %q; want %q", status, out, stderr, tc.want)
			}
		})
	}
	t.Run("a token the block lacks refuses the start", func(t *testing.T) {
		_, status, stderr := agentEnvOutput(t, []string{"WORK_TOKEN="}, "--home", home, "--daemon", "work")
		if status == 0 || !strings.Contains(stderr, "WORK_TOKEN") {
			t.Errorf("status %d, stderr %q", status, stderr)
		}
	})
}

func TestPlanAgentRouteFlags(t *testing.T) {
	lookup := func(string) (string, bool) { return "", false }
	plan, err := PlanLaunch([]string{"agent", "--home", "/h", "--server=https://s", "--token-file", "/t", "--allow-insecure", "--ref", "demo#ab12", "--", "omp"}, lookup, "v")
	if err != nil || plan.AgentRoute.Home != "/h" || plan.AgentRoute.Server != "https://s" || plan.AgentRoute.TokenFile != "/t" || !plan.AgentRoute.AllowInsecure || plan.AgentRef != "demo#ab12" {
		t.Errorf("plan %+v, err %v", plan, err)
	}
	for _, args := range [][]string{
		{"agent", "--daemon", "work", "--server", "https://s", "--", "omp"},
		{"agent", "--daemon", "--", "omp"},
		{"agent", "--home=", "--", "omp"},
	} {
		if _, err := PlanLaunch(args, lookup, "v"); err == nil {
			t.Errorf("%q was accepted", args)
		}
	}
}
