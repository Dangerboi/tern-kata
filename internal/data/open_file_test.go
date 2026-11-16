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
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

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

// inPaneRecordingTern puts the test in a Tern Pane whose `tern` answers every call with nothing and records what it was asked.
func inPaneRecordingTern(t *testing.T, f docbankFixture) *[][]string {
	t.Helper()
	t.Setenv("TERN_PANE", "7")
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) { asked = append(asked, args); return nil, nil }
	return &asked
}

func TestOpenAttachmentShowsAnImageInTernWhenRunningInAPane(t *testing.T) {
	f := newDocbankFixture(t)
	asked := inPaneRecordingTern(t, f)

	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "shot.PNG")))
	if got.Err != nil || got.Path == "" {
		t.Fatalf("opened %+v", got)
	}
	if want := [][]string{{"open", "--preview", got.Path}}; !slices.EqualFunc(*asked, want, slices.Equal) {
		t.Fatalf("tern was asked %v, want %v", *asked, want)
	}
	if len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("the OS opener ran too: %v (outside Tern %v)", *f.opened, got.OutsideTern)
	}
}

func TestOpenAttachmentUsesTheOSOpenerWhenTernCannotShowIt(t *testing.T) {
	for name, tc := range map[string]struct {
		file string
		env  func(*testing.T)
		runs func(context.Context, []string) ([]byte, error)
		note string
	}{
		"tern exits non-zero": {"shot.png", func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(context.Context, []string) ([]byte, error) {
			return []byte("no window answers"), errors.New("exit status 1")
		}, "no window answers"},
		"not in a Tern Pane": {"shot.png", func(*testing.T) {}, func(context.Context, []string) ([]byte, error) { t.Error("tern ran outside a Pane"); return nil, nil }, ""},
		"a video":            {"clip.mp4", func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(context.Context, []string) ([]byte, error) { t.Error("tern ran for a video"); return nil, nil }, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			tc.env(t)
			f.gateway.runTern = tc.runs
			got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, tc.file)))
			if got.Err != nil || !got.OutsideTern || !slices.Equal(*f.opened, []string{got.Path}) || !strings.Contains(got.Note, tc.note) || (tc.note == "" && got.Note != "") {
				t.Fatalf("opened %+v, OS opener ran for %v, want note %q", got, *f.opened, tc.note)
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
	asked := inPaneRecordingTern(t, f)

	cmd := f.openCmd(t, "shot.png")
	cmd.Way = model.WayBeside
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if want := [][]string{{"open", "--split", "right", got.Path}}; got.Err != nil || !slices.EqualFunc(*asked, want, slices.Equal) {
		t.Fatalf("opened %+v, tern was asked %v, want %v", got, *asked, want)
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

func TestOpenAttachmentWithTheSystemAppRefusesATypeTheOSMayRun(t *testing.T) {
	f := newDocbankFixture(t)
	cmd := f.openCmd(t, "report.bat")
	cmd.Way = model.WaySystem
	if got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd)); got.Err == nil || len(*f.opened) != 0 {
		t.Fatalf("opened %+v, OS opener ran for %v", got, *f.opened)
	}
	f = newDocbankFixture(t)
	pdf := f.openCmd(t, "report.pdf")
	pdf.Way = model.WaySystem
	if got := as[model.AttachmentOpened](t, run(t, f.gateway, pdf)); got.Err != nil || !slices.Equal(*f.opened, []string{got.Path}) {
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

func TestAnOSOpenerThatFailsIsReportedWithWhereTheFileIs(t *testing.T) {
	failing := func(string) error { return errors.New("xdg-open: exit status 3") }
	for name, way := range map[string]model.AttachmentWay{"the system app": model.WaySystem, "the default way": model.WayOpen} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			f.gateway.openFile = failing
			cmd := f.openCmd(t, "shot.png")
			cmd.Way = way
			got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
			if got.Err == nil || got.Path == "" || !strings.Contains(got.Err.Error(), "the file is at "+got.Path) || !strings.Contains(got.Err.Error(), "exit status 3") {
				t.Fatalf("opened %+v, want the failure and the file's path", got)
			}
		})
	}
}

func TestOpenFileReportsAnOpenerThatExitsNonZeroAndAcceptsOneThatSucceeds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the stand-in replaces xdg-open")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	opener := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(opener, []byte("#!/bin/sh\necho 'no display' >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := openFile("x.png"); err == nil || !strings.Contains(err.Error(), "no display") || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("a failing opener gave %v", err)
	}
	if err := os.WriteFile(opener, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := openFile("x.png"); err != nil {
		t.Fatalf("a succeeding opener gave %v", err)
	}
}

func TestOpenFileReturnsAtOnceWhenAnOpenerThatSucceededLeavesAChildHoldingStderr(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the stand-in replaces xdg-open")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nsleep 5 &\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := openFile("x.png"); err != nil {
		t.Fatalf("a succeeding opener whose child keeps stderr gave %v", err)
	}
	if took := time.Since(start); took >= openerWait/2 {
		t.Fatalf("openFile took %v, want it to return well under %v", took, openerWait)
	}
}

func TestExecTernKeepsStdoutApartFromStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'warning: slow' >&2\ncase \"$1\" in download) echo /saved/here.txt;; *) echo 'tern: no such block' >&2; exit 1;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tern"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := NewGateway(nil)
	if got, err := g.ternDownload(context.Background(), "x.txt"); err != nil || got != "/saved/here.txt" {
		t.Fatalf("the saved path is %q (%v), want stdout's line only", got, err)
	}
	_, err := g.ternOutput(context.Background(), 0, []string{"close", "7"})
	if err == nil || strings.Count(err.Error(), "no such block") != 1 || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("failure %v, want the exit status and tern's words once", err)
	}
}
