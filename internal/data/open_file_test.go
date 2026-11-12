package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestIsOpenableAllowsOnlyViewingTypes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"shot.png", true},
		{"X.PNG", true},
		{"photo.JpEg", true},
		{"clip.mp4", true},
		{"song.flac", true},
		{"paper.pdf", true},
		{"notes.txt", true},
		{"data.json", true},
		{"a.exe.png", true}, // the extension is what the OS reads
		{"a.png.exe", false},
		{"report.bat", false},
		{"report.bat.", false},
		{"report.bat ", false},
		{"x.png.", false},
		{"x.png ", false},
		{"x.txt:y.bat", false},
		{"logo.svg", false},
		{"app.jar", false},
		{"link.url", false},
		{"link.fileloc", false},
		{"link.inetloc", false},
		{"run.command", false},
		{"setup.msi", false},
		{"launcher.desktop", false},
		{"noextension", false},
		{".png", true},
		{"", false},
	} {
		if got := isOpenable(tc.name); got != tc.want {
			t.Errorf("isOpenable(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestOpenAttachmentSavesAFileItDoesNotOpen(t *testing.T) {
	f := newDocbankFixture(t)
	f.attach(t, writeFile(t, t.TempDir(), "report.bat", "echo hi"))

	got := as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}))
	if got.Err == nil || got.Path == "" || !strings.Contains(got.Err.Error(), "saved at "+got.Path+" and not opened: tern-kata opens only images, video, audio, PDFs and text") {
		t.Fatalf("opened %+v", got)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("the download was not kept: %v", err)
	}
	if len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("the opener ran: %v (outside Tern %v)", *f.opened, got.OutsideTern)
	}
}

// openCmd attaches name and returns the command that opens it.
func (f docbankFixture) openCmd(t *testing.T, name string) model.OpenAttachment {
	t.Helper()
	f.attach(t, writeFile(t, t.TempDir(), name, "content"))
	return model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}
}

func TestOpenAttachmentShowsAnImageInTernWhenRunningInAPane(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) { asked = append(asked, args); return nil, nil }

	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "shot.PNG")))
	if got.Err != nil || got.Path == "" {
		t.Fatalf("opened %+v", got)
	}
	if want := [][]string{{"open", "--preview", got.Path}}; !slices.EqualFunc(asked, want, slices.Equal) {
		t.Fatalf("tern was asked %v, want %v", asked, want)
	}
	if len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("the OS opener ran too: %v (outside Tern %v)", *f.opened, got.OutsideTern)
	}
}

func TestOpenAttachmentUsesTheOSOpenerWhenTernCannotShowIt(t *testing.T) {
	for name, tc := range map[string]struct {
		env  func(*testing.T)
		runs func(context.Context, []string) ([]byte, error)
	}{
		"tern exits non-zero": {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(context.Context, []string) ([]byte, error) {
			return []byte("no window answers"), errors.New("exit status 1")
		}},
		"not in a Tern Pane": {func(*testing.T) {}, func(context.Context, []string) ([]byte, error) { t.Error("tern ran outside a Pane"); return nil, nil }},
		"a video":            {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(context.Context, []string) ([]byte, error) { t.Error("tern ran for a video"); return nil, nil }},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			tc.env(t)
			f.gateway.runTern = tc.runs
			file := "shot.png"
			if name == "a video" {
				file = "clip.mp4"
			}
			got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, file)))
			if got.Err != nil || !got.OutsideTern || !slices.Equal(*f.opened, []string{got.Path}) {
				t.Fatalf("opened %+v, OS opener ran for %v (outside Tern %v)", got, *f.opened, got.OutsideTern)
			}
		})
	}
}

func TestOpenAttachmentFallsBackWhenTernIsNotOnThePath(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	t.Setenv("PATH", t.TempDir())
	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "shot.png")))
	if got.Err != nil || !got.OutsideTern || !slices.Equal(*f.opened, []string{got.Path}) {
		t.Fatalf("opened %+v, OS opener ran for %v (outside Tern %v)", got, *f.opened, got.OutsideTern)
	}
}

func TestOpenAttachmentRunsTheTernOnThePathWithThePaneEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	f := newDocbankFixture(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$TERN_PANE $*\" > " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tern"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TERN_PANE", "7")
	f.gateway.runTern = execTern

	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "notes.md")))
	ran, err := os.ReadFile(log)
	if err != nil || got.Err != nil || strings.TrimSpace(string(ran)) != "7 open --preview "+got.Path || len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("opened %+v, tern ran %q (%v), OS opener %v", got, ran, err, *f.opened)
	}
}

func TestOpenAttachmentBesideAsksTernForASplitWhateverThePreviewSettingSays(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) { asked = append(asked, args); return nil, nil }

	cmd := f.openCmd(t, "shot.png")
	cmd.Way = model.WayBeside
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if want := [][]string{{"open", "--split", "right", got.Path}}; got.Err != nil || !slices.EqualFunc(asked, want, slices.Equal) {
		t.Fatalf("opened %+v, tern was asked %v, want %v", got, asked, want)
	}
	if len(*f.opened) != 0 {
		t.Fatalf("the OS opener ran: %v", *f.opened)
	}
}

func TestOpenAttachmentBesideDoesNotFallBackToTheOSOpener(t *testing.T) {
	for name, tc := range map[string]struct {
		file string
		pane bool
	}{"tern fails": {"shot.png", true}, "not in a Pane": {"shot.png", false}, "a video": {"clip.mp4", true}} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			if tc.pane {
				t.Setenv("TERN_PANE", "7")
			}
			f.gateway.runTern = func(context.Context, []string) ([]byte, error) { return nil, errors.New("exit status 1") }
			cmd := f.openCmd(t, tc.file)
			cmd.Way = model.WayBeside
			if got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd)); got.Err == nil || len(*f.opened) != 0 {
				t.Fatalf("opened %+v, OS opener ran for %v", got, *f.opened)
			}
		})
	}
}

func TestOpenAttachmentWithTheSystemAppSendsAnyType(t *testing.T) {
	f := newDocbankFixture(t)
	cmd := f.openCmd(t, "report.bat")
	cmd.Way = model.WaySystem
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if got.Err != nil || !slices.Equal(*f.opened, []string{got.Path}) {
		t.Fatalf("opened %+v, OS opener ran for %v", got, *f.opened)
	}
}

func TestOpenAttachmentSaveCopiesIntoDownloadsAndNeverOverwrites(t *testing.T) {
	f := newDocbankFixture(t)
	downloads := filepath.Join(t.TempDir(), "Downloads")
	f.gateway.downloadsDir = func() (string, error) { return downloads, nil }
	cmd := f.openCmd(t, "notes.txt")
	cmd.Way = model.WaySave

	first := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	second := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if first.Err != nil || second.Err != nil || first.Path != filepath.Join(downloads, "notes.txt") || second.Path != filepath.Join(downloads, "notes (1).txt") {
		t.Fatalf("saved %+v then %+v", first, second)
	}
	if body, err := os.ReadFile(second.Path); err != nil || string(body) != "content" {
		t.Fatalf("the copy holds %q (%v)", body, err)
	}
	if len(*f.opened) != 0 {
		t.Fatalf("saving opened %v", *f.opened)
	}
}

func TestSaveUniqueTakesTheFirstFreeName(t *testing.T) {
	src := writeFile(t, t.TempDir(), "report.tar.gz", "new")
	dir := filepath.Join(t.TempDir(), "nested", "Downloads")
	for _, existing := range []string{"report.tar.gz", "report.tar (1).gz", "report.tar (3).gz"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, existing), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path, err := saveUnique(src, dir)
	if want := filepath.Join(dir, "report.tar (2).gz"); err != nil || path != want {
		t.Fatalf("saved to %q (%v), want %q", path, err, want)
	}
	for name, body := range map[string]string{"report.tar.gz": "old", "report.tar (1).gz": "old", "report.tar (2).gz": "new", "report.tar (3).gz": "old"} {
		if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != body {
			t.Errorf("%s holds %q, want %q", name, got, body)
		}
	}
	if dotfile, err := saveUnique(writeFile(t, t.TempDir(), ".env", "x"), dir); err != nil || filepath.Base(dotfile) != ".env" {
		t.Fatalf("dotfile saved to %q (%v)", dotfile, err)
	}
	if again, err := saveUnique(writeFile(t, t.TempDir(), ".env", "y"), dir); err != nil || filepath.Base(again) != ".env (1)" {
		t.Fatalf("second dotfile saved to %q (%v)", again, err)
	}
}
