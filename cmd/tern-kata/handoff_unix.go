//go:build !windows

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"

	"github.com/stencil-hq/tern-sdk/go/tern"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func execKata(kata string, args, environ []string, actor string) error {
	return syscall.Exec(kata, append([]string{"kata", "tui"}, args...), childEnv(environ, actor, false))
}

func editorCommand(editor, file string) *exec.Cmd {
	return exec.Command("sh", "-c", editor+" \"$1\"", "tern-kata-editor", file)
}

// ttyInput reads the terminal in short polls that retry when a signal interrupts them and ends once cancelled. The SDK reads
// an *os.File this way only when its one poll of the file at start succeeded; a signal that interrupts that poll (the Go runtime
// sends them to busy threads, so a loaded machine sees it) leaves a plain blocking read that nothing but a key can end, and
// Session.Close waits for that read, so the Pane never exits after q.
type ttyInput struct {
	f      *os.File
	closed atomic.Bool
}

// ttyPollMillis bounds how long Read takes to notice a cancel.
const ttyPollMillis = 10

func (t *ttyInput) Read(p []byte) (int, error) {
	fd := int(t.f.Fd())
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // G115: a file descriptor fits in int32
	for {
		if t.closed.Load() {
			return 0, os.ErrClosed
		}
		n, err := unix.Poll(fds, ttyPollMillis)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 || fds[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
			continue
		}
		n, err = unix.Read(fd, p)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return n, nil
	}
}

func (t *ttyInput) cancel() error {
	t.closed.Store(true)
	return nil
}

// paneInput gives the SDK the interruptible input when stdin and stdout are terminals; otherwise the options stay as they are
// and Connect reports the terminal unsupported, as it does for any pipe.
func paneInput(opts *tern.Options) {
	in := int(os.Stdin.Fd())
	if !term.IsTerminal(in) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	input := &ttyInput{f: os.Stdin}
	opts.In = input
	opts.CancelRead = input.cancel
	opts.MakeRaw = func() (func() error, error) {
		state, err := term.MakeRaw(in)
		if err != nil {
			return nil, err
		}
		return func() error { return term.Restore(in, state) }, nil
	}
}
