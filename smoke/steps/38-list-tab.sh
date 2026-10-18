# tab in the Issue list moves focus to the detail when the two sit side by side (the default width): the footer
# swaps the list's `tab detail` hint for the detail's `esc back`, and esc returns to the list.
lt_hint() { printf "[data-id^='dock.hints.grid.'][data-id\$='.%s']" "$1"; }
expect_tree "$(lt_hint tab)" '.count == 1 and (.nodes[0].text | contains("tab detail"))'
ctl key tab
expect_tree "$(lt_hint escape)" '.count == 1 and (.nodes[0].text | contains("esc back"))'
ctl key escape
expect_tree "$(lt_hint tab)" '.count == 1 and (.nodes[0].text | contains("tab detail"))'
