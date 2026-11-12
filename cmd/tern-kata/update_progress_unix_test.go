//go:build !windows

package main

import (
	"strings"
	"sync"
	"testing"
	"time"

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
		t.Skip("the pty is not a terminal to the reporter")
	}
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}

	// A pty master has no read deadline, so a goroutine drains it (the cleanup's close ends it) and the test polls what it saw.
	var mu sync.Mutex
	var got strings.Builder
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			mu.Lock()
			got.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	const clearReport = "\x1b]7501;state=clear\x1b\\"
	var seen string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		mu.Lock()
		seen = got.String()
		mu.Unlock()
		if strings.HasSuffix(seen, clearReport) {
			break
		}
	}
	if !strings.Contains(seen, "state=working") || !strings.HasSuffix(seen, clearReport) {
		t.Errorf("reports %q, want a working report and the clear last", seen)
	}
}
