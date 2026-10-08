#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
server_pid=
export CGO_ENABLED=0
export KATA_HOME="$tmp/kata"
export TERN_CONFIG_DIR="$tmp/config"
export TERN_DAEMON_SOCKET="$tmp/tern-daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs"
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP

cleanup() {
  status=$?
  trap - EXIT
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

steps=("$repo"/smoke/steps/*)
if (( $# )); then steps=("$@"); fi
for step in "${steps[@]}"; do
  printf '\nRunning %s\n' "$(basename "$step")"
  case "$step" in
    *.ctl) ctl --file "$step" ;;
    *.sh) source "$step" ;;
  esac
done
tern ctl --control "$tmp/control.sock" key q
tern ctl --control "$tmp/control.sock" ready
"$repo/smoke/plugin.sh" "$tmp/tern-kata"
printf '\nSmoke passed\n'
