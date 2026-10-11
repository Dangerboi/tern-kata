#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
server_pid=
proxy_pid=
export CGO_ENABLED=0
export KATA_HOME="$tmp/kata"
export TERN_CONFIG_DIR="$tmp/config"
export TERN_DAEMON_SOCKET="$tmp/tern-daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs"
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP

cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "$proxy_pid" ]]; then
    kill "$proxy_pid" 2>/dev/null || true
    wait "$proxy_pid" 2>/dev/null || true
  fi
  if [[ -n "$server_pid" ]]; then
    if [[ $status -ne 0 ]]; then
      tern ctl --control "$tmp/control.sock" tree >"$tmp/tree.json" 2>/dev/null || true
      tern ctl --control "$tmp/control.sock" shot failure >/dev/null 2>&1 || true
    fi
    tern ctl --control "$tmp/control.sock" quit >/dev/null 2>&1 || true
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -ne 0 ]]; then
    printf 'Smoke failed; artifacts: %s\n' "$tmp" >&2
    [[ ! -f "$tmp/tern.log" ]] || cat "$tmp/tern.log" >&2
  else
    rm -rf "$tmp"
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$tmp/workspace"
kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json
kata --workspace "$tmp/workspace" create "Smoke first Issue" --body "First seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke second Issue" --body "Second seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke third Issue" --body "Third seeded body" --json
(cd "$repo" && go build -o "$tmp/tern-kata" .)

tern serve --control "$tmp/control.sock" --out "$tmp/shots" --dir "$tmp/workspace" >"$tmp/tern.log" 2>&1 &
server_pid=$!
for ((attempt = 0; attempt < 200; attempt++)); do
  [[ ! -S "$tmp/control.sock" ]] || break
  kill -0 "$server_pid"
  sleep 0.1
done
[[ -S "$tmp/control.sock" ]]

# Tern 0.7 lays out only the part of a long scroller near the screen, and `ctl tree` lists only laid-out nodes, so
# steps could not find rows or tabs below the fold. This sheet lays everything out, as Tern 0.6 did.
printf '* { content-visibility: visible !important; }\n' >"$tmp/visible.css"
tern ctl --control "$tmp/control.sock" css smoke-visible "$tmp/visible.css"

printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP EDITOR=%s/smoke/editor.sh KATA_HOME=%s %s/tern-kata"\n' "$repo" "$KATA_HOME" "$tmp" >"$tmp/launch.ctl"
tern ctl --control "$tmp/control.sock" --file "$tmp/launch.ctl"

ctl() { tern ctl --control "$tmp/control.sock" "$@"; }
expect_tree() {
  for ((attempt = 0; attempt < 200; attempt++)); do
    ctl tree "$1" >"$tmp/assert-tree.json"
    if jq -e "$2" "$tmp/assert-tree.json" >/dev/null; then return; fi
    sleep 0.1
  done
  cat "$tmp/assert-tree.json" >&2
  printf 'Native tree assertion failed: %s\n' "$2" >&2
  return 1
}
# The mode the footer hints show the keys are in: collections, detail, list or other.
hint_mode() {
  ctl tree "[data-id='dock.hints']" | jq -r '.nodes[0].text | if contains("open Collection") then "collections" elif contains("j/k row") then "detail" elif contains("next Issue") then "list" else "other" end'
}
hint_expect_mode() {
  for ((attempt = 0; attempt < 100; attempt++)); do
    [[ $(hint_mode) != "$1" ]] || return 0
    sleep 0.1
  done
  printf 'Expected keys in %s, found %s\n' "$1" "$(hint_mode)" >&2
  return 1
}
quit_app() {
  ctl key q
  expect_tree "[data-id^='main.']" '.ok and .count == 0'
  for ((attempt = 0; attempt < 200; attempt++)); do
    ctl state >"$tmp/state.json" 2>/dev/null || true
    if jq -e '.focused.prompt == true and .focused.running == null and .focused.busy == false' "$tmp/state.json" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.05
  done
  printf 'Timed out waiting for shell to become idle after quitting app\n' >&2
  return 1
}


stop_live_daemon() {
  kata daemon stop
  for ((attempt = 0; attempt < 100; attempt++)); do
    if kata daemon status --json | jq -e '.daemons | length == 0' >/dev/null; then return; fi
    sleep 0.1
  done
  printf 'Daemon did not finish stopping within 10 seconds\n' >&2
  return 1
}

steps=("$repo"/smoke/steps/*)
if (( $# )); then steps=("$@"); fi
for step in "${steps[@]}"; do
  printf '\nRunning %s\n' "$(basename "$step")"
  case "$step" in
    *.ctl) ctl --file "$step" ;;
    *.sh) source "$step" ;;
  esac
done
quit_app
"$repo/smoke/plugin.sh" "$tmp/tern-kata"
printf '\nSmoke passed\n'
