#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
# A run that starts stops the kata and Docbank daemons that killed runs left behind (smoke/sweep-daemons.sh).
[[ -n ${SMOKE_SHARD:-} ]] || "$repo/smoke/sweep-daemons.sh"

# The scripts that run beside the steps, each given the binary.
extra_scripts=(plugin.sh supervision.sh release-check.sh)

# With no argument the suite fans out. The steps wait on kata's daemon clocks (a timed claim's sweep, a job's delivery
# window), so those steps and the two other scripts run beside the rest on their own headless Tern and kata daemon;
# a shard is one such run (SMOKE_SHARD set, the steps given as arguments, the binary built once in SMOKE_BIN).
# With arguments the run is only those: step files in the order given on one server, then plugin.sh, supervision.sh and
# release-check.sh when named (./smoke/run.sh smoke/steps/72-x.sh smoke/supervision.sh), handed the binary built once.
# A failed run ends with a line naming the failing step or script and the artifacts folder.
if [[ -z ${SMOKE_SHARD:-} && $# -eq 0 ]]; then
  export CGO_ENABLED=0
  source "$repo/smoke/ctl.sh"
  bin_dir=$(mktemp -d)
  # The jobs are subshells that wait for a shard or script, so the signal goes to each job's own child; each of those
  # runs its cleanup on TERM, and the subshell ends with it, so `wait` returns once every cleanup is done.
  stop_jobs() {
    local job
    for job in $(jobs -p); do
      pkill -TERM -P "$job" 2>/dev/null || true
    done
    wait 2>/dev/null || true
  }
  trap 'stop_jobs; rm -rf "$bin_dir"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  # -buildvcs=false keeps a build at a release tag a dev build: a stamped one opens its release notes over the Pane at its first start.
  (cd "$repo" && go build -buildvcs=false -o "$bin_dir/tern-kata" ./cmd/tern-kata)
  # Steps 81 and 82 and supervision.sh serve a release built at the current version; one build serves all three.
  build_release_dist "$bin_dir/dist"
  slow=(51-claim-expiry.sh 60-cron-made-issue.sh 62-job-card.sh 81-install-from-release.sh 82-update-from-release.sh)
  # The rest is cut at steps that start from the freshly launched Pane (each shard has its own seeded Project), so the
  # five groups run side by side: steps 01-21, 22-31, 32-44, 45-57 and 58 on.
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
  # Each job gets its own fail log, where its shard appends "<step>; artifacts: <folder>"; a job that fails without a
  # line there (killed, or a script) is named by this function, so the last line always names every failure.
  timed() { # timed <name> <command...>
    local start=$SECONDS status=0 log
    log=$(mktemp "$bin_dir/failure.XXXXXX")
    SMOKE_FAIL_LOG=$log "${@:2}" || status=$?
    printf '%s %s\n' "$((SECONDS - start))" "$1" >>"$durations"
    if (( status )) && [[ ! -s $log ]]; then printf '%s; failed without naming a step (see its output above)\n' "$1" >"$log"; fi
    return "$status"
  }
  export SMOKE_SHARD=1 SMOKE_BIN="$bin_dir/tern-kata" SMOKE_DIST="$bin_dir/dist"
  for group in "${groups[@]}"; do
    set -- $group
    first=$(basename "$1") last=$(basename "${!#}")
    timed "steps ${first:0:2}-${last:0:2}" "$0" "$@" & jobs+=($!)
  done
  for name in "${slow[@]}"; do timed "$name" "$0" "$repo/smoke/steps/$name" & jobs+=($!); done
  for extra in "${extra_scripts[@]}"; do
    timed "$extra" "$repo/smoke/$extra" "$SMOKE_BIN" & jobs+=($!)
  done
  failed=0
  for job in "${jobs[@]}"; do wait "$job" || failed=1; done
  if (( failed )); then
    printf 'Smoke failed at %s\n' "$(cat "$bin_dir"/failure.* | paste -sd, | sed 's/,/, /g')" >&2
    exit 1
  fi
  slowest=$(sort -rn "$durations" | head -3 | awk '{ s = $1; $1 = ""; printf "%s%s %d s", sep, substr($0, 2), s; sep = ", " }')
  printf '\nSmoke passed in %d s; slowest: %s\n' "$SECONDS" "$slowest"
  exit 0
fi

# The arguments are step files, plus the three scripts the fan-out runs beside them when named.
steps=()
extras=()
for arg in "$@"; do
  if [[ " ${extra_scripts[*]} " == *" $(basename "$arg") "* ]]; then extras+=("$arg"); else steps+=("$arg"); fi
done
(( $# )) || steps=("$repo"/smoke/steps/*)

if (( ${#steps[@]} == 0 )); then
  export CGO_ENABLED=0
  bin_dir=$(mktemp -d)
  trap 'rm -rf "$bin_dir"' EXIT
  if [[ -n ${SMOKE_BIN:-} ]]; then
    cp "$SMOKE_BIN" "$bin_dir/tern-kata"
  else
    (cd "$repo" && go build -buildvcs=false -o "$bin_dir/tern-kata" ./cmd/tern-kata)
  fi
  current=
  for extra in "${extras[@]}"; do
    current=$(basename "$extra")
    "$extra" "$bin_dir/tern-kata" || { printf '\nSmoke failed at %s; its artifacts are named above\n' "$current" >&2; exit 1; }
  done
  printf '\nSmoke passed\n'
  exit 0
fi

tmp=$(mktemp -d)
# Every daemon home lives in $tmp; smoke-owner names this script, so the sweep knows when the folder's run is gone.
echo $$ >"$tmp/smoke-owner"
current=
in_extra=
server_pid=
proxy_pid=
# HTTP servers the steps start (release dist/ served from a local port): stopped on every exit.
http_pids=()
export CGO_ENABLED=0
export KATA_HOME="$tmp/kata"
export TERN_CONFIG_DIR="$tmp/config"
# SMOKE_TERN_SETTINGS=<file> runs Tern with that settings.json (a user's material, themes and widgets) instead of an empty
# config, to reproduce what the headless run does not show.
if [[ -n ${SMOKE_TERN_SETTINGS:-} ]]; then mkdir -p "$TERN_CONFIG_DIR" && cp "$SMOKE_TERN_SETTINGS" "$TERN_CONFIG_DIR/settings.json"; fi
export XDG_CONFIG_HOME="$tmp/xdg-config"
export XDG_STATE_HOME="$tmp/xdg-state"
# tern-kata downloads Attachments into os.UserCacheDir: keep them in $tmp, not the user's ~/.cache.
export XDG_CACHE_HOME="$tmp/xdg-cache"
export TERN_DAEMON_SOCKET="$tmp/tern-daemon.sock"
export STENCIL_LOG_DIR="$tmp/logs"
# The Pane's hourly `kata update --check` asks GitHub: the steps run without the network.
export TERN_KATA_NO_UPDATE_CHECK=1
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
  "$repo/smoke/sweep-daemons.sh" "$tmp"
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
# q asks "Quit tern-kata?" unless kata's [tui] confirm_quit is false: the steps quit with one key, and the quit step turns it on.
printf '[tui]\nconfirm_quit = false\n' >"$KATA_HOME/config.toml"
kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json
kata --workspace "$tmp/workspace" create "Smoke first Issue" --body "First seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke second Issue" --body "Second seeded body" --json
kata --workspace "$tmp/workspace" create "Smoke third Issue" --body "Third seeded body" --json
if [[ -n ${SMOKE_BIN:-} ]]; then
  cp "$SMOKE_BIN" "$tmp/tern-kata"
else
  (cd "$repo" && go build -buildvcs=false -o "$tmp/tern-kata" ./cmd/tern-kata)
fi

source "$repo/smoke/ctl.sh"
tern serve --control "$tmp/control.sock" --out "$tmp/shots" --dir "$tmp/workspace" >"$tmp/tern.log" 2>&1 &
server_pid=$!
wait_for socket_ready "$server_pid" "$tmp/control.sock"

# Tern 0.7 lays out only the part of a long scroller near the screen, and `ctl tree` lists only laid-out nodes, so
# steps could not find rows or tabs below the fold. This sheet lays everything out, as Tern 0.6 did.
printf '* { content-visibility: visible !important; }\n' >"$tmp/visible.css"
tern ctl --control "$tmp/control.sock" css smoke-visible "$tmp/visible.css"

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
# VISUAL wins over EDITOR in tern-kata, so one inherited from the shell would replace the editor fixture. Headless Tern starts its
# shells with a PATH of the system folders only; the Pane runs kata (`daemon recover` for a stopped profile), so it gets the run's PATH.
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP -u VISUAL PATH=%s EDITOR=%s/smoke/editor.sh KATA_HOME=%s %s/tern-kata"\n' "$PATH" "$repo" "$KATA_HOME" "$tmp" >"$tmp/launch.ctl"
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
# The mode the footer hints show the keys are in: collections, detail, list or other. The Menu is the footer of only move, open
# and back (no "new"): Board, Config and Attention footers also say " open" but carry other words.
hint_mode() {
  ctl tree "[data-id='dock.hints']" | jq -r '.nodes[0].text | if contains("J/K row") then "detail" elif contains("layout") or contains("expand") then "list" elif contains(" move") and contains(" open") and contains(" back") and (contains(" new") | not) then "collections" else "other" end'
}
hint_in_mode() { [[ $(hint_mode) == "$1" ]]; }
hint_expect_mode() {
  wait_for -i 0.02 hint_in_mode "$1" || { printf 'Expected keys in %s, found %s\n' "$1" "$(hint_mode)" >&2; return 1; }
}
# pick_option <field data-id> <nth>: picks a popup card's select option with clicks, each acknowledged by the Pane before the
# next: the button click, until the footer offers "pick choice" (the Pane has the menu open), then the option click, until it
# no longer does. A step then reads the Pane's state, not only Tern's, before its next click.
pick_option() {
  ctl click "[data-id='$1.control'] .sf-select-btn"
  expect_tree "[data-id='dock.hints']" '.ok and .count == 1 and (.nodes[0].text | contains("pick choice"))'
  ctl click "[data-id='$1.control'] .sf-select-opt:nth-child($2)"
  expect_tree "[data-id='dock.hints']" '.ok and .count == 1 and (.nodes[0].text | contains("pick choice") | not)'
}
quit_app() {
  ctl key q
  expect_tree "[data-id^='main.']" '.ok and .count == 0'
  wait_for -i 0.05 shell_idle
}
shell_idle() {
  ctl state >"$tmp/state.json" 2>/dev/null || true
  jq -e '.focused.prompt == true and .focused.running == null and .focused.busy == false' "$tmp/state.json" >/dev/null 2>&1
}


kata_daemon_gone() { kata daemon status --json | jq -e '.daemons | length == 0' >/dev/null; }
stop_live_daemon() {
  kata daemon stop
  wait_for kata_daemon_gone
}

for step in "${steps[@]}"; do
  current=$(basename "$step")
  printf '\nRunning %s\n' "$(basename "$step")"
  case "$step" in
    *.ctl) ctl --file "$step" ;;
    *.sh) source "$step" ;;
    *) printf 'Unknown step %s: a step is a .ctl or .sh file under smoke/steps\n' "$step" >&2; exit 1 ;;
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
  "$extra" "$tmp/tern-kata"
done
in_extra=
current=
printf '\nSmoke passed\n'
