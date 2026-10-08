# Switch by the same keys used in a Pane (esc opens the picker in a bound directory), then return to the bound Project.
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl key Down
ctl key enter
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list.topbar.heading']" '.ok and .nodes[0].text == "Inbox"'
expect_tree "[data-id='main.columns.list.issues']" '.ok and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Smoke first Issue") | not)'
ctl key esc
ctl key enter
expect_tree "[data-id='main.columns.list.topbar.heading']" '.ok and .nodes[0].text == "smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 3'
