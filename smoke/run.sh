#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)

# With no argument the suite fans out. The steps wait on kata's daemon clocks (a timed claim's sweep, a job's delivery
# window), so those steps and the two other scripts run beside the rest on their own headless Tern and kata daemon;
# a shard is one such run (SMOKE_SHARD set, the steps given as arguments, the binary built once in SMOKE_BIN).
if [[ -z ${SMOKE_SHARD:-} && $# -eq 0 ]]; then
  export CGO_ENABLED=0
  bin_dir=$(mktemp -d)
  trap 'rm -rf "$bin_dir"; trap - TERM; kill 0' INT TERM
  (cd "$repo" && go build -o "$bin_dir/tern-kata" ./cmd/tern-kata)
  (cd "$repo" && GOOS=windows go vet ./...)
  slow=(51-claim-expiry.sh 60-cron-made-issue.sh 62-job-card.sh)
  # The rest is cut at steps that start from the freshly launched Pane (each shard has its own seeded Project), so the
  # five groups run side by side: steps 01-21, 22-31, 32-44, 45-57 and 58-76.
  groups=("" "" "" "" "")
  for step in "$repo"/smoke/steps/*; do
    name=$(basename "$step")
    [[ " ${slow[*]} " != *" $name "* ]] || continue
    number=$((10#${name:0:2}))
    index=0
    (( number < 22 )) || index=1
    (( number < 32 )) || index=2
    (( number < 45 )) || index=3
    (( number < 58 )) || index=4
    groups[index]+="$step "
  done
  jobs=()
  export SMOKE_SHARD=1 SMOKE_BIN="$bin_dir/tern-kata"
  for group in "${groups[@]}"; do
    "$0" $group & jobs+=($!)
  done
  for name in "${slow[@]}"; do "$0" "$repo/smoke/steps/$name" & jobs+=($!); done
  "$repo/smoke/plugin.sh" "$SMOKE_BIN" & jobs+=($!)
  "$repo/smoke/supervision.sh" "$SMOKE_BIN" & jobs+=($!)
  failed=0
  for job in "${jobs[@]}"; do wait "$job" || failed=1; done
  rm -rf "$bin_dir"
  (( failed == 0 )) || { printf 'Smoke failed\n' >&2; exit 1; }
  printf '\nSmoke passed\n'
  exit 0
fi

tmp=$(mktemp -d)
server_pid=
proxy_pid=
export CGO_ENABLED=0
export KATA_HOME="$tmp/kata"
export TERN_CONFIG_DIR="$tmp/config"
export XDG_CONFIG_HOME="$tmp/xdg-config"
export XDG_STATE_HOME="$tmp/xdg-state"
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

mkdir -p "$tmp/workspace" "$KATA_HOME"
# q asks "Quit tern-kata? y/n" unless kata's [tui] confirm_quit is false: the steps quit with one key, and the quit step turns it on.
printf '[tui]\nconfirm_quit = false\n' >"$KATA_HOME/config.toml"
kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json
kata --workspace "$tmp/workspace" create "Smoke first Issue" --body "First seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke second Issue" --body "Second seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke third Issue" --body "Third seeded body" --json
if [[ -n ${SMOKE_BIN:-} ]]; then
  cp "$SMOKE_BIN" "$tmp/tern-kata"
else
  (cd "$repo" && go build -o "$tmp/tern-kata" ./cmd/tern-kata)
  (cd "$repo" && GOOS=windows go vet ./...)
fi

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

ctl() {
  if [[ ${1:-} == --file && $# -eq 2 ]] && grep -q '^run .*tern-kata' "$2"; then
    ctl_typed "$2"
  else
    tern ctl --control "$tmp/control.sock" "$@"
  fi
}
# Tern's `run` types the line and then waits a fixed 1.2 s for it to print, whether or not it has. A launch file's `run`
# lines that start tern-kata are typed and entered instead, and the wait ends when the program has drawn its columns or
# an error, which is what the steps' next clicks and keys need. Any other program (kata tui) draws neither and keeps
# Tern's own wait.
ctl_typed() {
  local i
  sed -E 's/^run (.*)$/type \1\nkey Enter/' "$1" >"$tmp/typed.ctl"
  tern ctl --control "$tmp/control.sock" --file "$tmp/typed.ctl"
  for ((i = 0; i < 250; i++)); do
    tern ctl --control "$tmp/control.sock" tree "[data-id^='main.columns.'], [data-id='main.error']" | jq -e '.count > 0' >/dev/null && return 0
    sleep 0.05
  done
}
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP EDITOR=%s/smoke/editor.sh KATA_HOME=%s %s/tern-kata"\n' "$repo" "$KATA_HOME" "$tmp" >"$tmp/launch.ctl"
ctl --file "$tmp/launch.ctl"

expect_tree() {
  for ((attempt = 0; attempt < 1000; attempt++)); do
    ctl tree "$1" >"$tmp/assert-tree.json"
    if jq -e "$2" "$tmp/assert-tree.json" >/dev/null; then return; fi
    sleep 0.02
  done
  cat "$tmp/assert-tree.json" >&2
  printf 'Native tree assertion failed: %s\n' "$2" >&2
  return 1
}
# The mode the footer hints show the keys are in: collections, detail, list or other.
hint_mode() {
  ctl tree "[data-id='dock.hints']" | jq -r '.nodes[0].text | if contains("open Collection") then "collections" elif contains("j/k row") then "detail" elif contains("↑↓ move") then "list" else "other" end'
}
hint_expect_mode() {
  for ((attempt = 0; attempt < 500; attempt++)); do
    [[ $(hint_mode) != "$1" ]] || return 0
    sleep 0.02
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
  printf 'Step passed %s\n' "$(basename "$step")"
done
quit_app
[[ -z ${SMOKE_SHARD:-} ]] || exit 0
"$repo/smoke/plugin.sh" "$tmp/tern-kata"
"$repo/smoke/supervision.sh" "$tmp/tern-kata"
printf '\nSmoke passed\n'
