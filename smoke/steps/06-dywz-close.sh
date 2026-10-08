# Restore the bound Project and use a dedicated parent-close guard fixture.
ctl key q
expect_tree ".sf-list" '.ok and .count == 0'
ctl --file "$tmp/launch.ctl"
kata --workspace "$tmp/workspace" create "Smoke close guard parent" --json >"$tmp/close-guard-parent.json"
close_guard_parent=$(jq -r '.issue.uid' "$tmp/close-guard-parent.json")
kata --workspace "$tmp/workspace" create "Smoke close guard child" --parent "$close_guard_parent" --json >"$tmp/close-guard-child.json"
expect_tree "[data-id='main.columns.list.issues.$close_guard_parent']" '.ok and .count == 1 and (.nodes[0].text | contains("▸"))'
ctl click "[data-id='main.columns.list.issues.$close_guard_parent']"
expect_tree "[data-id='main.columns.list.issues.$close_guard_parent']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key x
expect_tree "[data-id='layer.close.quick']" '.ok and .count == 1'
ctl click "[data-id='layer.close.quick']"
expect_tree "[data-id='layer.close.error']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("open children"))'
ctl click "[data-id='layer.close.cancel']"
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
kata --workspace "$tmp/workspace" create "Smoke close target" --json >"$tmp/close-target.json"
close_uid=$(jq -r '.issue.uid' "$tmp/close-target.json")
expect_tree "[data-id$='.$close_uid']" '.ok and .count == 1'
ctl click "[data-id$='.$close_uid']"
expect_tree "[data-id$='.$close_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key x
expect_tree "[data-id='layer.close.quick']" '.ok and .count == 1'
ctl click "[data-id='layer.close.quick']"
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
ctl click "[data-id='main.columns.collections.logbook']"
expect_tree "[data-id$='.$close_uid']" '.ok and .count == 1 and (.nodes[0].text | contains("closed"))'
ctl click "[data-id$='.$close_uid']"
expect_tree "[data-id$='.$close_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key r
expect_tree "[data-id$='.$close_uid']" '.ok and .count == 0'
ctl key esc
ctl key G
ctl key enter
expect_tree "[data-id$='.$close_uid']" '.ok and .count == 1 and (.nodes[0].text | contains("open"))'

for fixture in close-guard-child close-guard-parent close-target; do
  uid=$(jq -r '.issue.uid' "$tmp/$fixture.json")
  short=$(jq -r '.issue.short_id' "$tmp/$fixture.json")
  kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
done
