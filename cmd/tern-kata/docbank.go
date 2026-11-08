package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/docbanksetup"
)

const docbankUsage = "usage: tern-kata docbank supervised [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>] | docbank supervise on|off | docbank setup [--url <url> (--key-file <file>|- | --key-env <NAME>) | --port <port>] [--folder <path>] [--yes | --no-restart] [--home <dir>] [--db <file>] [--schema <name>] [--daemon <name> | --server <url>]"

type DocbankSetup struct {
	docbanksetup.Remote
	Port    int
	portArg string // --port as typed; planDocbank turns it into Port once the other checks have passed
}

// docbankRouteFlags are the route flags that pick which kata daemon's config is written; the others only carry the Pane's credentials
// to an agent, which setup has no use for.
var docbankRouteFlags = slices.DeleteFunc(slices.Clone(routeFlags), func(f tableFlag) bool {
	return !slices.Contains([]string{"home", "db", "schema", "daemon", "server"}, f.name)
})

// docbankFlags is the flag table of `docbank setup`: its parser and every completion script read it; docbankUsage and the help are
// prose that names each flag by hand (a test holds them to the table).
var docbankFlags = slices.Concat([]tableFlag{
	{"url", true, "the address of a Docbank someone else runs; without it Docbank is set up on this machine", func(p *LaunchPlan, v string) { p.Docbank.URL = v }},
	{"port", true, "the port Docbank on this machine listens on (default 8080); without --url only", func(p *LaunchPlan, v string) { p.Docbank.portArg = v }},
	{"key-file", true, "a file holding the Docbank API key, or - for standard input", func(p *LaunchPlan, v string) { p.Docbank.KeyFile = v }},
	{"key-env", true, "the name of a variable holding the Docbank API key", func(p *LaunchPlan, v string) { p.Docbank.KeyEnv = v }},
	{"folder", true, "the vault folder kata uses (default /kata)", func(p *LaunchPlan, v string) { p.Docbank.Folder = v }},
	{"yes", false, "restart the kata daemon without asking", func(p *LaunchPlan, _ string) { p.Docbank.Yes = true }},
	{"no-restart", false, "leave the kata daemon alone and print the restart command", func(p *LaunchPlan, _ string) { p.Docbank.NoRestart = true }},
}, docbankRouteFlags)

func planDocbank(args []string) (LaunchPlan, error) {
	plan := LaunchPlan{Subcommand: "docbank"}
	operands, err := scanOperands(args, flagReader("docbank", args, &plan))
	if err != nil {
		return LaunchPlan{}, err
	}
	d := plan.Docbank
	remote := d.URL != ""
	portGiven := d.portArg != ""
	switch {
	case slices.Equal(operands, []string{"supervised"}) || (len(operands) == 2 && operands[0] == "supervise" && (operands[1] == "on" || operands[1] == "off")):
		if d != (DocbankSetup{}) || portGiven {
			return LaunchPlan{}, fmt.Errorf("docbank %s takes no setup flag; %s", operands[0], docbankUsage)
		}
		if operands[0] == "supervise" && !reflect.ValueOf(plan.AgentRoute).IsZero() {
			return LaunchPlan{}, fmt.Errorf("docbank supervise takes no flag: the marker is in tern-kata's config folder; %s", docbankUsage)
		}
		plan.Operands = operands
		return plan, nil
	case !slices.Equal(operands, []string{"setup"}):
		return LaunchPlan{}, fmt.Errorf("docbank: expected `setup`, `supervised` or `supervise on|off`; %s", docbankUsage)
	case d.Yes && d.NoRestart:
		return LaunchPlan{}, fmt.Errorf("docbank setup: --yes and --no-restart exclude each other; %s", docbankUsage)
	case remote && portGiven:
		return LaunchPlan{}, fmt.Errorf("docbank setup: --port is for Docbank on this machine and excludes --url; %s", docbankUsage)
	case !remote && (d.KeyFile != "" || d.KeyEnv != ""):
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file and --key-env need --url: setting Docbank up on this machine makes its own key; %s", docbankUsage)
	case remote && d.KeyFile == "" && d.KeyEnv == "":
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file or --key-env is required with --url; %s", docbankUsage)
	case remote && d.KeyFile != "" && d.KeyEnv != "":
		return LaunchPlan{}, fmt.Errorf("docbank setup: --key-file and --key-env exclude each other; %s", docbankUsage)
	}
	if portGiven {
		n, err := parsePort(d.portArg)
		if err != nil {
			return LaunchPlan{}, fmt.Errorf("docbank setup: %w; %s", err, docbankUsage)
		}
		plan.Docbank.Port = n
	}
	plan.Operands = operands
	return plan, nil
}

func parsePort(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("--port must be a number from 1 to 65535, got %q", v)
	}
	return n, nil
}

// The route's KATA_HOME, KATA_DB and KATA_POSTGRES_SCHEMA are set in this process, which only runs the one command.
func newDocbankEnv(plan LaunchPlan, stdin io.Reader, out io.Writer) (docbanksetup.Env, error) {
	daemon, err := data.ResolveLocalDaemon(plan.AgentRoute)
	if err != nil {
		return docbanksetup.Env{}, err
	}
	// A terminal on standard input can be asked; with --key-file - the input is the key, which leaves nothing to ask on.
	terminal := stdinIsTerminal(stdin)
	if plan.Docbank.KeyFile == "-" && terminal {
		return docbanksetup.Env{}, usageError{fmt.Errorf("docbank setup: --key-file - reads the key from standard input, which is a terminal here; pipe the key in (for instance `... | tern-kata docbank setup --key-file - ...`) or use --key-file <file> or --key-env")}
	}
	// Resolved before the route's KATA_HOME is set below, which the default daemon's home is read from.
	supervision := docbanksetup.NewSupervisionEnv(out)
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
			fmt.Fprintf(&prefix, "%s=%s ", v.name, docbanksetup.ShellWord(v.value))
		}
	}
	locate := data.Locator(data.LocateWithKata)
	if daemon.Name != "" {
		locate = data.LocateNamed(daemon.Name, "")
	}
	env := supervision
	env.KataHome = daemon.Home
	env.Daemon = daemon.Name
	env.CommandPrefix = prefix.String()
	env.Runner = docbanksetup.ExecRunner{}
	env.HTTP = &http.Client{
		// X-Api-Key must never follow a redirect: Go would copy it to the new host.
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	env.Locate = locate
	env.LookupEnv = os.LookupEnv
	env.Stdin = stdin
	if f, ok := stdin.(*os.File); ok && terminal {
		reader := bufio.NewReader(f)
		env.Confirm = func(question string) (bool, error) { return docbanksetup.AskYesNo(reader, out, question) }
	}
	return env, nil
}

var stdinIsTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func runDocbank(ctx context.Context, plan LaunchPlan, env docbanksetup.Env) error {
	switch plan.Operands[0] {
	case "supervised":
		return printDocbankSupervised(env)
	case "supervise":
		return runDocbankSupervise(plan, env)
	}
	d := plan.Docbank
	var err error
	if d.URL == "" {
		err = env.SetupLocal(ctx, docbanksetup.Local{Common: d.Common, Port: d.Port})
	} else {
		err = env.SetupRemote(ctx, d.Remote)
	}
	if err != nil {
		return subcommandError{err}
	}
	return nil
}

// docbankEnvFor is the environment of the docbank form plan names: `supervise` needs no kata daemon to be resolved, the others
// resolve the one the route chooses.
func docbankEnvFor(plan LaunchPlan, out io.Writer) (docbanksetup.Env, error) {
	if plan.Operands[0] != "supervise" {
		return newDocbankEnv(plan, os.Stdin, out)
	}
	return docbanksetup.NewSupervisionEnv(out), nil
}

// printDocbankSupervised is `docbank supervised`, the plugin host half's one question about Docbank: the rule is
// docbanksetup.Env.Supervised. It prints `on` and exits 0 when the plugin keeps Docbank running; otherwise it prints nothing on
// stdout and exits 1 with the reason on stderr.
func printDocbankSupervised(env docbanksetup.Env) error {
	on, why := env.Supervised()
	if !on {
		return subcommandError{errors.New("the plugin does not keep Docbank running: " + why)}
	}
	_, err := fmt.Fprintln(env.Out, "on")
	return err
}

func runDocbankSupervise(plan LaunchPlan, env docbanksetup.Env) error {
	on := plan.Operands[1] == "on"
	if err := env.SetSupervision(on); err != nil {
		return subcommandError{err}
	}
	if on {
		_, err := fmt.Fprintf(env.Out, "Removed %s: the plugin keeps a local Docbank running again (within 10 seconds), when kata's [docbank] url and Docbank's [server] api_port are this machine's.\n", env.ManualPath())
		return err
	}
	_, err := fmt.Fprintf(env.Out, "Created %s: the plugin no longer starts Docbank. A Docbank that is running keeps running.\n", env.ManualPath())
	return err
}
