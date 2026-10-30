# w then g on the selected Issue starts the agent in the background: a block in the hidden session "kata agents", its tab named
# project#id, an agents.json entry marked background and not open. Then w opens the Agent card and enter opens the agent beside
# the Pane: the block moves into the Pane's tab and agents.json marks it open. w then g again starts another background agent,
# adding a tab to the session that is still there.
# A stand-in script from agent.toml's `command` plays omp. The Pane's blocks live on a session daemon of the step's own
# (`tern daemon --socket`): the headless `tern serve` window has no session daemon, so `tern ls`, `tern new` and `tern move` fail
# in its panes without one (checked on Tern 0.9.8). That window is not attached to the daemon either: its Pane is the window's own
# pane (env has TERN_WINDOW_PANE, no TERN_PANE; `ctl state` lists the one pane, not the daemon's blocks). So the step makes a block
# on the daemon for the Pane to stand beside and hands its id to the Pane as TERN_PANE, which is how Tern tells a real Pane.
# Not driven here, and why: going back when a working agent follows a waiting one. The relay's agent states come from the window
# plugin (window.luau sends `agent-states` to the Pane's surface with cx.session:event from cx.agents:list()), which run.sh does
# not link, and smoke/plugin.sh's window has no daemon blocks: cx.agents lists the window's own panes, whose ids are never the
# daemon block ids that `tern split` and `tern move` use. Tern takes a block for an agent from its own detection (omp, and programs
# speaking OSC 7501, which its notes say show working / waiting / finished); no ctl command or CLI call injects a surface event.
# The driver and gateway tests cover going back with faked tern output; a manual run in a real window is the check for it.
ba_dir="$tmp/workspace"
ba_config="$tmp/ba-config"
ba_state="$tmp/ba-state"
ba_sock="$tmp/ba-daemon.sock"
ba_standin="$tmp/ba-omp"
mkdir -p "$ba_config/tern-kata" "$ba_state"
printf 'command = ["%s"]\n' "$ba_standin" >"$ba_config/tern-kata/agent.toml"
# The stand-in records what it was started with and waits, as an interactive agent does.
printf '#!/bin/sh\nprintf "%%s\\n" "$@" >%q\nexec sleep 600\n' "$tmp/ba-argv" >"$ba_standin"
chmod +x "$ba_standin"
ba_tern() { TERN_DAEMON_SOCKET="$ba_sock" tern "$@"; }
ba_daemon_pid=
ba_stop() {
  [[ -z "$ba_daemon_pid" ]] || { kill "$ba_daemon_pid" 2>/dev/null || true; wait "$ba_daemon_pid" 2>/dev/null || true; }
  ba_daemon_pid=
  pkill -f -- "$ba_standin" 2>/dev/null || true
}
trap 'ba_status=$?; ba_stop; (exit $ba_status); cleanup' EXIT
tern daemon --socket "$ba_sock" >"$tmp/ba-daemon.log" 2>&1 &
ba_daemon_pid=$!
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ ! -S "$ba_sock" ]] || break
  sleep 0.1
done
[[ -S "$ba_sock" ]]
# The daemon exits once idle: the block the Pane stands in keeps it up.
ba_pane=$(ba_tern new session ba-pane --json -- sleep 600 | jq -r '.block')

kata --workspace "$ba_dir" create "Smoke background agent" --body "Started with w then g" --json >"$tmp/ba-issue.json"
ba_uid=$(jq -r '.issue.uid' "$tmp/ba-issue.json")
ba_short=$(jq -r '.issue.short_id' "$tmp/ba-issue.json")
ba_row="[data-id='main.columns.list.issues.$ba_uid']"
quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_DAEMON_SOCKET=%s TERN_PANE=%s XDG_CONFIG_HOME=%s XDG_STATE_HOME=%s KATA_HOME=%s %s/tern-kata)"\n' "$ba_dir" "$ba_sock" "$ba_pane" "$ba_config" "$ba_state" "$KATA_HOME" "$tmp" >"$tmp/ba-launch.ctl"
ctl --file "$tmp/ba-launch.ctl"
expect_tree "$ba_row" '.ok and .count == 1'
ctl click "$ba_row"

# The agent's tabs in the hidden session: the tab names and the block ids, from the daemon.
ba_tabs() { ba_tern ls --json | jq -c '[.sessions[] | select(.name == "kata agents") | .tabs[] | {name, blocks: [.blocks[].id]}]'; }
ba_wait_tabs() { # jq arguments: the options, then a predicate on ba_tabs
  for ((attempt = 0; attempt < 100; attempt++)); do
    ba_tabs | jq -e "$@" >/dev/null 2>&1 && return 0
    sleep 0.1
  done
  printf 'kata agents never matched %s: %s\n' "${*: -1}" "$(ba_tabs)" >&2
  return 1
}
ctl key w
expect_tree "[data-id^='layer.agent']" '.ok and .count >= 1'
ctl key g
expect_tree "[data-id\$='.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("Started an agent on") and contains("background"))'
ba_wait_tabs --arg n "smoke#$ba_short" '[.[] | select(.name == $n)] | length == 1'
ba_block=$(ba_tabs | jq -r --arg n "smoke#$ba_short" '.[] | select(.name == $n) | .blocks[0]')
# Hidden: the session is not shown, so the block is not beside the Pane.
ba_tern ls --json | jq -e '[.sessions[] | select(.name == "kata agents")][0].shown == false' >/dev/null
# The stand-in ran with the Issue's prompt file as omp's first message.
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ ! -s "$tmp/ba-argv" ]] || break
  sleep 0.1
done
grep -q '^@.*/agent-prompts/prompt-.*\.md$' "$tmp/ba-argv"
# agents.json (the Agent mapping) holds the block as a background agent that is not open.
jq -e --arg block "$ba_block" --arg uid "$ba_uid" '.agents | length == 1 and .[0].block == $block and .[0].issue_uid == $uid and .[0].background == true and .[0].open == false' \
  "$ba_state/tern-kata/agents.json" >/dev/null
# The Agent badge shows on the row, with no state until the window half relays one.
expect_tree "$ba_row" '.ok and (.nodes[0].text | contains("agent"))'

# w on an Issue that has an agent opens the Agent card; enter on the agent moves its block beside the Pane.
ctl key w
expect_tree "[data-id^='layer.agent']" '.ok and .count >= 1'
ctl key Enter
expect_tree "[data-id^='layer.agent']" '.ok and .count == 0'
ba_where() { ba_tern ls --json | jq -r --arg b "$1" '[.sessions[] as $s | $s.tabs[] as $t | $t.blocks[] | select(.id == ($b | tonumber)) | "\($s.name)/\($t.id)"][0] // "none"'; }
ba_pane_tab=$(ba_where "$ba_pane")
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ $(ba_where "$ba_block") != "$ba_pane_tab" ]] || break
  sleep 0.1
done
[[ $(ba_where "$ba_block") == "$ba_pane_tab" ]]
# The tab it left is gone with it, and agents.json says it is open.
ba_wait_tabs 'length == 0'
jq -e --arg block "$ba_block" '.agents | length == 1 and .[0].block == $block and .[0].background == true and .[0].open == true' \
  "$ba_state/tern-kata/agents.json" >/dev/null

# w on an Issue that has an agent opens the card, and g there starts another in the background: it finds the session that is
# still there and adds a tab to it.
ctl key w
expect_tree "[data-id^='layer.agent']" '.ok and .count >= 1'
ctl key g
expect_tree "[data-id^='layer.agent']" '.ok and .count == 0'
ba_wait_tabs 'length == 1'
jq -e --arg block "$ba_block" '.agents | length == 2 and all(.[]; .background == true) and ([.[] | select(.open)] | length == 1 and .[0].block == $block)' \
  "$ba_state/tern-kata/agents.json" >/dev/null
