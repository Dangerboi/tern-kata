package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
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
}

func newAttachFixture(t *testing.T) attachFixture {
	t.Helper()
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	saved := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: repo, Actor: "tester", Name: "demo"}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	return attachFixture{kata, gateway, repo, createIssue(t, gateway, repo, saved.Project, "Screenshot"), saved.Project}
}

func (f attachFixture) attach(t *testing.T, workspace, ref, path string) model.FileAttached {
	t.Helper()
	return as[model.FileAttached](t, run(t, f.gateway, model.AttachFile{Workspace: workspace, Ref: ref, Actor: "tester", Path: path}))
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

// pointers is the Issue's attachment.* meta as kata holds it, by stored name.
func (f attachFixture) pointers(t *testing.T, uid string) map[string]map[string]any {
	t.Helper()
	pointers := map[string]map[string]any{}
	for key, value := range f.issue(t, uid).Metadata {
		if name, ok := strings.CutPrefix(key, "attachment."); ok {
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

func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
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
	host, _ := os.Hostname()
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

func TestAttachFromALinkedWorktreeUsesThePrimaryCheckout(t *testing.T) {
	f := newAttachFixture(t)
	worktree := filepath.Join(t.TempDir(), "linked")
	git(t, f.repo, "worktree", "add", "-q", worktree)
	// The Binding is committed in a real repository, so a worktree carries it.
	if raw, err := os.ReadFile(filepath.Join(f.repo, ".kata.toml")); err != nil {
		t.Fatal(err)
	} else {
		writeFile(t, worktree, ".kata.toml", string(raw))
	}
	got := f.attach(t, worktree, f.issue(t, f.uid).ShortID, writeFile(t, t.TempDir(), "shot.png", "x"))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if want := filepath.Join(f.repo, ".kata-attachments", f.uid, "shot.png"); got.Path != want {
		t.Fatalf("stored at %s, want %s", got.Path, want)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".kata-attachments")); !os.IsNotExist(err) {
		t.Fatalf("the linked worktree got a store: %v", err)
	}
}

func TestAttachOutsideGitUsesTheDirectoryHoldingTheBinding(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "plain")
	uid := createIssue(t, gateway, workspace, bound, "Plain")
	sub := filepath.Join(workspace, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := as[model.FileAttached](t, run(t, gateway, model.AttachFile{Workspace: sub, Ref: uid, Actor: "tester", Path: writeFile(t, t.TempDir(), "log.txt", "x")}))
	if want := filepath.Join(workspace, ".kata-attachments", uid, "log.txt"); got.Err != nil || got.Path != want {
		t.Fatalf("got %+v, want %s", got, want)
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
			results[i] = f.attach(t, f.repo, f.uid, source)
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
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/metadata") {
			http.Error(w, `{"status":500,"error":{"code":"internal","message":"disk full"}}`, http.StatusInternalServerError)
			return
		}
		f.kata.handler.ServeHTTP(w, r)
	}))
	t.Cleanup(broken.Close)
	c, err := client.New(broken.URL)
	if err != nil {
		t.Fatal(err)
	}
	gateway := newGateway(t, newLocator(Target{Client: c}))
	got := as[model.FileAttached](t, run(t, gateway, model.AttachFile{Workspace: f.repo, Ref: f.uid, Actor: "tester", Path: writeFile(t, t.TempDir(), "shot.png", "x")}))
	if got.Err == nil {
		t.Fatalf("got %+v, want the pointer write's failure", got)
	}
	left, _ := filepath.Glob(filepath.Join(f.repo, ".kata-attachments", f.uid, "*"))
	if len(left) != 0 {
		t.Fatalf("copies left behind: %v", left)
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
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{"attachment.theirs.png": map[string]any{"host": "some-other-host", "path": elsewhere}}},
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
		if want := numberedName("shot.png", i+1); got.Name != want {
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
