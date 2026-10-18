# O on the selected Issue opens the Recurrence card; saving with its defaults makes the Issue the first occurrence of a
# weekly recurrence. Closing it as done with the kata CLI creates the next occurrence, and the Pane lists it. Own Issue;
# it cleans up.
mr_dir="$tmp/workspace"
kata --workspace "$mr_dir" create "Smoke recurring Issue" --body "Recurs weekly" --json >"$tmp/recurring.json"
mr_uid=$(jq -r '.issue.uid' "$tmp/recurring.json")
mr_short=$(jq -r '.issue.short_id' "$tmp/recurring.json")
expect_tree "[data-id='main.columns.list.issues.$mr_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$mr_uid']"
expect_tree "[data-id='main.columns.list.issues.$mr_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key O
expect_tree "[data-id='layer.recurrence.frequency']" '.ok and .count == 1 and (.nodes[0] | [.. | objects | .text? // empty] | join(" ") | contains("[ Weekly ]"))'
expect_tree "[data-id='layer.recurrence.every']" '.ok and .count == 1 and (.nodes[0].text | contains("Every (weeks): 1"))'
# Weekly shows no Day or Month field; clicking Monthly shows Day, clicking Weekly hides it again.
expect_tree "[data-id='layer.recurrence.day']" '.ok and .count == 0'
ctl click "[data-id='layer.recurrence.frequency.2']"
expect_tree "[data-id='layer.recurrence.day']" '.ok and .count == 1'
expect_tree "[data-id='layer.recurrence.every']" '.ok and .count == 1 and (.nodes[0].text | contains("Every (months): 1"))'
ctl click "[data-id='layer.recurrence.frequency.1']"
expect_tree "[data-id='layer.recurrence.day']" '.ok and .count == 0'
ctl click "[data-id='layer.recurrence.actions.save']"
expect_tree "[data-id='layer.recurrence']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list.issues.$mr_uid']" '.ok and .count == 1 and (.nodes[0].text | contains("↻"))'

kata --workspace "$mr_dir" close "$mr_short" --done --message "Finished the recurring fixture and verified it by hand" --commit abc1234def56 --json >/dev/null
expect_tree "[data-id^='main.columns.list.issues.']" '.ok and ([.nodes[] | select((.text | contains("open ↻ Smoke recurring Issue")) and (.text | startswith("'"$mr_short"' ") | not))] | length == 1)'

kata --workspace "$mr_dir" list --status all --json >"$tmp/recurring-list.json"
for uid in $(jq -r '.issues[] | select(.title == "Smoke recurring Issue") | .uid' "$tmp/recurring-list.json"); do
  short=$(jq -r --arg uid "$uid" '.issues[] | select(.uid == $uid) | .short_id' "$tmp/recurring-list.json")
  kata --workspace "$mr_dir" delete "$uid" --force --confirm "DELETE smoke#$short" --json >/dev/null
done
