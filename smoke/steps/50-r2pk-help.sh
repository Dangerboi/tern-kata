ctl key PageDown
ctl key PageUp
expect_tree '[data-id="main.columns.list.issues"]' '.ok and .count == 1'
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
expect_tree '[data-id="main.help.sheet.title"]' '.ok and .nodes[0].text == "tern-kata — keybindings"'
expect_tree '[data-id="main.help.sheet.groups.group-1.row-0"], [data-id="main.help.sheet.groups.group-1.row-6"]' '.ok and ([.nodes[].text] | join(" ") | contains("first Issue"))'
expect_tree '[data-id="main.help.sheet.return"]' '.ok and .nodes[0].text == "press ? to return"'
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
