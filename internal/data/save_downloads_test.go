package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// saveFixture is a docbank fixture whose local Downloads folder is a temp dir and whose `tern` is stubbed.
// The fixture clears the Tern environment, so inPane is set after it.
func saveFixture(t *testing.T, inPane bool, name string, tern func(args []string) ([]byte, error)) (model.AttachmentOpened, string, *[][]string) {
	t.Helper()
	return saveFixtureWithVersion(t, inPane, modernTernVersion, nil, name, tern)
}

// saveFixtureWithVersion is saveFixture on a Tern that answers `--version` with version (an error when versionErr is set).
// The version question is not among the calls it returns.
func saveFixtureWithVersion(t *testing.T, inPane bool, version string, versionErr error, name string, tern func(args []string) ([]byte, error)) (model.AttachmentOpened, string, *[][]string) {
	t.Helper()
	f := newDocbankFixture(t)
	if inPane {
		t.Setenv("TERN_PANE", "7")
	}
	downloads := filepath.Join(t.TempDir(), "Downloads")
	f.gateway.downloadsDir = func() (string, error) { return downloads, nil }
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte(version), versionErr
		}
		asked = append(asked, args)
		return tern(args)
	}
	cmd := f.openCmd(t, name)
	cmd.Way = model.WaySave
	return as[model.AttachmentOpened](t, run(t, f.gateway, cmd)), downloads, &asked
}

func TestSaveInTernReportsThePathTernPrintedAndCopiesNothingHere(t *testing.T) {
	const there = "C:\\Users\\me\\Downloads\\notes (1).txt"
	got, downloads, asked := saveFixture(t, true, "notes.txt", func([]string) ([]byte, error) { return []byte(there + "\n"), nil })
	if got.Err != nil || got.Path != there || !got.Remote {
		t.Fatalf("saved %+v", got)
	}
	if len(*asked) != 1 || len((*asked)[0]) != 2 || (*asked)[0][0] != "download" || filepath.Base((*asked)[0][1]) != "notes.txt" {
		t.Fatalf("tern was asked %v", *asked)
	}
	if _, err := os.Stat(downloads); err == nil {
		t.Fatalf("a copy was also made in %s", downloads)
	}
}

func TestSaveInTernFallsBackToThisMachineWhenTernCannotDownloadHere(t *testing.T) {
	cases := map[string]func([]string) ([]byte, error){
		"it prints no path": func([]string) ([]byte, error) { return nil, nil },
		"the window is on this machine": func([]string) ([]byte, error) {
			return []byte("tern download: the pane's files are on this computer already\n"), errors.New("exit status 1")
		},
	}
	for name, tern := range cases {
		t.Run(name, func(t *testing.T) {
			got, downloads, asked := saveFixture(t, true, "notes.txt", tern)
			if len(*asked) != 1 {
				t.Fatalf("tern was asked %v, want one download", *asked)
			}
			if want := filepath.Join(downloads, "notes.txt"); got.Err != nil || got.Path != want || got.Remote {
				t.Fatalf("saved %+v, want %s here", got, want)
			}
			if body, err := os.ReadFile(got.Path); err != nil || string(body) != "content" {
				t.Fatalf("the copy holds %q (%v)", body, err)
			}
		})
	}
}

func TestSaveInTernStillSavesHereWhenTernFailsAndSaysWhy(t *testing.T) {
	cases := map[string]func([]string) ([]byte, error){
		"a message and a non-zero exit": func([]string) ([]byte, error) {
			return []byte("tern download: the link to the window dropped\n"), errors.New("exit status 1")
		},
		"a non-zero exit without a message": func([]string) ([]byte, error) { return nil, errors.New("signal: killed") },
	}
	for name, tern := range cases {
		t.Run(name, func(t *testing.T) {
			got, downloads, _ := saveFixture(t, true, "notes.txt", tern)
			if want := filepath.Join(downloads, "notes.txt"); got.Err != nil || got.Path != want || got.Remote {
				t.Fatalf("saved %+v, want %s here", got, want)
			}
			if !strings.HasPrefix(got.Note, "tern download failed: ") || (name == "a message and a non-zero exit" && !strings.Contains(got.Note, "the link to the window dropped")) {
				t.Fatalf("note %q does not say what Tern said", got.Note)
			}
		})
	}
}

func TestSaveInTernSaysNothingExtraForTernsOrdinaryRefusals(t *testing.T) {
	got, _, _ := saveFixture(t, true, "notes.txt", func([]string) ([]byte, error) {
		return []byte("tern download: the pane's files are on this computer already\n"), errors.New("exit status 1")
	})
	if got.Err != nil || got.Note != "" {
		t.Fatalf("saved %+v", got)
	}
}

func TestSaveOutsideTernNeverRunsTern(t *testing.T) {
	got, downloads, asked := saveFixture(t, false, "notes.txt", func([]string) ([]byte, error) { return []byte("/elsewhere/notes.txt\n"), nil })
	if want := filepath.Join(downloads, "notes.txt"); got.Err != nil || got.Path != want || got.Remote || len(*asked) != 0 {
		t.Fatalf("saved %+v, tern asked %v", got, *asked)
	}
}
