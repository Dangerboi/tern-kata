package data

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	if len(list) != 1 || list[0] != (model.Attachment{ID: got.ID, Name: "shot.png", MediaType: "image/png", Version: list[0].Version, Size: 6}) || list[0].Version == "" {
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

func TestOpenAttachmentDownloadsToTheCacheOnceAndOpensIt(t *testing.T) {
	f := newDocbankFixture(t)
	added := f.attach(t, writeFile(t, t.TempDir(), "notes.txt", "hello"))
	open := func() model.AttachmentOpened {
		return as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: f.attachments(t)[0]}))
	}

	got := open()
	a := f.attachments(t)[0]
	want := filepath.Join(filepath.Dir(got.Path), "notes.txt")
	if !strings.HasPrefix(got.Path, filepath.Join(f.cache, f.uid, added.ID)+string(filepath.Separator)) || got.Err != nil || got.Path != want || a.Version == "" {
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

func TestOpenAttachmentDownloadsAgainWhenTheIdPointsAtAnotherVersion(t *testing.T) {
	f := newDocbankFixture(t)
	f.attach(t, writeFile(t, t.TempDir(), "notes.txt", "hello"))
	a := f.attachments(t)[0]
	open := func(a model.Attachment) model.AttachmentOpened {
		return as[model.AttachmentOpened](t, run(t, f.gateway, model.OpenAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Attachment: a}))
	}
	if first := open(a); first.Err != nil {
		t.Fatal(first.Err)
	}
	f.docbank.Forget()
	// The same version is served from the cache; another version of the same id and size is not, so it asks Docbank, which has lost the bytes.
	if same := open(a); same.Err != nil {
		t.Fatalf("same version: %v", same.Err)
	}
	a.Version = "another-version"
	if changed := open(a); changed.Err == nil {
		t.Fatalf("a changed version was served from the cache: %+v", changed)
	}
}

func TestMigrateListsAFileThatIsNotRegularInsteadOfReadingIt(t *testing.T) {
	f := newDocbankFixture(t)
	dir := filepath.Join(f.repo, ".kata-attachments", f.uid, "folder.bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f.patchKey(t, "tern-kata.attachment.folder.bin", map[string]any{"host": f.host, "path": dir})
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if len(got.Moved) != 0 || len(got.Left) != 1 || !got.Left[0].Failed || !strings.Contains(got.Left[0].Reason, "directory") || f.docbank.Uploads() != 0 {
		t.Fatalf("migrated %+v (uploads %d)", got, f.docbank.Uploads())
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.folder.bin"]; !ok {
		t.Fatal("the key was cleared")
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

	got := as[model.AttachmentUnlinked](t, run(t, f.gateway, model.UnlinkAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", ID: one.ID, Name: "a.txt"}))
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

func TestMigrateMovesAnOldPointerUnderTheBareKeyAndLeavesNativeReferences(t *testing.T) {
	f := newDocbankFixture(t)
	dir := filepath.Join(f.repo, ".kata-attachments", f.uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, dir, "old.txt", "old")
	f.patchKey(t, "attachment.old.txt", map[string]any{"host": f.host, "path": path})
	native := map[string]any{"name": "n.png", "media_type": "image/png", "size": float64(1), "docbank_version": float64(1)}
	f.patchKey(t, "attachment.01JSMOKE0000000000000000", native)

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err != nil || len(got.Moved) != 1 || got.Moved[0].Name != "old.txt" || len(got.Left) != 0 {
		t.Fatalf("migrated %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	if _, still := meta["attachment.old.txt"]; still {
		t.Fatal("the 0.3 key of the moved file survived")
	}
	if _, ok := meta["attachment.01JSMOKE0000000000000000"]; !ok {
		t.Fatal("the native-shaped reference was cleared")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the local copy is still there: %v", err)
	}
}

// makeUndeletable makes removing path fail until the test ends: a read-only folder on Unix; on Windows, where folder modes do not stop a delete,
// a handle held open (Go opens files without delete sharing), which still lets the file be read.
func makeUndeletable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func TestMigrateTellsTheUserToDeleteACopyItCouldNotDelete(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes from a read-only folder")
	}
	f := newDocbankFixture(t)
	path := f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	makeUndeletable(t, path)

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if len(got.Moved) != 1 || len(got.Left) != 1 || !got.Left[0].Failed || !strings.Contains(got.Left[0].Reason, "delete the file by hand") || !strings.Contains(got.Left[0].Reason, path) {
		t.Fatalf("migrated %+v", got)
	}
	if got.Err == nil || got.Err.Error() != "1 attachment(s) need attention; see the list." {
		t.Fatalf("summary %v", got.Err)
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

// failAfterCommittingMetadataWrites applies every Issue metadata write whose body matches, then answers it with a server error, as a
// lost response would look to the client.
func failAfterCommittingMetadataWrites(server *kataServer, match func(body string) bool) {
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/metadata") {
			return false
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if !match(string(body)) {
			return false
		}
		server.handler.ServeHTTP(httptest.NewRecorder(), r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"response lost"}}`))
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

func TestMigrateKeepsTheNewAttachmentWhenTheClearCommittedButTheAnswerWasLost(t *testing.T) {
	f := newDocbankFixture(t)
	path := f.oldFile(t, f.uid, "shot.png", f.host, "pixels")
	failAfterCommittingMetadataWrites(f.kata, func(body string) bool { return strings.Contains(body, "tern-kata.attachment.shot.png") })

	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if got.Err != nil || len(got.Moved) != 1 || len(got.Left) != 0 || got.Moved[0].ID == "" {
		t.Fatalf("migrated %+v", got)
	}
	if list := f.attachments(t); len(list) != 1 || list[0].Name != "shot.png" {
		t.Fatalf("the Issue lists %+v, want the new attachment kept", list)
	}
	if _, ok := f.issue(t, f.uid).Metadata["tern-kata.attachment.shot.png"]; ok {
		t.Fatal("the old key survived")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the local copy is still there: %v", err)
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

func TestMigrateLeavesASymlinkInTheOldStoreWithoutReadingItsTarget(t *testing.T) {
	f := newDocbankFixture(t)
	secret := writeFile(t, t.TempDir(), "id_rsa", "private")
	dir := filepath.Join(f.repo, ".kata-attachments", f.uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "x")
	if err := os.Symlink(secret, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	f.patchKey(t, "tern-kata.attachment.x", map[string]any{"host": f.host, "path": link})
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if len(got.Moved) != 0 || len(got.Left) != 1 || !strings.Contains(got.Left[0].Reason, "outside") || f.docbank.Uploads() != 0 {
		t.Fatalf("migrated %+v (uploads %d)", got, f.docbank.Uploads())
	}
	if data, err := os.ReadFile(secret); err != nil || string(data) != "private" {
		t.Fatalf("target %q, %v", data, err)
	}
}

func TestMigrateLeavesAnIssueFolderThatIsASymlink(t *testing.T) {
	f := newDocbankFixture(t)
	elsewhere := t.TempDir()
	secret := writeFile(t, elsewhere, "x", "private")
	store := filepath.Join(f.repo, ".kata-attachments")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(store, f.uid)); err != nil {
		t.Skip("no symlinks here:", err)
	}
	f.patchKey(t, "tern-kata.attachment.x", map[string]any{"host": f.host, "path": filepath.Join(store, f.uid, "x")})
	got := as[model.AttachmentsMigrated](t, run(t, f.gateway, model.MigrateAttachments{Workspace: f.repo, Actor: "tester"}))
	if len(got.Moved) != 0 || len(got.Left) != 1 || !strings.Contains(got.Left[0].Reason, "outside") || f.docbank.Uploads() != 0 {
		t.Fatalf("migrated %+v (uploads %d)", got, f.docbank.Uploads())
	}
	if data, err := os.ReadFile(secret); err != nil || string(data) != "private" {
		t.Fatalf("target %q, %v", data, err)
	}
}

// closeEvents is the Body of every Events row of the Issue, as the detail shows them after loading it from the daemon.
func (f docbankFixture) closeEvents(t *testing.T) []string {
	t.Helper()
	loaded := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.repo, UID: f.uid}))
	m := model.Model{SelectedUID: f.uid}
	m, _ = model.Update(m, model.ResizeMsg{Cols: 120})
	m, _ = model.Update(m, loaded)
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabEvents})
	var bodies []string
	for _, item := range m.DetailRows() {
		if item.Kind == model.RowEvent {
			bodies = append(bodies, item.Activity.Body)
		}
	}
	return bodies
}

func TestCloseCitingAnAttachmentRecordsItAndTheHistoryNamesItEvenAfterUnlinking(t *testing.T) {
	f := newDocbankFixture(t)
	dir := t.TempDir()
	cited := f.attach(t, writeFile(t, dir, "report.pdf", "pdf bytes"))
	f.attach(t, writeFile(t, dir, "other.txt", "other"))

	closed := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{
		Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Key: "k-attachment", Reason: model.ReasonDone,
		Message:  "Implemented the requested change and verified the result.",
		Evidence: []model.CloseEvidence{{Kind: model.EvidenceAttachment, Value: cited.ID}},
	}))
	if closed.Err != nil {
		t.Fatalf("close %+v", closed)
	}
	stored, _ := closePayload(t, f.gateway, f.repo, f.uid)["evidence"].([]any)
	entry, _ := stored[0].(map[string]any)
	frozen, _ := entry["attachment"].(map[string]any)
	if len(stored) != 1 || entry["type"] != "attachment" || entry["attachment_id"] != cited.ID || frozen["name"] != "report.pdf" {
		t.Fatalf("stored evidence %v", stored)
	}
	has := func(bodies []string, want string) bool {
		return slices.ContainsFunc(bodies, func(b string) bool { return strings.Contains(b, "(attachment "+want+")") })
	}
	if bodies := f.closeEvents(t); !has(bodies, "report.pdf") {
		t.Fatalf("Events rows %q do not name the attachment", bodies)
	}

	detached := as[model.AttachmentUnlinked](t, run(t, f.gateway, model.UnlinkAttachment{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", ID: cited.ID, Name: "report.pdf"}))
	if detached.Err != nil {
		t.Fatalf("detach %+v", detached)
	}
	if bodies := f.closeEvents(t); !has(bodies, "report.pdf [removed]") {
		t.Fatalf("after unlinking, Events rows %q do not name the removed attachment", bodies)
	}
}

func TestCloseCitingAnAttachmentTheIssueDoesNotHaveIsRefused(t *testing.T) {
	f := newDocbankFixture(t)
	closed := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{
		Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Key: "k-missing", Reason: model.ReasonDone,
		Message:  "Implemented the requested change and verified the result.",
		Evidence: []model.CloseEvidence{{Kind: model.EvidenceAttachment, Value: "nope"}},
	}))
	if closed.Err == nil || f.issue(t, f.uid).Status == "closed" {
		t.Fatalf("close %+v, status %q", closed, f.issue(t, f.uid).Status)
	}
}
