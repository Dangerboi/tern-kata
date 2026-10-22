# The Jobs Page lists a Cron job made with kata cron job create: its state, name, kind and schedule in words, and the inspector
# beside it shows the definition and the parent, which Enter opens on the Issues Page. Own Issue and job; it cleans up.
jb_dir="$tmp/workspace"
kata --workspace "$jb_dir" create "Smoke jobs parent" --body "Parent of a recurring job" --json >"$tmp/jobs-parent.json"
jb_uid=$(jq -r '.issue.uid' "$tmp/jobs-parent.json")
jb_short=$(jq -r '.issue.short_id' "$tmp/jobs-parent.json")
cat >"$tmp/jobs-job.json" <<'JSON'
{"name":"Smoke hourly report","definition":{"version":1,"kind":"job","enabled":true,
 "trigger":{"kind":"interval","interval_seconds":3600},
 "action":{"kind":"notify","recipient":"current-owner-or-author"},
 "issue":{"kind":"per-run","title":"Smoke tick"},"overlap":"allow","catchup":"all"}}
JSON
kata --workspace "$jb_dir" cron job create --parent "$jb_short" --file "$tmp/jobs-job.json" --json >"$tmp/jobs-created.json"
jb_job=$(jq -r '.job.uid' "$tmp/jobs-created.json")
jb_event=$(jq -r '.job.definition_event_uid' "$tmp/jobs-created.json")

ctl key 3
expect_tree "[data-id='main.columns.list.jobs.$jb_job']" '.ok and .count == 1 and (.nodes[0].text | contains("active") and contains("Smoke hourly report") and contains("recurring child") and contains("every hour") and contains("next "))'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke jobs parent")))'
ctl key enter
ctl key enter
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke jobs parent")))'

# Archived jobs stay out of the list until `a`.
ctl key 3
kata --workspace "$jb_dir" cron job delete "$jb_job" --expected-event-uid "$jb_event" --json >/dev/null
expect_tree "[data-id='main.columns.list.jobs.$jb_job']" '.count == 0'
ctl key a
expect_tree "[data-id='main.columns.list.jobs.$jb_job']" '.ok and .count == 1 and (.nodes[0].text | contains("archived"))'
# The Page keeps its archived toggle: turn it back off for later steps.
ctl key a
expect_tree "[data-id='main.columns.list.jobs.$jb_job']" '.count == 0'
ctl key 1
kata --workspace "$jb_dir" delete "$jb_uid" --force --confirm "DELETE smoke#$jb_short" --json >/dev/null
