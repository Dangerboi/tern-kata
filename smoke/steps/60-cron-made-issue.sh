# An Issue a native job made is marked in the list and its detail names the job, and the parent's detail lists the job. The
# job fires every 60 seconds; kata's daemon delivers within about 90 seconds, so this step waits for the child.
cm_dir="$tmp/workspace"
kata --workspace "$cm_dir" create "Smoke cron parent" --body "Parent of a minutely job" --json >"$tmp/cron-parent.json"
cm_parent=$(jq -r '.issue.uid' "$tmp/cron-parent.json")
cm_short=$(jq -r '.issue.short_id' "$tmp/cron-parent.json")
cat >"$tmp/cron-job.json" <<'JSON'
{"name":"Smoke minutely tick","definition":{"version":1,"kind":"job","enabled":true,
 "trigger":{"kind":"interval","interval_seconds":60},
 "action":{"kind":"notify","recipient":"current-owner-or-author"},
 "issue":{"kind":"per-run","title":"Smoke cron tick"},"overlap":"allow","catchup":"all"}}
JSON
kata --workspace "$cm_dir" cron job create --parent "$cm_short" --file "$tmp/cron-job.json" --json >"$tmp/cron-created.json"
cm_job=$(jq -r '.job.uid' "$tmp/cron-created.json")
cm_event=$(jq -r '.job.definition_event_uid' "$tmp/cron-created.json")

cm_child=
for ((attempt = 0; attempt < 240; attempt++)); do
  cm_child=$(kata --workspace "$cm_dir" list --json | jq -r --arg job "$cm_job" '[.issues[] | select(.metadata["cron.occurrence"].job_uid == $job)][0].uid // empty')
  [[ -z "$cm_child" ]] || break
  sleep 1
done
[[ -n "$cm_child" ]]

# The child is nested under its parent, so expand the parent to see the child's row marked.
ctl key 1
expect_tree "[data-id$='.$cm_parent']" '.ok and .count == 1 and (.nodes[0].text | contains("◷") | not)'
ctl click "[data-id$='.$cm_parent']"
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Cron jobs") and contains("Smoke minutely tick") and contains("active")))'
ctl key Right
expect_tree "[data-id$='.$cm_child']" '.ok and .count == 1 and (.nodes[0].text | contains("◷"))'
ctl click "[data-id$='.$cm_child']"
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Cron job") and contains("Smoke minutely tick") and contains("scheduled")))'
ctl key Enter
ctl key Enter
expect_tree "[data-id='main.columns.list.jobs.$cm_job']" '.ok and .count == 1'
ctl key 1

kata --workspace "$cm_dir" cron job delete "$cm_job" --expected-event-uid "$cm_event" --json >/dev/null
kata --workspace "$cm_dir" delete "$cm_child" --force --confirm "DELETE smoke#$(kata --workspace "$cm_dir" show "$cm_child" --json | jq -r '.issue.short_id')" --json >/dev/null
kata --workspace "$cm_dir" delete "$cm_parent" --force --confirm "DELETE smoke#$cm_short" --json >/dev/null
[[ -S "$tmp/control.sock" ]]
