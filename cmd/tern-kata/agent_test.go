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

const (
	agentHelperEnv = "TERN_KATA_AGENT_HELPER"
	agentBlockEnv  = "TERN_KATA_AGENT_BLOCK_ENV"
)

// TestAgentHelper is the child process of runAgent: it runs tern-kata's main with the arguments after the -- of its own.
func TestAgentHelper(t *testing.T) {
	if os.Getenv(agentHelperEnv) == "" {
		t.Skip("child of the agent tests")
	}
	// The test binary's init resets some kata variables, so the block's own environment is applied here, last.
	for _, kv := range strings.Split(os.Getenv(agentBlockEnv), "\n") {
		if name, value, ok := strings.Cut(kv, "="); ok {
			os.Setenv(name, value)
		}
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
		`printf 'HOME=%s SERVER=%s TOKEN=%s FILE=%s INSECURE=%s TRUST=%s DB=%s DSN=%s SCHEMA=%s REF=%s' "${KATA_HOME-unset}" "${KATA_SERVER-unset}" "${KATA_AUTH_TOKEN-unset}" "${KATA_AUTH_TOKEN_FILE-unset}" "${KATA_ALLOW_INSECURE-unset}" "${KATA_TRUST_PRIVATE_NETWORK-unset}" "${KATA_DB-unset}" "${KATA_DSN-unset}" "${KATA_POSTGRES_SCHEMA-unset}" "$KATA_REF"`)
	return runAgent(t, env, args...)
}

func TestAgentWrapperRoutesKataToThePanesDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	if err := os.WriteFile(home+"/config.toml", []byte("[[daemon]]\nname = \"work\"\nurl = \"https://kata.example\"\ntoken_env = \"WORK_TOKEN\"\nallow_insecure = true\n[[daemon]]\nname = \"bare\"\nurl = \"https://bare.example\"\n[[daemon]]\nname = \"mine\"\nlocal = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dsnFile := home + "/pane-dsn"
	paneToken := home + "/pane-token" // the wrapper deletes it once read, so each case writes it again
	// The block's own environment, as Tern's daemon gave it: the default remote, its token (also as a file) and a plaintext opt-in.
	block := []string{agentBlockEnv + "=KATA_HOME=/default\nKATA_SERVER=https://default.example\nKATA_AUTH_TOKEN=default-token\nKATA_AUTH_TOKEN_FILE=/default/token\nKATA_ALLOW_INSECURE=1\nKATA_TRUST_PRIVATE_NETWORK=1\nKATA_DB=/block/main.db\nKATA_DSN=postgres://block@h/db\nKATA_POSTGRES_SCHEMA=block_schema", "WORK_TOKEN=block-work-token"}
	want := func(home, server, token, file, insecure, trust string) string {
		return "HOME=" + home + " SERVER=" + server + " TOKEN=" + token + " FILE=" + file + " INSECURE=" + insecure + " TRUST=" + trust + " DB=unset DSN=unset SCHEMA=unset REF=demo#ab12"
	}
	// wantStorage is want for a route that keeps the Pane's home and so carries the Pane's own storage settings.
	wantStorage := func(home, db, dsn, schema string) string {
		return "HOME=" + home + " SERVER=unset TOKEN=unset FILE=unset INSECURE=unset TRUST=unset DB=" + db + " DSN=" + dsn + " SCHEMA=" + schema + " REF=demo#ab12"
	}
	for name, tc := range map[string]struct {
		flags []string
		want  string
	}{
		"no route leaves the block as it is":           {nil, "HOME=/default SERVER=https://default.example TOKEN=default-token FILE=/default/token INSECURE=1 TRUST=1 DB=/block/main.db DSN=postgres://block@h/db SCHEMA=block_schema REF=demo#ab12"},
		"the Pane's own storage replaces the block's":  {[]string{"--home", home, "--db", "/pane/p.db", "--schema", "pane_s", "--dsn-file", dsnFile}, wantStorage(home, "/pane/p.db", "postgres://pane:pw@h/db", "pane_s")},
		"by KATA_HOME alone":                           {[]string{"--home", home}, want(home, "unset", "unset", "unset", "unset", "unset")},
		"by KATA_SERVER without a token":               {[]string{"--home", home, "--server", "https://pane.example"}, want(home, "https://pane.example", "unset", "unset", "unset", "unset")},
		"by KATA_SERVER with the Pane's token":         {[]string{"--home", home, "--server", "https://pane.example", "--token-file", paneToken, "--allow-insecure"}, want(home, "https://pane.example", "pane-token", "unset", "1", "unset")},
		"by KATA_SERVER on a private network":          {[]string{"--home", home, "--server", "http://100.64.0.5:7777", "--token-file", paneToken, "--trust-private-network"}, want(home, "http://100.64.0.5:7777", "pane-token", "unset", "unset", "1")},
		"the Pane's own KATA_AUTH_TOKEN_FILE":          {[]string{"--home", home, "--server", "https://pane.example", "--auth-file", "/pane/token"}, want(home, "https://pane.example", "unset", "/pane/token", "unset", "unset")},
		"a remote entry, the Pane's token":             {[]string{"--home", home, "--daemon", "work", "--token-file", paneToken}, want(home, "https://kata.example", "pane-token", "unset", "1", "unset")},
		"a remote entry without a token":               {[]string{"--home", home, "--daemon", "bare"}, want(home, "https://bare.example", "unset", "unset", "unset", "unset")},
		"the local default":                            {[]string{"--home", home, "--daemon", "mine"}, want(home, "unset", "unset", "unset", "unset", "unset")},
		"a plain local entry keeps the Pane's storage": {[]string{"--home", home, "--daemon", "mine", "--db", "/pane/p.db"}, wantStorage(home, "/pane/p.db", "unset", "unset")},
		"the local default, the Pane's token":          {[]string{"--home", home, "--daemon", "mine", "--token-file", paneToken}, want(home, "unset", "pane-token", "unset", "unset", "unset")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(paneToken, []byte("pane-token\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dsnFile, []byte("postgres://pane:pw@h/db\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			out, status, stderr := agentEnvOutput(t, block, tc.flags...)
			if status != 0 || out != tc.want {
				t.Errorf("status %d, output %q, stderr %q; want %q", status, out, stderr, tc.want)
			}
		})
	}
	t.Run("the Pane-written token file is gone once read, the Pane's own is not", func(t *testing.T) {
		written, own := home+"/written-token", home+"/own-token"
		for _, f := range []string{written, own} {
			if err := os.WriteFile(f, []byte("tok\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, status, stderr := agentEnvOutput(t, block, "--server", "https://pane.example", "--token-file", written, "--auth-file", own); status != 0 {
			t.Fatalf("status %d, stderr %q", status, stderr)
		}
		if _, err := os.Stat(written); err == nil {
			t.Error("the token file outlived the agent's start")
		}
		if _, err := os.Stat(own); err != nil {
			t.Errorf("the Pane's own token file was removed: %v", err)
		}
	})
	t.Run("a token file kata would refuse stops the start", func(t *testing.T) {
		_, status, stderr := agentEnvOutput(t, block, "--server", "https://pane.example", "--token-file", "relative")
		if status == 0 || !strings.Contains(stderr, "absolute") {
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
		{"agent", "--server", "https://s", "--allow-insecure=false", "--", "omp"},
		{"agent", "--server", "https://s", "--trust-private-network=0", "--", "omp"},
	} {
		if _, err := PlanLaunch(args, lookup, "v"); err == nil {
			t.Errorf("%q was accepted", args)
		}
	}
}

func TestAgentWrapperSetsTheCredentialVariablesUnderTheirOwnNamesAndDeletesTheFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	envFile := dir + "/creds"
	if err := os.WriteFile(envFile, []byte("WORK_TOKEN=work-value\nPANE_AUTH=auth-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, status, stderr := runAgent(t, []string{"KATA_AUTH_TOKEN=block-token"}, "agent", "--home", dir, "--env-file", envFile, "--", "sh", "-c",
		`printf '%s|%s|%s' "$WORK_TOKEN" "$PANE_AUTH" "${KATA_AUTH_TOKEN-unset}"`)
	if status != 0 || out != "work-value|auth-value|unset" {
		t.Errorf("status %d, output %q, stderr %q", status, out, stderr)
	}
	if _, err := os.Stat(envFile); err == nil {
		t.Error("the env file outlived the agent's start")
	}
}
