# tab in the Issue list moves focus to the detail when the two sit side by side (the default width): the footer
# swaps the list's `tab detail` hint for the detail's `esc back`, and esc returns to the list.
lt_hints="[data-id='dock.hints']"
expect_tree "$lt_hints" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("tab detail")'
ctl key tab
expect_tree "$lt_hints" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("esc back")'
ctl key escape
expect_tree "$lt_hints" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("tab detail")'
