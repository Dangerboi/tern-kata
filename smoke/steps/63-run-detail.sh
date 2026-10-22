# The Jobs Page says this machine's worker serves the Project, and R runs an adapter job on it now. The Runs Page then follows the run:
# the agent's output tail shows while it runs and it ends succeeded. A run held after the worker and its agent were killed shows its
# reason, and u resumes it from the Pane. Own worker (temp XDG config and state, the worker test stub as the agent), jobs and Pane; it
# cleans up. `o` opens a Tern tab in the live Tern, which the headless harness does not have, so the driver tests cover its command.
rd_dir="$tmp/workspace"
rd_bin="$tmp/rd-bin"
rd_config="$tmp/rd-config"
rd_state="$tmp/rd-state"
rd_checkout="$tmp/rd-checkout"
mkdir -p "$rd_bin" "$rd_config/tern-kata" "$rd_state" "$rd_checkout"
(cd "$repo" && go build -o "$rd_bin/omp" ./internal/worker/workertest/stub)
cat >"$rd_config/tern-kata/worker.toml" <<TOML
executor = "smoke-box"
actor = "smoke"
[[projects]]
name = "smoke"
[checkouts]
repo = "$rd_checkout"
[profiles.default]
command = "$rd_bin/omp"
[profiles.slow]
command = "$rd_bin/omp"
args = ["--stub-text", "stub says all checked", "--stub-pause", "8000"]
[profiles.hang]
command = "$rd_bin/omp"
args = ["--stub-hang-first", "--stub-text", "resumed ok"]
TOML
rd_env=(env XDG_CONFIG_HOME="$rd_config" XDG_STATE_HOME="$rd_state")
rd_job() { # name profile
  cat >"$tmp/rd-job.json" <<JSON
{"name":"$1","definition":{"version":1,"kind":"job","enabled":true,"trigger":{"kind":"manual"},
 "action":{"kind":"execute","prompt":"say done"},"checkout_key":"repo","options":{"profile":"$2"},
 "issue":{"kind":"per-run","title":"Smoke run detail"},"overlap":"forbid","catchup":"skip","grace_seconds":60}}
JSON
  kata --workspace "$rd_dir" cron job create --file "$tmp/rd-job.json" --json | jq -r '.job.uid'
}
rd_slow=$(rd_job "Smoke slow run" slow)
rd_hang=$(rd_job "Smoke hang run" hang)

rd_worker_pid=
rd_start_worker() {
  "${rd_env[@]}" "$tmp/tern-kata" worker >>"$tmp/rd-worker.log" 2>&1 &
  rd_worker_pid=$!
  for ((attempt = 0; attempt < 200; attempt++)); do
    if "${rd_env[@]}" "$tmp/tern-kata" worker status 2>/dev/null | grep -q 'admitting'; then return 0; fi
    sleep 0.1
  done
  cat "$tmp/rd-worker.log" >&2
  printf 'The smoke worker never admitted\n' >&2
  return 1
}
rd_stop() {
  [[ -z "$rd_worker_pid" ]] || { kill "$rd_worker_pid" 2>/dev/null || true; wait "$rd_worker_pid" 2>/dev/null || true; }
  rd_worker_pid=
  pkill -9 -f -- '--stub-hang-first' 2>/dev/null || true
}
trap 'rd_status=$?; rd_stop; (exit $rd_status); cleanup' EXIT
rd_start_worker

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP XDG_CONFIG_HOME=%s XDG_STATE_HOME=%s KATA_HOME=%s %s/tern-kata)"\n' "$rd_dir" "$rd_config" "$rd_state" "$KATA_HOME" "$tmp" >"$tmp/rd-launch.ctl"
ctl --file "$tmp/rd-launch.ctl"

# The Jobs Page says the worker runs and serves the Project.
ctl key 3
expect_tree "[data-id='main.columns.list.jobs.$rd_slow']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.worker']" '.ok and .count == 1 and (.nodes[0].text | contains("smoke-box running and serving smoke"))'

# R runs the selected job now; the Runs Page shows it running with the agent's words so far, then succeeded.
ctl click "[data-id='main.columns.list.jobs.$rd_slow']"
ctl key R
ctl key 4
expect_tree "[data-id^='main.columns.list.runs.']" '.ok and .count >= 1 and (.nodes[0].text | contains("running") and contains("Smoke slow run") and contains("on smoke-box"))'
# The Runs Page keeps its selection across Pages, which earlier steps left on their own run: g selects the newest.
ctl key g
ctl key enter
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("stub says all checked")))'
expect_tree "[data-id^='main.columns.list.runs.']" '.ok and (.nodes[0].text | contains("succeeded") and contains("Smoke slow run"))'
ctl key escape

# A run whose worker and agent were killed is held; the Pane shows why, and u resumes it.
rd_run=
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ -z "$rd_run" ]] || break # the first run's slot frees a moment after kata shows it succeeded
  rd_run=$("${rd_env[@]}" "$tmp/tern-kata" run "$rd_hang" 2>/dev/null | awk '{print $2}') || true
  [[ -n "$rd_run" ]] || sleep 0.2
done
[[ -n "$rd_run" ]]
for ((attempt = 0; attempt < 200; attempt++)); do
  pgrep -f -- '--stub-hang-first' >/dev/null && break
  sleep 0.1
done
pgrep -f -- '--stub-hang-first' >/dev/null
kill -9 "$rd_worker_pid"
wait "$rd_worker_pid" 2>/dev/null || true
rd_worker_pid=
pkill -9 -f -- '--stub-hang-first' || true
rd_start_worker
for ((attempt = 0; attempt < 200; attempt++)); do
  if "${rd_env[@]}" "$tmp/tern-kata" worker status | grep -q "held"; then break; fi
  sleep 0.1
done
ctl key ctrl+r
expect_tree "[data-id='main.columns.list.runs.$rd_run']" '.ok and .count == 1 and (.nodes[0].text | contains("held") and contains("interrupted"))'
ctl click "[data-id='main.columns.list.runs.$rd_run']"
ctl key enter
for ((attempt = 0; attempt < 100; attempt++)); do
  ctl key ctrl+r
  ctl tree "[data-id^='main.columns.detail']" | jq -e '[.nodes[].text] | any(contains("u continues"))' >/dev/null && break
  sleep 0.3
done
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("interrupted"))) and ([.nodes[].text] | any(contains("u continues")))'
ctl key u
expect_tree "[data-id='main.columns.list.runs.$rd_run']" '.ok and .count == 1 and (.nodes[0].text | contains("succeeded"))'

# Clean up: end the worker, delete the jobs, and give the later steps the plain Pane back.
rd_stop
trap cleanup EXIT
quit_app
for job in "$rd_slow" "$rd_hang"; do
  event=$(kata --workspace "$rd_dir" cron job show "$job" --json | jq -r '.job.definition_event_uid')
  kata --workspace "$rd_dir" cron job delete "$job" --expected-event-uid "$event" --json >/dev/null
done
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues']" '.ok and .count >= 1'
