package data

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"

	"go.kenn.io/kata"

	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestIsProgramRefusesWhatTheOSWouldRun(t *testing.T) {
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD;.PLUGIN")
	for _, tc := range []struct {
		goos, path string
		want       bool
	}{
		{"windows", `C:\cache\report.bat`, true},
		{"windows", `C:\cache\REPORT.EXE`, true},
		{"windows", `C:\cache\x.Hta`, true},
		{"windows", `C:\cache\x.js`, true},
		{"windows", `C:\cache\x.plugin`, true}, // from PATHEXT
		{"windows", `C:\cache\x.lnk`, true},
		{"windows", `C:\cache\notes.txt`, false},
		{"windows", `C:\cache\noextension`, false},
		{"windows", `C:\cache\archive.bat.txt`, false},
		{"darwin", "/cache/Tool.APP", true},
		{"darwin", "/cache/run.command", true},
		{"darwin", "/cache/x.pkg", true},
		{"darwin", "/cache/report.bat", false},
		{"darwin", "/cache/notes.txt", false},
		{"linux", "/cache/x.desktop", true},
		{"linux", "/cache/X.Desktop", true},
		{"linux", "/cache/report.bat", false},
		{"linux", "/cache/notes.txt", false},
	} {
		if got := isProgram(tc.goos, tc.path); got != tc.want {
			t.Errorf("isProgram(%s, %s) = %v, want %v", tc.goos, tc.path, got, tc.want)
		}
	}
}

// programName is a file name the OS this test runs on would run when opened.
func programName() string {
	switch runtime.GOOS {
	case "windows":
		return "report.bat"
	case "darwin":
		return "report.command"
	}
	return "report.desktop"
}

func TestOpenAttachmentSavesAProgramButDoesNotOpenIt(t *testing.T) {
	f := newDocbankFixture(t)
	f.attach(t, writeFile(t, t.TempDir(), programName(), "echo hi"))

	got := as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}))
	if got.Err == nil || got.Path == "" || !strings.Contains(got.Err.Error(), "saved at "+got.Path+" and not opened because it is a program") {
		t.Fatalf("opened %+v", got)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("the download was not kept: %v", err)
	}
	if len(*f.opened) != 0 {
		t.Fatalf("the opener ran for a program: %v", *f.opened)
	}
}

// failOnMetadataWrites makes every Issue metadata write whose body matches fail with a server error.
func failOnMetadataWrites(server *kataServer, match func(body string) bool) {
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/metadata") {
			return false
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if !match(string(body)) {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"disk full"}}`))
		return true
	}
	server.intercept.Store(&hook)
}

func TestMigrateUnlinksTheNewAttachmentWhenTheOldKeyCannotBeCleared(t *testing.T) {
	f := newDocbankFixture(t)
	path := f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	failOnMetadataWrites(f.kata, func(body string) bool { return strings.Contains(body, "tern-kata.attachment.shot.png") })

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err == nil || len(got.Moved) != 0 || len(got.Left) != 1 || !got.Left[0].Failed || got.Left[0].ID != "" {
		t.Fatalf("migrated %+v", got)
	}
	if list := f.attachments(t); len(list) != 0 {
		t.Fatalf("the failed move left %+v on the Issue, so the next run would upload again", list)
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.shot.png"]; !ok {
		t.Fatal("the old key was cleared")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the local copy is gone: %v", err)
	}

	f.kata.intercept.Store(nil)
	again := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if again.Err != nil || len(again.Moved) != 1 || len(f.attachments(t)) != 1 {
		t.Fatalf("second run %+v, attachments %+v", again, f.attachments(t))
	}
}

func TestMigrateListsTheNewAttachmentWhenItCannotBeUnlinkedEither(t *testing.T) {
	docbank := docbanktest.New(t)
	f := fixtureOn(t, startKataWithDocbank(t, "0.21.0", kata.DocbankConfig{URL: docbank.URL, APIKey: docbanktest.Key}))
	f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	failOnMetadataWrites(f.kata, func(string) bool { return true })

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err == nil || len(got.Left) != 1 || !strings.Contains(got.Left[0].Reason, "a duplicate may exist") || !strings.Contains(got.Left[0].Reason, "could not be unlinked") || got.Left[0].ID == "" {
		t.Fatalf("migrated %+v", got)
	}
}
