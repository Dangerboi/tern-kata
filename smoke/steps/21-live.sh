ctl tree "[data-id^='main.columns.list.issues.']" >"$tmp/live-before.json"
selected_before=$(jq -r '.nodes[] | select(.["aria-selected"] == "true") | .["data-id"]' "$tmp/live-before.json")
kata --workspace "$tmp/workspace" create "Smoke live Issue" --body "Created outside the Pane" --json
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke live Issue"))'
ctl tree "[data-id^='main.columns.list.issues.']" >"$tmp/live-after.json"
[[ $(jq -r '.nodes[] | select(.["aria-selected"] == "true") | .["data-id"]' "$tmp/live-after.json") == "$selected_before" ]]

# Use a fixed throwaway TCP target so locate cannot auto-start away a failure.
quit_app
stop_live_daemon
kata daemon start --listen 127.0.0.1:0
stream_target=$(kata daemon locate --json | jq -r .address)
stop_live_daemon
printf 'ready\nrun "env -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s KATA_SERVER=http://%s %s/tern-kata"\n' "$KATA_HOME" "$stream_target" "$tmp" >"$tmp/live-launch.ctl"
ctl --file "$tmp/live-launch.ctl"
expect_tree "[data-id='main.error']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("kata server not responding:"))'
expect_tree "[data-id='main.retry']" '.ok and .count == 1'
kata daemon start --listen "$stream_target"
ctl click "[data-id='main.retry']"
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke live Issue"))'

stop_live_daemon
expect_tree "[data-id='main.stale']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Read-only: reconnecting"))'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke live Issue"))'
kata daemon start --listen "$stream_target"
kata --workspace "$tmp/workspace" create "Smoke reconnected Issue" --json
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke reconnected Issue"))'
expect_tree "[data-id='main.stale']" '.ok and .count == 0'
