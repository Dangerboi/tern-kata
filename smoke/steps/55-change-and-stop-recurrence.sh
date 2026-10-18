# O on a recurring Issue opens the Recurrence card as "Change recurrence", prefilled; saving changes the schedule and the
# Issue detail reads it. Stopping asks y/n first and the detail then reads "stopped"; closing the Issue as done with the
# kata CLI afterwards creates no next occurrence. Own Issue; it cleans up.
cs_dir="$tmp/workspace"
kata --workspace "$cs_dir" create "Smoke changing recurrence" --body "Recurs, then stops" --json >"$tmp/changing.json"
cs_uid=$(jq -r '.issue.uid' "$tmp/changing.json")
cs_short=$(jq -r '.issue.short_id' "$tmp/changing.json")
expect_tree "[data-id='main.columns.list.issues.$cs_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$cs_uid']"
expect_tree "[data-id='main.columns.list.issues.$cs_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key O
expect_tree "[data-id='layer.recurrence.frequency']" '.ok and .count == 1'
ctl click "[data-id='layer.recurrence.actions.save']"
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list.issues.$cs_uid']" '.ok and .count == 1 and (.nodes[0].text | contains("↻"))'

# Change: the card is prefilled from the schedule; every 2 weeks.
ctl key O
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 1 and (.nodes[0].text | contains("Change recurrence"))'
expect_tree "[data-id='layer.recurrence.actions.stop']" '.ok and .count == 1'
expect_tree "[data-id='layer.recurrence.every']" '.ok and .count == 1 and (.nodes[0].text | contains("Every: 1"))'
ctl key Tab
ctl key Backspace
ctl type '"2"'
ctl click "[data-id='layer.recurrence.actions.save']"
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 0'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Every 2 weeks on")))'

# Stop: y/n first, n keeps the card, y stops.
ctl key O
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 1'
ctl key Ctrl+x
expect_tree "[data-id='layer.recurrence.stop']" '.ok and .count == 1'
ctl key n
expect_tree "[data-id='layer.recurrence.stop']" '.ok and .count == 0'
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 1'
ctl key Ctrl+x
expect_tree "[data-id='layer.recurrence.stop']" '.ok and .count == 1'
ctl key y
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 0'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("stopped")))'
# A stopped Issue gets no card: kata refuses a second recurrence.
ctl key O
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 0'

kata --workspace "$cs_dir" close "$cs_short" --done --message "Finished the stopped recurring fixture and verified it by hand" --commit abc1234def56 --json >/dev/null
kata --workspace "$cs_dir" list --status all --json >"$tmp/changing-list.json"
jq -e '[.issues[] | select(.title == "Smoke changing recurrence")] | length == 1' "$tmp/changing-list.json" >/dev/null
for uid in $(jq -r '.issues[] | select(.title == "Smoke changing recurrence") | .uid' "$tmp/changing-list.json"); do
  short=$(jq -r --arg uid "$uid" '.issues[] | select(.uid == $uid) | .short_id' "$tmp/changing-list.json")
  kata --workspace "$cs_dir" delete "$uid" --force --confirm "DELETE smoke#$short" --json >/dev/null
done
