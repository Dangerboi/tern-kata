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
