package worker

import "time"

// Clock is the worker's only source of time and waiting, so tests run the whole worker on an injected one (ADR-0006's
// test seam). Everything that sleeps, times out or stamps a heartbeat goes through it.
type Clock interface {
	Now() time.Time
	// After delivers once d has passed on this clock.
	After(d time.Duration) <-chan time.Time
}

// SystemClock is the wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time                         { return time.Now() }
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
