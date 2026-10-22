#!/usr/bin/env bash
# The plugin's host half supervises the worker with no Tern window: it starts `tern-kata worker` in a headless session
# daemon, restarts it with backoff, and starts it again after a daemon restart. Own temp state; no window is ever opened.
# Usage: smoke/supervision.sh [path to a built tern-kata]
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
daemon_pid=
tern_bin=$(command -v tern)
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" TERN_CONFIG_DIR="$tmp/config" TERN_DAEMON_SOCKET="$tmp/daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs" XDG_CONFIG_HOME="$tmp/xdg-config" XDG_STATE_HOME="$tmp/xdg-state"
export STENCIL_LOG=warn,tern::plugin=warn
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP TERN_PANE_SOCKET TERN_WINDOW_SOCKET
state="$XDG_STATE_HOME/tern-kata"
cleanup() {
  status=$?
  trap - EXIT
  stop_daemon
  # A worker the daemon's death left behind is the smoke's own: end it.
  [[ ! -f "$state/worker.lock" ]] || kill "$(cat "$state/worker.lock")" 2>/dev/null || true
  tern plugin unlink tern-kata >/dev/null 2>&1 || true
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Supervision smoke artifacts: %s\n' "$tmp" >&2; [[ ! -f "$STENCIL_LOG_DIR/tern-daemon.log" ]] || tail -n 40 "$STENCIL_LOG_DIR/tern-daemon.log" >&2; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$tmp/bin" "$tmp/workspace" "$tmp/checkout" "$XDG_CONFIG_HOME/tern-kata" "$state"
if [[ $# -gt 0 ]]; then cp "$1" "$tmp/bin/tern-kata"; else (cd "$repo" && go build -o "$tmp/bin/tern-kata" ./cmd/tern-kata); fi
# The stub stands for omp. Both programs sit in a folder the daemon's PATH does not list: the host half must find them.
(cd "$repo" && go build -o "$tmp/bin/omp" ./internal/worker/workertest/stub)
export TERN_KATA_INSTALL_DIR="$tmp/bin"

kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json >/dev/null
cat >"$tmp/job.json" <<JSON
{"name":"Supervised smoke job","definition":{"version":1,"kind":"job","enabled":true,"trigger":{"kind":"manual"},
 "action":{"kind":"execute","prompt":"say done"},"checkout_key":"repo",
 "issue":{"kind":"per-run","title":"Supervised run"},"overlap":"forbid","catchup":"skip","grace_seconds":60}}
JSON
job=$(kata --workspace "$tmp/workspace" cron job create --file "$tmp/job.json" --json | jq -r '.job.uid')

start_daemon() {
  # A short PATH, as a daemon started by a desktop launcher has: it lists neither tern-kata nor omp.
  env PATH=/usr/bin:/bin "$tern_bin" daemon --socket "$TERN_DAEMON_SOCKET" >"$tmp/daemon.out" 2>&1 &
  daemon_pid=$!
  for ((i = 0; i < 100; i++)); do [[ -S "$TERN_DAEMON_SOCKET" ]] && return 0; sleep 0.1; done
  echo "daemon did not start" >&2
  return 1
}
stop_daemon() {
  [[ -z "$daemon_pid" ]] || { kill "$daemon_pid" 2>/dev/null || true; wait "$daemon_pid" 2>/dev/null || true; }
  daemon_pid=
  rm -f "$TERN_DAEMON_SOCKET"
}
worker_pid() { [[ -f "$state/worker.lock" ]] && cat "$state/worker.lock" || true; }
alive() { [[ -n "${1:-}" ]] && kill -0 "$1" 2>/dev/null; }
# The worker answers on its control socket: `worker status` works.
wait_worker() { # $1 seconds
  for ((i = 0; i < $1 * 10; i++)); do
    if "$tmp/bin/tern-kata" worker status >"$tmp/status.txt" 2>&1; then return 0; fi
    sleep 0.1
  done
  echo "no worker answered within $1 s" >&2
  cat "$tmp/status.txt" >&2
  return 1
}
wait_new_worker() { # $1 the old pid, $2 seconds
  for ((i = 0; i < $2 * 10; i++)); do
    new=$(worker_pid)
    if [[ -n "$new" && "$new" != "$1" ]] && alive "$new" && "$tmp/bin/tern-kata" worker status >/dev/null 2>&1; then return 0; fi
    sleep 0.1
  done
  echo "no new worker within $2 s (was $1)" >&2
  return 1
}

# No worker.toml: the host half does nothing, quietly.
(cd "$repo" && tern plugin link . --json >/dev/null)
start_daemon
sleep 3
[[ -z "$(worker_pid)" ]] || { echo "a worker started without worker.toml" >&2; exit 1; }
if grep -qs 'tern-kata:' "$STENCIL_LOG_DIR/tern-daemon.log"; then echo "host half logged without worker.toml" >&2; exit 1; fi

cat >"$XDG_CONFIG_HOME/tern-kata/worker.toml" <<TOML
executor = "smoke-machine"
actor = "smoke"
[[projects]]
name = "smoke"
[checkouts]
repo = "$tmp/checkout"
[profiles.default]
command = "omp"
args = ["--stub-text", "supervised ok"]
TOML
# Every window is closed (none was ever opened); the worker comes up from the daemon alone.
wait_worker 30
grep -q 'Supervised by Tern' "$tmp/status.txt" || { echo "worker status does not say Tern supervises it" >&2; cat "$tmp/status.txt" >&2; exit 1; }
first=$(worker_pid)

run_job() {
  "$tmp/bin/tern-kata" run "$job" | grep -q accepted
  for ((i = 0; i < 100; i++)); do
    "$tmp/bin/tern-kata" worker status >"$tmp/status.txt"
    if [[ $(grep -c '"Supervised smoke job"  succeeded' "$tmp/status.txt") -ge $1 ]]; then return 0; fi
    sleep 0.1
  done
  echo "the job did not succeed $1 time(s)" >&2
  cat "$tmp/status.txt" >&2
  return 1
}
run_job 1

# A crashing worker is restarted, and each quick crash waits longer than the last.
kill -9 "$first"
wait_new_worker "$first" 20
second=$(worker_pid)
t0=$(date +%s.%N)
kill -9 "$second"
wait_new_worker "$second" 20
elapsed=$(echo "$(date +%s.%N) - $t0" | bc)
third=$(worker_pid)
(( $(echo "$elapsed >= 1.8" | bc) )) || { echo "restart after a quick second crash came after $elapsed s: backoff missing" >&2; exit 1; }
run_job 2

# A daemon restart: the host half loads again and the worker runs again.
stop_daemon
for ((i = 0; i < 100; i++)); do alive "$third" || break; sleep 0.1; done
start_daemon
wait_worker 40
fourth=$(worker_pid)
alive "$fourth"
run_job 3
# The worker the old daemon left running keeps its lock, so the new host half's attempts are refused; when it ends, the
# next attempt starts a worker.
alive "$third" && kill -9 "$third"
wait_new_worker "$third" 100
run_job 4
printf 'Supervision smoke passed: no config = nothing, worker started and run with no window, quick crashes back off, worker back after a daemon restart and when the worker it left behind ends\n'
