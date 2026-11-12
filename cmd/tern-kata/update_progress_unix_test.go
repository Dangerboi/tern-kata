//go:build !windows

package main

import (
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/progress"
)

func TestUpToDateUpdateClearsTheBarAfterAPluginDownload(t *testing.T) {
	f := newUpdateFixture(t, "v0.2.0", "0.2.0", nil)
	pluginDir := t.TempDir()
	installPlugin(t, pluginDir, "0.1.0")
	f.fakeTern(t, pluginDir, 0)
	master, slave := openPty(t)
	t.Setenv("TERN_TSP", "")
	f.env.Progress = progress.New(slave)
	if f.env.Progress == nil {
		t.Fatal("the reporter is nil on a pty slave, so progress reporting is never enabled")
	}
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}

	// With the slave closed the master reads what was written, then fails with EIO, which is how a closed Linux pty ends.
	slave.Close()
	out, err := io.ReadAll(master)
	if err != nil && !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	seen := string(out)
	const clearReport = "\x1b]7501;state=clear\x1b\\"
	if !strings.Contains(seen, "state=working") || !strings.HasSuffix(seen, clearReport) {
		t.Errorf("reports %q, want a working report and the clear last", seen)
	}
}
