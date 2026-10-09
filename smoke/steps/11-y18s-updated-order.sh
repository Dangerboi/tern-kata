# The Issue list is ordered by last update, newest first: an older Issue that
# kata updates moves to the top, and the selection stays on the Issue it was on.
kata --workspace "$tmp/workspace" create "Smoke order older" --force-new --json >"$tmp/order-older.json"
sleep 1
kata --workspace "$tmp/workspace" create "Smoke order newer" --force-new --json >"$tmp/order-newer.json"
expect_tree "[data-id='main.columns.list.issues']" '.nodes[0].text | contains("Smoke order newer")'
ctl tree "[data-id^='main.columns.list.issues.']" >"$tmp/order-before.json"
selected_before=$(jq -r '.nodes[] | select(.["aria-selected"] == "true") | .["data-id"]' "$tmp/order-before.json")
sleep 1
kata --workspace "$tmp/workspace" comment "$(jq -r '.issue.uid' "$tmp/order-older.json")" --body "Bumps the update time" --json >/dev/null
expect_tree "[data-id='main.columns.list.issues']" '.nodes[0].text | contains("Smoke order older")'
ctl tree "[data-id^='main.columns.list.issues.']" >"$tmp/order-after.json"
[[ $(jq -r '.nodes[] | select(.["aria-selected"] == "true") | .["data-id"]' "$tmp/order-after.json") == "$selected_before" ]]
for fixture in order-older order-newer; do
  kata --workspace "$tmp/workspace" delete "$(jq -r '.issue.uid' "$tmp/$fixture.json")" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/$fixture.json")" --json >/dev/null
done
