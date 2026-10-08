ctl key x
expect_tree "[data-id='layer.close.quick']" '.ok and .count == 1'
ctl click "[data-id='layer.close.quick']"
expect_tree "[data-id='layer.close.error']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("open children"))'
ctl click "[data-id='layer.close.cancel']"
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
close_uid=$(kata --workspace "$tmp/workspace" create "Smoke close target" --json | jq -r .issue.uid)
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
