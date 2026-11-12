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
	"time"

	"golang.org/x/mod/semver"

	"github.com/Dangerboi/tern-kata/internal/filelock"
	"github.com/Dangerboi/tern-kata/internal/progress"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

const (
	latestBase  = "https://github.com/Dangerboi/tern-kata/releases/latest/download"
	releaseBase = "https://github.com/Dangerboi/tern-kata/releases/download"
)

const goInstallCommand = "go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest"

type updateEnv struct {
	Dirs worker.Dirs
	// Release is the version the linker stamped, empty for a build that is not a release; Build is buildVersion(), which also
	// names a `go install` module version.
	Release, Build string
	// Executable is the running binary's path; update replaces that file, after resolving symlinks.
	Executable func() (string, error)
	// BaseURL is TERN_KATA_BASE_URL: when set, version.txt, the archive and checksums.txt all come from it.
	BaseURL string
	Client  *http.Client
	Stdin   io.Reader
	// PreflightTimeout is how long the new binary may take to check this machine; zero is preflightTimeout.
	PreflightTimeout time.Duration
	// Progress reports the download and swap to Tern's progress bar; nil (and any Reporter off a terminal) writes nothing.
	Progress *progress.Reporter
}

func newUpdateEnv() (updateEnv, error) {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return updateEnv{}, subcommandError{err}
	}
	return updateEnv{
		Dirs: dirs, Release: version, Build: buildVersion(), Executable: os.Executable,
		BaseURL: os.Getenv("TERN_KATA_BASE_URL"), Client: &http.Client{Timeout: 10 * time.Minute},
		Stdin: os.Stdin, Progress: progress.New(os.Stdout),
	}, nil
}

// runUpdate is `tern-kata update`: it brings this binary to the latest release and prints what it did. Every failure is a
// subcommandError (exit 1), and nothing is changed before the swap. The order: read the latest version; when it is
// newer, download and check the release, run its pre-flight (which may refuse this machine, or be forced past) and swap
// the binary; then update this machine's plugin copy, which also runs when the binary is already the latest, and give the
// supervised Worker an idle stop.
func runUpdate(ctx context.Context, plan LaunchPlan, env updateEnv, out io.Writer) error {
	if err := updateRelease(ctx, env, plan.Force, out); err != nil {
		return subcommandError{err} // a failed update ends at once, whatever --pause says
	}
	if plan.Pause {
		fmt.Fprintln(out, "Press Enter to close")
		_, _ = bufio.NewReader(env.Stdin).ReadString('\n')
	}
	return nil
}

func updateRelease(ctx context.Context, env updateEnv, force bool, out io.Writer) error {
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
	// The plugin download of the up-to-date path reports too, so the bar is cleared on every way out, before --pause's prompt.
	defer env.Progress.Clear()

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
		updatePlugin(ctx, current, func() (unpackedRelease, func(), error) { return downloadBesideBinary(ctx, env, target, current) }, out)
		return nil
	}

	release, cleanup, err := downloadBesideBinary(ctx, env, target, latest) // beside the binary: the swap's renames stay on one volume
	if err != nil {
		return fmt.Errorf("%w; nothing was installed", err)
	}
	defer cleanup()
	hold, err := runPreflightOf(ctx, env, release, force, out)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errInterrupted
	}
	if err := swapBinary(target, release.Binary); err != nil {
		return fmt.Errorf("cannot replace %s: %w; nothing was installed", target, err)
	}
	env.Progress.Clear()

	fmt.Fprintf(out, "Updated tern-kata from %s to %s\n", current, latest)
	updatePlugin(ctx, latest, func() (unpackedRelease, func(), error) { return release, func() {}, nil }, out)
	updateWorker(ctx, env, latest, hold, out)
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

func downloadBesideBinary(ctx context.Context, env updateEnv, target, version string) (unpackedRelease, func(), error) {
	work, err := os.MkdirTemp(filepath.Dir(target), ".tern-kata-update-")
	if err != nil {
		return unpackedRelease{}, nil, fmt.Errorf("cannot prepare a download next to %s: %w", target, err)
	}
	cleanup := func() { _ = os.RemoveAll(work) }
	release, err := downloadRelease(ctx, env, version, work)
	if err != nil {
		cleanup()
		return unpackedRelease{}, nil, err
	}
	return release, cleanup, nil
}
