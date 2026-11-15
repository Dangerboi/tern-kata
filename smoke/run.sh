#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)

run_extra() { # run_extra <script> <binary>: plugin.sh and supervision.sh take the binary, release-check.sh builds its own
  case $(basename "$1") in
    release-check.sh) "$1" ;;
    *) "$1" "$2" ;;
  esac
}

# With no argument the suite fans out. The steps wait on kata's daemon clocks (a timed claim's sweep, a job's delivery
# window), so those steps and the two other scripts run beside the rest on their own headless Tern and kata daemon;
# a shard is one such run (SMOKE_SHARD set, the steps given as arguments, the binary built once in SMOKE_BIN).
# With arguments the run is only those: step files in the order given on one server, then plugin.sh, supervision.sh and
# release-check.sh when named (./smoke/run.sh smoke/steps/72-x.sh smoke/supervision.sh), handed the binary built once.
# A failed run ends with a line naming the failing step or script and the artifacts folder.
if [[ -z ${SMOKE_SHARD:-} && $# -eq 0 ]]; then
  export CGO_ENABLED=0
  bin_dir=$(mktemp -d)
  trap 'rm -rf "$bin_dir"; trap - TERM; kill 0' INT TERM
  # Each shard appends "<step>; artifacts: <folder>" here when it fails, for the last line.
  export SMOKE_FAIL_LOG="$bin_dir/failures"
  : >"$SMOKE_FAIL_LOG"
  (cd "$repo" && go build -o "$bin_dir/tern-kata" ./cmd/tern-kata)
  # Steps 81 and 82 and supervision.sh serve a release built at the current version; one build serves all three.
  release_version=$(sed -n 's/^version *= *"\(.*\)"/\1/p' "$repo/plugin.toml")
  "$repo/scripts/release.sh" "v$release_version" "$bin_dir/dist" >/dev/null
  slow=(51-claim-expiry.sh 60-cron-made-issue.sh 62-job-card.sh 81-install-from-release.sh 82-update-from-release.sh)
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
  # The slowest job sets the suite's time: each records "<seconds> <name>" here, and a passing run names the slowest three.
  durations="$bin_dir/durations"
  timed() { # timed <name> <command...>
    local start=$SECONDS status=0
    "${@:2}" || status=$?
    printf '%s %s\n' "$((SECONDS - start))" "$1" >>"$durations"
    return "$status"
  }
  export SMOKE_SHARD=1 SMOKE_BIN="$bin_dir/tern-kata" SMOKE_DIST="$bin_dir/dist"
  for group in "${groups[@]}"; do
    set -- $group
    first=$(basename "$1") last=$(basename "${!#}")
    timed "steps ${first:0:2}-${last:0:2}" "$0" "$@" & jobs+=($!)
  done
  for name in "${slow[@]}"; do timed "$name" "$0" "$repo/smoke/steps/$name" & jobs+=($!); done
  for extra in plugin.sh supervision.sh release-check.sh; do
    (timed "$extra" run_extra "$repo/smoke/$extra" "$SMOKE_BIN" || { printf '%s; its artifacts are named above\n' "$extra" >>"$SMOKE_FAIL_LOG"; exit 1; }) & jobs+=($!)
  done
  failed=0
  for job in "${jobs[@]}"; do wait "$job" || failed=1; done
  if (( failed )); then
    printf 'Smoke failed at %s\n' "$(paste -sd, "$SMOKE_FAIL_LOG" | sed 's/,/, /g')" >&2
    rm -rf "$bin_dir"
    exit 1
  fi
  slowest=$(sort -rn "$durations" | head -3 | awk '{ s = $1; $1 = ""; printf "%s%s %d s", sep, substr($0, 2), s; sep = ", " }')
  rm -rf "$bin_dir"
  printf '\nSmoke passed in %d s; slowest: %s\n' "$SECONDS" "$slowest"
  exit 0
fi

# The arguments are step files, plus the three scripts the fan-out runs beside them when named.
steps=()
extras=()
for arg in "$@"; do
  case $(basename "$arg") in
    plugin.sh|supervision.sh|release-check.sh) extras+=("$arg") ;;
    *) steps+=("$arg") ;;
  esac
done
(( $# )) || steps=("$repo"/smoke/steps/*)

if (( ${#steps[@]} == 0 )); then
  export CGO_ENABLED=0
  bin_dir=$(mktemp -d)
  trap 'rm -rf "$bin_dir"' EXIT
  if [[ -n ${SMOKE_BIN:-} ]]; then
    cp "$SMOKE_BIN" "$bin_dir/tern-kata"
  else
    (cd "$repo" && go build -o "$bin_dir/tern-kata" ./cmd/tern-kata)
  fi
  current=
  for extra in "${extras[@]}"; do
    current=$(basename "$extra")
    run_extra "$extra" "$bin_dir/tern-kata" || { printf '\nSmoke failed at %s; its artifacts are named above\n' "$current" >&2; exit 1; }
  done
  printf '\nSmoke passed\n'
  exit 0
fi

tmp=$(mktemp -d)
current=
in_extra=
server_pid=
proxy_pid=
# HTTP servers the steps start (release dist/ served from a local port): stopped on every exit.
http_pids=()
export CGO_ENABLED=0
export KATA_HOME="$tmp/kata"
export TERN_CONFIG_DIR="$tmp/config"
export XDG_CONFIG_HOME="$tmp/xdg-config"
export XDG_STATE_HOME="$tmp/xdg-state"
# tern-kata downloads Attachments into os.UserCacheDir: keep them in $tmp, not the user's ~/.cache.
export XDG_CACHE_HOME="$tmp/xdg-cache"
export TERN_DAEMON_SOCKET="$tmp/tern-daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs"
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP

cleanup() {
  status=$?
  trap - EXIT
  for http_pid in ${http_pids[@]+"${http_pids[@]}"}; do
    kill "$http_pid" 2>/dev/null || true
    wait "$http_pid" 2>/dev/null || true
  done
  if [[ -n "$proxy_pid" ]]; then
    kill "$proxy_pid" 2>/dev/null || true
    wait "$proxy_pid" 2>/dev/null || true
  fi
  if [[ -n "$server_pid" ]]; then
    if [[ $status -ne 0 && -z $in_extra ]]; then
      tern ctl --control "$tmp/control.sock" tree >"$tmp/tree.json" 2>/dev/null || true
      tern ctl --control "$tmp/control.sock" shot failure >/dev/null 2>&1 || true
    fi
    tern ctl --control "$tmp/control.sock" quit >/dev/null 2>&1 || true
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  kata daemon stop >/dev/null 2>&1 || true
  # A step that set up its own Docbank exported DOCBANK_HOME into $tmp; stop that daemon even when the step failed.
  [[ ${DOCBANK_HOME:-} != "$tmp"/* ]] || docbank daemon stop >/dev/null 2>&1 || true
  if [[ $status -ne 0 ]]; then
    if [[ -n $in_extra ]]; then
      rm -rf "$tmp" # the steps passed; the failed script keeps its own folder
      printf 'Smoke failed at %s; its artifacts are named above\n' "$current" >&2
    else
      [[ ! -f "$tmp/tern.log" ]] || cat "$tmp/tern.log" >&2
      printf 'Smoke failed at %s; artifacts: %s\n' "${current:-setup}" "$tmp" >&2
    fi
    [[ -z ${SMOKE_FAIL_LOG:-} ]] || printf '%s; artifacts: %s\n' "${current:-setup}" "$tmp" >>"$SMOKE_FAIL_LOG"
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

source "$repo/smoke/ctl.sh"
ctl() {
  if [[ ${1:-} == --file && $# -eq 2 ]]; then
    # A scenario's own `ready` line has the same 20 s limit: wait for the prompt here first, so the line returns at once.
    if grep -q '^ready' "$2"; then ctl_waiting ready >/dev/null; fi
    if grep -q '^run .*tern-kata' "$2"; then ctl_typed "$2"; return; fi
  fi
  ctl_call "$@"
}
# Tern's `run` types the line and then waits a fixed 1.2 s for it to print, whether or not it has. A launch file's `run`
# lines that start tern-kata are typed and entered instead, and the wait ends when the program has drawn its columns or
# an error, which is what the steps' next clicks and keys need. Any other program (kata tui) draws neither and keeps
# Tern's own wait.
ctl_typed() {
  local deadline=$((SECONDS + wait_limit))
  sed -E 's/^run (.*)$/type \1\nkey Enter/' "$1" >"$tmp/typed.ctl"
  tern ctl --control "$tmp/control.sock" --file "$tmp/typed.ctl"
  while (( SECONDS < deadline )); do
    tern ctl --control "$tmp/control.sock" tree "[data-id^='main.columns.'], [data-id='main.error']" | jq -e '.count > 0' >/dev/null && return 0
    sleep 0.05
  done
  printf 'tern-kata drew no columns or error within %s s of %s\n' "$wait_limit" "$1" >&2
  return 1
}
# VISUAL wins over EDITOR in tern-kata, so one inherited from the shell would replace the editor fixture.
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP -u VISUAL EDITOR=%s/smoke/editor.sh KATA_HOME=%s %s/tern-kata"\n' "$repo" "$KATA_HOME" "$tmp" >"$tmp/launch.ctl"
ctl --file "$tmp/launch.ctl"

expect_tree() {
  local deadline=$((SECONDS + wait_limit))
  while (( SECONDS < deadline )); do
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
  local deadline=$((SECONDS + wait_limit))
  while (( SECONDS < deadline )); do
    [[ $(hint_mode) != "$1" ]] || return 0
    sleep 0.02
  done
  printf 'Expected keys in %s, found %s\n' "$1" "$(hint_mode)" >&2
  return 1
}
# pick_option <field data-id> <nth>: picks a popup card's select option with clicks, each acknowledged by the Pane before the
# next: the button click, until the footer offers "pick choice" (the Pane has the menu open), then the option click, until it
# no longer does. The Pane renders the menu's open state itself, and a render answering an earlier click that reaches Tern
# after a later one overrides it: the menu opens again after a pick, or closes under the next click.
pick_option() {
  ctl click "[data-id='$1.control'] .sf-select-btn"
  expect_tree "[data-id='dock.hints']" '.ok and .count == 1 and (.nodes[0].text | contains("pick choice"))'
  ctl click "[data-id='$1.control'] .sf-select-opt:nth-child($2)"
  expect_tree "[data-id='dock.hints']" '.ok and .count == 1 and (.nodes[0].text | contains("pick choice") | not)'
}
quit_app() {
  ctl key q
  expect_tree "[data-id^='main.']" '.ok and .count == 0'
  local deadline=$((SECONDS + wait_limit))
  while (( SECONDS < deadline )); do
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

for step in "${steps[@]}"; do
  current=$(basename "$step")
  printf '\nRunning %s\n' "$(basename "$step")"
  case "$step" in
    *.ctl) ctl --file "$step" ;;
    *.sh) source "$step" ;;
  esac
  printf 'Step passed %s\n' "$(basename "$step")"
done
current=quit_app
quit_app
current=
[[ -z ${SMOKE_SHARD:-} ]] || exit 0
in_extra=1
for extra in ${extras[@]+"${extras[@]}"}; do
  current=$(basename "$extra")
  run_extra "$extra" "$tmp/tern-kata"
done
in_extra=
current=
printf '\nSmoke passed\n'
