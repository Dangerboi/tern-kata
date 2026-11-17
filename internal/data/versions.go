package data

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.kenn.io/kit/consolewindow"
	"golang.org/x/mod/semver"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/filelock"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// The version check answers the Pane's CheckVersions with what its session host holds. It runs a program again only when its
// file changed since a run of it worked: a minute's check is two stats, the release check's small file and one daemon ping. A
// run that failed is tried at every check until one works.

// fileStamp tells a file from its replacement without reading it.
type fileStamp struct {
	size, modNanos int64
}

func stampOf(path string) (fileStamp, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, false
	}
	return fileStamp{info.Size(), info.ModTime().UnixNano()}, true
}

// probed is the version a program said when its file at path had stamp; "" when it named none.
type probed struct {
	path    string
	stamp   fileStamp
	version string
}

// versionWatch is what the check keeps between checks. One check reads it at a time.
type versionWatch struct {
	mu sync.Mutex
	// exe is the Pane's binary and started its stamp when the Pane started; "" watches no binary.
	exe          string
	started      fileStamp
	binary, kata probed
}

// WithExecutable watches the binary at path (after symlinks, the file `tern-kata update` replaces), as it is now, for the
// version check: a later file there is the binary the Pane would run once reopened. Taking the path at the start also keeps the
// file the update installs in view on Windows, where the running binary is moved aside.
func (g *Gateway) WithExecutable(path string) *Gateway {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if stamp, ok := stampOf(path); ok {
		g.versions.exe, g.versions.started = path, stamp
	}
	return g
}

const (
	// versionRunTimeout bounds a program's answer to --version or `version --json`.
	versionRunTimeout = 10 * time.Second
	// releaseCheckTimeout bounds `kata update --check`, which asks GitHub.
	releaseCheckTimeout = 30 * time.Second
	// releaseCheckEvery is how often a machine asks for a new kata release.
	releaseCheckEvery = 24 * time.Hour
	// noUpdateCheck turns the release check off, as it does the window plugin's.
	noUpdateCheck = "TERN_KATA_NO_UPDATE_CHECK"
	// releaseCheckFile keeps the release check's time and answer in tern-kata's config folder, for every Pane on the machine.
	releaseCheckFile = "kata-release-check.json"
)

func (g *Gateway) checkVersions(ctx context.Context, cmd model.CheckVersions) model.VersionsChecked {
	g.pause(ctx, cmd.After)
	out := model.VersionsChecked{Request: cmd.Request, Connection: cmd.Connection}
	w := &g.versions
	w.mu.Lock()
	out.Installed = w.installed(ctx)
	if kata := kataExecutable(); kata != "" {
		out.KataCLI = w.kataVersion(ctx, kata)
		if os.Getenv(noUpdateCheck) == "" {
			out.KataLatest = g.kataRelease(ctx, kata)
		}
	}
	w.mu.Unlock()
	if cmd.Workspace != "" {
		out.Daemon = g.daemonVersion(ctx, cmd.Workspace)
	}
	return out
}

// installed is the version of the binary now at the Pane's path, "" while it is the file the Pane started from. A file missing
// for the moment (mid-swap), or one whose --version failed (a timeout, a copy still being written), keeps what was read before,
// and a failed run is tried again at the next check.
func (w *versionWatch) installed(ctx context.Context) string {
	if w.exe == "" {
		return ""
	}
	stamp, ok := stampOf(w.exe)
	switch {
	case !ok:
	case stamp == w.started:
		w.binary = probed{}
	case stamp != w.binary.stamp:
		if version, ran := ternKataVersion(ctx, w.exe); ran {
			w.binary = probed{stamp: stamp, version: version}
		}
	}
	return w.binary.version
}

// ternKataVersion is what `<exe> --version` names ("tern-kata v0.16.0"), "" when it names none; ran is false when the run
// itself failed.
func ternKataVersion(ctx context.Context, exe string) (version string, ran bool) {
	out, err := runVersion(ctx, versionRunTimeout, exe, "--version")
	if err != nil {
		return "", false
	}
	name, version, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok || name != "tern-kata" || strings.ContainsAny(version, " \n") {
		return "", true
	}
	return version, true
}

// kataVersion is the version of the kata at path, read again only when that file changed; a failed run is tried again at the
// next check, the version read before at the same path standing meanwhile.
func (w *versionWatch) kataVersion(ctx context.Context, kata string) string {
	stamp, ok := stampOf(kata)
	if !ok {
		return ""
	}
	if w.kata.path == kata && w.kata.stamp == stamp {
		return w.kata.version
	}
	if version, ran := kataCLIVersion(ctx, kata); ran {
		w.kata = probed{path: kata, stamp: stamp, version: version}
	} else if w.kata.path != kata {
		return ""
	}
	return w.kata.version
}

// kataCLIVersion is the version `kata version --json` names, "" when it names none; ran is false when the run itself failed.
func kataCLIVersion(ctx context.Context, kata string) (version string, ran bool) {
	out, err := runVersion(ctx, versionRunTimeout, kata, "version", "--json")
	if err != nil {
		return "", false
	}
	var answer struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(out, &answer) != nil {
		return "", true
	}
	return semverOf(answer.Version), true
}

// semverOf is version with a leading v, "" when that is no semantic version.
func semverOf(version string) string {
	if version = "v" + strings.TrimPrefix(version, "v"); semver.IsValid(version) {
		return version
	}
	return ""
}

// releaseCheck is the daily kata release check's last time and answer: Latest is the newer release it found, "" when none.
type releaseCheck struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest,omitempty"`
}

func readReleaseCheck(path string) releaseCheck {
	var state releaseCheck
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	return state
}

func (g *Gateway) releaseCheckDue(state releaseCheck) bool {
	since := g.now().Sub(state.Checked)
	return since < 0 || since >= releaseCheckEvery
}

// kataRelease is the newer kata release the daily check found. The check runs when the last one on this machine is a day old,
// under a lock so two Panes never ask together (one that finds the lock taken reads the last answer); a failed check counts as
// one and keeps the last answer.
func (g *Gateway) kataRelease(ctx context.Context, kata string) string {
	dirs, _ := g.workerDirs()
	if dirs.Config == "" {
		return ""
	}
	path := filepath.Join(dirs.Config, releaseCheckFile)
	state := readReleaseCheck(path)
	if !g.releaseCheckDue(state) {
		return state.Latest
	}
	if os.MkdirAll(dirs.Config, 0o700) != nil {
		return state.Latest
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return state.Latest
	}
	defer lock.Close()
	if filelock.TryLock(lock) != nil {
		return state.Latest
	}
	defer filelock.Unlock(lock)
	if state = readReleaseCheck(path); !g.releaseCheckDue(state) {
		return state.Latest
	}
	state.Checked = g.now()
	if latest, ok := askKataRelease(ctx, kata); ok {
		state.Latest = latest
	}
	if raw, err := json.Marshal(state); err == nil {
		_ = atomicfile.Write(path, raw, atomicfile.Options{Mode: 0o600})
	}
	return state.Latest
}

// askKataRelease runs `kata update --check --json`: the newer release it names, "" when kata is up to date.
func askKataRelease(ctx context.Context, kata string) (string, bool) {
	out, err := runVersion(ctx, releaseCheckTimeout, kata, "update", "--check", "--json")
	if err != nil {
		return "", false
	}
	var answer struct {
		UpdateAvailable bool   `json:"update_available"`
		LatestVersion   string `json:"latest_version"`
	}
	if json.Unmarshal(out, &answer) != nil {
		return "", false
	}
	if !answer.UpdateAvailable {
		return "", true
	}
	return semverOf(answer.LatestVersion), true
}

// daemonVersion is the version the daemon for workspace says now, "" when it cannot be asked.
func (g *Gateway) daemonVersion(ctx context.Context, workspace string) string {
	read, err := callAs(g, ctx, workspace, func(ctx context.Context, c *Client) (model.VersionsChecked, error) {
		ping, err := c.PingWithResponse(ctx)
		if err != nil {
			return model.VersionsChecked{}, err
		}
		return model.VersionsChecked{Daemon: semverOf(ping.JSON200.Version)}, nil
	})
	if err != nil {
		return ""
	}
	return read.Daemon
}

// runVersion runs a program that only reports and returns its standard output.
func runVersion(ctx context.Context, timeout time.Duration, exe string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.WaitDelay = kataRunWaitDelay
	consolewindow.Hide(cmd)
	return cmd.Output()
}
