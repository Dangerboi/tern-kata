//go:build windows

package main

import (
	"context"
	"time"
)

// heightChanges ticks twice a second: a Windows console has no resize signal, so the Pane's height is read on each tick.
func heightChanges(ctx context.Context) <-chan struct{} {
	changes := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case changes <- struct{}{}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return changes
}
