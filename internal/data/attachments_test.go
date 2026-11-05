package data

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/kata"

	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// docbankFixture is an issueFixture on a daemon whose attachments go to a fake Docbank, with downloads and "open" kept in the test.
type docbankFixture struct {
	issueFixture
	docbank *docbanktest.Fake
	cache   string
	opened  *[]string
}

func newDocbankFixture(t *testing.T) docbankFixture {
	t.Helper()
	docbank := docbanktest.New(t)
	f := fixtureOn(t, startKataWithDocbank(t, "0.21.0", kata.DocbankConfig{URL: docbank.URL, APIKey: docbanktest.Key}))
	cache := t.TempDir()
	opened := new([]string)
	f.gateway.cacheDir = func() (string, error) { return cache, nil }
	f.gateway.openFile = func(path string) error { *opened = append(*opened, path); return nil }
	return docbankFixture{f, docbank, cache, opened}
}

func (f docbankFixture) attach(t *testing.T, path string) model.FileAttached {
	t.Helper()
	return as[model.FileAttached](t, run(t, f.gateway, model.AttachFile{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Path: path}))
}

func (f docbankFixture) attachments(t *testing.T) []model.Attachment {
	t.Helper()
	return model.Attachments(f.issue(t, f.uid).Metadata)
}

func TestAttachUploadsThroughKataAndListsTheReference(t *testing.T) {
	f := newDocbankFixture(t)
	source := writeFile(t, t.TempDir(), "shot.png", "pixels")

	got := f.attach(t, `"`+source+`"`)
	if got.Err != nil || got.Name != "shot.png" || got.ID == "" || got.UID != f.uid {
		t.Fatalf("attached %+v", got)
	}
	list := f.attachments(t)
	if len(list) != 1 || list[0] != (model.Attachment{ID: got.ID, Name: "shot.png", MediaType: "image/png", Size: 6}) {
		t.Fatalf("the Issue lists %+v", list)
	}
	if f.docbank.Uploads() != 1 {
		t.Fatalf("Docbank got %d uploads, want 1", f.docbank.Uploads())
	}
	if _, old := f.issue(t, f.uid).Metadata["tern-kata.attachment.shot.png"]; old {
		t.Fatal("a tern-kata pointer was written")
	}
}

func TestAttachRefusesWhatIsNotAFile(t *testing.T) {
	f := newDocbankFixture(t)
	dir := t.TempDir()
	for _, path := range []string{"", filepath.Join(dir, "missing.png"), dir} {
		if got := f.attach(t, path); got.Err == nil {
			t.Fatalf("%q was accepted: %+v", path, got)
		}
	}
	if f.docbank.Uploads() != 0 {
		t.Fatal("something was uploaded")
	}
}

func TestIssueAttachmentsListsTheNativeReferencesByName(t *testing.T) {
	f := newDocbankFixture(t)
	dir := t.TempDir()
	f.attach(t, writeFile(t, dir, "b.log", "b"))
	f.attach(t, writeFile(t, dir, "a.txt", "a"))
	f.patchKey(t, "tern-kata.attachment.old.png", map[string]any{"host": "x", "path": "/x"})

	c, err := f.gateway.client(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.IssueAttachments(context.Background(), f.project.ID, f.issue(t, f.uid).ShortID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range list {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []string{"a.txt", "b.log"}) || list[0].ID == "" {
		t.Fatalf("listed %+v", list)
	}
}

func TestOpenAttachmentDownloadsToTheCacheOnceAndOpensIt(t *testing.T) {
	f := newDocbankFixture(t)
	added := f.attach(t, writeFile(t, t.TempDir(), "notes.txt", "hello"))
	open := func() model.AttachmentOpened {
		return as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}))
	}

	got := open()
	want := filepath.Join(f.cache, f.uid, added.ID, "notes.txt")
	if got.Err != nil || got.Path != want {
		t.Fatalf("opened %+v, want %s", got, want)
	}
	if content, err := os.ReadFile(want); err != nil || string(content) != "hello" {
		t.Fatalf("cached %q, %v", content, err)
	}
	// Docbank loses the bytes: the second open uses the cache, which holds an immutable version.
	f.docbank.Forget()
	if again := open(); again.Err != nil || again.Path != want {
		t.Fatalf("second open %+v", again)
	}
	if !slices.Equal(*f.opened, []string{want, want}) {
		t.Fatalf("opened %v", *f.opened)
	}
}

func TestOpenAttachmentSaysWhenDocbankLostTheBytes(t *testing.T) {
	f := newDocbankFixture(t)
	f.attach(t, writeFile(t, t.TempDir(), "notes.txt", "hello"))
	f.docbank.Forget()
	got := as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "no longer has this attachment's content") || len(*f.opened) != 0 {
		t.Fatalf("opened %+v (opened %v)", got, *f.opened)
	}
}

func TestDetachUnlinksTheReferenceAndKeepsTheOthers(t *testing.T) {
	f := newDocbankFixture(t)
	dir := t.TempDir()
	one := f.attach(t, writeFile(t, dir, "a.txt", "a"))
	two := f.attach(t, writeFile(t, dir, "b.txt", "b"))

	got := as[model.FileDetached](t, run(t, f.gateway, model.DetachFile{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", ID: one.ID, Name: "a.txt"}))
	if got.Err != nil || got.Name != "a.txt" {
		t.Fatalf("detached %+v", got)
	}
	list := f.attachments(t)
	if len(list) != 1 || list[0].ID != two.ID {
		t.Fatalf("the Issue lists %+v", list)
	}
}

func TestTheSameNameAttachedTwiceMakesTwoReferences(t *testing.T) {
	f := newDocbankFixture(t)
	source := writeFile(t, t.TempDir(), "shot.png", "pixels")
	a, b := f.attach(t, source), f.attach(t, source)
	if a.Err != nil || b.Err != nil || a.ID == b.ID || len(f.attachments(t)) != 2 {
		t.Fatalf("got %+v and %+v, then %+v", a, b, f.attachments(t))
	}
}

func TestAttachToADaemonWithoutDocbankSaysSoPlainly(t *testing.T) {
	f := newIssueFixture(t)
	got := as[model.FileAttached](t, run(t, f.gateway, model.AttachFile{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Path: writeFile(t, t.TempDir(), "a.txt", "a")}))
	if got.Err == nil || got.Err.Error() != model.DocbankOffMessage {
		t.Fatalf("attached %+v, want the not-configured message", got)
	}
}

func TestADetailSaysWhetherTheDaemonHasDocbank(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    issueFixture
		off  bool
	}{
		{"without", newIssueFixture(t), true},
		{"with", newDocbankFixture(t).issueFixture, false},
	} {
		loaded := as[model.DetailLoaded](t, run(t, tc.f.gateway, model.LoadDetail{Workspace: tc.f.repo, UID: tc.f.uid}))
		if loaded.Selected == nil || loaded.Selected.AttachmentsOff != tc.off {
			t.Errorf("%s Docbank: %+v, want AttachmentsOff %v", tc.name, loaded.Selected, tc.off)
		}
	}
}

func TestAttachSaysPlainlyWhenDocbankRefusesTheKeyOrIsGone(t *testing.T) {
	docbank := docbanktest.New(t)
	f := fixtureOn(t, startKataWithDocbank(t, "0.21.0", kata.DocbankConfig{URL: docbank.URL, APIKey: "wrong"}))
	source := writeFile(t, t.TempDir(), "a.txt", "a")
	attach := func() model.FileAttached {
		return as[model.FileAttached](t, run(t, f.gateway, model.AttachFile{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Path: source}))
	}
	if got := attach(); got.Err == nil || !strings.Contains(got.Err.Error(), "refused the API key") {
		t.Fatalf("a wrong key answered %+v", got)
	}
	docbank.Stop()
	if got := attach(); got.Err == nil || !strings.Contains(got.Err.Error(), "could not reach Docbank") {
		t.Fatalf("a stopped Docbank answered %+v", got)
	}
}

func TestAttachRefusesAFileOverTheLimitBeforeReadingIt(t *testing.T) {
	f := newDocbankFixture(t)
	big := filepath.Join(t.TempDir(), "big.bin")
	file, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(model.MaxAttachmentBytes + 1); err != nil {
		t.Skip("no sparse files here:", err)
	}
	_ = file.Close()
	if got := f.attach(t, big); got.Err == nil || !strings.Contains(got.Err.Error(), "256 MiB") {
		t.Fatalf("attached %+v", got)
	}
}

// oldFile writes an old store copy of an Issue's file and the key that points to it.
func (f issueFixture) oldFile(t *testing.T, uid, name, host, content string) string {
	t.Helper()
	dir := filepath.Join(f.repo, ".kata-attachments", uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, dir, name, content)
	f.patchKeyOf(t, uid, "tern-kata.attachment."+name, map[string]any{"host": host, "path": path})
	return path
}

func TestMigrateMovesLocalCopiesAndListsTheRest(t *testing.T) {
	f := newDocbankFixture(t)
	mine := f.oldFile(t, f.uid, "shot.png", "testbox", "pixels") // host names compare without regard to case
	if err := os.WriteFile(filepath.Join(f.repo, ".kata-attachments", ".gitignore"), []byte("*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.patchKey(t, "tern-kata.attachment.elsewhere.log", map[string]any{"host": "other-box", "path": "/home/o/x.log"})
	f.patchKey(t, "tern-kata.attachment.gone.txt", map[string]any{"host": f.host, "path": filepath.Join(f.repo, ".kata-attachments", f.uid, "gone.txt")})
	f.patchKey(t, "tern-kata.attachment.outside.txt", map[string]any{"host": f.host, "path": writeFile(t, t.TempDir(), "outside.txt", "o")})

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err != nil || len(got.Moved) != 1 || got.Moved[0].Name != "shot.png" || got.Moved[0].ID == "" || len(got.Left) != 3 {
		t.Fatalf("migrated %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if _, still := meta["tern-kata.attachment.shot.png"]; still {
		t.Fatal("the old key of the moved file survived")
	}
	for _, kept := range []string{"elsewhere.log", "gone.txt", "outside.txt"} {
		if _, ok := meta["tern-kata.attachment."+kept]; !ok {
			t.Errorf("the key of %s was cleared", kept)
		}
	}
	list := f.attachments(t)
	if len(list) != 1 || list[0].Name != "shot.png" || list[0].Size != 6 {
		t.Fatalf("the Issue lists %+v", list)
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Fatalf("the local copy is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".kata-attachments")); !os.IsNotExist(err) {
		t.Fatalf("the emptied store is still there: %v", err)
	}
	report := got.Report()
	for _, want := range []string{"Moved 1", "other-box", "gone from this machine", "outside this Project's old attachment folder"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
}

func TestMigrateIsRefusedByADaemonWithoutDocbank(t *testing.T) {
	f := newIssueFixture(t)
	path := f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err == nil || got.Err.Error() != model.DocbankOffMessage || len(got.Moved) != 0 {
		t.Fatalf("migrated %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the local copy was touched: %v", err)
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.shot.png"]; !ok {
		t.Fatal("the key was cleared")
	}
}

func TestMigrateKeepsTheCopyWhenTheUploadFails(t *testing.T) {
	docbank := docbanktest.New(t)
	f := fixtureOn(t, startKataWithDocbank(t, "0.21.0", kata.DocbankConfig{URL: docbank.URL, APIKey: "wrong"}))
	path := f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err == nil || len(got.Moved) != 0 || len(got.Left) != 1 || !got.Left[0].Failed {
		t.Fatalf("migrated %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the copy was deleted: %v", err)
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.shot.png"]; !ok {
		t.Fatal("the key was cleared")
	}
}

func TestMigrateDoesNotDeleteAFileOutsideTheIssuesFolder(t *testing.T) {
	f := newDocbankFixture(t)
	other := createIssue(t, f.gateway, f.repo, f.project, "Other")
	path := f.oldFile(t, other, "x.txt", f.host, "x")
	// This Issue's key points at the other Issue's copy.
	f.patchKeyOf(t, f.uid, "tern-kata.attachment.x.txt", map[string]any{"host": f.host, "path": path})
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	var left int
	for _, l := range got.Left {
		if l.Path == path && strings.Contains(l.Reason, "outside") {
			left++
		}
	}
	if left != 1 {
		t.Fatalf("migrated %+v, want the pointer at another Issue's copy left", got)
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.x.txt"]; !ok {
		t.Fatal("the key was cleared")
	}
}
