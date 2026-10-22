# esc on the Jobs Page opens the Menu, as on the Issues Page, and esc closes it again; P opens the Menu's Projects list from there.
ctl key 3
expect_tree "[data-id^='main.columns.list.jobs']" '.ok'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
ctl key P
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
expect_tree "[data-id='layer.collections.list.back']" '.ok and .count == 1'
# esc leaves the Projects list for the Menu's top level, a second esc closes the Menu.
ctl key esc
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
ctl key 1
