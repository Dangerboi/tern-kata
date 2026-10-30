# S on the selected Issue starts the agent in the background: a block in the hidden session "kata agents", its tab named project#id,
# an agents.json entry marked background and not open, and a second S adds a tab to the session that now exists. A stand-in script
# from agent.toml's `command` plays omp. The Pane gets its own session daemon (`tern daemon --socket`): the headless `tern serve`
# window has no session daemon, so `tern ls`, `tern new` and `tern move` fail in its panes without one (checked on Tern 0.9.8).
# Not driven here, and why:
#   - Opening beside the Pane and going back need the Pane to be a block of that daemon (TERN_PANE, a window to draw it); a
#     `tern serve` pane has neither, and the harness has no window to attach to a separate daemon.
#   - Going back needs the relay's agent states, which come from the window plugin (smoke/plugin.sh links it, but the headless
#     window there has the same missing daemon), and Tern must detect the stand-in as an agent block (rule not documented; a plain
#     script may not count).
# The driver and gateway tests cover both with faked tern output; a manual run in a real window is the check for them.
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
# The daemon exits once idle: a session of its own keeps it up.
ba_tern new session ba-keeper --json -- sleep 600 >/dev/null

kata --workspace "$ba_dir" create "Smoke background agent" --body "Started with S" --json >"$tmp/ba-issue.json"
ba_uid=$(jq -r '.issue.uid' "$tmp/ba-issue.json")
ba_short=$(jq -r '.issue.short_id' "$tmp/ba-issue.json")
ba_row="[data-id='main.columns.list.issues.$ba_uid']"
quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_DAEMON_SOCKET=%s XDG_CONFIG_HOME=%s XDG_STATE_HOME=%s KATA_HOME=%s %s/tern-kata)"\n' "$ba_dir" "$ba_sock" "$ba_config" "$ba_state" "$KATA_HOME" "$tmp" >"$tmp/ba-launch.ctl"
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
ctl key S
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

# A second S finds the session and adds a tab to it.
ctl key S
ba_wait_tabs 'length == 2'
jq -e '.agents | length == 2 and all(.[]; .background == true and .open == false)' "$ba_state/tern-kata/agents.json" >/dev/null
