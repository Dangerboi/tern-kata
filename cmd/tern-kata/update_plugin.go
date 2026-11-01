package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ternTimeout bounds each `tern` command the plugin step runs.
const ternTimeout = time.Minute

var pluginTOMLVersion = regexp.MustCompile(`(?m)^version\s*=\s*"([^"\n]+)"`)

// updatePlugin is the plugin step of `update`: it brings this machine's copy of the window plugin to version, the binary's
// release (with its leading "v"). It acts only on a copy installed from a release whose plugin.toml names another version;
// no tern, a linked copy and an absent copy are left as they are (the first two with a note). release is the unpacked
// archive of version when the update already downloaded it, else fetch downloads it, and only when the step needs it. A
// failure is a warning on out: the binary is current whatever happens here.
func updatePlugin(ctx context.Context, version string, release *unpackedRelease, fetch func() (unpackedRelease, func(), error), out io.Writer) {
	ternPath, err := exec.LookPath("tern")
	if err != nil {
		fmt.Fprintln(out, "This machine's tern-kata plugin was not updated: tern is not on PATH. Update it with `tern plugin install <folder> --force`, or run the plugin-only install script.")
		return
	}
	warn := func(format string, args ...any) {
		fmt.Fprintf(out, "warning: this machine's tern-kata plugin was not updated: "+format+"\n", args...)
	}
	dirOut, err := runTern(ctx, ternPath, "plugin", "dir")
	if err != nil {
		warn("%v", err)
		return
	}
	pluginDir := strings.TrimSpace(dirOut)
	if pluginDir == "" {
		warn("`tern plugin dir` printed nothing")
		return
	}
	if link, err := os.ReadFile(filepath.Join(pluginDir, "tern-kata.path")); err == nil {
		fmt.Fprintf(out, "The tern-kata plugin is linked to %s: left as it is; update that checkout.\n", strings.TrimSpace(string(link)))
		return
	}
	text, err := os.ReadFile(filepath.Join(pluginDir, "tern-kata", "plugin.toml"))
	if err != nil {
		return // no installed copy: nothing to update
	}
	match := pluginTOMLVersion.FindSubmatch(text)
	want := strings.TrimPrefix(version, "v")
	if match == nil {
		warn("the installed copy's plugin.toml names no version")
		return
	}
	if have := string(match[1]); have == want {
		return
	}
	if release == nil {
		fetched, cleanup, err := fetch()
		if err != nil {
			warn("%v", err)
			return
		}
		defer cleanup()
		release = &fetched
	}
	if _, err := runTern(ctx, ternPath, "plugin", "install", filepath.Join(release.Dir, "plugin"), "--force"); err != nil {
		warn("%v", err)
		return
	}
	fmt.Fprintf(out, "Updated this machine's tern-kata plugin from %s to %s\n", match[1], want)
}

// runTern runs tern with args and returns its stdout; a failure carries tern's own words.
func runTern(ctx context.Context, ternPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ternTimeout)
	defer cancel()
	var stdout, stderr strings.Builder
	cmd := exec.CommandContext(ctx, ternPath, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String() + " " + stdout.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("`tern %s` failed: %s", strings.Join(args, " "), detail)
	}
	return stdout.String(), nil
}
