package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The test binary is also the release binary a served archive holds. Started with TERN_KATA_TEST_BINARY set it is a helper
// process instead of the tests: with "hold" it prints
// "ready" and runs until its stdin closes, which keeps its file in use. Asked `update --preflight` it answers with what
// TERN_KATA_TEST_PREFLIGHT holds on stdout, TERN_KATA_TEST_PREFLIGHT_ERR on stderr and TERN_KATA_TEST_PREFLIGHT_EXIT as its
// exit status; TERN_KATA_TEST_PREFLIGHT_HANG names a file it creates, then runs until it is killed.
func init() {
	mode := os.Getenv("TERN_KATA_TEST_BINARY")
	if mode == "" {
		return
	}
	if mode == "hold" {
		fmt.Println("ready")
		_, _ = os.Stdin.Read(make([]byte, 1))
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "update" && os.Args[2] == "--preflight" {
		if marker := os.Getenv("TERN_KATA_TEST_PREFLIGHT_HANG"); marker != "" {
			_ = os.WriteFile(marker, nil, 0o600)
			time.Sleep(time.Minute)
		}
		fmt.Print(os.Getenv("TERN_KATA_TEST_PREFLIGHT"))
		fmt.Fprint(os.Stderr, os.Getenv("TERN_KATA_TEST_PREFLIGHT_ERR"))
		status, _ := strconv.Atoi(os.Getenv("TERN_KATA_TEST_PREFLIGHT_EXIT"))
		os.Exit(status)
	}
	os.Exit(2)
}

// testRelease is the archive of the test binary under this OS's archive name, and the text of checksums.txt for it.
var testRelease = sync.OnceValues(func() ([]byte, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(binary)
	if err != nil {
		return nil, err
	}
	env := updateEnv{}
	files := map[string][]byte{env.binaryName(): content, "LICENSE": []byte("license"), "plugin/plugin.toml": []byte("version = \"0.0.0\"\n")}
	var archive bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&archive)
		for name, data := range files {
			w, err := zw.Create(name)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(data); err != nil {
				return nil, err
			}
		}
		err = zw.Close()
		return archive.Bytes(), err
	}
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	err = gz.Close()
	return archive.Bytes(), err
})

// updateFixture is a machine with an installed tern-kata at the version it reports and a served dist/.
type updateFixture struct {
	env      updateEnv
	target   string
	state    string
	requests *atomic.Int32
	out      bytes.Buffer
	ternLog  string
}

const oldBinaryText = "the installed binary"

// newUpdateFixture installs a stand-in binary reporting current, and serves a dist/ whose version.txt says latest; spoil
// changes checksums.txt's text, to break the archive's line.
func newUpdateFixture(t *testing.T, current, latest string, spoil func(string) string) *updateFixture {
	t.Helper()
	archive, err := testRelease()
	if err != nil {
		t.Fatal(err)
	}
	env := updateEnv{}
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  " + env.archiveName() + "\n"
	if spoil != nil {
		sums = spoil(sums)
	}
	var requests atomic.Int32
	dist := http.NewServeMux()
	dist.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, latest) })
	dist.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sums) })
	dist.HandleFunc("/"+env.archiveName(), func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		dist.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("TERN_KATA_BASE_URL", server.URL)

	root := t.TempDir()
	f := &updateFixture{target: filepath.Join(root, "bin", env.binaryName()), state: filepath.Join(root, "state"), requests: &requests}
	if err := os.MkdirAll(filepath.Dir(f.target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.target, []byte(oldBinaryText), 0o755); err != nil {
		t.Fatal(err)
	}
	f.env, err = newUpdateEnv()
	if err != nil {
		t.Fatal(err)
	}
	f.env.Dirs = worker.Dirs{Config: filepath.Join(root, "config"), State: f.state}
	f.env.Release, f.env.Build = current, current
	f.env.Executable = func() (string, error) { return f.target, nil }
	f.env.Client = server.Client()
	f.env.Stdin = strings.NewReader("")
	t.Setenv("TERN_KATA_TEST_BINARY", latest) // the swapped-in binary is a helper process, which answers a pre-flight healthy unless a test says otherwise
	f.preflight(t, `{"refusals":[],"warnings":[]}`, 0, "")
	f.fakeTern(t, t.TempDir(), 0) // the plugin step finds an empty plugin folder, never this machine's
	return f
}

// preflight sets what the downloaded binary answers to `update --preflight`.
func (f *updateFixture) preflight(t *testing.T, stdout string, status int, stderr string) {
	t.Helper()
	t.Setenv("TERN_KATA_TEST_PREFLIGHT", stdout)
	t.Setenv("TERN_KATA_TEST_PREFLIGHT_EXIT", strconv.Itoa(status))
	t.Setenv("TERN_KATA_TEST_PREFLIGHT_ERR", stderr)
	t.Setenv("TERN_KATA_TEST_PREFLIGHT_HANG", "")
}

func (f *updateFixture) run(plan LaunchPlan) error {
	f.out.Reset()
	return runUpdate(context.Background(), plan, f.env, &f.out)
}

// unchanged fails the test unless the installed binary is as it was and no folder of the update is left beside it.
func (f *updateFixture) unchanged(t *testing.T) {
	t.Helper()
	if got, err := os.ReadFile(f.target); err != nil || string(got) != oldBinaryText {
		t.Errorf("the binary changed: %q, %v", got, err)
	}
	f.noLeftovers(t)
}

func (f *updateFixture) noLeftovers(t *testing.T) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(f.target), ".tern-kata-update-*"))
	if len(left) > 0 {
		t.Errorf("the update left %v", left)
	}
}

func TestUpdateSaysUpToDateAndChangesNothing(t *testing.T) {
	for _, latest := range []string{"0.3.0", "0.2.9"} {
		f := newUpdateFixture(t, "v0.3.0", latest, nil)
		if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
			t.Fatalf("latest %s: %v", latest, err)
		}
		if got := f.out.String(); got != "tern-kata v0.3.0 is up to date\n" {
			t.Errorf("latest %s: output %q", latest, got)
		}
		if f.requests.Load() != 1 {
			t.Errorf("latest %s: %d requests, want only version.txt", latest, f.requests.Load())
		}
		f.unchanged(t)
	}
}

func TestUpdateSwapsTheBinaryForTheLatestRelease(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0\n", nil)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Updated tern-kata from v0.1.0 to v0.2.0", "Reopen tern-kata Panes to use v0.2.0", "npx skills update -g", "bunx skills update -g"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, f.out.String())
		}
	}
	binary, _ := os.Executable()
	want, _ := os.ReadFile(binary)
	if got, err := os.ReadFile(f.target); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("the binary is not the release's (%d bytes, %v)", len(got), err)
	}
	f.noLeftovers(t)
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(f.target); err != nil || info.Mode()&0o111 == 0 {
			t.Errorf("the new binary is not executable: %v, %v", info, err)
		}
		if entries, _ := os.ReadDir(filepath.Dir(f.target)); len(entries) != 1 {
			t.Errorf("the binary's folder holds %d entries, want only the binary", len(entries))
		}
	}
}

func TestUpdateRefusesADownloadThatFailsItsChecksum(t *testing.T) {
	for name, spoil := range map[string]func(string) string{
		"wrong hash":    func(s string) string { return strings.Repeat("0", 64) + s[64:] },
		"no line": func(string) string { return "" },
	} {
		f := newUpdateFixture(t, "v0.1.0", "0.2.0", spoil)
		err := f.run(LaunchPlan{Subcommand: "update"})
		if err == nil || !strings.Contains(err.Error(), "nothing was installed") || exitStatus(err) != 1 {
			t.Errorf("%s: error %v (exit %d)", name, err, exitStatus(err))
		}
		f.unchanged(t)
	}
}

func TestUpdateReportsAnArchiveThatCannotBeDownloaded(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	upstream, err := url.Parse(f.env.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, f.env.archiveName()) {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	f.env.BaseURL = server.URL
	f.env.Client = server.Client()
	err = f.run(LaunchPlan{Subcommand: "update"})
	if err == nil || !strings.Contains(err.Error(), "nothing was installed") || !strings.Contains(err.Error(), "404") || exitStatus(err) != 1 {
		t.Errorf("error %v (exit %d)", err, exitStatus(err))
	}
	f.unchanged(t)
}

func TestUpdateLeavesBuildsThatAreNotReleasesAlone(t *testing.T) {
	for _, tc := range []struct{ build, want string }{
		{"v0.9.0", "go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest"},
		{"dev", "development build"},
	} {
		f := newUpdateFixture(t, "", "0.2.0", nil)
		f.env.Build = tc.build
		err := f.run(LaunchPlan{Subcommand: "update"})
		if err == nil || !strings.Contains(err.Error(), tc.want) || exitStatus(err) != 1 {
			t.Errorf("build %q: error %v (exit %d), want %q", tc.build, err, exitStatus(err), tc.want)
		}
		if f.requests.Load() != 0 {
			t.Errorf("build %q: asked the network", tc.build)
		}
		f.unchanged(t)
	}
}

func TestUpdateIsRefusedWhileAnotherUpdateRuns(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	unlock, err := lockUpdate(f.state)
	if err != nil {
		t.Fatal(err)
	}
	err = f.run(LaunchPlan{Subcommand: "update"})
	if err == nil || err.Error() != "another tern-kata update is running" || exitStatus(err) != 1 {
		t.Errorf("error %v (exit %d)", err, exitStatus(err))
	}
	if f.requests.Load() != 0 {
		t.Error("a refused update asked the network")
	}
	f.unchanged(t)
	unlock()
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Errorf("after the lock was released: %v", err)
	}
}

func TestUpdatePauseWaitsForEnterOnlyOnSuccess(t *testing.T) {
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	stdin := strings.NewReader("\n")
	f.env.Stdin = stdin
	if err := f.run(LaunchPlan{Subcommand: "update", Pause: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.out.String(), "Press Enter to close\n") || stdin.Len() != 0 {
		t.Errorf("a successful update did not wait for Enter (unread %d):\n%s", stdin.Len(), f.out.String())
	}

	f = newUpdateFixture(t, "v0.1.0", "0.2.0", func(string) string { return "" })
	stdin = strings.NewReader("\n")
	f.env.Stdin = stdin
	if err := f.run(LaunchPlan{Subcommand: "update", Pause: true}); err == nil {
		t.Fatal("a failed update succeeded")
	}
	if strings.Contains(f.out.String(), "Press Enter") || stdin.Len() != 1 {
		t.Errorf("a failed update waited for Enter:\n%s", f.out.String())
	}
}

func TestUpdateCommandLine(t *testing.T) {
	plan, err := PlanLaunch([]string{"update", "--force", "--pause"}, lookupFrom(nil), "v")
	if err != nil || plan.Subcommand != "update" || !plan.Force || !plan.Pause {
		t.Errorf("update --force --pause: %+v, %v", plan, err)
	}
	if plan, err := PlanLaunch([]string{"update"}, lookupFrom(nil), "v"); err != nil || plan.Force || plan.Pause {
		t.Errorf("update: %+v, %v", plan, err)
	}
	for args, want := range map[string]string{
		"update now":    "usage: tern-kata update [--force] [--pause]",
		"update --as x": "update: unknown flag --as",
		"update --to 1": "update: unknown flag --to",
	} {
		if _, err := PlanLaunch(strings.Fields(args), lookupFrom(nil), "v"); err == nil || err.Error() != want {
			t.Errorf("%s: error %v, want %q", args, err, want)
		}
	}
	if plan, _ := PlanLaunch([]string{"update", "--help"}, lookupFrom(nil), "v"); !plan.Help || !strings.Contains(usage, "tern-kata update [--force] [--pause]") {
		t.Error("--help does not list update")
	}
	if got := strings.Join(complete(t, "upd"), " "); got != "update" {
		t.Errorf("tern-kata upd<tab> = %q", got)
	}
	if got := strings.Join(complete(t, "update", "--"), " "); got != "--force --pause" {
		t.Errorf("tern-kata update --<tab> = %q", got)
	}
}
