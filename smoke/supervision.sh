#!/usr/bin/env bash
# The plugin's host half supervises the worker with no Tern window: it starts `tern-kata worker` in a headless session
# daemon, restarts it with backoff, and starts it again after a daemon restart. Own temp state; no window is ever opened.
# Usage: smoke/supervision.sh [path to a built tern-kata]
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
daemon_pid=
serve_pid=
keepalive=
tern_bin=$(command -v tern)
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" TERN_CONFIG_DIR="$tmp/config" TERN_DAEMON_SOCKET="$tmp/daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs" XDG_CONFIG_HOME="$tmp/xdg-config" XDG_STATE_HOME="$tmp/xdg-state"
# stencil_tern=info logs the daemon's own stop ("daemon stopped kind=\"idle\""), which explains a host half that went quiet.
export STENCIL_LOG=warn,tern::plugin=warn,stencil_tern=info
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP TERN_PANE_SOCKET TERN_WINDOW_SOCKET
state="$XDG_STATE_HOME/tern-kata"
cleanup() {
  status=$?
  trap - EXIT
  [[ -z "$keepalive" ]] || { kill "$keepalive" 2>/dev/null || true; wait "$keepalive" 2>/dev/null || true; }
  stop_daemon
  [[ -z "$serve_pid" ]] || { kill "$serve_pid" 2>/dev/null || true; wait "$serve_pid" 2>/dev/null || true; }
  # A worker the daemon's death left behind is the smoke's own: end it, asking the binary this run built.
  "$tmp/bin/tern-kata" worker stop --now >/dev/null 2>&1 || true
  tern plugin unlink tern-kata >/dev/null 2>&1 || true
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Supervision smoke artifacts: %s\n' "$tmp" >&2; [[ ! -f "$STENCIL_LOG_DIR/tern-daemon.log" ]] || tail -n 40 "$STENCIL_LOG_DIR/tern-daemon.log" >&2; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source "$repo/smoke/ctl.sh"
mkdir -p "$tmp/bin" "$tmp/workspace" "$tmp/checkout" "$XDG_CONFIG_HOME/tern-kata" "$state"
if [[ $# -gt 0 ]]; then cp "$1" "$tmp/bin/tern-kata"; else (cd "$repo" && go build -buildvcs=false -o "$tmp/bin/tern-kata" ./cmd/tern-kata); fi
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
  wait_for socket_ready "$daemon_pid" "$TERN_DAEMON_SOCKET"
}
stop_daemon() {
  [[ -z "$daemon_pid" ]] || { kill "$daemon_pid" 2>/dev/null || true; wait "$daemon_pid" 2>/dev/null || true; }
  daemon_pid=
  rm -f "$TERN_DAEMON_SOCKET"
}
worker_pid() { [[ -f "$state/worker.lock" ]] && cat "$state/worker.lock" || true; }
alive() { [[ -n "${1:-}" ]] && kill -0 "$1" 2>/dev/null; }
worker_gone() { ! alive "$1"; }
wait_gone() { wait_for worker_gone "$1"; } # $1 the pid
# The worker answers on its control socket: `worker status` works.
worker_answers() { "$tmp/bin/tern-kata" worker status >"$tmp/status.txt" 2>&1; }
# The waits below run up to wait_limit seconds; a caller that proves a bound passes the seconds as the last argument.
wait_worker() { # [$1 seconds]
  wait_for ${1:+-t "$1"} worker_answers || { cat "$tmp/status.txt" >&2; return 1; }
}
new_worker_answers() { # $1 the old pid
  local new
  new=$(worker_pid)
  [[ -n "$new" && "$new" != "$1" ]] && alive "$new" && "$tmp/bin/tern-kata" worker status >/dev/null 2>&1
}
wait_new_worker() { wait_for ${2:+-t "$2"} new_worker_answers "$1"; } # $1 the old pid, [$2 seconds]

# The daemon reloads a linked plugin whenever a file in its folder changes, and a reloaded host half starts the worker at
# once, with no wait. Link a private copy, so other work in the repository (a parallel check step) cannot reload it
# in the middle of the backoff measurement.
mkdir -p "$tmp/plugin"
copy_plugin "$tmp/plugin"
(cd "$tmp/plugin" && tern plugin link . --json >/dev/null)
# No worker.toml: the host half does nothing, quietly.
kv_file="$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json"
kv_summary() { jq -c '.["worker.summary"] // empty' "$kv_file" 2>/dev/null || true; }
# From Tern 0.9.21 the host half renders the worker widget and drops a summary an earlier load or tern-kata left in the
# plugin's tern.kv. A seeded one is the control for the later "no summary stored" check: it shows kv_file is the file the
# plugin uses, since the plugin removes it.
if tern_at_least 0.9.21; then
  mkdir -p "$(dirname "$kv_file")"
  printf '{"worker.summary":{"at":1,"worker":{}}}\n' >"$kv_file"
fi
start_daemon
sleep 3 # proves a bound: no worker or log line may come from the host half within 3 s of its start without worker.toml
[[ -z "$(worker_pid)" ]] || { echo "a worker started without worker.toml" >&2; exit 1; }
if grep -qs 'tern-kata:' "$STENCIL_LOG_DIR/tern-daemon.log"; then echo "host half logged without worker.toml" >&2; exit 1; fi
if tern_at_least 0.9.21; then
  [[ -z "$(kv_summary)" ]] || { echo "the host half left the seeded worker.summary in $kv_file" >&2; exit 1; }
fi

cat >"$XDG_CONFIG_HOME/tern-kata/worker.toml" <<TOML
executor = "smoke-machine"
actor = "smoke"
[[projects]]
name = "smoke"
[checkouts]
repo = "$tmp/checkout"
[profiles.default]
command = "omp"
args = ["--stub-text", "supervised ok", "--stub-fail-when", "FAILME"]
TOML
# Every window is closed (none was ever opened); the worker comes up from the daemon alone.
wait_worker
grep -q 'Supervised by Tern' "$tmp/status.txt" || { echo "worker status does not say Tern supervises it" >&2; cat "$tmp/status.txt" >&2; exit 1; }
first=$(worker_pid)

# The job forbids overlap: a fresh worker may still be settling the last run (it shows succeeded before kata holds it
# settled), so a refusal saying so is retried for a while; so is a fresh worker that has not resolved its Project yet.
job_accepted() { # $1 the job's uid
  run_out=$("$tmp/bin/tern-kata" run "$1" 2>&1) && return 0
  grep -Eq 'is not settled|no job .* in a Project this worker serves' <<<"$run_out" || { echo "$run_out" >&2; exit 1; }
  return 1
}
runs_succeeded() { # $1 how many
  "$tmp/bin/tern-kata" worker status >"$tmp/status.txt"
  [[ $(grep -c '"Supervised smoke job"  succeeded' "$tmp/status.txt") -ge $1 ]]
}
accept_run() { # $1 the job's uid
  wait_for job_accepted "$1"
  grep -q accepted <<<"$run_out" || { echo "$run_out" >&2; return 1; }
}
run_job() { # $1 how many runs have succeeded when it returns
  accept_run "$job"
  wait_for runs_succeeded "$1" || { cat "$tmp/status.txt" >&2; return 1; }
}
run_job 1
# On a Tern whose host half cannot render widgets, the host half keeps a summary of the running worker in the plugin's tern.kv
# (the kv.json in the plugin's data folder), which the window half's own worker widget reads, and removes it when no worker
# runs. The poll that stores it comes within 10 s.
# The summary the host half stores while it cannot render the widget is read through kv_summary (above).
summary_is() { # $1 present|gone
  tern plugin list >/dev/null 2>&1 # idle with no worker, the session daemon exits after about 12 s
  if [[ $1 == present ]]; then [[ -n "$(kv_summary)" ]]; else [[ -z "$(kv_summary)" ]]; fi
}
wait_summary() { # $1 present|gone
  wait_for summary_is "$1" || { echo "worker.summary: $(kv_summary)" >&2; return 1; }
}
if tern_at_least 0.9.21; then
  # Tern 0.9.21+ renders the worker widget in the host half, so no summary goes to tern.kv (plugin.sh reads the widget itself);
  # the poll that would have stored one comes within 10 s, so 13 s of looks prove it stored none (a bound the step proves).
  # The host half logs a failed registration at warn level.
  for tick in {1..13}; do
    tern plugin list >/dev/null 2>&1
    [[ -z "$(kv_summary)" ]] || { echo "worker.summary was stored although the host half renders the widget: $(kv_summary)" >&2; exit 1; }
    sleep 1
  done
  ! grep -q 'could not register the worker widget' "$STENCIL_LOG_DIR/tern-daemon.log" || { echo "the host half could not register the worker widget" >&2; exit 1; }
else
  wait_summary present
  kv_summary | jq -e --argjson pid "$(worker_pid)" '.worker | .executor == "smoke-machine" and .pid == $pid and .capacity >= 1 and (.running | type) == "number" and (.held | length) == 0' >/dev/null || { echo "worker.summary is not the running worker's summary: $(kv_summary)" >&2; exit 1; }
fi

# A crashing worker is restarted, and each quick crash waits longer than the last.
kill -9 "$first"
wait_new_worker "$first"
second=$(worker_pid)
epoch_us crash_at
kill -9 "$second"
wait_new_worker "$second"
epoch_us restarted_at
third=$(worker_pid)
restart_ms=$(( (restarted_at - crash_at) / 1000 ))
(( restart_ms >= 1800 )) || { echo "restart after a quick second crash came after $restart_ms ms: backoff missing" >&2; exit 1; }
run_job 2

# A daemon restart: the host half loads again and the worker runs again.
stop_daemon
# The worker may outlive the daemon (the next stage ends it): give it a moment to go with it. A short grace, not a
# requirement, so it keeps its 10 s and a worker that stays is not waited for until wait_limit.
wait_for -t 10 worker_gone "$third" || true
start_daemon
wait_worker
fourth=$(worker_pid)
alive "$fourth"
run_job 3
# The worker the old daemon left running keeps its lock, so the new host half starts none while it runs; when it ends, the
# next look starts a worker.
alive "$third" && kill -9 "$third"
wait_new_worker "$third"
run_job 4
good_config=$(<"$XDG_CONFIG_HOME/tern-kata/worker.toml")
# A crash loop on a broken worker.toml grows the wait past one poll (10 s); fixing the file starts a worker within a poll.
fifth=$(worker_pid)
printf 'this is = = not toml\n' >"$XDG_CONFIG_HOME/tern-kata/worker.toml"
log_from=$(( $(wc -l <"$STENCIL_LOG_DIR/tern-daemon.log") + 1 ))
kill -9 "$fifth"
backoff_grew() { tail -n +"$log_from" "$STENCIL_LOG_DIR/tern-daemon.log" | grep -Eq 'restarting in ([2-9][0-9]|1[1-9]) s'; }
wait_for backoff_grew || { echo "the broken config never grew the wait past 10 s" >&2; exit 1; }
printf '%s\n' "$good_config" >"$XDG_CONFIG_HOME/tern-kata/worker.toml"
wait_new_worker "$fifth" 13 || { echo "fixing worker.toml did not start a worker within one poll" >&2; exit 1; } # 13 s proves a bound: within one poll
# The supervision rule lives in Go (worker.SupervisionOn, whose config folder is worker.ConfigDir). It answers `worker stop`
# ("a fresh worker" or "nothing will start it again") and the host half asks it through `worker supervised`; the host half
# either starts a worker within a poll (10 s) or does not. This end-to-end step checks the host half acts on the answer.
config="$XDG_CONFIG_HOME/tern-kata"
# stop_expecting <fresh|none>: `worker stop` the running worker and check what it promises.
stop_expecting() {
  local pid out
  pid=$(worker_pid)
  out=$("$tmp/bin/tern-kata" worker stop 2>&1) || { echo "worker stop failed: $out" >&2; return 1; }
  case "$1" in
    fresh) grep -q 'The plugin will start a fresh worker' <<<"$out" || { echo "worker stop did not promise a fresh worker: $out" >&2; return 1; } ;;
    none) grep -q 'Nothing will start it again' <<<"$out" || { echo "worker stop did not say nothing will start it again: $out" >&2; return 1; } ;;
  esac
  wait_gone "$pid"
}
# wait_started <old pid> [seconds]: wait for a new worker, asking the daemon something at every look. Left idle with no
# worker running the session daemon exits after about 12 s, and with it the host half under test. The seconds are for a step
# that proves a bound (one poll is 10 s, so 13 s covers one); without them it waits up to wait_limit.
started_worker() { tern plugin list >/dev/null 2>&1; new_worker_answers "$1"; }
wait_started() { wait_for ${2:+-t "$2"} started_worker "$1"; }
# no_worker_within <seconds> <reason>: the host half starts none (one poll is 10 s, so 13 s covers one), the daemon kept busy.
# A bound the step proves, so the seconds stay fixed.
no_worker_within() {
  for tick in $(seq "$1"); do sleep 1; tern plugin list >/dev/null 2>&1; done
  [[ -z "$(worker_pid)" ]] || ! alive "$(worker_pid)" || { echo "a worker started although $2" >&2; return 1; }
}
# worker.manual beside worker.toml: stop promises nothing follows, and the host half starts none.
: >"$config/worker.manual"
stop_expecting none
no_worker_within 13 "worker.manual is present"
wait_summary gone
# Removing worker.manual: the host half starts a worker within a poll, and stop promises a fresh one, which comes.
rm "$config/worker.manual"
wait_started "" 13 || { echo "removing worker.manual did not start a worker within one poll" >&2; exit 1; } # 13 s proves a bound: within one poll
manual_worker=$(worker_pid)
stop_expecting fresh
wait_started "$manual_worker" || { echo "the host half did not start the fresh worker stop promised" >&2; exit 1; }
# No worker.toml: stop promises nothing follows, and the host half starts none. Restoring it starts one within a poll.
rm "$config/worker.toml"
stop_expecting none
no_worker_within 13 "worker.toml is absent"
printf '%s\n' "$good_config" >"$config/worker.toml"
wait_started "" 13 || { echo "restoring worker.toml did not start a worker within one poll" >&2; exit 1; } # 13 s proves a bound: within one poll
# A plugin reload gives the host half fresh state while its worker runs on, as a Tern update's handoff does. The reloaded
# host half starts no second worker for worker.lock to refuse and knows the held answer (an older host.luau would log it
# as an unknown status); it names the running worker once, over two reloads and three looks, and starts a worker within a
# poll of its end.
# The reloads are the daemon's first clients, and a daemon with no panes stops about 12 s after its last client leaves,
# taking the host half under test with it, worker or no worker. The stages from here on have stretches with no client of
# their own (the release cross-builds before the update take that long on a busy machine), so a client asks the daemon
# something every 2 s until the end.
adopted=$(worker_pid)
(while sleep 2; do tern plugin list >/dev/null 2>&1; done) &
keepalive=$!
log_from=$(( $(wc -l <"$STENCIL_LOG_DIR/tern-daemon.log") + 1 ))
tern plugin reload tern-kata >/dev/null
# The fixed 15 s of looks prove a bound: the reload must leave the running worker alone within them.
for k in {0..14}; do # looks at 0.5 s (first load), then 3.5 s and 13.5 s (the load after the second reload)
  sleep 1
  if ((k == 2)); then tern plugin reload tern-kata >/dev/null; else tern plugin list >/dev/null 2>&1; fi
done
since_reload=$(tail -n +"$log_from" "$STENCIL_LOG_DIR/tern-daemon.log")
[[ "$(worker_pid)" == "$adopted" ]] && alive "$adopted" || { echo "the reload replaced the running worker" >&2; exit 1; }
if grep -q 'already running on this machine' <<<"$since_reload"; then echo "the reloaded host half started a second worker" >&2; exit 1; fi
if grep -q '`worker supervised` exited with status' <<<"$since_reload"; then echo "the reloaded host half does not know the held answer" >&2; exit 1; fi
[[ $(grep -c "worker $adopted .*is already running" <<<"$since_reload") -eq 1 ]] || { echo "the reloaded host half did not name the running worker once" >&2; exit 1; }
# From Tern 0.9.16 the reloaded host half is told when the worker its earlier load started ends: it starts a worker at once
# and logs the old one's exit. An older Tern sends nothing, so the poll starts one within 10 s and nothing is logged.
log_from=$(( $(wc -l <"$STENCIL_LOG_DIR/tern-daemon.log") + 1 ))
epoch_us killed_at
kill -9 "$adopted"
wait_started "$adopted" 13 || { echo "the reloaded host half did not start a worker within a poll of the running one's end" >&2; exit 1; } # 13 s proves a bound: within one poll
epoch_us started_at
since_kill_ms=$(( (started_at - killed_at) / 1000 ))
if tern_at_least 0.9.16; then
  (( since_kill_ms < 4000 )) || { echo "the reloaded host half started a worker $since_kill_ms ms after the running one's end, not at once" >&2; exit 1; }
  [[ $(tail -n +"$log_from" "$STENCIL_LOG_DIR/tern-daemon.log" | grep -c "worker $adopted exited with status") -eq 1 ]] || { echo "the reloaded host half did not log the old worker's exit once" >&2; exit 1; }
fi
run_job 5
# Tern 0.9.24 gives the worker no time limit and streams its stderr to the daemon log as it comes. A job that fails three
# times in a row makes the worker write "job ... is paused after 3 failed runs" to stderr: that line must reach the daemon
# log once prefixed (the worker prefixes its own lines, so a doubled prefix is a bug), while the worker runs.
if tern_at_least 0.9.24; then
  sed 's/"Supervised smoke job"/"Failing smoke job"/; s/say done/FAILME/; s/Supervised run/Failing run/' "$tmp/job.json" >"$tmp/failjob.json"
  failjob=$(kata --workspace "$tmp/workspace" cron job create --file "$tmp/failjob.json" --json | jq -r '.job.uid')
  for attempt in 1 2 3; do
    accept_run "$failjob"
  done
  worker_line_logged() { grep -Eq 'tern-kata worker: .*is paused after 3 failed runs' "$STENCIL_LOG_DIR/tern-daemon.log"; }
  wait_for worker_line_logged || { echo "a line the worker wrote to stderr never reached the daemon log" >&2; exit 1; }
  ! grep -q 'tern-kata worker: tern-kata worker: ' "$STENCIL_LOG_DIR/tern-daemon.log" || { echo "a worker line was logged with a doubled prefix" >&2; exit 1; }
fi
# `update` moves a supervised worker to the new binary: a release build older than the dist/ is what the host half starts,
# `update` against the local dist/ swaps it and stops the worker when idle, and the fresh worker the host half starts
# reports the new version. The plugin is linked here, so the update leaves it and says so.
up_version=$(plugin_version "$repo/plugin.toml")
release_dist up_dist "$tmp/update-dist"
serve_dir "$up_dist" "$tmp/update-server.log"
build_old_release "$tmp/bin/tern-kata.old"
mv "$tmp/bin/tern-kata.old" "$tmp/bin/tern-kata"
old_worker=$(worker_pid)
stop_expecting fresh
wait_started "$old_worker" || { echo "no worker started from the older release build" >&2; exit 1; }
old_worker=$(worker_pid)
"$tmp/bin/tern-kata" worker status | grep -q 'tern-kata v0.0.1' || { echo "the worker is not the older build" >&2; exit 1; }
TERN_KATA_BASE_URL="$serve_url" "$tmp/bin/tern-kata" update >"$tmp/update.out"
grep -q 'Stopping when idle' "$tmp/update.out" || { echo "update did not stop the worker" >&2; cat "$tmp/update.out" >&2; exit 1; }
grep -q 'The plugin will start a fresh worker' "$tmp/update.out"
grep -q 'tern-kata plugin is linked' "$tmp/update.out"
wait_gone "$old_worker"
wait_started "$old_worker" || { echo "the host half did not start a worker after update" >&2; exit 1; }
"$tmp/bin/tern-kata" worker status | grep -q "tern-kata v$up_version" || { echo "the new worker does not run v$up_version" >&2; exit 1; }
printf 'Supervision smoke passed: no config = nothing, worker started and run with no window, quick crashes back off, worker back after a daemon restart and when the worker it left behind ends, a reloaded host half leaves the running worker alone and starts one when it ends, update moves the supervised worker to the new release, host half and `worker stop` agree on worker.manual and worker.toml (marker, no marker, no config) over the same config folder, fixing a broken worker.toml drops the wait\n'
