#!/usr/bin/env bash
# The window half's release check, with a staged plugin/ folder, told it is installed, whose version is older than a
# locally served version.txt: one toast at window start, none after a restart, none with TERN_KATA_NO_UPDATE_CHECK, and a
# linked plugin never checks (the toggle command says so). A plugin reload, which runs the plugin again without a window
# start, checks again and reports a finished plugin update. Own temp state and headless Tern; no kata daemon is needed.
# Usage: smoke/release-check.sh
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
pid=
serve_pid=
export TERN_CONFIG_DIR="$tmp/config" TERN_DAEMON_SOCKET="$tmp/daemon.sock" STENCIL_LOG_DIR="$tmp/logs"
export XDG_CONFIG_HOME="$tmp/xdg-config" XDG_STATE_HOME="$tmp/xdg-state" KATA_HOME="$tmp/kata"
export STENCIL_LOG=warn,stencil_plugins=debug,stencil_tern=debug
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP TERN_KATA_NO_UPDATE_CHECK
cleanup() {
  status=$?
  trap - EXIT
  stop_tern
  [[ -z "$serve_pid" ]] || { kill "$serve_pid" 2>/dev/null || true; wait "$serve_pid" 2>/dev/null || true; }
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Release check smoke artifacts: %s\n' "$tmp" >&2; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source "$(dirname "$0")/ctl.sh"
ctl() { ctl_call "$@"; }
trace_failures

# A staged plugin/ folder as a release carries it, with a version older than any release.
mkdir -p "$tmp/plugin" "$tmp/dist" "$tmp/workspace" "$tmp/crates/tern/goldens" "$tmp/crates/plugins/fixtures"
copy_plugin "$tmp/plugin"
set_plugin_version "$tmp/plugin/plugin.toml" 0.0.1
printf '9.9.9\n' >"$tmp/dist/version.txt"
export STENCIL_FIXTURE_ROOT="$tmp/crates/tern"

serve_dir "$tmp/dist" "$tmp/http.log"
export TERN_KATA_BASE_URL="$serve_url"
fetches() { grep -c 'GET /version.txt' "$tmp/http.log" || true; }
fetches_before=$(fetches)

# Headless Tern loads a plugin only from its fixtures folder, so the window half sees that folder as its own. What the
# plugin asks with `tern plugin dir` is answered by a stand-in on the window's PATH: the fixtures folder (the plugin is
# installed there) or the real plugins folder (it is linked to a checkout somewhere else). Anything else goes to tern.
real_tern=$(command -v tern)
mkdir -p "$tmp/stub-bin"
cat >"$tmp/stub-bin/tern" <<STUB
#!/bin/sh
if [ "\$1 \$2" = "plugin dir" ]; then printf '%s\n' "\$STUB_PLUGIN_DIR"; exit 0; fi
exec "$real_tern" "\$@"
STUB
chmod +x "$tmp/stub-bin/tern"
ln -sfn "$tmp/plugin" "$tmp/crates/plugins/fixtures/tern-kata"
installed_dir="$STENCIL_FIXTURE_ROOT/../plugins/fixtures"

start_tern() {
  rm -f "$tmp/control.sock"
  PATH="$tmp/stub-bin:$PATH" STUB_PLUGIN_DIR="$plugin_dir" tern serve --control "$tmp/control.sock" --out "$tmp/shots" --dir "$tmp/workspace" >"$tmp/tern.log" 2>&1 &
  pid=$!
  wait_for socket_ready "$pid" "$tmp/control.sock"
  ctl ready
  # Headless Tern loads plugins only from the fixtures folder: it holds a link to the plugin folder in use.
  ctl plugins fixtures
}
stop_tern() {
  [[ -z "$pid" ]] || { ctl quit >/dev/null 2>&1 || true; kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; }
  pid=
}
# Toasts are not terminal text, and timers and fetch callbacks run on frames: each poll pumps the Pane (a `run` line draws one).
printf 'run "true"\n' >"$tmp/pump.ctl"
pump() { ctl --file "$tmp/pump.ctl" >/dev/null; }
toast_after_pump() { tree_has "$1" || { pump; tree_has "$1"; }; }
expect_toast() { wait_for toast_after_pump "$1"; }
# No toast, after the check had every chance to run: 15 pumps over about 1.5 s, a bound the step proves.
expect_no_toast() {
  local round
  for round in {1..15}; do pump; sleep 0.1; done
  if tree_has "$1"; then echo "unexpected toast: $1" >&2; return 1; fi
}
out_toast='tern-kata v9.9.9 is available (this window has v0.0.1)'
clear_kv() { find "$TERN_CONFIG_DIR" -name kv.json -delete; }


# Installed (the plugin dir the plugin is told holds it), newer release: one toast at window start.
plugin_dir="$installed_dir"
start_tern
expect_toast "$out_toast"
[[ $(fetches) -eq $((fetches_before + 1)) ]]
stop_tern

# A restart within the day neither fetches nor toasts.
start_tern
expect_no_toast "$out_toast"
[[ $(fetches) -eq $((fetches_before + 1)) ]]
stop_tern

# A plugin reload runs the plugin again without a window start: with the stored state gone it checks again.
start_tern
clear_kv
ctl plugins fixtures
fetched_twice() { [[ $(fetches) -eq $((fetches_before + 2)) ]] || { pump; [[ $(fetches) -eq $((fetches_before + 2)) ]]; }; }
wait_for fetched_twice
stop_tern

# A pending key naming this plugin's own version, left behind by a plugin step that ended well (done), toasts after a
# reload and is cleared.
start_tern
kv=$(find "$TERN_CONFIG_DIR" -name kv.json)
jq -n --argjson at "$(date +%s)" '{"update.pending": {version: "0.0.1", at: $at, done: true}, "release_check.last": $at}' >"$kv"
ctl plugins fixtures
expect_toast 'Updated the tern-kata plugin to v0.0.1.'
[[ $(jq 'has("update.pending")' "$kv") == false ]]
stop_tern

# From Tern 0.9.16 a step that outlives the reload ends as an event, which reports it. A key not marked done means the step
# is still running: the reload alone says nothing and leaves the key for the event.
if tern_at_least 0.9.16; then
  start_tern
  kv=$(find "$TERN_CONFIG_DIR" -name kv.json)
  jq -n --argjson at "$(date +%s)" '{"update.pending": {version: "0.0.1", at: $at}, "release_check.last": $at}' >"$kv"
  ctl plugins fixtures
  expect_no_toast 'Updated the tern-kata plugin'
  [[ $(jq 'has("update.pending")' "$kv") == true ]]
  stop_tern
fi

# The variable keeps checks off, though the stored state is gone.
clear_kv
TERN_KATA_NO_UPDATE_CHECK=1 start_tern
expect_no_toast "$out_toast"
[[ $(fetches) -eq $((fetches_before + 2)) ]]
stop_tern

# A linked plugin never checks, and the toggle command says why.
plugin_dir="$TERN_CONFIG_DIR/plugins"
clear_kv
start_tern
expect_no_toast "$out_toast"
[[ $(fetches) -eq $((fetches_before + 2)) ]]
printf 'palette "Toggle update checks"\nkey Enter\n' >"$tmp/palette.ctl"
ctl --file "$tmp/palette.ctl"
expect_toast 'A linked plugin never checks'
stop_tern
printf '\nRelease check smoke passed\n'
