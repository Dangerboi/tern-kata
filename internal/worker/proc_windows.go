//go:build windows

package worker

import "os/exec"

// unsupported is why the worker runs no agent here: proving an agent's process tree gone needs a Job Object
// (ADR-0006), which is not built yet. `worker status` and `worker doctor` say so.
const unsupported = "running agents is not supported on Windows yet: the worker cannot prove a process tree gone without a Job Object"

func isolate(*exec.Cmd)                        {}
func signalGroup(int, bool)                    {}
func groupAlive(int) bool                      { return false }
func startTicks(int) uint64                    { return 0 }
func exitSignal(interface{ Sys() any }) string { return "" }
