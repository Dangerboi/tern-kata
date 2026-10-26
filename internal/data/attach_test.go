package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// attachFixture is a git repository bound to a Project on an in-process daemon, holding one Issue.
type attachFixture struct {
	kata      *kataServer
	gateway   *Gateway
	repo, uid string
	project   model.Project
	host      string
}

func newAttachFixture(t *testing.T) attachFixture {
	t.Helper()
	kata := startKata(t, "0.20.0")
	const host = "TestBox"
	gateway := newGateway(t, newLocator(kata.target())).WithHost(host)
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	saved := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: repo, Actor: "tester", Name: "demo"}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	return attachFixture{kata, gateway, repo, createIssue(t, gateway, repo, saved.Project, "Screenshot"), saved.Project, host}
}

func (f attachFixture) attach(t *testing.T, workspace, ref, path string) model.FileAttached {
	t.Helper()
	return as[model.FileAttached](t, run(t, f.gateway, model.AttachFile{Workspace: workspace, Ref: ref, Actor: "tester", Path: path}))
}

// attachResult is attach for a goroutine other than the test's: t.Fatal is not allowed there, so a harness failure comes back as Err.
func (f attachFixture) attachResult(workspace, ref, path string) model.FileAttached {
	messages := make(chan model.Msg, 1)
	f.gateway.Run(context.Background(), model.AttachFile{Workspace: workspace, Ref: ref, Actor: "tester", Path: path}, messages)
	select {
	case msg := <-messages:
		if got, ok := msg.(model.FileAttached); ok {
			return got
		}
		return model.FileAttached{Err: fmt.Errorf("gateway answered %T", msg)}
	default:
		return model.FileAttached{Err: errors.New("gateway produced no message")}
	}
}

func (f attachFixture) detach(t *testing.T, workspace, ref, name string) model.FileDetached {
	t.Helper()
	return as[model.FileDetached](t, run(t, f.gateway, model.DetachFile{Workspace: workspace, Ref: ref, Actor: "tester", Name: name}))
}

func (f attachFixture) issue(t *testing.T, uid string) g.ShowIssueOut {
	t.Helper()
	response, err := f.kata.target().Client.ShowIssueByUIDWithResponse(context.Background(), &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: uid}})
	if err != nil {
		t.Fatal(err)
	}
	return response.JSON200.Issue
}

// pointers is the Issue's tern-kata.attachment.* meta as kata holds it, by stored name.
func (f attachFixture) pointers(t *testing.T, uid string) map[string]map[string]any {
	t.Helper()
	pointers := map[string]map[string]any{}
	for key, value := range f.issue(t, uid).Metadata {
		if name, ok := strings.CutPrefix(key, "tern-kata.attachment."); ok {
			pointers[name] = value.(map[string]any)
		}
	}
	return pointers
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAttachCopiesIntoTheStoreAndPointsAtTheCopy(t *testing.T) {
	f := newAttachFixture(t)
	source := writeFile(t, t.TempDir(), "shot.png", "pixels")

	got := f.attach(t, f.repo, "", source)
	if got.Err == nil {
		t.Fatalf("an empty ref was accepted: %+v", got)
	}
	short := f.issue(t, f.uid).ShortID
	got = f.attach(t, f.repo, short, source)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	want := filepath.Join(f.repo, ".kata-attachments", f.uid, "shot.png")
	if got.Name != "shot.png" || got.Path != want {
		t.Fatalf("got %+v, want name shot.png at %s", got, want)
	}
	if content, err := os.ReadFile(want); err != nil || string(content) != "pixels" {
		t.Fatalf("copy %q, %v", content, err)
	}
	host := f.host
	if pointer := f.pointers(t, f.uid)["shot.png"]; pointer["host"] != host || pointer["path"] != want || len(pointer) != 2 {
		t.Fatalf("pointer %v, want host %q path %q", pointer, host, want)
	}
	if ignore, err := os.ReadFile(filepath.Join(f.repo, ".kata-attachments", ".gitignore")); err != nil || string(ignore) != "*\n" {
		t.Fatalf("store .gitignore %q, %v", ignore, err)
	}
	for _, untouched := range []string{".gitignore", filepath.Join(".git", "info", "exclude")} {
		if raw, err := os.ReadFile(filepath.Join(f.repo, untouched)); err == nil && strings.Contains(string(raw), "kata-attachments") {
			t.Fatalf("%s was edited: %q", untouched, raw)
		}
	}
	if out, err := exec.Command("git", "-C", f.repo, "status", "--porcelain", "--", ".kata-attachments").Output(); err != nil || len(out) != 0 {
		t.Fatalf("git sees the store: %q, %v", out, err)
	}
}

func TestAttachWritesTheStoreGitignoreOnce(t *testing.T) {
	f := newAttachFixture(t)
	source := writeFile(t, t.TempDir(), "a.txt", "a")
	f.attach(t, f.repo, f.uid, source)
	ignore := filepath.Join(f.repo, ".kata-attachments", ".gitignore")
	writeFile(t, filepath.Dir(ignore), ".gitignore", "edited by hand\n")
	f.attach(t, f.repo, f.uid, source)
	if raw, _ := os.ReadFile(ignore); string(raw) != "edited by hand\n" {
		t.Fatalf("second attach rewrote the store .gitignore: %q", raw)
	}
}

func TestAttachRepairsAnEmptyStoreGitignore(t *testing.T) {
	f := newAttachFixture(t)
	source := writeFile(t, t.TempDir(), "a.txt", "a")
	ignore := filepath.Join(f.repo, ".kata-attachments", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(ignore), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Dir(ignore), ".gitignore", "")
	if got := f.attach(t, f.repo, f.uid, source); got.Err != nil {
		t.Fatal(got.Err)
	}
	if raw, _ := os.ReadFile(ignore); string(raw) != "*\n" {
		t.Fatalf("store .gitignore holds %q", raw)
	}
}

func TestAttachNamesClashesWithTheLowestFreeNumber(t *testing.T) {
	f := newAttachFixture(t)
	dir := t.TempDir()
	var names []string
	for range 3 {
		got := f.attach(t, f.repo, f.uid, writeFile(t, dir, "shot.png", "x"))
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		names = append(names, got.Name)
	}
	if !slices.Equal(names, []string{"shot.png", "shot-2.png", "shot-3.png"}) {
		t.Fatalf("names %v", names)
	}
	if got := f.attach(t, f.repo, f.uid, writeFile(t, dir, "notes", "x")); got.Name != "notes" {
		t.Fatalf("%+v", got)
	}
	if got := f.attach(t, f.repo, f.uid, writeFile(t, dir, "notes", "x")); got.Name != "notes-2" {
		t.Fatalf("%+v", got)
	}
}

func TestConcurrentAttachesOfOneNameAreAllKept(t *testing.T) {
	f := newAttachFixture(t)
	source := writeFile(t, t.TempDir(), "shot.png", "x")
	const attaches = 6
	results := make([]model.FileAttached, attaches)
	var wg sync.WaitGroup
	for i := range attaches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = f.attachResult(f.repo, f.uid, source)
		}()
	}
	wg.Wait()
	pointers := f.pointers(t, f.uid)
	seen := map[string]bool{}
	for _, r := range results {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if seen[r.Name] || pointers[r.Name]["path"] != r.Path {
			t.Fatalf("name %q reused or not pointed at its copy: %v", r.Name, pointers[r.Name])
		}
		seen[r.Name] = true
		if _, err := os.Stat(r.Path); err != nil {
			t.Fatal(err)
		}
	}
	if len(pointers) != attaches {
		t.Fatalf("%d pointers, want %d: %v", len(pointers), attaches, pointers)
	}
}

func TestAttachIsRefusedOutsideTheIssuesBinding(t *testing.T) {
	f := newAttachFixture(t)
	source := writeFile(t, t.TempDir(), "a.txt", "x")

	other := t.TempDir()
	if saved := as[model.ProjectSaved](t, run(t, f.gateway, model.BindProject{Workspace: other, Actor: "tester", Name: "elsewhere"})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if got := f.attach(t, other, f.uid, source); got.Err == nil || !strings.Contains(got.Err.Error(), `"demo"`) || !strings.Contains(got.Err.Error(), `"elsewhere"`) {
		t.Fatalf("got %+v, want a refusal naming demo to attach from", got)
	}
	unbound := t.TempDir()
	if got := f.attach(t, unbound, f.uid, source); got.Err == nil || !strings.Contains(got.Err.Error(), `"demo"`) {
		t.Fatalf("got %+v, want a refusal naming demo", got)
	}
	for _, workspace := range []string{other, unbound} {
		if _, err := os.Stat(filepath.Join(workspace, ".kata-attachments")); !os.IsNotExist(err) {
			t.Fatalf("a refused attach made a store in %s: %v", workspace, err)
		}
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("a refused attach wrote a pointer")
	}
}

func TestAttachToAClosedIssue(t *testing.T) {
	f := newAttachFixture(t)
	if closed := as[model.CloseResultMsg](t, run(t, f.gateway, model.CloseIssueCmd{Workspace: f.repo, UID: f.uid, ProjectID: f.project.ID, Actor: "tester", Reason: model.ReasonDone, Quick: true, Key: "close-key"})); closed.Err != nil {
		t.Fatal(closed.Err)
	}
	if got := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "a.txt", "x")); got.Err != nil {
		t.Fatal(got.Err)
	}
}

func TestAttachNormalisesThePath(t *testing.T) {
	f := newAttachFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	absolute := writeFile(t, t.TempDir(), "abs.txt", "abs")
	writeFile(t, home, "home.txt", "home")
	writeFile(t, f.repo, "rel.txt", "rel")
	n := 0
	for input, name := range map[string]string{
		"  " + absolute + "\t":                        "abs.txt",
		`"` + absolute + `"`:                          "abs.txt",
		"'" + absolute + "'":                          "abs.txt",
		"~" + string(filepath.Separator) + "home.txt": "home.txt",
		"rel.txt":       "rel.txt",
		`  "rel.txt"  `: "rel.txt",
	} {
		n++
		uid := createIssue(t, f.gateway, f.repo, f.project, strings.Repeat(string(rune('a'+n)), 9))
		if got := f.attach(t, f.repo, uid, input); got.Err != nil || got.Name != name {
			t.Errorf("%q: got %+v, want %s", input, got, name)
		}
	}
}

func TestAttachRefusesADirectoryAndAMissingPath(t *testing.T) {
	f := newAttachFixture(t)
	dir := t.TempDir()
	if got := f.attach(t, f.repo, f.uid, dir); got.Err == nil || !strings.Contains(got.Err.Error(), "directory") {
		t.Fatalf("directory: %+v", got)
	}
	if got := f.attach(t, f.repo, f.uid, filepath.Join(dir, "nope.png")); got.Err == nil || !strings.Contains(got.Err.Error(), "does not exist") {
		t.Fatalf("missing: %+v", got)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("a refused attach wrote a pointer")
	}
}

func TestAttachCopiesWhatASymlinkPointsTo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	f := newAttachFixture(t)
	dir := t.TempDir()
	target := writeFile(t, dir, "real.png", "target bytes")
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got := f.attach(t, f.repo, f.uid, link)
	if got.Err != nil || got.Name != "link.png" {
		t.Fatal(got)
	}
	if info, err := os.Lstat(got.Path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the copy is not a regular file: %v, %v", info, err)
	}
	if content, _ := os.ReadFile(got.Path); string(content) != "target bytes" {
		t.Fatalf("copy %q", content)
	}
}

func TestAFailedPointerWriteLeavesNoCopy(t *testing.T) {
	f := newAttachFixture(t)
	broken := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/metadata") {
			http.Error(w, `{"status":422,"error":{"code":"validation","message":"refused"}}`, http.StatusUnprocessableEntity)
			return
		}
		f.kata.handler.ServeHTTP(w, r)
	}), "0.20.0")
	gateway := newGateway(t, newLocator(broken.target())).WithHost("TestBox")
	got := as[model.FileAttached](t, run(t, gateway, model.AttachFile{Workspace: f.repo, Ref: f.uid, Actor: "tester", Path: writeFile(t, t.TempDir(), "shot.png", "x")}))
	if got.Err == nil {
		t.Fatalf("got %+v, want the pointer write's failure", got)
	}
	left, _ := filepath.Glob(filepath.Join(f.repo, ".kata-attachments", f.uid, "*"))
	if len(left) != 0 {
		t.Fatalf("copies left behind: %v", left)
	}
}

func TestALostPointerAnswerKeepsTheCopy(t *testing.T) {
	f := newAttachFixture(t)
	lossy := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/metadata") {
			f.kata.handler.ServeHTTP(httptest.NewRecorder(), r) // kata commits the pointer; the answer never arrives
			http.Error(w, `{"status":502,"error":{"code":"bad_gateway","message":"upstream"}}`, http.StatusBadGateway)
			return
		}
		f.kata.handler.ServeHTTP(w, r)
	}), "0.20.0")
	gateway := newGateway(t, newLocator(lossy.target())).WithHost("TestBox")
	got := as[model.FileAttached](t, run(t, gateway, model.AttachFile{Workspace: f.repo, Ref: f.uid, Actor: "tester", Path: writeFile(t, t.TempDir(), "shot.png", "x")}))
	if got.Err == nil {
		t.Fatalf("got %+v, want the pointer write's failure", got)
	}
	kept, _ := filepath.Glob(filepath.Join(f.repo, ".kata-attachments", f.uid, "*"))
	if len(kept) != 1 {
		t.Fatalf("copies %v, want the one the committed pointer names", kept)
	}
	if len(f.pointers(t, f.uid)) != 1 {
		t.Fatal("kata did not commit the pointer")
	}
}

func TestDetachClearsThePointerAndDeletesTheCopy(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	got := f.detach(t, f.repo, f.uid, "shot.png")
	if got.Err != nil || got.Fate != model.FileDeleted || got.Path != attached.Path {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(attached.Path); !os.IsNotExist(err) {
		t.Fatalf("the copy survived: %v", err)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("the pointer survived")
	}
	if again := f.detach(t, f.repo, f.uid, "shot.png"); again.Err == nil || !strings.Contains(again.Err.Error(), "shot.png") {
		t.Fatalf("detaching a missing name: %+v", again)
	}
}

func TestDetachWithTheFileAlreadyGone(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	if err := os.Remove(attached.Path); err != nil {
		t.Fatal(err)
	}
	if got := f.detach(t, f.repo, f.uid, "shot.png"); got.Err != nil || got.Fate != model.FileGone {
		t.Fatalf("got %+v", got)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("the pointer survived")
	}
}

func TestDetachLeavesAFileOnAnotherHost(t *testing.T) {
	f := newAttachFixture(t)
	elsewhere := writeFile(t, t.TempDir(), "theirs.png", "x")
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{"tern-kata.attachment.theirs.png": map[string]any{"host": "some-other-host", "path": elsewhere}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.detach(t, f.repo, f.uid, "theirs.png")
	if got.Err != nil || got.Fate != model.FileLeft || got.Host != "some-other-host" {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got.Report(), "some-other-host") {
		t.Fatalf("report %q does not name the host", got.Report())
	}
	if _, err := os.Stat(elsewhere); err != nil {
		t.Fatalf("a file on another host's path was deleted: %v", err)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("the pointer survived")
	}
}

func TestAttachAndDetachTakeEveryRefForm(t *testing.T) {
	f := newAttachFixture(t)
	short := f.issue(t, f.uid).ShortID
	dir := t.TempDir()
	for i, ref := range []string{short, "demo#" + short, f.uid} {
		got := f.attach(t, f.repo, ref, writeFile(t, dir, "shot.png", "x"))
		if got.Err != nil {
			t.Fatalf("ref %q: %v", ref, got.Err)
		}
		if want := []string{"shot.png", "shot-2.png", "shot-3.png"}[i]; got.Name != want {
			t.Fatalf("ref %q stored %q, want %q", ref, got.Name, want)
		}
	}
	if got := f.detach(t, f.repo, "demo#"+short, "shot-2.png"); got.Err != nil || got.Fate != model.FileDeleted {
		t.Fatalf("%+v", got)
	}
	if got := f.attach(t, f.repo, "ghost#"+short, writeFile(t, dir, "a", "x")); got.Err == nil || !strings.Contains(got.Err.Error(), "ghost") {
		t.Fatalf("unknown project: %+v", got)
	}
}

func TestDetachIsRefusedOutsideTheIssuesBinding(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))

	other := t.TempDir()
	if saved := as[model.ProjectSaved](t, run(t, f.gateway, model.BindProject{Workspace: other, Actor: "tester", Name: "elsewhere"})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	for workspace, bound := range map[string]string{other: `"elsewhere"`, t.TempDir(): "not bound"} {
		got := f.detach(t, workspace, f.uid, "shot.png")
		if got.Err == nil || !strings.Contains(got.Err.Error(), `"demo"`) || !strings.Contains(got.Err.Error(), bound) {
			t.Fatalf("got %+v, want a refusal naming demo and %s", got, bound)
		}
	}
	if _, err := os.Stat(attached.Path); err != nil {
		t.Fatalf("a refused detach deleted the copy: %v", err)
	}
	if len(f.pointers(t, f.uid)) != 1 {
		t.Fatal("a refused detach cleared the pointer")
	}
}

func TestDetachNeverDeletesAnotherIssuesCopy(t *testing.T) {
	f := newAttachFixture(t)
	second := createIssue(t, f.gateway, f.repo, f.project, "Other")
	theirs := f.attach(t, f.repo, second, writeFile(t, t.TempDir(), "keep.png", "x"))
	if theirs.Err != nil {
		t.Fatal(theirs.Err)
	}
	host := f.host
	setPointer(t, f, "borrowed.png", map[string]any{"host": host, "path": theirs.Path})
	got := f.detach(t, f.repo, f.uid, "borrowed.png")
	if got.Err != nil || got.Fate != model.FileLeft {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(theirs.Path); err != nil {
		t.Fatalf("another Issue's copy was deleted: %v", err)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("the pointer survived")
	}
}

func TestDetachClearsAMalformedPointerAndTouchesNoFile(t *testing.T) {
	f := newAttachFixture(t)
	setPointer(t, f, "odd", "not a pointer")
	got := f.detach(t, f.repo, f.uid, "odd")
	if got.Err != nil || got.Fate != model.FileLeft || !strings.Contains(got.Report(), "no file was touched") {
		t.Fatalf("got %+v, report %q", got, got.Report())
	}
	if _, still := f.issue(t, f.uid).Metadata["tern-kata.attachment.odd"]; still {
		t.Fatal("the malformed key survived")
	}
}

func TestAttachFailsBeforeCopyingWhenTheStoreCannotBeSetUp(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory this user cannot write to")
	}
	f := newAttachFixture(t)
	store := filepath.Join(f.repo, ".kata-attachments")
	dir := filepath.Join(store, f.uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(store, 0o755) })
	if got := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "a.txt", "x")); got.Err == nil {
		t.Fatalf("attach went on without the store's .gitignore: %+v", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 || len(f.pointers(t, f.uid)) != 0 {
		t.Fatalf("a copy %v or pointer was left", left)
	}
}

// setPointer writes the tern-kata.attachment.<name> key of the fixture's Issue as any writer could.
func setPointer(t *testing.T, f attachFixture, name string, value any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{"tern-kata.attachment." + name: value}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAttachWritesExactlyHostAndPathAndDecodesBack(t *testing.T) {
	f := newAttachFixture(t)
	got := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	meta := f.issue(t, f.uid).Metadata
	if want := (map[string]any{"host": f.host, "path": got.Path}); !reflect.DeepEqual(meta["tern-kata.attachment.shot.png"], want) {
		t.Fatalf("meta value %v, want %v", meta["tern-kata.attachment.shot.png"], want)
	}
	if a := model.Attachments(meta); len(a) != 1 || a[0] != (model.Attachment{Name: "shot.png", Host: f.host, Path: got.Path}) {
		t.Fatalf("decoded %v", a)
	}
}

func TestAttachWithoutAHostNameLeavesNoCopyOrPointer(t *testing.T) {
	f := newAttachFixture(t)
	f.gateway.WithHost("")
	got := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "host name") {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".kata-attachments")); err == nil {
		t.Fatal("a store was made")
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("a pointer was written")
	}
}

func TestDetachIgnoresHostCaseAndNeverDeletesForAnEmptyHost(t *testing.T) {
	f := newAttachFixture(t)
	copy1 := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "a.png", "x"))
	setPointer(t, f, "a.png", map[string]any{"host": strings.ToUpper(f.host), "path": copy1.Path})
	if got := f.detach(t, f.repo, f.uid, "a.png"); got.Err != nil || got.Fate != model.FileDeleted {
		t.Fatalf("case-differing host: %+v", got)
	}
	copy2 := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "b.png", "x"))
	setPointer(t, f, "b.png", map[string]any{"host": "", "path": copy2.Path})
	f.gateway.WithHost("")
	if got := f.detach(t, f.repo, f.uid, "b.png"); got.Err != nil || got.Fate != model.FileLeft {
		t.Fatalf("empty host: %+v", got)
	}
	if _, err := os.Stat(copy2.Path); err != nil {
		t.Fatalf("an empty-host pointer deleted the file: %v", err)
	}
}

// racingGateway is a gateway whose daemon first runs before when a clear of the key named tern-kata.attachment.<name> arrives.
func (f attachFixture) racingGateway(t *testing.T, name string, before func()) *Gateway {
	t.Helper()
	racing := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/metadata") {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			if bytes.Contains(body, []byte(`"tern-kata.attachment.`+name+`":null`)) {
				before()
			}
		}
		f.kata.handler.ServeHTTP(w, r)
	}), "0.20.0")
	return newGateway(t, newLocator(racing.target())).WithHost(f.host)
}

// setKey writes the tern-kata.attachment.<name> meta key straight to kata, bypassing every guard; nil deletes it.
func (f attachFixture) setKey(t *testing.T, name string, value any) {
	t.Helper()
	actor := "rival"
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{"tern-kata.attachment." + name: value}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDetachClearsAKeyWhoseValueHasANonCanonicalNumber(t *testing.T) {
	for name, literal := range map[string]string{
		"bare":    `1.0`,
		"pointer": `{"host":"Elsewhere","path":"/x/y.png","n":1.0}`,
		"big":     `{"host":"Elsewhere","path":"/x/y.png","n":12345678901234567890}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newAttachFixture(t)
			setPointer(t, f, "odd", json.RawMessage(literal))
			got := f.detach(t, f.repo, f.uid, "odd")
			if got.Err != nil || !got.Cleared {
				t.Fatalf("got %+v, want the key cleared", got)
			}
			if _, still := f.issue(t, f.uid).Metadata["tern-kata.attachment.odd"]; still {
				t.Fatal("the key is still set")
			}
		})
	}
}

func TestDetachRefusesAPointerRewrittenSinceItWasRead(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	rival := map[string]any{"host": f.host, "path": "/elsewhere/shot.png"}
	gateway := f.racingGateway(t, "shot.png", func() { f.setKey(t, "shot.png", rival) })
	got := as[model.FileDetached](t, run(t, gateway, model.DetachFile{Workspace: f.repo, Ref: f.uid, Actor: "tester", Name: "shot.png"}))
	if got.Err == nil || got.Cleared || !strings.Contains(got.Err.Error(), "changed") {
		t.Fatalf("got %+v, want the changed-Attachment refusal", got)
	}
	if !reflect.DeepEqual(f.pointers(t, f.uid)["shot.png"], rival) {
		t.Fatalf("the newer pointer did not survive: %v", f.pointers(t, f.uid))
	}
	if _, err := os.Stat(attached.Path); err != nil {
		t.Fatalf("a file was touched: %v", err)
	}
}

func TestDetachOfAKeyDeletedSinceItWasReadSaysNoSuchAttachment(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	gateway := f.racingGateway(t, "shot.png", func() { f.setKey(t, "shot.png", nil) })
	got := as[model.FileDetached](t, run(t, gateway, model.DetachFile{Workspace: f.repo, Ref: f.uid, Actor: "tester", Name: "shot.png"}))
	if got.Cleared || got.Err == nil || !strings.Contains(got.Err.Error(), "has no attachment named") {
		t.Fatalf("got %+v, want the no-such-Attachment error", got)
	}
	if _, err := os.Stat(attached.Path); err != nil {
		t.Fatalf("a file was touched: %v", err)
	}
}

func TestDetachReportsACopyItCouldNotDelete(t *testing.T) {
	f := newAttachFixture(t)
	attached := f.attach(t, f.repo, f.uid, writeFile(t, t.TempDir(), "shot.png", "x"))
	dir := filepath.Dir(attached.Path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if probe, err := os.CreateTemp(dir, "probe"); err == nil {
		probe.Close()
		t.Skip("a read-only folder is not enforced here")
	}
	got := f.detach(t, f.repo, f.uid, "shot.png")
	if !got.Cleared || got.Err == nil || got.Host != f.host || got.Path != attached.Path || got.Fate != model.FileLeft {
		t.Fatalf("got %+v", got)
	}
	if want := "Removed shot.png; could not delete " + attached.Path + ": "; !strings.HasPrefix(got.Report(), want) {
		t.Fatalf("report %q, want it to start %q", got.Report(), want)
	}
	if len(f.pointers(t, f.uid)) != 0 {
		t.Fatal("the pointer survived")
	}
}
