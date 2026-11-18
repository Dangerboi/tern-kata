ctl key PageDown
ctl key PageUp
expect_tree '[data-id="main.columns.list.issues"]' '.ok and .count == 1'
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
expect_tree '[data-id="main.head.title"]' '.ok and .nodes[0].text == "Keyboard shortcuts"'
expect_tree '[data-id$=".group-1.row-4"]' '.ok and .count == 1'
expect_tree '[data-id="main.head.close"]' '.ok and .count == 1'
expect_tree '[data-id="main.columns"]' '.ok and .count == 0'
expect_tree '[data-id="main.help"] .sf-table' '.ok and .count == 0'
expect_tree '[data-id="dock.hints"]' '.ok and .count == 0'
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 0'
expect_tree '[data-id="main.columns"]' '.ok and .count == 1'
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
ctl key Escape
expect_tree '[data-id="main.help"]' '.ok and .count == 0'
expect_tree '[data-id="main.columns"]' '.ok and .count == 1'
