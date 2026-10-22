//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// heightChanges signals each time the Pane's terminal is resized: SIGWINCH, which also arrives when only the height changed.
func heightChanges(ctx context.Context) <-chan struct{} {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	changes := make(chan struct{}, 1)
	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-signals:
				select {
				case changes <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return changes
}
