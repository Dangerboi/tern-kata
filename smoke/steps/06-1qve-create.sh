# Restore the bound Project so the created Issue and child are both visible.
ctl key q
expect_tree ".sf-list" '.ok and .count == 0'
ctl --file "$tmp/launch.ctl"
ctl plugins expect '"Smoke first Issue"'
ctl key n
expect_tree "[data-id='layer.create.title']" '.count == 1'
ctl type '"Smoke key-created parent"'
ctl key Tab
expect_tree "[data-id='layer.create.body']" '.nodes[0].class | contains("focused")'
ctl type '"Created body by keys"'
ctl key Ctrl+o
expect_tree "[data-id='layer.create']" '.count == 0'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | select(.["aria-selected"] == "true") | .. | objects | .text? // empty] | any(contains("Smoke key-created parent"))'
ctl key N
expect_tree "[data-id='layer.create.title']" '.count == 1'
ctl type '"Smoke key-created child"'
ctl key Ctrl+o
expect_tree "[data-id='layer.create']" '.count == 0'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | select(.["aria-selected"] == "true") | .. | objects | .text? // empty] | any(contains("Smoke key-created child") and contains("│"))'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke key-created parent") and contains("▾"))'
kata --workspace "$tmp/workspace" create "Smoke create refresh trigger" --json
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke create refresh trigger"))'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | select(.["aria-selected"] == "true") | .. | objects | .text? // empty] | any(contains("Smoke key-created child"))'
