ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Keyboard help") and contains("first Issue"))'
expect_tree '[data-id="dock.hints"]' '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("esc close help")'
ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 0'
ctl key '?'
expect_tree '[data-id="layer.help"]' '.ok and .count == 1'
ctl key Escape
expect_tree '[data-id="layer.help"]' '.ok and .count == 0'
