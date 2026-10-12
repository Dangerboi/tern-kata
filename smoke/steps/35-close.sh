# Closing a Pane that runs tern-kata: Tern asks at once, as it does for kata's own TUI (the reference), also with
# the live stream open and with the daemon unreachable; once the user confirms no tern-kata process is left;
# cancelling the prompt leaves tern-kata working; q still quits. Tern raises the prompt itself, so the step pins that
# tern-kata never delays it. Each program runs in a tab of its own, so the Pane the next steps use is untouched.
# Own Issues; it cleans up.
pc_now() { date +%s%N; }
pc_pids() { pgrep -f "^$tmp/tern-kata" | sort | paste -sd' ' - || true; }
# Types $1 in the shell of a new tab.
pc_run() {
  ctl 'tab new' >/dev/null
  ctl ready >/dev/null
  printf 'run %s\n' "$(jq -rn --arg command "$1" '$command | @json')" >"$tmp/pc-run.ctl"
  ctl --file "$tmp/pc-run.ctl" >/dev/null
}
# Starts tern-kata in a new tab with the extra environment $1 and sets pc_pid to its process.
pc_start() {
  local before=" $(pc_pids) " pid
  pc_run "(cd $tmp/workspace && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP $1 KATA_HOME=$KATA_HOME $tmp/tern-kata)"
  pc_pid=
  for ((attempt = 0; attempt < 100; attempt++)); do
    for pid in $(pc_pids); do [[ $before == *" $pid "* ]] || pc_pid=$pid; done
    [[ -z $pc_pid ]] || return 0
    sleep 0.1
  done
  printf 'tern-kata did not start in the new tab\n' >&2
  return 1
}
# Presses Tern's close chord and prints the milliseconds until its prompt is up; the prompt stays open.
pc_prompt_ms() {
  local known start
  known=$(ctl system | jq '.dialogs | length')
  start=$(pc_now)
  ctl key "$pc_chord" >/dev/null
  until (( $(ctl system | jq '.dialogs | length') > known )); do
    if (( $(pc_now) - start > 5000000000 )); then printf 'No close prompt within 5 s of %s\n' "$pc_chord" >&2; return 1; fi
  done
  echo $(( ($(pc_now) - start) / 1000000 ))
}
# Prints the milliseconds since $2 until process $1 is gone; it fails after 10 s.
pc_gone_ms() {
  while [[ " $(pc_pids) " == *" $1 "* ]]; do
    if (( $(pc_now) - $2 > 10000000000 )); then printf 'tern-kata %s still running 10 s after the close\n' "$1" >&2; return 1; fi
    sleep 0.005
  done
  echo $(( ($(pc_now) - $2) / 1000000 ))
}
# Confirms the open prompt and requires the tab to be gone, tern-kata $1 to end within 2 s and no other to remain.
pc_confirm() {
  local start gone
  start=$(pc_now)
  ctl dialog button 0 >/dev/null
  gone=$(pc_gone_ms "$1" "$start")
  (( gone < 2000 )) || { printf 'tern-kata took %s ms to end after confirming\n' "$gone" >&2; return 1; }
  [[ $(pc_pids) == "$pc_main_pids" ]] || { printf 'tern-kata processes left: %s, want %s\n' "$(pc_pids)" "$pc_main_pids" >&2; return 1; }
  expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
  [[ $(ctl state | jq '.tabs | length') == "$pc_tabs" ]]
  printf 'tern-kata gone %s ms after confirming\n' "$gone"
}

pc_tabs=$(ctl state | jq '.tabs | length')
pc_main_pids=$(pc_pids)
[[ -n $pc_main_pids ]]
ctl confirm-close running >/dev/null

# The reference: another full-screen program. The headless Linux runner sends meta for the close chord's ctrl.
pc_chord=meta+shift+w
pc_run "(cd $tmp/workspace && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=$KATA_HOME kata tui)"
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ $(ctl state | jq -r '.focused.running') != null ]] && break
  sleep 0.1
done
sleep 1
pc_reference=$(pc_prompt_ms) || { pc_chord=ctrl+shift+w; pc_reference=$(pc_prompt_ms); }
ctl dialog button 0 >/dev/null
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
[[ $(ctl state | jq '.tabs | length') == "$pc_tabs" ]]

# tern-kata with its live stream open: an Issue made outside shows in the list, so the stream is up.
pc_start ""
expect_tree "[data-id^='main.columns.list.issues.']" '.ok and .count > 0'
kata --workspace "$tmp/workspace" create "Smoke close live Issue" --json >"$tmp/pc-live.json"
pc_live=$(jq -r '.issue.uid' "$tmp/pc-live.json")
expect_tree "[data-id\$='.$pc_live']" '.ok and .count == 1'
pc_ms=$(pc_prompt_ms)
printf 'Close prompt: kata tui %s ms, tern-kata with its stream open %s ms\n' "$pc_reference" "$pc_ms"
(( pc_ms - pc_reference <= 1000 )) || { printf 'The close prompt took %s ms longer than kata tui\n' "$((pc_ms - pc_reference))" >&2; return 1; }

# Cancelling keeps tern-kata working: its keys and its live stream.
ctl dialog cancel >/dev/null
[[ $(ctl system | jq '.dialogs | length') == 0 ]]
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
ctl key Escape
expect_tree '[data-id="main.help"]' '.ok and .count == 0'
kata --workspace "$tmp/workspace" create "Smoke close second live Issue" --json >"$tmp/pc-live2.json"
pc_live2=$(jq -r '.issue.uid' "$tmp/pc-live2.json")
expect_tree "[data-id\$='.$pc_live2']" '.ok and .count == 1'
[[ " $(pc_pids) " == *" $pc_pid "* ]]

# Confirming ends it at once and leaves no tern-kata behind.
pc_prompt_ms >/dev/null
pc_confirm "$pc_pid"
kata --workspace "$tmp/workspace" delete "$pc_live" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/pc-live.json")" --json >/dev/null
kata --workspace "$tmp/workspace" delete "$pc_live2" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/pc-live2.json")" --json >/dev/null

# tern-kata waiting on an unreachable daemon prompts as fast and ends as fast.
pc_start "KATA_SERVER=http://127.0.0.1:1"
expect_tree "[data-id='main.error']" '.ok and .count == 1'
pc_ms=$(pc_prompt_ms)
printf 'Close prompt: kata tui %s ms, tern-kata waiting on the daemon %s ms\n' "$pc_reference" "$pc_ms"
(( pc_ms - pc_reference <= 1000 )) || { printf 'The close prompt took %s ms longer than kata tui\n' "$((pc_ms - pc_reference))" >&2; return 1; }
pc_confirm "$pc_pid"

# q is unchanged: tern-kata ends, the shell is back at its prompt, and closing the idle shell asks nothing.
pc_start ""
expect_tree "[data-id^='main.columns.list.issues.']" '.ok and .count > 0'
pc_quit_start=$(pc_now)
ctl key q
printf 'tern-kata gone %s ms after q\n' "$(pc_gone_ms "$pc_pid" "$pc_quit_start")"
for ((attempt = 0; attempt < 100; attempt++)); do
  ctl state >"$tmp/pc-state.json"
  jq -e '.focused.prompt == true and .focused.running == null and .focused.busy == false' "$tmp/pc-state.json" >/dev/null && break
  sleep 0.1
done
jq -e '.focused.prompt == true and .focused.running == null' "$tmp/pc-state.json" >/dev/null
ctl close >/dev/null
[[ $(pc_pids) == "$pc_main_pids" ]]

ctl confirm-close never >/dev/null
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
[[ $(ctl state | jq '.tabs | length') == "$pc_tabs" ]]
