package data

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

const whatsNewChangelog = `## [Unreleased]

## [0.17.0] - 2026-11-20

- Seventeen.

## [0.16.0] - 2026-11-18

- Sixteen.
`

// whatsNewGateway is a Gateway in a Tern Pane whose `tern open` calls are recorded, with its state folder at state.
func whatsNewGateway(t *testing.T, state string) (*Gateway, *[][]string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	g := newGateway(t, newLocator(Target{}))
	t.Setenv("TERN_PANE", "7")
	g.workerDirs = func() (worker.Dirs, error) { return worker.Dirs{Config: state, State: state}, nil }
	g.changelog = whatsNewChangelog
	var opened [][]string
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		opened = append(opened, args)
		return nil, nil
	}
	return g, &opened
}

func showWhatsNew(t *testing.T, g *Gateway, version string) model.WhatsNewShown {
	t.Helper()
	return as[model.WhatsNewShown](t, run(t, g, model.ShowWhatsNew{Version: version, PaneBlock: "7"}))
}

// openedFile is the Markdown the one `tern open --split right` call opened; the focus then goes back to the Pane's block.
func openedFile(t *testing.T, opened [][]string) string {
	t.Helper()
	if len(opened) != 2 || !slices.Equal(opened[0][:3], []string{"open", "--split", "right"}) || len(opened[0]) != 4 ||
		!slices.Equal(opened[1], []string{"focus", "7"}) {
		t.Fatalf("tern ran %q, want `open --split right <file>` then `focus 7`", opened)
	}
	raw, err := os.ReadFile(opened[0][3])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The first Pane of a new version opens the notes since the version last shown; every later Pane of it, here or in another
// Pane sharing the state folder, opens nothing.
func TestTheFirstPaneOfANewVersionShowsWhatsNewOnce(t *testing.T) {
	state := t.TempDir()
	g, opened := whatsNewGateway(t, state)
	if got := showWhatsNew(t, g, "v0.16.0"); got.Err != nil {
		t.Fatal(got.Err)
	}
	if text := openedFile(t, *opened); !strings.Contains(text, "Sixteen.") || strings.Contains(text, "Seventeen.") {
		t.Fatalf("first run opened:\n%s", text)
	}
	*opened = nil
	showWhatsNew(t, g, "v0.16.0")
	if len(*opened) != 0 {
		t.Fatalf("a second Pane of v0.16.0 opened %q", *opened)
	}

	other, otherOpened := whatsNewGateway(t, state)
	showWhatsNew(t, other, "v0.17.0")
	if text := openedFile(t, *otherOpened); !strings.Contains(text, "Seventeen.") || strings.Contains(text, "Sixteen.") {
		t.Errorf("v0.17.0 after v0.16.0 opened:\n%s", text)
	}
}

func TestADevBuildShowsNothingAndRecordsNothing(t *testing.T) {
	state := t.TempDir()
	g, opened := whatsNewGateway(t, state)
	showWhatsNew(t, g, "dev")
	if len(*opened) != 0 {
		t.Fatalf("a dev build opened %q", *opened)
	}
	showWhatsNew(t, g, "v0.17.0")
	if text := openedFile(t, *opened); !strings.Contains(text, "Seventeen.") || strings.Contains(text, "Sixteen.") {
		t.Errorf("the first release after a dev build opened:\n%s", text)
	}
}

// Outside a Tern Pane nothing can open beside it: the version counts as shown, so a later Pane does not show it either.
func TestOutsideTernTheVersionIsRecordedAndNothingOpens(t *testing.T) {
	state := t.TempDir()
	g, opened := whatsNewGateway(t, state)
	t.Setenv("TERN_PANE", "")
	t.Setenv("TERN_WINDOW_SOCKET", "")
	showWhatsNew(t, g, "v0.17.0")
	t.Setenv("TERN_PANE", "7")
	showWhatsNew(t, g, "v0.17.0")
	if len(*opened) != 0 {
		t.Errorf("tern ran %q", *opened)
	}
	if _, err := os.Stat(filepath.Join(state, whatsNewFile)); err != nil {
		t.Errorf("nothing recorded: %v", err)
	}
}
