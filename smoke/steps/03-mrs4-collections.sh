# Switch by the same keys used in a Pane, then return to the bound Project.
ctl key esc
ctl key g
ctl key enter
ctl plugins expect Inbox
expect_tree "[data-id='main.columns.collections.inbox']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id='main.issues']" '.ok and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Smoke first Issue") | not)'
ctl key esc
ctl key G
ctl key enter
ctl plugins expect smoke
expect_tree "[data-id^='main.issues.01']" '.ok and .count == 3'
