//go:build !windows && !linux && !darwin

package worker

// startTicks is unavailable here, so a reused pid goes unguarded.
func startTicks(int) uint64 { return 0 }
