ctl key PageDown
ctl key PageUp
expect_tree '[data-id="main.columns.list.issues"]' '.ok and .count == 1'
ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Keyboard help"))'
expect_tree '[data-id="layer.help"] .sf-text' '.ok and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("first Issue"))'
expect_tree '[data-id="dock.hints"]' '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("esc close help")'
ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 0'
ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 1'
ctl key Escape
expect_tree '[data-id="layer.help"]' '.ok and .count == 0'
