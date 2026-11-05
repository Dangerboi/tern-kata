package data

import (
	"os"
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
	if len(*f.opened) != 0 {
		t.Fatalf("the opener ran: %v", *f.opened)
	}
}
