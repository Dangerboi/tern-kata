# The Runs Page lists a run an executor observed with kata cron run observe: its status, job name, Issue, executor and summary.
# Enter shows the detail with the token counts, `i` opens the run's Issue on the Issues Page, and `r` on the Jobs Page opens the
# runs of one job. Own Issue and job; it cleans up.
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

ctl key 4
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.ok and .count == 1 and (.nodes[0].text | contains("succeeded") and contains("Smoke triage job") and contains("Smoke run Issue") and contains("smoke-laptop") and contains("1m 35s") and contains("triaged four items"))'
ctl key enter
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Input tokens1200"))) and ([.nodes[].text] | any(. == "340")) and ([.nodes[].text] | any(. == "'"$rn_job"'"))'
ctl key i
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke run Issue")))'

# The Jobs Page's r opens the runs of the selected job.
ctl key 3
expect_tree "[data-id='main.columns.list.jobs.$rn_job']" '.count == 1'
# Earlier steps leave their own jobs in the Project: select this one.
ctl click "[data-id='main.columns.list.jobs.$rn_job']"
ctl key r
expect_tree "[data-id='main.columns.list.runs.$rn_run']" '.count == 1'
ctl key 1
kata --workspace "$rn_dir" cron job delete "$rn_job" --expected-event-uid "$rn_event" --json >/dev/null
kata --workspace "$rn_dir" delete "$rn_issue" --force --confirm "DELETE smoke#$rn_short" --json >/dev/null
