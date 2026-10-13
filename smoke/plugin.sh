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
# The startup-failure toasts, told apart by these fragments: which one shows is the contract with the binary.
failed_start_toast='tern-kata failed to start'
not_installed_toast='tern-kata is not installed'
didnt_start_toast="tern-kata didn't start"
expect_no_kata_toast() {
  expect_no_toast "$failed_start_toast"
  expect_no_toast "$not_installed_toast"
  expect_no_toast "$didnt_start_toast"
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
show_issue_link() {
  command="printf '\\033[2J\\033[H\\033]8;;$1\\007Plugin link\\033]8;;\\007\\n'"
  printf 'run %s\n' "$(jq -rRn --arg command "$command" '$command | @json')" >"$tmp/link.ctl"
  ctl --file "$tmp/link.ctl"
  ctl expect Plugin
  ctl tree '.tv-grid' >"$tmp/link-tree.json"
  read -r x y < <(jq -r '.nodes[0].rect | "\(.[0]+4) \(.[1]+8)"' "$tmp/link-tree.json")
}
click_issue_link() {
  show_issue_link "$1"
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
# A Pane started by hand is a kata Pane when its title is `tern-kata` or its program's file name is tern-kata or
# tern-kata.exe, whatever the path: a link then focuses it and opens no second Pane. The stand-in never sets the title, so
# only its program name can identify it; the real binary (through its wrapper script) sets the title. The wrapper under
# another name is identified by title alone, and a longer name containing tern-kata is not a kata Pane at all.
standin_dir="$tmp/standin"
mkdir -p "$standin_dir"
# It prints while it waits for Enter so frames keep coming, or ctl focus would stall.
printf '#!/bin/bash\nwhile ! read -t 0.2 line; do echo tick; done\n' >"$standin_dir/tern-kata"
# A file name that holds backslashes stands for a Windows path: Linux has no .exe files, but the program name Tern
# reports is what the plugin splits.
cp "$standin_dir/tern-kata" "$standin_dir/C:\\Tools\\tern-kata.exe"
cp "$standin_dir/tern-kata" "$standin_dir/my-tern-kata"
ln -s "$tmp/pane-bin/tern-kata" "$standin_dir/kata-pane"
chmod +x "$standin_dir"/*
# ctl focus stalls while a hand-started Pane runs, so the link is shown before the split and clicked in place.
start_hand_pane() { # $1 the shell word that starts the Pane, $2 optional text its surface shows once up (else the stand-in's tick)
  show_issue_link "kata://smoke/$first"
  ctl split right
  ctl ready
  printf 'run %s\n' "$(jq -rRn --arg command "$1" '$command | @json')" >"$tmp/start.ctl"
  ctl --file "$tmp/start.ctl"
  if [[ -n "${2:-}" ]]; then ctl plugins expect "$2"; else ctl expect tick; fi
  started=$(ctl state | jq -r '.focused.id')
}
link_focuses_pane() { # the click opened no Pane and focused the one started
  ctl click "$x" "$y" cmd
  ctl state >"$tmp/full-path.json"
  jq -e --argjson id "$started" '.tabs[0].panes == 2 and .focused.id == $id' "$tmp/full-path.json" >/dev/null
}
# The program ends back at the shell prompt, so the Pane is closed by hand.
end_hand_started_pane() { # $1 the key that ends the program
  ctl key "$1"
  ctl ready
  ctl close
  expect_panes 1
}
start_hand_pane "$standin_dir/tern-kata"
link_focuses_pane
end_hand_started_pane Enter
start_hand_pane "$standin_dir/C:\\\\Tools\\\\tern-kata.exe"
link_focuses_pane
end_hand_started_pane Enter
start_hand_pane "$standin_dir/kata-pane" Daemon
link_focuses_pane
end_hand_started_pane q
# The real binary starts on the other Issue, so the link's select-issue must arrive for the first one to show.
start_hand_pane "$tmp/pane-bin/tern-kata --issue smoke/$second" '"Plugin second body"'
link_focuses_pane
ctl plugins expect '"Plugin first body"'
end_hand_started_pane q
start_hand_pane "$standin_dir/my-tern-kata"
ctl click "$x" "$y" cmd
# expect_panes waits for every Pane to reach its prompt, which the running stand-in never does.
wait_panes() {
  for ((attempt=0; attempt<100; attempt++)); do
    [[ $(ctl state | jq -r '.tabs[0].panes') == "$1" ]] && return 0
    sleep 0.1
  done
  echo "expected $1 Panes, state: $(ctl state | jq -c '.tabs[0]')" >&2
  return 1
}
wait_panes 3
ctl key q
wait_panes 2
# Focus went back to the left Pane; ctl focus would stall, so a click on the stand-in's Pane focuses it.
read -r x y < <(ctl tree '.tv-grid' | jq -r '.nodes[1].rect | "\(.[0]+4) \(.[1]+8)"')
ctl click "$x" "$y"
end_hand_started_pane Enter
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
# A launch of tern-kata that fails inside the 2 s window closes its Pane and toasts by exit status: 2 (Go panic) and 3
# (the binary's own error) are a failed start; 1 can be either (PowerShell's not-found, and Windows reports every failing
# launch as 1); anything else, such as 127, is "not installed".
wait_no_kata_toast() {
  for ((attempt=0; attempt<100; attempt++)); do
    if ! toast_shown "$failed_start_toast" && ! toast_shown "$not_installed_toast" && ! toast_shown "$didnt_start_toast"; then return 0; fi
    ctl --file "$tmp/pump.ctl"
    sleep 0.1
  done
  echo "kata toast did not clear" >&2
  return 1
}
expect_launch_toast() { # $1 the one startup-failure toast that must show
  wait_no_kata_toast
  ctl plugins run plugin.tern-kata.open
  ctl focus left
  expect_toast "$1"
  for toast in "$failed_start_toast" "$not_installed_toast" "$didnt_start_toast"; do
    [[ $toast == "$1" ]] || expect_no_toast "$toast"
  done
  expect_panes 1
}
for status in 2 3; do
  printf '#!/bin/sh\nexit %s\n' "$status" >"$tmp/pane-bin/tern-kata"
  expect_launch_toast "$failed_start_toast"
done
printf '#!/bin/sh\nexit 1\n' >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$didnt_start_toast"
printf '#!/bin/sh\nexit 127\n' >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$not_installed_toast"
# The real binary failing before it connects exits 3, which reads as a failed start.
printf '#!/bin/sh\necho start\nexec %q --issue\n' "$tmp/app/tern-kata" >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$failed_start_toast"
rm "$tmp/pane-bin/tern-kata"
expect_launch_toast "$not_installed_toast"
# The install command asks first: dismissing the picker opens no Pane, choosing a row opens one running the install one-liner.
printf 'palette "Install tern-kata"\nkey Enter\n' >"$tmp/install.ctl"
ctl --file "$tmp/install.ctl"
ctl ready
toast_shown 'Windows (PowerShell)'
toast_shown 'Linux or macOS'
ctl key Escape
expect_panes 1
ctl --file "$tmp/install.ctl"
ctl ready
toast_shown 'Linux or macOS'
ctl key Enter
ctl state >"$tmp/install-state.json"
jq -e '.tabs[0].panes == 2' "$tmp/install-state.json" >/dev/null
# The Pane waits for Enter after the script, then closes.
ctl expect Press
ctl key Enter
expect_panes 1
jq -Rse 'contains("plugin window halves loaded count=1") and (contains("plugin bind left out") | not) and (contains("plugin hook exceeded its budget") | not)' "$tmp/tern.log" >/dev/null
printf 'Plugin smoke passed: consecutive palette/key splits without reload, real link open/focus/select, shell-syntax Project link, quick quit without toast, links into Panes started by full path, backslash path and title alone, a longer name not matched, startup-failure and not-installed toasts by exit status\n'
