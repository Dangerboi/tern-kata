package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/docbanksetup"
)

// docbankUsage is what `tern-kata docbank` refuses a command line with.
const docbankUsage = "usage: tern-kata docbank setup [--url <url> (--key-file <file>|- | --key-env <NAME>) | --port <port>] [--folder <path>] [--yes | --no-restart] [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]"

// docbankFlag sets one of the flags of `docbank setup` that the plan records by name.
func docbankFlag(name string) func(*LaunchPlan, string) {
	return func(p *LaunchPlan, v string) {
		if p.DocbankFlags == nil {
			p.DocbankFlags = map[string]string{}
		}
		p.DocbankFlags[name] = v
	}
}

// docbankRouteFlags are the route flags that pick which kata daemon's config is written; the others only carry the Pane's credentials
// to an agent, which setup has no use for.
var docbankRouteFlags = slices.DeleteFunc(slices.Clone(routeFlags), func(f tableFlag) bool {
	return !slices.Contains([]string{"home", "db", "schema", "daemon", "server"}, f.name)
})

// docbankFlags is the flag table of `docbank setup`: its parser, usage, help and every completion script read it.
var docbankFlags = slices.Concat([]tableFlag{
	{"url", true, "the address of a Docbank someone else runs; without it Docbank is set up on this machine", docbankFlag("url")},
	{"port", true, "the port Docbank on this machine listens on (default 8080); without --url only", docbankFlag("port")},
	{"key-file", true, "a file holding the Docbank API key, or - for standard input", docbankFlag("key-file")},
	{"key-env", true, "the name of a variable holding the Docbank API key", docbankFlag("key-env")},
	{"folder", true, "the vault folder kata uses (default /kata)", docbankFlag("folder")},
	{"yes", false, "restart the kata daemon without asking", func(p *LaunchPlan, _ string) { p.DocbankYes = true }},
	{"no-restart", false, "leave the kata daemon alone and print the restart command", func(p *LaunchPlan, _ string) { p.DocbankNoRestart = true }},
}, docbankRouteFlags)

func docbankFlagNames() []string { return flagNames(docbankFlags) }

// planDocbank parses `tern-kata docbank setup [flags]` (args[0] is "docbank"). With --url it points kata at a Docbank someone else
// runs (--key-file or --key-env); without it, it sets Docbank up on this machine (--port).
func planDocbank(args []string) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: "docbank"}
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
		matched, err := parseTableFlag(docbankFlags, "docbank", docbankUsage, args, &i, &plan)
		if err != nil {
			return LaunchPlan{}, err
		}
		if !matched {
			return LaunchPlan{}, fmt.Errorf("docbank: unknown flag %s", arg)
		}
	}
	flags := plan.DocbankFlags
	remote := flags["url"] != ""
	switch {
	case !slices.Equal(operands, []string{"setup"}):
		return LaunchPlan{}, fmt.Errorf("docbank: expected `setup`; %s", docbankUsage)
	case plan.DocbankYes && plan.DocbankNoRestart:
		return LaunchPlan{}, fmt.Errorf("docbank setup: --yes and --no-restart exclude each other; %s", docbankUsage)
	case remote && flags["port"] != "":
		return LaunchPlan{}, fmt.Errorf("docbank setup: --port is for Docbank on this machine and excludes --url; %s", docbankUsage)
	case !remote && (flags["key-file"] != "" || flags["key-env"] != ""):
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file and --key-env need --url: setting Docbank up on this machine makes its own key; %s", docbankUsage)
	case remote && flags["key-file"] == "" && flags["key-env"] == "":
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file or --key-env is required with --url; %s", docbankUsage)
	case remote && flags["key-file"] != "" && flags["key-env"] != "":
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file and --key-env exclude each other; %s", docbankUsage)
	}
	if v, ok := flags["port"]; ok {
		if _, err := parsePort(v); err != nil {
			return LaunchPlan{}, fmt.Errorf("docbank setup: %w; %s", err, docbankUsage)
		}
	}
	plan.Operands = operands
	return plan, nil
}

// parsePort reads --port: a whole number from 1 to 65535.
func parsePort(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("--port must be a number from 1 to 65535, got %q", v)
	}
	return n, nil
}

// docbankHome is Docbank's home: DOCBANK_HOME, else ~/.docbank.
func docbankHome() (string, error) {
	home := os.Getenv("DOCBANK_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".docbank")
	}
	return filepath.Abs(home)
}

// newDocbankEnv is the production environment for plan: the kata daemon its route chooses, kata's own discovery, the real programs.
// The route's KATA_HOME, KATA_DB and KATA_POSTGRES_SCHEMA are set in this process, which only runs the one command.
func newDocbankEnv(plan LaunchPlan, stdin io.Reader, out io.Writer) (docbanksetup.Env, error) {
	daemon, err := data.ResolveLocalDaemon(plan.AgentRoute)
	if err != nil {
		return docbanksetup.Env{}, err
	}
	// prefix is the part of the route a person must retype to run the restart command themselves.
	var prefix strings.Builder
	for _, v := range []struct{ name, value string }{{"KATA_HOME", daemon.Base}, {"KATA_DB", daemon.DB}, {"KATA_POSTGRES_SCHEMA", daemon.Schema}} {
		if v.value == "" {
			continue
		}
		if err := os.Setenv(v.name, v.value); err != nil {
			return docbanksetup.Env{}, err
		}
		if v.name != "KATA_HOME" || plan.AgentRoute.Home != "" {
			fmt.Fprintf(&prefix, "%s=%s ", v.name, v.value)
		}
	}
	dbHome, _ := docbankHome() // only the local form needs it, and it says so when it is empty
	locate := data.Locator(data.LocateWithKata)
	if daemon.Name != "" {
		locate = data.LocateNamed(daemon.Name, "")
	}
	env := docbanksetup.Env{
		KataHome:      daemon.Home,
		DocbankHome:   dbHome,
		Daemon:        daemon.Name,
		CommandPrefix: prefix.String(),
		Runner:        docbanksetup.ExecRunner{},
		HTTP:          &http.Client{Timeout: 30 * time.Second},
		Locate:        locate,
		LookupEnv:     os.LookupEnv,
		Stdin:         stdin,
		Out:           out,
	}
	// A terminal on standard input can be asked; with --key-file - the input is the key, which leaves nothing to ask on.
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		reader := bufio.NewReader(f)
		env.Confirm = func(question string) (bool, error) { return docbanksetup.AskYesNo(reader, out, question) }
	}
	return env, nil
}

// runDocbank is `docbank setup`: it points the chosen kata daemon at the Docbank --url names, or at one it sets up on this machine.
func runDocbank(ctx context.Context, plan LaunchPlan, env docbanksetup.Env) error {
	if plan.DocbankFlags["key-file"] == "-" {
		env.Confirm = nil
	}
	var err error
	if plan.DocbankFlags["url"] == "" {
		port, _ := strconv.Atoi(plan.DocbankFlags["port"]) // checked when the plan was made; absent is 0
		err = env.SetupLocal(ctx, docbanksetup.Local{
			Port:      port,
			Folder:    plan.DocbankFlags["folder"],
			Yes:       plan.DocbankYes,
			NoRestart: plan.DocbankNoRestart,
		})
	} else {
		err = env.SetupRemote(ctx, docbanksetup.Remote{
			URL:       plan.DocbankFlags["url"],
			KeyFile:   plan.DocbankFlags["key-file"],
			KeyEnv:    plan.DocbankFlags["key-env"],
			Folder:    plan.DocbankFlags["folder"],
			Yes:       plan.DocbankYes,
			NoRestart: plan.DocbankNoRestart,
		})
	}
	if err != nil {
		return subcommandError{err}
	}
	return nil
}
