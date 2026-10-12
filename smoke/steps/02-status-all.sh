# The bound Project opens with status all: a closed Issue is listed beside open ones, with no status suffix.
kata --workspace "$tmp/workspace" create "Smoke statusall closed Issue" --json >"$tmp/statusall.json"
hxwz_uid=$(jq -r '.issue.uid' "$tmp/statusall.json")
hxwz_short=$(jq -r '.issue.short_id' "$tmp/statusall.json")
kata --as dana --workspace "$tmp/workspace" close "$hxwz_short" --done --message "Finished the status filter fixture and verified it by hand" --commit abc1234def56 --json
expect_tree "[data-id$='.$hxwz_uid']" '.count == 1'
expect_tree "[data-id^='main.columns.list.issues.01']" '.count >= 4'
expect_tree "[data-id='main.topbar.left.heading']" '.nodes[0].text == "Project: smoke"'
kata --workspace "$tmp/workspace" delete "$hxwz_uid" --force --confirm "DELETE smoke#$hxwz_short" --json
expect_tree "[data-id$='.$hxwz_uid']" '.count == 0'
