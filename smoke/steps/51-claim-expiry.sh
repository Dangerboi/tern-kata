# A timed claim from the CLI shows "<owner> until HH:MM" in the Issue detail, and the Pane drops it once the daemon's sweep expires the claim (about every 30 s).
kata --workspace "$tmp/workspace" create "Smoke claim expiry Issue" --json >"$tmp/expiry.json"
expiry_uid=$(jq -r '.issue.uid' "$tmp/expiry.json")
expiry_short=$(jq -r '.issue.short_id' "$tmp/expiry.json")
quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id$='.$expiry_uid']" '.ok and .count == 1'
ctl click "[data-id$='.$expiry_uid']"
ctl key enter
owner="main.columns.detail.viewport.content.properties.owner.v"
expect_tree "[data-id='$owner']" '.ok and .count == 1 and .nodes[0].text == "Unassigned"'
kata --workspace "$tmp/workspace" claim "$expiry_uid" --ttl 1m --json >/dev/null
expect_tree "[data-id='$owner']" '.ok and .count == 1 and (.nodes[0].text | test(" until [0-9]{2}:[0-9]{2}$"))'
# expect_tree gives up after 20 s; the claim lasts a minute and the sweep runs about every 30 s.
for ((expired = 0; expired < 1200; expired++)); do
  ctl tree "[data-id='$owner']" | jq -e '.nodes[0].text == "Unassigned"' >/dev/null && break
  sleep 0.1
done
expect_tree "[data-id='$owner']" '.ok and .count == 1 and .nodes[0].text == "Unassigned"'
kata --workspace "$tmp/workspace" delete "$expiry_uid" --force --confirm "DELETE smoke#$expiry_short" --json
