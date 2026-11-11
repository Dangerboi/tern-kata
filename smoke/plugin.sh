#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
pid=
server_pid=
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" TERN_CONFIG_DIR="$tmp/config"
export TERN_DAEMON_SOCKET="$tmp/daemon.sock" STENCIL_LOG_DIR="$tmp/logs"
# The linked plugin's host half reads worker.toml from the config folder: point it at an empty one.
export XDG_CONFIG_HOME="$tmp/xdg-config" XDG_STATE_HOME="$tmp/xdg-state"
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
  [[ -z "$server_pid" ]] || { kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; }
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Plugin smoke artifacts: %s\n' "$tmp" >&2; fi
  exit "$status"
}
trap cleanup EXIT
source "$(dirname "$0")/ctl.sh"
ctl() { ctl_call "$@"; }
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
# Both install hints point to the palette command.
install_hint='Install tern-kata from the command palette'
update_hint='run Update tern-kata'
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
# A second plugin stands in for the Pane's `W`: its commands open a tab through the window's own new-tab path with a command
# shaped like an agent's (prompt file named for the starting Pane, here the focused one), as `tern new tab` does.
mkdir -p "$tmp/crates/plugins/fixtures/tab-launcher"
printf 'schema = 1\nid = "tab-launcher"\nname = "Tab launcher"\nversion = "0.0.1"\ndescription = "Smoke stand-in for W."\nwindow = "window.luau"\n' >"$tmp/crates/plugins/fixtures/tab-launcher/plugin.toml"
cat >"$tmp/crates/plugins/fixtures/tab-launcher/window.luau" <<'LUAU'
local function tab(cx: WindowCx, name: string)
    cx.layout:new_tab({command = "sleep 600 @/state/agent-prompts/" .. name .. ".md"})
end
tern.command({id = "agent-tab", title = "Launch agent tab", run = function(cx)
    local focused = cx.session:focused()
    tab(cx, string.format("prompt-%.0f-12345", focused or 0))
end})
tern.command({id = "plain-tab", title = "Launch plain tab", run = function(cx) tab(cx, "prompt-12345") end})
tern.command({id = "gone-tab", title = "Launch gone-starter tab", run = function(cx) tab(cx, "prompt-999999-12345") end})
LUAU
ln -s "$(command -v kata)" "$tmp/bin/kata"
ln -s "$(command -v tern)" "$tmp/bin/tern"

# zsh is required: headless Tern runs the window plugin's timers only when it draws a frame (stencil-hq/tern-sdk#298),
# and since Tern 0.9.14 too few frames come for the plugin to hand a new kata Pane its Issue. Each interactive zsh redraws
# its prompt every 0.1 s while it waits at it, so frames keep coming; nothing is written while a program runs in the Pane.
command -v zsh >/dev/null 2>&1 || { echo "plugin.sh needs zsh" >&2; exit 1; }
export SHELL="$(command -v zsh)"
# Window process PATH lacks tern-kata; Pane shells resolve it via their shell rc.
export ZDOTDIR="$tmp" BASH_ENV="$tmp/.bashrc" ENV="$tmp/.bashrc"
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.zshenv"
cat >"$tmp/.zshrc" <<'ZSHRC'
TRAPALRM() { zle && zle reset-prompt; }
{ while kill -0 $$ 2>/dev/null; do sleep 0.1; kill -ALRM $$ 2>/dev/null; done } &!
ZSHRC
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.bashrc"
export PATH="$tmp/bin:/usr/bin:/bin"
# Update tern-kata reads version.txt from here.
mkdir -p "$tmp/dist"
printf '9.9.9\n' >"$tmp/dist/version.txt"
update_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
python3 -m http.server "$update_port" --bind 127.0.0.1 --directory "$tmp/dist" >"$tmp/http.log" 2>&1 &
server_pid=$!
for ((attempt=0; attempt<100; attempt++)); do curl -sf "http://127.0.0.1:$update_port/version.txt" >/dev/null && break; sleep 0.1; done
export TERN_KATA_BASE_URL="http://127.0.0.1:$update_port" TERN_KATA_NO_UPDATE_CHECK=1
# q asks "Quit tern-kata? y/n" unless kata's [tui] confirm_quit is false: the steps quit with one key.
mkdir -p "$KATA_HOME"
printf '[tui]\nconfirm_quit = false\n' >"$KATA_HOME/config.toml"
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
# With no agent waiting, the go-to-waiting palette command only toasts: no focus change, no Pane opened or closed.
ctl state >"$tmp/waiting-before.json"
printf 'palette "Go to the agent waiting for you"\nkey Enter\n' >"$tmp/waiting.ctl"
ctl --file "$tmp/waiting.ctl"
expect_toast "No agent is waiting for you"
ctl state >"$tmp/waiting-after.json"
jq -e --slurpfile before "$tmp/waiting-before.json" '.tabs[0].panes == $before[0].tabs[0].panes and .focused.id == $before[0].focused.id' "$tmp/waiting-after.json" >/dev/null
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
# A link in a tab with no kata Pane opens one in that tab, leaving the other tab's kata Pane alone; a second link reuses it.
ctl tab new
ctl ready
click_issue_link "kata://smoke/$first"
ctl plugins expect '"Plugin first body"'
ctl state >"$tmp/link-tab.json"
tab_kata_pane=$(jq -r '.focused.id' "$tmp/link-tab.json")
jq -e --argjson id "$kata_pane" '.tabs[1].active and .tabs[0].panes == 2 and .tabs[1].panes == 2 and .focused.id != $id' "$tmp/link-tab.json" >/dev/null
ctl focus left
click_issue_link "https://kata.invalid/smoke/$second"
ctl plugins expect '"Plugin second body"'
ctl state >"$tmp/link-tab-focus.json"
jq -e --argjson id "$tab_kata_pane" '.tabs[1].active and .tabs[0].panes == 2 and .tabs[1].panes == 2 and .focused.id == $id' "$tmp/link-tab-focus.json" >/dev/null
ctl key q
for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
ctl tab close
ctl ready
ctl state >"$tmp/link-tab-back.json"
jq -e --argjson id "$kata_pane" '(.tabs | length) == 1 and .focused.id == $id' "$tmp/link-tab-back.json" >/dev/null
ctl key q
expect_panes 1
# Shell syntax in a reference is passed to kata as data: the odd Project's Issue opens in a new kata Pane.
click_issue_link "kata://$odd_project/$odd"
ctl plugins expect '"Plugin odd body"'
expect_panes 2
ctl key q
expect_panes 1
expect_no_kata_toast
# A remote host's Pane ids carry its host tag above bit 44, which Tern 0.9.4 dropped for plugins, so they could not split
# or find such a Pane. A link there opens a kata split and selects its Issue; a second link reuses that Pane.
ctl remote loopback loop
ctl ready
remote_pane=$(ctl state | jq -r '.focused.id')
(( remote_pane >= 1 << 44 )) || { echo "remote Pane id $remote_pane carries no host tag" >&2; exit 1; }
click_issue_link "kata://smoke/$first"
ctl plugins expect '"Plugin first body"'
ctl state >"$tmp/remote-open.json"
remote_kata=$(jq -r '.focused.id' "$tmp/remote-open.json")
jq -e '(.tabs | length) == 2 and .tabs[1].active and .tabs[1].panes == 2' "$tmp/remote-open.json" >/dev/null
[[ $remote_kata != "$remote_pane" ]]
# Its tab face carries the kata mark: the face's own id is the one `ctl state` shows, though Lua got it packed, and the mark
# replaces the face's terminal icon, so the icon is no longer there.
for ((attempt=0; attempt<100; attempt++)); do
  ctl --file "$tmp/pump.ctl"
  [[ $(ctl tree ".tn-face[data-pane='$remote_kata']" | jq '.count') == 1 && $(ctl tree ".tn-face[data-pane='$remote_kata'] > *" | jq '.count') == 0 ]] && break
  sleep 0.1
done
ctl tree ".tn-face[data-pane='$remote_kata']" | jq -e '.count == 1' >/dev/null
ctl tree ".tn-face[data-pane='$remote_kata'] > *" | jq -e '.count == 0' >/dev/null
ctl focus left
click_issue_link "https://kata.invalid/smoke/$second"
ctl plugins expect '"Plugin second body"'
ctl state >"$tmp/remote-focus.json"
jq -e '.tabs[1].panes == 2' "$tmp/remote-focus.json" >/dev/null
[[ $(jq -r '.focused.id' "$tmp/remote-focus.json") == "$remote_kata" ]]
ctl key q
for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
ctl remote disconnect loop
ctl state | jq -e '(.tabs | length) == 1 and .tabs[0].active' >/dev/null
# A `W` agent's tab lands right of the tab of the Pane that started it; no other tab moves. Tabs are named by their first
# Pane's program, so the order is read from the program names: a shell tab is `zsh`/`bash`/..., a launched one is `sleep`.
tab_order() { ctl state | jq -r '[.tabs[] | if .panes == 1 and (.title | test("sleep")) then "agent" else "shell" end] | join(",")'; }
wait_tab_order() {
  for ((attempt=0; attempt<100; attempt++)); do
    ctl ready >/dev/null
    [[ $(tab_order) == "$1" ]] && return 0
    sleep 0.1
  done
  echo "expected tab order $1, got $(tab_order): $(ctl state | jq -c '.tabs')" >&2
  return 1
}
ctl state | jq -e '.tabs | length == 1' >/dev/null
ctl tab new
ctl ready
ctl tab new
ctl ready
wait_tab_order shell,shell,shell
# Back in the first tab, the agent tab goes second, ahead of the two tabs already there.
ctl tab 1
ctl ready
ctl plugins run plugin.tab-launcher.agent-tab
wait_tab_order shell,agent,shell,shell
# From the last tab it is the new last tab.
ctl tab 4
ctl ready
ctl plugins run plugin.tab-launcher.agent-tab
wait_tab_order shell,agent,shell,shell,agent
# A prompt file with no starter or a starter that is gone leaves the tab last.
ctl tab 1
ctl ready
ctl plugins run plugin.tab-launcher.plain-tab
wait_tab_order shell,agent,shell,shell,agent,agent
ctl tab 1
ctl ready
ctl plugins run plugin.tab-launcher.gone-tab
wait_tab_order shell,agent,shell,shell,agent,agent,agent
# Back to one shell tab for what follows: close the launched tabs, then the extra shell tabs.
while index=$(ctl state | jq -r '[.tabs | to_entries[] | select(.value.title | test("sleep")) | .key + 1] | first // empty') && [[ -n $index ]]; do
  ctl tab "$index"
  ctl tab close
  ctl ready
done
while [[ $(ctl state | jq '.tabs | length') -gt 1 ]]; do
  ctl tab 2
  ctl tab close
  ctl ready
done
ctl tab 1
expect_panes 1
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
expect_launch_toast() { # $1 the one startup-failure toast that must show, the rest text it must also carry
  wait_no_kata_toast
  ctl plugins run plugin.tern-kata.open
  ctl focus left
  expect_toast "$1"
  for hint in "${@:2}"; do toast_shown "$hint"; done
  for toast in "$failed_start_toast" "$not_installed_toast" "$didnt_start_toast"; do
    [[ $toast == "$1" ]] || expect_no_toast "$toast"
  done
  expect_panes 1
}
for status in 2 3; do
  printf '#!/bin/sh\nexit %s\n' "$status" >"$tmp/pane-bin/tern-kata"
  expect_launch_toast "$failed_start_toast" "$update_hint"
done
printf '#!/bin/sh\nexit 1\n' >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$didnt_start_toast" 'run Install tern-kata' "$update_hint"
printf '#!/bin/sh\nexit 127\n' >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$not_installed_toast" "$install_hint"
# The real binary failing before it connects exits 3, which reads as a failed start.
printf '#!/bin/sh\necho start\nexec %q --issue\n' "$tmp/app/tern-kata" >"$tmp/pane-bin/tern-kata"
expect_launch_toast "$failed_start_toast" "$update_hint"
rm "$tmp/pane-bin/tern-kata"
expect_launch_toast "$not_installed_toast" "$install_hint"
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
# The stand-in ends only on an empty line (Enter): the plugin sends a link's Issue to a kata Pane it opened as a non-empty line.
# Update tern-kata on a local session host runs `tern-kata update --pause` in a Pane and no plugin-only step: the binary
# brings the plugin along itself.
printf '#!/bin/bash\necho "$@" >%q\nprintf "Press Enter to close "\nwhile :; do read -t 0.2 line && [[ -z $line ]] && break; done\n' "$tmp/update-args.txt" >"$tmp/pane-bin/tern-kata"
chmod +x "$tmp/pane-bin/tern-kata"
# A curl stand-in on the PATH records any plugin-only step (`sh -c "curl … | sh"`), which a local session host never runs.
printf '#!/bin/sh\necho "$@" >>%q\necho "curl: smoke stand-in refuses" >&2\nexit 1\n' "$tmp/plugin-step.log" >"$tmp/bin/curl"
chmod +x "$tmp/bin/curl"
printf 'palette "Update tern-kata"\nkey Enter\n' >"$tmp/update.ctl"
ctl --file "$tmp/update.ctl"
for ((attempt=0; attempt<100; attempt++)); do
  [[ -s "$tmp/update-args.txt" ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
[[ $(cat "$tmp/update-args.txt") == "update --pause" ]]
ctl state | jq -e '.tabs[0].panes == 2' >/dev/null
ctl expect Press
# The local update finishes first, so the checks below exercise the watcher with a local session host.
ctl key Enter
for ((attempt=0; attempt<100; attempt++)); do
  [[ $(ctl state | jq -r '.tabs[0].panes') == 1 ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
expect_panes 1
# The watcher polls once a second: give it time to act on the closed Pane.
for ((attempt=0; attempt<30; attempt++)); do ctl --file "$tmp/pump.ctl"; sleep 0.1; done
[[ ! -e "$tmp/plugin-step.log" ]]
expect_no_toast 'tern-kata plugin updated'
# A plugin reload while a second update Pane is open (a local update brings the plugin along) must not turn that Pane into a
# kata Pane: a link opens a new kata Pane instead of focusing the update Pane.
rm -f "$tmp/update-args.txt"
ctl --file "$tmp/update.ctl"
for ((attempt=0; attempt<100; attempt++)); do
  [[ -s "$tmp/update-args.txt" ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
ctl expect Press
# The plugin is linked, and a linked window plugin is left as it is: for the remote plugin step below it must look installed,
# so `tern plugin dir` names the fixtures folder it loads from. The reload below makes the plugin ask again.
real_tern=$(readlink "$tmp/bin/tern")
printf '#!/bin/sh\n[ "$1 $2" = "plugin dir" ] && { echo %q; exit 0; }\nexec %q "$@"\n' "$STENCIL_FIXTURE_ROOT/../plugins/fixtures" "$real_tern" >"$tmp/stub-tern"
chmod +x "$tmp/stub-tern"
mv -f "$tmp/stub-tern" "$tmp/bin/tern"
ctl plugins fixtures
ctl focus left
click_issue_link "kata://smoke/$first"
for ((attempt=0; attempt<100; attempt++)); do
  ctl --file "$tmp/pump.ctl"
  [[ $(ctl state | jq -r '.tabs[0].panes') == 3 ]] && break
  sleep 0.1
done
ctl state | jq -e '.tabs[0].panes == 3' >/dev/null
# The new kata Pane is the stand-in too: Enter ends it, leaving the update Pane to the right.
ctl key Enter
expect_panes 2
ctl focus right
ctl key Enter
for ((attempt=0; attempt<100; attempt++)); do
  [[ $(ctl state | jq -r '.tabs[0].panes') == 1 ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
expect_panes 1
# On a remote session host the update also runs the plugin step on this machine; the curl stand-in makes it fail, which toasts
# the script's last stderr line and leaves no pending key behind.
rm -f "$tmp/plugin-step.log" "$tmp/update-args.txt"
ctl remote loopback loop
ctl ready
ctl --file "$tmp/update.ctl"
for ((attempt=0; attempt<100; attempt++)); do
  [[ -s "$tmp/update-args.txt" ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
[[ $(cat "$tmp/update-args.txt") == "update --pause" ]]
ctl expect Press
ctl key Enter
expect_toast 'tern-kata plugin was not updated'
expect_toast 'curl: smoke stand-in refuses'
[[ -s "$tmp/plugin-step.log" ]]
! grep -q 'update.pending' "$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json" || { echo "update.pending left behind" >&2; exit 1; }
for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
# From Tern 0.9.16 a plugin reload while the plugin step still runs delivers the step's end to the reloaded window VM as a
# process_exit event: the toast then comes from the marker match, not from the old VM's callback. The curl stand-in records
# that it started, sleeps, then answers; the plugin reloads in between.
if [[ $(printf '%s\n0.9.16\n' "$("$real_tern" --version | sed -n 's/^tern \([0-9.]*\).*/\1/p')" | sort -V | sed -n 1p) == 0.9.16 ]]; then
  reload_during_step() {
    rm -f "$tmp/step-started" "$tmp/update-args.txt"
    printf '#!/bin/sh\n: >%q\nsleep 4\n%s\n' "$tmp/step-started" "$1" >"$tmp/bin/curl"
    chmod +x "$tmp/bin/curl"
    ctl --file "$tmp/update.ctl"
    for ((attempt=0; attempt<100; attempt++)); do
      [[ -s "$tmp/update-args.txt" ]] && break
      ctl --file "$tmp/pump.ctl"
      sleep 0.1
    done
    [[ $(cat "$tmp/update-args.txt") == "update --pause" ]]
    ctl expect Press
    ctl key Enter
    for ((attempt=0; attempt<100; attempt++)); do
      [[ -e "$tmp/step-started" ]] && break
      ctl --file "$tmp/pump.ctl"
      sleep 0.1
    done
    [[ -e "$tmp/step-started" ]] || { echo "the plugin step never started" >&2; exit 1; }
    grep -q 'update.pending' "$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json" || { echo "update.pending not set while the step runs" >&2; exit 1; }
    ctl plugins fixtures
    # The stand-in is still asleep, so only the reloaded VM can see the step end.
    for ((attempt=0; attempt<100; attempt++)); do
      ! grep -q 'update.pending' "$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json" && break
      ctl --file "$tmp/pump.ctl"
      sleep 0.1
    done
  }
  reload_during_step 'echo "exit 0"'
  expect_toast 'tern-kata plugin updated to v9.9.9'
  ! grep -q 'update.pending' "$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json" || { echo "update.pending left behind after a reloaded success" >&2; exit 1; }
  for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
  reload_during_step 'echo "curl: smoke stand-in refuses after a reload" >&2; exit 1'
  expect_toast 'curl: smoke stand-in refuses after a reload'
  expect_toast 'tern-kata plugin was not updated'
  ! grep -q 'update.pending' "$TERN_CONFIG_DIR/plugin-data/tern-kata/kv.json" || { echo "update.pending left behind after a reloaded failure" >&2; exit 1; }
  for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
  # The original refusing stand-in again, for the checks below.
  printf '#!/bin/sh\necho "$@" >>%q\necho "curl: smoke stand-in refuses" >&2\nexit 1\n' "$tmp/plugin-step.log" >"$tmp/bin/curl"
  chmod +x "$tmp/bin/curl"
fi
# A window plugin already at the release's version is left alone: no plugin step, and the toast says it is up to date.
own_version=$(sed -n 's/^version *= *"\(.*\)"/\1/p' "$repo/plugin.toml")
printf '%s\n' "$own_version" >"$tmp/dist/version.txt"
rm -f "$tmp/plugin-step.log" "$tmp/update-args.txt"
ctl --file "$tmp/update.ctl"
for ((attempt=0; attempt<100; attempt++)); do
  [[ -s "$tmp/update-args.txt" ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
ctl expect Press
ctl key Enter
expect_toast "tern-kata plugin v$own_version is up to date"
[[ ! -e "$tmp/plugin-step.log" ]]
for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
# A linked window plugin is left as it is, as the install script would: no plugin step, and the toast says so.
ln -sfn "$real_tern" "$tmp/bin/tern"
ctl plugins fixtures
printf '9.9.9\n' >"$tmp/dist/version.txt"
rm -f "$tmp/plugin-step.log" "$tmp/update-args.txt"
ctl --file "$tmp/update.ctl"
for ((attempt=0; attempt<100; attempt++)); do
  [[ -s "$tmp/update-args.txt" ]] && break
  ctl --file "$tmp/pump.ctl"
  sleep 0.1
done
ctl expect Press
ctl key Enter
expect_toast 'tern-kata plugin is linked, left as it is'
[[ ! -e "$tmp/plugin-step.log" ]]
for ((attempt=0; attempt<100; attempt++)); do [[ $(ctl state | jq '.tabs[1].panes') == 1 ]] && break; sleep 0.1; done
ctl remote disconnect loop
jq -Rse 'contains("plugin window halves loaded count=2") and (contains("plugin bind left out") | not) and (contains("plugin hook exceeded its budget") | not)' "$tmp/tern.log" >/dev/null
printf 'Plugin smoke passed: consecutive palette/key splits without reload, real link open/focus/select, links reuse only the current tab'"'"'s kata Pane, shell-syntax Project link, links on a remote host'"'"'s Pane, quick quit without toast, the go-to-waiting command toasting with no agent waiting and moving nothing, links into Panes started by full path, backslash path and title alone, a longer name not matched, an agent tab opened right of the Pane'"'"'s tab and no other tab moved, startup-failure and not-installed toasts by exit status, Update tern-kata on a local session host running update --pause and no plugin step, the update Pane not a kata Pane after a plugin reload, a failed plugin step after Update tern-kata on a remote host toasting its last stderr line, no plugin step and an up-to-date toast when the window plugin is already at the release, no plugin step and a linked toast when the window plugin is linked, and (Tern 0.9.16 and later) a plugin step still running at a plugin reload ending as a process_exit in the reloaded window toasting its success with the pending key gone and its failure with the last stderr line\n'
