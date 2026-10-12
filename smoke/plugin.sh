#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
pid=
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" TERN_CONFIG_DIR="$tmp/config"
export TERN_DAEMON_SOCKET="$tmp/daemon.sock" STENCIL_LOG_DIR="$tmp/logs"
export STENCIL_FIXTURE_ROOT="$tmp/crates/tern"
export STENCIL_LOG=warn,stencil_plugins=debug,stencil_tern=debug
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "$pid" ]]; then
    if [[ $status -ne 0 ]]; then
      ctl tree >"$tmp/tree.json" 2>/dev/null || true
      ctl state >"$tmp/state.json" 2>/dev/null || true
    fi
    ctl quit >/dev/null 2>&1 || true
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ -f "$TERN_CONFIG_DIR/plugins/tern-kata.path" ]]; then
    tern plugin unlink tern-kata --json || status=1
    [[ ! -f "$TERN_CONFIG_DIR/plugins/tern-kata.path" ]] || status=1
  fi
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Plugin smoke artifacts: %s\n' "$tmp" >&2; fi
  exit "$status"
}
trap cleanup EXIT
ctl() { tern ctl --control "$tmp/control.sock" "$@"; }
# Waits (real time) until the active tab has $1 Panes: a quit closes the launch-command Pane.
expect_panes() {
  for ((attempt=0; attempt<100; attempt++)); do
    ctl ready
    [[ $(ctl state | jq -r '.tabs[0].panes') == "$1" ]] && return 0
    sleep 0.1
  done
  echo "expected $1 Panes, state: $(ctl state | jq -c '.tabs[0]')" >&2
  return 1
}
# Toasts are not terminal text: poll the rendered tree for one. Headless Tern runs plugin timers only on frames, and
# a Pane that already exited produces none, so each attempt runs a command in the original Pane.
toast_shown() { ctl tree | jq -e --arg text "$1" 'tostring | contains($text)' >/dev/null; }
# `! f` under set -e never fails the script, so the negated checks go through this.
expect_no_toast() {
  if toast_shown "$1"; then echo "unexpected toast: $1" >&2; return 1; fi
}
expect_no_kata_toast() {
  expect_no_toast 'tern-kata is not installed'
  expect_no_toast 'tern-kata exited with status'
}
expect_toast() {
  printf 'run "true"\n' >"$tmp/pump.ctl"
  for ((attempt=0; attempt<20; attempt++)); do
    toast_shown "$1" && return 0
    ctl --file "$tmp/pump.ctl"
  done
  echo "toast not shown: $1" >&2
  return 1
}
mkdir -p "$tmp/workspace" "$tmp/bin" "$tmp/pane-bin" "$tmp/app" "$tmp/crates/tern/goldens" "$tmp/crates/plugins/fixtures"
if [[ $# -gt 0 ]]; then cp "$1" "$tmp/app/tern-kata"; else (cd "$repo" && go build -o "$tmp/app/tern-kata" ./cmd/tern-kata); fi
printf '#!/bin/sh\nexec env KATA_HOME=%q %q "$@"\n' "$KATA_HOME" "$tmp/app/tern-kata" >"$tmp/pane-bin/tern-kata"
chmod +x "$tmp/pane-bin/tern-kata"
ln -s "$repo" "$tmp/crates/plugins/fixtures/tern-kata"
ln -s "$(command -v kata)" "$tmp/bin/kata"
ln -s "$(command -v tern)" "$tmp/bin/tern"

if command -v zsh >/dev/null 2>&1; then
  export SHELL="$(command -v zsh)"
fi
# Window process PATH lacks tern-kata; Pane shells resolve it via their shell rc.
export ZDOTDIR="$tmp" BASH_ENV="$tmp/.bashrc" ENV="$tmp/.bashrc"
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.zshenv"
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.bashrc"
export PATH="$tmp/bin:/usr/bin:/bin"
kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json
kata --workspace "$tmp/workspace" create 'Plugin first Issue' --body 'Plugin first body' --json >"$tmp/first.json"
kata --workspace "$tmp/workspace" create 'Plugin second Issue' --body 'Plugin second body' --json >"$tmp/second.json"
first=$(jq -r '.issue.short_id' "$tmp/first.json")
second=$(jq -r '.issue.short_id' "$tmp/second.json")
# A Project name full of shell syntax: its links must still open, since the reference never reaches a shell.
odd_project='odd;$x&y'
mkdir -p "$tmp/odd"
kata --workspace "$tmp/odd" init --project "$odd_project" --json
kata --workspace "$tmp/odd" create 'Plugin odd Issue' --body 'Plugin odd body' --json >"$tmp/odd.json"
odd=$(jq -r '.issue.short_id' "$tmp/odd.json")
(cd "$repo" && tern plugin link . --json)
! command -v tern-kata >/dev/null 2>&1 || { echo "tern-kata must not be on window PATH" >&2; exit 1; }
tern serve --control "$tmp/control.sock" --out "$tmp/shots" --dir "$tmp/workspace" >"$tmp/tern.log" 2>&1 &
pid=$!
for ((attempt=0; attempt<200; attempt++)); do
  [[ ! -S "$tmp/control.sock" ]] || break
  kill -0 "$pid"
  sleep 0.1
done
[[ -S "$tmp/control.sock" ]]
ctl ready
# Each split Pane gets half the window and kata lays out from 80 columns.
read -r window_height < <(ctl stats | jq -r '.viewport[1]')
ctl resize 1600 "$window_height"
ctl plugins fixtures
printf 'palette "Open kata"\nkey Enter\n' >"$tmp/palette.ctl"
ctl --file "$tmp/palette.ctl"
ctl plugins expect smoke
ctl state >"$tmp/palette-state.json"
jq -e --arg cwd "$tmp/workspace" '.tabs[0].panes == 2 and .focused.cwd == $cwd' "$tmp/palette-state.json" >/dev/null
palette_pane=$(jq -r '.focused.id' "$tmp/palette-state.json")
# A normal quit after the 2 s window is not an install failure.
# Timers run only on frames, so pump the original Pane for the 2.5 s to let the 2 s window really end.
ctl focus left
printf 'run "true"\n' >"$tmp/pump.ctl"
for ((attempt=0; attempt<25; attempt++)); do ctl --file "$tmp/pump.ctl"; sleep 0.1; done
ctl focus right
ctl key q
expect_panes 1
ctl focus left
ctl --file "$tmp/pump.ctl"
expect_no_kata_toast
ctl key F9
ctl plugins expect smoke
ctl state >"$tmp/key-state.json"
jq -e --arg cwd "$tmp/workspace" --argjson previous "$palette_pane" '.tabs[0].panes == 2 and .focused.cwd == $cwd and .focused.id != $previous' "$tmp/key-state.json" >/dev/null
ctl key q
expect_panes 1
# A quit inside the 2 s window is a clean exit too, not a startup failure.
expect_no_kata_toast
# A real OSC-8 terminal link exercises route.link, not a plugin-origin cx:open.
click_issue_link() {
  command="printf '\\033[2J\\033[H\\033]8;;$1\\007Plugin link\\033]8;;\\007\\n'"
  printf 'run %s\n' "$(jq -rRn --arg command "$command" '$command | @json')" >"$tmp/link.ctl"
  ctl --file "$tmp/link.ctl"
  ctl expect Plugin
  ctl tree '.tv-grid' >"$tmp/link-tree.json"
  read -r x y < <(jq -r '.nodes[0].rect | "\(.[0]+4) \(.[1]+8)"' "$tmp/link-tree.json")
  ctl click "$x" "$y" cmd
}
click_issue_link "kata://smoke/$first"
ctl plugins expect '"Plugin first body"'
ctl state >"$tmp/link-open.json"
kata_pane=$(jq -r '.focused.id' "$tmp/link-open.json")
jq -e '.tabs[0].panes == 2' "$tmp/link-open.json" >/dev/null
ctl focus left
click_issue_link "https://kata.invalid/smoke/$second"
ctl plugins expect '"Plugin second body"'
ctl state >"$tmp/link-focus.json"
jq -e --argjson id "$kata_pane" '.tabs[0].panes == 2 and .focused.id == $id' "$tmp/link-focus.json" >/dev/null
ctl key q
expect_panes 1
# Shell syntax in a reference is passed to kata as data: the odd Project's Issue opens in a new kata Pane.
click_issue_link "kata://$odd_project/$odd"
ctl plugins expect '"Plugin odd body"'
expect_panes 2
ctl key q
expect_panes 1
expect_no_kata_toast
# A tern-kata that fails after the 2 s window still closes its Pane, with no install toast. The stub prints while it
# runs so frames (and the 2 s window) advance, and ctl can focus the original Pane before the stub exits.
printf '#!/bin/sh\ni=0\nwhile [ $i -lt 30 ]; do echo tick; sleep 0.2; i=$((i+1)); done\nexit 1\n' >"$tmp/pane-bin/tern-kata"
ctl plugins run plugin.tern-kata.open
seen=0
for ((attempt=0; attempt<100; attempt++)); do
  panes=$(ctl state | jq -r '[.tabs[].panes] | add')
  if [[ $panes == 2 && $seen == 0 ]]; then seen=1; ctl focus left; fi
  [[ $seen == 1 && $panes == 1 ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
[[ $seen == 1 && $panes == 1 ]] || { echo "failing tern-kata Pane did not close (seen=$seen panes=$panes)" >&2; exit 1; }
expect_no_kata_toast
# A tern-kata failing inside the 2 s window closes its Pane and toasts its status.
printf '#!/bin/sh\nexit 3\n' >"$tmp/pane-bin/tern-kata"
ctl plugins run plugin.tern-kata.open
ctl focus left
expect_toast 'tern-kata exited with status 3'
expect_panes 1
rm "$tmp/pane-bin/tern-kata"
ctl plugins run plugin.tern-kata.open
ctl focus left
expect_toast 'tern-kata is not installed'
expect_toast 'go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest'
expect_panes 1
jq -Rse 'contains("plugin window halves loaded count=1") and (contains("plugin bind left out") | not) and (contains("plugin hook exceeded its budget") | not)' "$tmp/tern.log" >/dev/null
printf 'Plugin smoke passed: consecutive palette/key splits without reload, real link open/focus/select, shell-syntax Project link, quick quit without toast, startup-failure and missing-binary toasts\n'
