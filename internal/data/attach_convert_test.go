package data

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// TestMain keeps every test that builds a Gateway without its own state folder off the real one.
func TestMain(m *testing.M) {
	state, err := os.MkdirTemp("", "tern-kata-data-state")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_STATE_HOME", state)
	_ = os.Setenv("XDG_CONFIG_HOME", state)
	_ = os.Setenv("AppData", state) // os.UserConfigDir on Windows
	code := m.Run()
	_ = os.RemoveAll(state)
	os.Exit(code)
}

const nativeKey = "attachment.01JAAAAAAAAAAAAAAAAAAAAAAA"

func nativeValue() map[string]any {
	return map[string]any{"name": "shot.png", "media_type": "image/png", "size": float64(12), "docbank_version": float64(3)}
}

// setMeta writes meta keys of an Issue straight to kata, as any writer could.
func (f attachFixture) setMeta(t *testing.T, uid string, patch map[string]any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: patch},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// watch counts the requests of the daemon that match.
func (f attachFixture) watch(match func(*http.Request) bool) *atomic.Int32 {
	var n atomic.Int32
	hook := func(_ http.ResponseWriter, r *http.Request) bool {
		if match(r) {
			n.Add(1)
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	return &n
}

func isMetadataWrite(uid string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/metadata") && strings.Contains(r.URL.Path, uid)
	}
}

func isIssueList(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues")
}

func (f attachFixture) otherIssue(t *testing.T) string {
	t.Helper()
	return createIssue(t, f.gateway, f.repo, f.project, "Other")
}

// restarted is a gateway of a later Pane or command on the same machine: a new process, the same state folder.
func (f attachFixture) restarted(t *testing.T) attachFixture {
	t.Helper()
	next := f
	next.gateway = newGateway(t, newLocator(f.kata.target())).WithHost(f.host)
	next.gateway.workerDirs = f.gateway.workerDirs
	return next
}

func markerFiles(t *testing.T, f attachFixture) []string {
	t.Helper()
	dirs, err := f.gateway.workerDirs()
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	_ = filepath.WalkDir(dirs.State, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	return found
}

func TestAttachConvertsOldPointersInOnePatchAndTouchesNothingElse(t *testing.T) {
	f := newAttachFixture(t)
	a := writeFile(t, t.TempDir(), "a.png", "aaa")
	b := writeFile(t, t.TempDir(), "b.txt", "bbb")
	f.setMeta(t, f.uid, map[string]any{
		"attachment.a.png": map[string]any{"host": f.host, "path": a},
		"attachment.b.txt": map[string]any{"host": "elsewhere", "path": b},
		nativeKey:          nativeValue(),
		"note.x":           "keep",
	})
	other := f.otherIssue(t)
	patches := f.watch(isMetadataWrite(f.uid))

	got := f.attach(t, f.repo, other, writeFile(t, t.TempDir(), "c.txt", "ccc"))
	if got.Err != nil {
		t.Fatal(got.Err)
	}

	meta := f.issue(t, f.uid).Metadata
	if want := (map[string]any{"host": f.host, "path": a}); !reflect.DeepEqual(meta["tern-kata.attachment.a.png"], want) {
		t.Fatalf("a.png moved to %v, want %v", meta["tern-kata.attachment.a.png"], want)
	}
	if want := (map[string]any{"host": "elsewhere", "path": b}); !reflect.DeepEqual(meta["tern-kata.attachment.b.txt"], want) {
		t.Fatalf("b.txt moved to %v, want %v", meta["tern-kata.attachment.b.txt"], want)
	}
	for _, old := range []string{"attachment.a.png", "attachment.b.txt"} {
		if _, still := meta[old]; still {
			t.Fatalf("%s is still set", old)
		}
	}
	if !reflect.DeepEqual(meta[nativeKey], nativeValue()) || meta["note.x"] != "keep" {
		t.Fatalf("a native-shaped or unrelated key changed: %v", meta)
	}
	if n := patches.Load(); n != 1 {
		t.Fatalf("%d metadata writes to the Issue, want 1", n)
	}
	if list := model.Attachments(meta); len(list) != 2 || list[0].Name != "a.png" || list[1].Name != "b.txt" {
		t.Fatalf("the Attachments tab lists %v", list)
	}
	for path, content := range map[string]string{a: "aaa", b: "bbb"} {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != content {
			t.Fatalf("file %s: %q, %v", path, raw, err)
		}
	}
}

func TestConversionNamesAPointerWhoseNameIsTakenWithTheNextFreeNumber(t *testing.T) {
	f := newAttachFixture(t)
	taken := map[string]any{"host": f.host, "path": "/kept/shot.png"}
	f.setMeta(t, f.uid, map[string]any{
		"tern-kata.attachment.shot.png": taken,
		"attachment.shot.png":           map[string]any{"host": f.host, "path": "/old/shot.png"},
		"attachment.shot-2.png":         map[string]any{"host": f.host, "path": "/old/shot-2.png"},
	})
	if got := f.attach(t, f.repo, f.otherIssue(t), writeFile(t, t.TempDir(), "c.txt", "c")); got.Err != nil {
		t.Fatal(got.Err)
	}
	meta := f.issue(t, f.uid).Metadata
	want := map[string]map[string]any{
		"tern-kata.attachment.shot.png":   taken,
		"tern-kata.attachment.shot-2.png": {"host": f.host, "path": "/old/shot-2.png"},
		"tern-kata.attachment.shot-3.png": {"host": f.host, "path": "/old/shot.png"},
	}
	for key, value := range want {
		if !reflect.DeepEqual(meta[key], value) {
			t.Fatalf("%s holds %v, want %v", key, meta[key], value)
		}
	}
}

func TestConversionReachesAClosedIssue(t *testing.T) {
	f := newAttachFixture(t)
	f.setMeta(t, f.uid, map[string]any{"attachment.old.png": map[string]any{"host": f.host, "path": "/old.png"}})
	reason, protocol, source := g.CloseActionRequestBodyReason("done"), g.CloseActionRequestBodyRetryProtocol("close-v1"), g.CloseActionRequestBodySource("tui")
	if _, err := f.kata.target().Client.CloseIssueWithResponse(context.Background(), &g.CloseIssueRequestOptions{
		PathParams: &g.CloseIssuePath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.CloseIssueBody{Actor: new("tester"), Reason: &reason, RetryProtocol: &protocol, Source: &source},
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.attach(t, f.repo, f.otherIssue(t), writeFile(t, t.TempDir(), "c.txt", "c")); got.Err != nil {
		t.Fatal(got.Err)
	}
	if _, moved := f.issue(t, f.uid).Metadata["tern-kata.attachment.old.png"]; !moved {
		t.Fatalf("the closed Issue still holds %v", f.issue(t, f.uid).Metadata)
	}
}

func TestConversionReadsAnIssueThatChangedBeforeItsPatchAndTriesOnce(t *testing.T) {
	f := newAttachFixture(t)
	f.setMeta(t, f.uid, map[string]any{"attachment.a.png": map[string]any{"host": f.host, "path": "/a.png"}})
	other := f.otherIssue(t)
	var patches atomic.Int32
	var changed atomic.Bool
	hook := func(_ http.ResponseWriter, r *http.Request) bool {
		if isMetadataWrite(f.uid)(r) {
			patches.Add(1)
			if changed.CompareAndSwap(false, true) {
				f.setMeta(t, f.uid, map[string]any{"changed.meanwhile": "yes"})
			}
		}
		return false
	}
	f.kata.intercept.Store(&hook)

	if got := f.attach(t, f.repo, other, writeFile(t, t.TempDir(), "c.txt", "c")); got.Err != nil {
		t.Fatal(got.Err)
	}
	meta := f.issue(t, f.uid).Metadata
	if _, moved := meta["tern-kata.attachment.a.png"]; !moved || meta["changed.meanwhile"] != "yes" {
		t.Fatalf("meta %v", meta)
	}
	if _, still := meta["attachment.a.png"]; still {
		t.Fatal("the old key survived")
	}
	// The racing write, the refused patch and the retried one.
	if n := patches.Load(); n != 3 {
		t.Fatalf("%d metadata writes to the Issue, want 3", n)
	}
}

func TestAFailedPassLeavesNoMarkerAndTheNextPassFinishesIt(t *testing.T) {
	f := newAttachFixture(t)
	f.setMeta(t, f.uid, map[string]any{"attachment.a.png": map[string]any{"host": f.host, "path": "/a.png"}})
	other := f.otherIssue(t)
	refuse := func(w http.ResponseWriter, r *http.Request) bool {
		if isMetadataWrite(f.uid)(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":500,"error":{"code":"internal","message":"boom"}}`))
			return true
		}
		return false
	}
	f.kata.intercept.Store(&refuse)

	if got := f.attach(t, f.repo, other, writeFile(t, t.TempDir(), "c.txt", "c")); got.Err != nil {
		t.Fatalf("a failed conversion must not stop the attach: %v", got.Err)
	}
	meta := f.issue(t, f.uid).Metadata
	if _, still := meta["attachment.a.png"]; !still {
		t.Fatalf("the Issue changed: %v", meta)
	}
	if found := markerFiles(t, f); len(found) != 0 {
		t.Fatalf("a failed pass left %v", found)
	}

	f.kata.intercept.Store(nil)
	next := f.restarted(t)
	if got := next.attach(t, f.repo, other, writeFile(t, t.TempDir(), "d.txt", "d")); got.Err != nil {
		t.Fatal(got.Err)
	}
	meta = f.issue(t, f.uid).Metadata
	if _, moved := meta["tern-kata.attachment.a.png"]; !moved {
		t.Fatalf("the next pass did not finish: %v", meta)
	}
	if found := markerFiles(t, f); len(found) != 1 {
		t.Fatalf("a finished pass left markers %v, want one", found)
	}

	lists := f.watch(isIssueList)
	last := next.restarted(t)
	if got := last.attach(t, f.repo, other, writeFile(t, t.TempDir(), "e.txt", "e")); got.Err != nil {
		t.Fatal(got.Err)
	}
	if n := lists.Load(); n != 0 {
		t.Fatalf("a converted Project was listed %d times on the next start", n)
	}
}

func TestDetachConvertsBeforeItActsAndFindsTheConvertedName(t *testing.T) {
	f := newAttachFixture(t)
	copied := writeFile(t, t.TempDir(), "old.png", "x")
	f.setMeta(t, f.uid, map[string]any{
		"attachment.old.png":   map[string]any{"host": "elsewhere", "path": copied},
		"attachment.other.png": map[string]any{"host": "elsewhere", "path": copied},
	})
	got := f.detach(t, f.repo, f.uid, "old.png")
	if got.Err != nil || !got.Cleared {
		t.Fatalf("got %+v", got)
	}
	meta := f.issue(t, f.uid).Metadata
	for _, key := range []string{"tern-kata.attachment.old.png", "attachment.old.png", "attachment.other.png"} {
		if _, still := meta[key]; still {
			t.Fatalf("%s survived: %v", key, meta)
		}
	}
	if _, moved := meta["tern-kata.attachment.other.png"]; !moved {
		t.Fatalf("the other pointer was not converted before the detach: %v", meta)
	}
}

func TestAPaneStartingOnAProjectConvertsItsOldPointers(t *testing.T) {
	f := newAttachFixture(t)
	f.setMeta(t, f.uid, map[string]any{"attachment.a.png": map[string]any{"host": f.host, "path": "/a.png"}})
	loaded := as[model.ProjectLoaded](t, run(t, f.gateway, model.LoadProject{Workspace: f.repo, Actor: "tester"}))
	if loaded.Project.ID != f.project.ID {
		t.Fatalf("loaded %+v", loaded)
	}
	f.gateway.background.Wait()
	if _, moved := f.issue(t, f.uid).Metadata["tern-kata.attachment.a.png"]; !moved {
		t.Fatalf("meta %v", f.issue(t, f.uid).Metadata)
	}
}
