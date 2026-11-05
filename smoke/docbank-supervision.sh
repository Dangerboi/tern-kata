#!/usr/bin/env bash
# The plugin's host half keeps a local Docbank running with no Tern window: a stopped Docbank is started within one poll,
# a running one is left alone, docbank.manual stops it starting, and a kata pointed at a remote Docbank never starts one.
# Docbank is a stub `docbank` that keeps a detached process, as `docbank daemon start` does. Own temp state; no window opens.
# Usage: smoke/docbank-supervision.sh [path to a built tern-kata]
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
daemon_pid=
tern_bin=$(command -v tern)
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" DOCBANK_HOME="$tmp/docbank" TERN_CONFIG_DIR="$tmp/config" TERN_DAEMON_SOCKET="$tmp/daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs" XDG_CONFIG_HOME="$tmp/xdg-config" XDG_STATE_HOME="$tmp/xdg-state"
export STENCIL_LOG=warn,tern::plugin=warn
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP TERN_PANE_SOCKET TERN_WINDOW_SOCKET
config="$XDG_CONFIG_HOME/tern-kata"
fake="$tmp/fake-docbank"
cleanup() {
  status=$?
  trap - EXIT
  stop_daemon
  kill_docbank
  tern plugin unlink tern-kata >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Docbank supervision smoke artifacts: %s\n' "$tmp" >&2; [[ ! -f "$STENCIL_LOG_DIR/tern-daemon.log" ]] || tail -n 40 "$STENCIL_LOG_DIR/tern-daemon.log" >&2; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$tmp/bin" "$KATA_HOME" "$DOCBANK_HOME" "$config" "$fake" "$XDG_STATE_HOME"
if [[ $# -gt 0 ]]; then cp "$1" "$tmp/bin/tern-kata"; else (cd "$repo" && go build -o "$tmp/bin/tern-kata" ./cmd/tern-kata); fi
# The stub stands for docbank, in a folder the daemon's PATH does not list: the host half must find it. `daemon status`
# exits 0 while the process it started lives; `daemon start` detaches one and counts the call; a file named `refuse`
# makes it fail.
cat >"$tmp/bin/docbank" <<STUB
#!/bin/sh
f='$fake'
case "\$1 \$2" in
"daemon status")
  [ -f "\$f/pid" ] && kill -0 "\$(cat "\$f/pid")" 2>/dev/null && { echo "running: pid \$(cat "\$f/pid")"; exit 0; }
  echo "not running"; exit 1 ;;
"daemon start")
  echo "\$PWD" >>"\$f/start-cwd"
  echo start >>"\$f/starts"
  [ -f "\$f/refuse" ] && { echo "port in use" >&2; exit 1; }
  setsid sleep 3600 >/dev/null 2>&1 </dev/null &
  echo \$! >"\$f/pid"
  echo started ;;
esac
STUB
chmod +x "$tmp/bin/docbank"
export TERN_KATA_INSTALL_DIR="$tmp/bin"

local_kata=$'[docbank]\nurl = "http://127.0.0.1:18080"\napi_key = "smoke"\n'
printf '%s' "$local_kata" >"$KATA_HOME/config.toml"
printf '[server]\nbind_addr = "127.0.0.1"\napi_port = 18080\napi_key = "smoke"\n' >"$DOCBANK_HOME/config.toml"

docbank_pid() { [[ -f "$fake/pid" ]] && cat "$fake/pid" || true; }
alive() { [[ -n "${1:-}" ]] && kill -0 "$1" 2>/dev/null; }
kill_docbank() {
  local pid
  pid=$(docbank_pid)
  if alive "$pid"; then kill "$pid" 2>/dev/null || true; fi
  for ((i = 0; i < 50; i++)); do alive "$pid" || break; sleep 0.1; done
  return 0
}
starts() { [[ -f "$fake/starts" ]] && wc -l <"$fake/starts" | tr -d ' ' || echo 0; }
start_daemon() {
  # A short PATH, as a daemon started by a desktop launcher has: it lists neither tern-kata nor docbank.
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
# wait_docbank <seconds>: a Docbank runs within that time. Left idle with no worker running the session daemon exits after
# about 12 s, and with it the host half under test, so the daemon is asked something every second.
wait_docbank() {
  local k
  for ((k = 0; k < $1; k++)); do
    alive "$(docbank_pid)" && return 0
    tern plugin list >/dev/null 2>&1
    sleep 1
  done
  alive "$(docbank_pid)"
}
# no_docbank_within <seconds> <reason>: none is started (a poll is 10 s, so 13 s covers one).
no_docbank_within() {
  local k
  for ((k = 0; k < $1; k++)); do sleep 1; tern plugin list >/dev/null 2>&1; done
  ! alive "$(docbank_pid)" || { echo "Docbank was started although $2" >&2; return 1; }
}
log_count() { { grep -c "$1" "$STENCIL_LOG_DIR/tern-daemon.log" 2>/dev/null || true; } | head -n1; }

# The daemon reloads a linked plugin whenever a file in its folder changes: link a private copy.
mkdir -p "$tmp/plugin"
cp "$repo/plugin.toml" "$repo"/*.luau "$tmp/plugin/"
(cd "$tmp/plugin" && tern plugin link . --json >/dev/null)

# The supervision rule is Go's, answered on the command line.
"$tmp/bin/tern-kata" docbank supervised | grep -qx on || { echo "docbank supervised did not say on" >&2; exit 1; }

# A stopped Docbank is started within one poll, from the home folder, once.
start_daemon
wait_docbank 13 || { echo "a stopped Docbank was not started within one poll" >&2; exit 1; }
[[ $(starts) -eq 1 ]] || { echo "Docbank was started $(starts) times, want 1" >&2; exit 1; }
[[ "$(cat "$fake/start-cwd")" == "$HOME" ]] || { echo "Docbank was started in $(cat "$fake/start-cwd"), not in the home folder $HOME" >&2; exit 1; }
[[ $(log_count 'Docbank was not running') -eq 1 ]] || { echo "the host half did not log the start once" >&2; exit 1; }

# A running Docbank is left alone, whoever started it, over more than one poll.
for ((k = 0; k < 13; k++)); do sleep 1; tern plugin list >/dev/null 2>&1; done
[[ $(starts) -eq 1 ]] || { echo "a running Docbank was started again" >&2; exit 1; }
kill_docbank
(PATH="$tmp/bin:$PATH" docbank daemon start >/dev/null) # a Docbank started by hand
by_hand=$(docbank_pid)
for ((k = 0; k < 13; k++)); do sleep 1; tern plugin list >/dev/null 2>&1; done
[[ $(starts) -eq 2 && "$(docbank_pid)" == "$by_hand" ]] && alive "$by_hand" || { echo "a Docbank started by hand was not left alone" >&2; exit 1; }

# docbank.manual stops it starting; removing it starts one within a poll.
"$tmp/bin/tern-kata" docbank supervise off | grep -q 'Created' || { echo "supervise off did not say what it did" >&2; exit 1; }
[[ "$(stat -c %a "$config/docbank.manual")" == 600 ]] || { echo "docbank.manual is not mode 0600" >&2; exit 1; }
if "$tmp/bin/tern-kata" docbank supervised >/dev/null 2>&1; then echo "docbank supervised says on with docbank.manual" >&2; exit 1; fi
kill_docbank
no_docbank_within 13 "docbank.manual exists"
[[ $(starts) -eq 2 ]] || { echo "Docbank was started with docbank.manual present" >&2; exit 1; }
"$tmp/bin/tern-kata" docbank supervise on | grep -q 'Removed' || { echo "supervise on did not say what it did" >&2; exit 1; }
wait_docbank 13 || { echo "removing docbank.manual did not start Docbank within one poll" >&2; exit 1; }
[[ $(starts) -eq 3 ]] || { echo "Docbank was started $(starts) times, want 3" >&2; exit 1; }

# A start that fails is retried with backoff, and one line is logged for the failure, not one per poll.
kill_docbank
: >"$fake/refuse"
failed_before=$(log_count 'could not start Docbank')
for ((k = 0; k < 35; k++)); do sleep 1; tern plugin list >/dev/null 2>&1; done
[[ $(starts) -ge 5 ]] || { echo "the failed start was not retried (starts: $(starts))" >&2; exit 1; }
[[ $(( $(log_count 'could not start Docbank') - failed_before )) -eq 1 ]] || { echo "the failed start was not logged exactly once" >&2; exit 1; }
rm "$fake/refuse"
wait_docbank 25 || { echo "Docbank was not started once the failure cleared" >&2; exit 1; }

# A kata pointed at a Docbank elsewhere never starts one, nor does a port that is not Docbank's.
kill_docbank
printf '[docbank]\nurl = "https://docbank.example"\napi_key = "smoke"\n' >"$KATA_HOME/config.toml"
if "$tmp/bin/tern-kata" docbank supervised >/dev/null 2>&1; then echo "docbank supervised says on for a remote Docbank" >&2; exit 1; fi
before=$(starts)
no_docbank_within 13 "kata's Docbank is remote"
[[ $(starts) -eq $before ]] || { echo "a remote URL started Docbank" >&2; exit 1; }
printf '%s' "$local_kata" | sed 's/18080/18081/' >"$KATA_HOME/config.toml"
no_docbank_within 13 "the port is not Docbank's"
printf 'Docbank supervision smoke passed: a stopped Docbank is started within one poll from the home folder and logged once, a running one (started by hand too) is left alone, docbank.manual stops it starting and removing it starts one, a failing start backs off and logs once, a remote URL or another port never starts one\n'
