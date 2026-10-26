package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// stopSignals end a command early: an interrupt, a terminate, and the hangup a closed terminal sends.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// paneStopSignals end the Pane: a terminate or a hangup, as when its terminal goes. Ctrl+c reaches the Pane as a key, and an
// editor the Pane hands the terminal to gets its own.
var paneStopSignals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP}

// withSignals is parent ended by the first of signals as well; received is that signal, nil until one came, and stop
// gives the signals back to their default handling. Only the first is caught: a second one ends the process at once, so a
// command that does not wind down can still be killed with another ctrl+c.
func withSignals(parent context.Context, signals ...os.Signal) (ctx context.Context, received func() os.Signal, stop func()) {
	ctx, cancel := context.WithCancel(parent)
	arrived := make(chan os.Signal, 1)
	signal.Notify(arrived, signals...)
	var got atomic.Pointer[os.Signal]
	go func() {
		select {
		case s := <-arrived:
			signal.Stop(arrived)
			got.Store(&s)
			cancel()
		case <-ctx.Done():
		}
	}()
	received = func() os.Signal {
		if s := got.Load(); s != nil {
			return *s
		}
		return nil
	}
	return ctx, received, func() { signal.Stop(arrived); cancel() }
}

// interrupted is a command a signal stopped before it finished: main exits 128 plus the signal's number, as a shell reports a
// command a signal ended.
type interrupted struct {
	signal os.Signal
	err    error
}

func (e interrupted) Error() string {
	if e.err == nil {
		return e.signal.String()
	}
	return e.signal.String() + ": " + e.err.Error()
}

func (e interrupted) Unwrap() error { return e.err }

func (e interrupted) status() int {
	if s, ok := e.signal.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}
