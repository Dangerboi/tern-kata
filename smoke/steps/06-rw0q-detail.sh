# Detail reads title, properties and labels first, then body. Quick edits update the block in place.
kata --workspace "$tmp/workspace" create "Smoke layout Issue" --body "Layout body" --priority 1 --label layout-label --json >"$tmp/layout.json"
layout_uid=$(jq -r '.issue.uid' "$tmp/layout.json")
quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id$='.$layout_uid']" '.count == 1'
ctl click "[data-id$='.$layout_uid']"
ctl key enter
props="main.columns.detail.section0.properties.badges"
expect_tree "[data-id='$props.status']" '.ok and .count == 1 and .nodes[0].tone == "accent"'
expect_tree "[data-id='$props.priority']" '.ok and .count == 1 and .nodes[0].tone == "warning" and .nodes[0].text == "P1"'
expect_tree "[data-id='main.columns.detail.section1.labels.label-layout-label']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.detail.section2.body']" '.ok and .count == 1'
ctl key '!'
ctl key 0
ctl key enter
expect_tree "[data-id='$props.priority']" '.ok and .count == 1 and .nodes[0].tone == "error" and .nodes[0].text == "P0"'
ctl key escape
