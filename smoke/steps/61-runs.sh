# The Runs Page lists a run an executor observed with kata cron run observe: its status, job name, Issue, executor and summary.
# Enter shows the detail with the token counts, `i` opens the run's Issue on the Issues Page, and `r` on the Jobs Page opens the
# runs of one job. The since-you-last-looked mode (`n`) lists the runs ended since the mark above a Now line, `s` marks them seen,
# and the mode and mark survive a restart through seen.toml. Own Issue and job; it cleans up.
rn_dir="$tmp/workspace"
kata --workspace "$rn_dir" create "Smoke run Issue" --body "Issue of an observed run" --json >"$tmp/runs-issue.json"
rn_issue=$(jq -r '.issue.uid' "$tmp/runs-issue.json")
rn_short=$(jq -r '.issue.short_id' "$tmp/runs-issue.json")
cat >"$tmp/runs-job.json" <<'JSON'
{"name":"Smoke triage job","definition":{"version":1,"kind":"job","enabled":false,
 "trigger":{"kind":"manual"},"action":{"kind":"execute","prompt":"triage"},
 "issue":{"kind":"per-run","title":"Triage"},"overlap":"forbid","catchup":"skip","grace_seconds":60}}
JSON
kata --workspace "$rn_dir" cron job create --file "$tmp/runs-job.json" --json >"$tmp/runs-job-created.json"
rn_job=$(jq -r '.job.uid' "$tmp/runs-job-created.json")
rn_event=$(jq -r '.job.definition_event_uid' "$tmp/runs-job-created.json")
rn_run=01H5MK0000000000000000R001
cat >"$tmp/runs-observation.json" <<JSON
{"job_uid":"$rn_job","definition_event_uid":"$rn_event","issue_uid":"$rn_issue","executor_label":"smoke-laptop",
 "status":"succeeded","started_at":"2026-10-08T09:00:00Z","ended_at":"2026-10-08T09:01:35Z","expected_revision":0,
 "summary":{"version":1,"message":"triaged four items","input_tokens":1200,"output_tokens":340}}
JSON
kata --workspace "$rn_dir" cron run observe "$rn_run" --json-input "$tmp/runs-observation.json" --json >/dev/null
rn_second=01H5MK0000000000000000R002
cat >"$tmp/runs-second.json" <<JSON
{"job_uid":"$rn_job","definition_event_uid":"$rn_event","issue_uid":"$rn_issue","executor_label":"smoke-laptop",
 "status":"succeeded","started_at":"2026-10-08T10:00:00Z","ended_at":"2026-10-08T10:02:00Z","expected_revision":0,
 "summary":{"version":1,"message":"triaged nine items","input_tokens":777,"output_tokens":55}}
JSON
kata --workspace "$rn_dir" cron run observe "$rn_second" --json-input "$tmp/runs-second.json" --json >/dev/null
# A summary that leads with kata's `reason: <code>` line shows the code and the line after it; so does a hold an earlier
# tern-kata wrote as `<code>: <detail>`.
rn_reason=01H5MK0000000000000000R003
rn_legacy=01H5MK0000000000000000R004
rn_held() { # run uid, start hour, message as JSON string text
  cat >"$tmp/runs-held.json" <<JSON
{"job_uid":"$rn_job","definition_event_uid":"$rn_event","issue_uid":"$rn_issue","executor_label":"smoke-laptop",
 "status":"unknown","started_at":"2026-10-08T$2:00:00Z","ended_at":"2026-10-08T$2:01:00Z","expected_revision":0,
 "summary":{"version":1,"message":"$3"}}
JSON
  kata --workspace "$rn_dir" cron run observe "$1" --json-input "$tmp/runs-held.json" --json >/dev/null
}
rn_held "$rn_reason" 11 'reason: agent_quota\nthe provider answered 429'
rn_held "$rn_legacy" 12 'interrupted: the worker stopped while the agent ran'

ctl key 3
expect_tree "[data-id='main.columns.list.runs.$rn_reason']" '.ok and .count == 1 and (.nodes[0].text | contains("unknown") and contains("reason: agent_quota") and contains("the provider answered 429"))'
expect_tree "[data-id='main.columns.list.runs.$rn_legacy']" '.ok and .count == 1 and (.nodes[0].text | contains("reason: interrupted") and contains("the worker stopped while the agent ran") and (contains("interrupted: ") | not))'
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.ok and .count == 1 and (.nodes[0].text | contains("succeeded") and contains("Smoke triage job") and contains("Smoke run Issue") and contains("smoke-laptop") and contains("1m 35s") and contains("triaged four items"))'
ctl click "[data-id='main.columns.list.runs.$rn_run']"
ctl key enter
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Input tokens1200"))) and ([.nodes[].text] | any(. == "340")) and ([.nodes[].text] | any(. == "'"$rn_job"'"))'
# A double click on a run that is not selected does what selecting it and pressing Enter does: the keys move to its
# detail, which shows that run, and Esc gives them back to the list.
ctl key escape
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("back") | not)'
expect_tree "[data-id='main.columns.list.runs.$rn_second']" '.ok and .count == 1 and (.nodes[0].text | contains("triaged nine items"))'
ctl click "[data-id='main.columns.list.runs.$rn_run']"
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.ok and (.nodes[0].class | contains("selected"))'
ctl dblclick "[data-id='main.columns.list.runs.$rn_second']"
expect_tree "[data-id='main.columns.list.runs.$rn_second']" '.ok and (.nodes[0].class | contains("selected"))'
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("back"))'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Input tokens777"))) and ([.nodes[].text] | any(. == "55"))'
ctl key escape
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("back") | not)'
ctl key i
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke run Issue")))'

# The Jobs Page's r opens the runs of the selected job.
ctl key 2
expect_tree "[data-id='main.columns.list.jobs.$rn_job']" '.count == 1'
# Earlier steps leave their own jobs in the Project: select this one.
ctl click "[data-id='main.columns.list.jobs.$rn_job']"
ctl key r
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.count == 1'

# The since-you-last-looked mode: never marked, so every ended run is new and sits above the Now line, newest first.
ctl key 3
ctl key n
expect_tree "[data-id='main.columns.list.runs.now']" '.ok and .count == 1'
expect_tree "[data-id^='main.columns.list.runs.']" '.nodes as $n | def y(f): [$n[] | select(.text | f)][0].rect[1]; y(contains("the worker stopped")) as $a | y(contains("the provider answered")) as $b | y(contains("triaged nine items")) as $c | y(contains("triaged four items")) as $d | y(. == "Now") as $now | $a != null and $d != null and $now != null and $a < $b and $b < $c and $c < $d and $d < $now'
# s refuses while the job filter `r` set is on, since the mark would also cover runs it hides; f lifts the filter.
ctl key s
expect_tree "[data-id='main.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("Clear the job filter"))'
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.ok and .count == 1'
ctl key f
# s marks them seen: the mark is the newest end among the loaded runs.
ctl key s
expect_tree "[data-id='main.columns.list.runs.nothing-new']" '.ok and .count == 1 and (.nodes[0].text | contains("Nothing new since"))'
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.count == 0'
rn_seen="$XDG_CONFIG_HOME/tern-kata/seen.toml"
for ((attempt = 0; attempt < 100; attempt++)); do
  grep -qs 'timeline = true' "$rn_seen" && grep -qs '^ *seen = 2026-10-08T12:01:00Z' "$rn_seen" && break
  sleep 0.1
done
grep -q '^\[' "$rn_seen"
grep -q 'timeline = true' "$rn_seen"
grep -q '^ *seen = 2026-10-08T12:01:00Z' "$rn_seen"
# A restart opens the Runs Page in the mode, with the mark kept.
ctl key q
expect_tree .sf-list '.count == 0'
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'
ctl key 3
expect_tree "[data-id='main.columns.list.runs.now']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.runs.nothing-new']" '.ok and .count == 1 and (.nodes[0].text | contains("Nothing new since"))'
# n leaves the mode and the list of every run is back.
ctl key n
expect_tree "[data-id='main.columns.list.runs.now']" '.count == 0'
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.ok and .count == 1'
rm -f "$rn_seen"
ctl key 1
kata --workspace "$rn_dir" cron job delete "$rn_job" --expected-event-uid "$rn_event" --json >/dev/null
kata --workspace "$rn_dir" delete "$rn_issue" --force --confirm "DELETE smoke#$rn_short" --json >/dev/null
