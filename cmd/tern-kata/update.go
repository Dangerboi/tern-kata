package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/mod/semver"

	"github.com/Dangerboi/tern-kata/internal/filelock"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// Where the release's files live on GitHub: the newest release, and a given one below v<version>.
const (
	latestBase  = "https://github.com/Dangerboi/tern-kata/releases/latest/download"
	releaseBase = "https://github.com/Dangerboi/tern-kata/releases/download"
)

// goInstallCommand is what a user of a `go install` build runs to update.
const goInstallCommand = "go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest"

// updateEnv is what `tern-kata update` needs from the machine, so tests give it their own folders, binary and release server.
type updateEnv struct {
	Dirs worker.Dirs
	// Release is the version the linker stamped, empty for a build that is not a release; Build is buildVersion(), which also
	// names a `go install` module version.
	Release, Build string
	// Executable is the running binary's path; update replaces that file, after resolving symlinks.
	Executable func() (string, error)
	// BaseURL is TERN_KATA_BASE_URL: when set, version.txt, the archive and checksums.txt all come from it.
	BaseURL      string
	Client       *http.Client
	GOOS, GOARCH string
	// Stdin is what --pause waits on.
	Stdin io.Reader
}

// newUpdateEnv is the production environment: tern-kata's user folders, this binary and the real network.
func newUpdateEnv() (updateEnv, error) {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return updateEnv{}, subcommandError{err}
	}
	return updateEnv{
		Dirs: dirs, Release: version, Build: buildVersion(), Executable: os.Executable,
		BaseURL: os.Getenv("TERN_KATA_BASE_URL"), Client: &http.Client{Timeout: 10 * time.Minute},
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Stdin: os.Stdin,
	}, nil
}

// runUpdate is `tern-kata update`: it brings this binary to the latest release and prints what it did. Every failure is a
// subcommandError (exit 1), and nothing is changed before the swap. The order is the contract with the steps later releases
// add: they run between downloadRelease and swapBinary (checks of this machine, which may refuse) and after it (this
// machine's plugin and Worker).
func runUpdate(ctx context.Context, plan LaunchPlan, env updateEnv, out io.Writer) error {
	if err := updateRelease(ctx, env, out); err != nil {
		return subcommandError{err} // a failed update ends at once, whatever --pause says
	}
	if plan.Pause {
		fmt.Fprintln(out, "Press Enter to close")
		_, _ = bufio.NewReader(env.Stdin).ReadString('\n')
	}
	return nil
}

func updateRelease(ctx context.Context, env updateEnv, out io.Writer) error {
	if env.Release == "" {
		if env.Build != "dev" {
			return fmt.Errorf("this tern-kata was built with go install and is not updated here; update it with: %s", goInstallCommand)
		}
		return errors.New("this is a development build and is not updated here; rebuild it from source")
	}
	current := env.Release
	if !semver.IsValid(current) {
		return fmt.Errorf("this tern-kata's version %q is not a version number, so it cannot be compared with the latest release", current)
	}
	unlock, err := lockUpdate(env.Dirs.State)
	if err != nil {
		return err
	}
	defer unlock()

	target, err := env.target()
	if err != nil {
		return err
	}
	latest, err := fetchLatestVersion(ctx, env)
	if err != nil {
		return err
	}
	if semver.Compare(latest, current) <= 0 { // never a downgrade
		fmt.Fprintf(out, "tern-kata %s is up to date\n", current)
		return nil
	}

	work, err := os.MkdirTemp(filepath.Dir(target), ".tern-kata-update-") // beside the binary: the swap's renames stay on one volume
	if err != nil {
		return fmt.Errorf("cannot prepare the update next to %s: %w; nothing was installed", target, err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	release, err := downloadRelease(ctx, env, latest, work)
	if err != nil {
		return fmt.Errorf("%w; nothing was installed", err)
	}
	if err := swapBinary(target, release.Binary); err != nil {
		return fmt.Errorf("cannot replace %s: %w; nothing was installed", target, err)
	}

	fmt.Fprintf(out, "Updated tern-kata from %s to %s\n", current, latest)
	fmt.Fprintf(out, "Reopen tern-kata Panes to use %s\n", latest)
	fmt.Fprintln(out, "To update the agent skills too, run: npx skills update -g (or bunx skills update -g)")
	return nil
}

// target is the file update replaces: the running binary, with symlinks resolved, so a link to it is left in place.
func (e updateEnv) target() (string, error) {
	path, err := e.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find this tern-kata's file: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("cannot find this tern-kata's file: %w", err)
	}
	return resolved, nil
}

// lockUpdate takes <State>/update.lock without waiting, so two updates never interleave their swaps; the returned function
// drops it.
func lockUpdate(state string) (func(), error) {
	if err := os.MkdirAll(state, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(state, "update.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := filelock.TryLock(file); err != nil {
		_ = file.Close()
		if errors.Is(err, filelock.ErrLocked) {
			return nil, errors.New("another tern-kata update is running")
		}
		return nil, err
	}
	return func() {
		filelock.Unlock(file)
		_ = file.Close()
	}, nil
}
