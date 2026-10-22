# J on an Issue opens the Job card with that Issue as the parent; an every-60-seconds native job made from it makes a child
# Issue under the parent, pausing it with p makes none, resuming makes one again, and x archives it (R restores it).
# Own Issue and job; it cleans up. Kata's daemon delivers a due occurrence every 30 seconds at most, so this step waits.
jc_dir="$tmp/workspace"
kata --workspace "$jc_dir" create "Smoke job parent" --body "Parent of an every-minute job" --json >"$tmp/jobcard-parent.json"
jc_uid=$(jq -r '.issue.uid' "$tmp/jobcard-parent.json")
jc_short=$(jq -r '.issue.short_id' "$tmp/jobcard-parent.json")
jc_children() {
  kata --workspace "$jc_dir" list --json | jq '[.issues[] | select(.title | startswith("Smoke every minute tick"))] | length'
}
jc_wait_children() { # wait until the child count satisfies a jq test on the count, up to 150 seconds
  for ((attempt = 0; attempt < 75; attempt++)); do
    if [[ $(jc_children | jq "$1") == true ]]; then return 0; fi
    sleep 2
  done
  printf 'Child count never satisfied %s: %s\n' "$1" "$(jc_children)" >&2
  return 1
}

expect_tree "[data-id='main.columns.list.issues.$jc_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$jc_uid']"
expect_tree "[data-id='main.columns.list.issues.$jc_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key J
expect_tree "[data-id='layer.job']" '.ok and .count == 1 and (.nodes[0].text | contains("New job"))'
expect_tree "[data-id='layer.job.parent']" '.ok and .count == 1'
expect_tree "[data-id='layer.job.cron']" '.ok and .count == 1'
expect_tree "[data-id='layer.job.interval']" '.ok and .count == 0'
ctl type '"Smoke every minute"'
# An interval under a minute is refused naming the field, and nothing is made.
ctl click "[data-id='layer.job.trigger.choices.1']"
expect_tree "[data-id='layer.job.interval']" '.ok and .count == 1'
expect_tree "[data-id='layer.job.cron']" '.ok and .count == 0'
ctl click "[data-id='layer.job.interval']"
for _ in 1 2 3 4; do ctl key Backspace; done
ctl type '"30"'
expect_tree "[data-id='layer.job.preview']" '.ok and .count == 1 and (.nodes[0] | [.. | objects | .text? // empty] | join(" ") | contains("at least 60"))'
ctl click "[data-id='layer.job.title']"
ctl type '"Smoke every minute tick"'
ctl key Ctrl+o
expect_tree "[data-id='layer.job.error']" '.ok and .count == 1 and (.nodes[0].text | contains("Interval:"))'
kata --workspace "$jc_dir" cron job list --json | jq -e '.jobs | length == 0' >/dev/null
# Sixty seconds reads in words and lists the next runs.
ctl click "[data-id='layer.job.interval']"
for _ in 1 2; do ctl key Backspace; done
ctl type '"60"'
expect_tree "[data-id='layer.job.preview']" '.ok and .count == 1 and (.nodes[0] | [.. | objects | .text? // empty] | join(" ") | contains("every minute") and contains("Next runs"))'
ctl key Ctrl+o
expect_tree "[data-id='layer.job']" '.ok and .count == 0'
jc_job=$(kata --workspace "$jc_dir" cron job list --json | jq -r '.jobs[0].uid')
kata --workspace "$jc_dir" cron job list --json | jq -e '.jobs | length == 1 and .[0].name == "Smoke every minute" and .[0].definition.enabled == true and .[0].definition.trigger.interval_seconds == 60 and .[0].definition.issue.parent_uid == "'"$jc_uid"'"' >/dev/null

# The Jobs Page lists it; a child appears under the parent.
ctl key 3
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("active") and contains("Smoke every minute") and contains("every minute"))'
jc_wait_children '. >= 1'

# p pauses: the row reads disabled and no further child is made.
ctl key p
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("disabled"))'
jc_paused=$(jc_children)
sleep 75
[[ $(jc_children) == "$jc_paused" ]]

# p resumes: the row reads active and the next sweep delivers again.
ctl key p
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("active"))'
jc_wait_children ". > $jc_paused"

# e opens the card prefilled; a rename saves.
ctl key e
expect_tree "[data-id='layer.job']" '.ok and .count == 1 and (.nodes[0].text | contains("Edit job"))'
expect_tree "[data-id='layer.job.parent']" '.ok and .count == 0'
expect_tree "[data-id='layer.job.name']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke every minute"))'
ctl type '" again"'
ctl key Ctrl+o
expect_tree "[data-id='layer.job']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke every minute again"))'

# x archives it: it leaves the list, a shows it archived, R restores it, x archives it for good.
ctl key x
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 0'
kata --workspace "$jc_dir" cron job list --json | jq -e '.jobs | length == 0' >/dev/null
ctl key a
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("archived"))'
ctl key R
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("active"))'
ctl key x
expect_tree "[data-id='main.columns.list.jobs.$jc_job']" '.ok and .count == 1 and (.nodes[0].text | contains("archived"))'
ctl key a
ctl key 1

# Remove only this step's fixtures.
kata --workspace "$jc_dir" list --json >"$tmp/jobcard-fixtures.json"
while read -r uid short; do
  kata --workspace "$jc_dir" delete "$uid" --force --confirm "DELETE smoke#$short" --json >/dev/null
done < <(jq -r '.issues[] | select(.title | startswith("Smoke every minute tick") or . == "Smoke job parent") | "\(.uid) \(.short_id)"' "$tmp/jobcard-fixtures.json")
