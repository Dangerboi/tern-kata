# tab in the Issue list moves focus to the detail when the two sit side by side (the default width): the footer
# swaps the list's hints (`tab detail`, `f filter`) for the detail's (`+ label`, `esc back`), and esc returns to the list.
lt_hint() { printf "[data-id^='dock.hints.grid.'][data-id\$='.%s']" "$1"; }
expect_tree "$(lt_hint tab)" '.count == 1 and (.nodes[0].text | contains("tab detail"))'
expect_tree "$(lt_hint f)" '.count == 1 and (.nodes[0].text | contains("f filter"))'
expect_tree "$(lt_hint +)" '.count == 0'
ctl key tab
expect_tree "$(lt_hint escape)" '.count == 1 and (.nodes[0].text | contains("esc back"))'
expect_tree "$(lt_hint +)" '.count == 1 and (.nodes[0].text | contains("+ label"))'
expect_tree "$(lt_hint f)" '.count == 0'
ctl key escape
expect_tree "$(lt_hint tab)" '.count == 1 and (.nodes[0].text | contains("tab detail"))'
