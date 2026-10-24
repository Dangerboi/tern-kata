# The Federation Page (F) lists this daemon's spoke federations under a header naming the active daemon. The smoke daemon is
# unfederated, so the Page says "no spoke federation enrollments"; r reads again and esc returns to the Page shown before.
ctl key F
expect_tree "[data-id='main.columns.list.notice']" '.ok and .count == 1 and (.nodes[0].text == "no spoke federation enrollments")'
expect_tree "[data-id='main.columns.list.summary']" '.ok and (.nodes[0].text == "0 spoke federations")'
expect_tree "[data-id='main.columns.list.header']" '.ok and (.nodes[0].text | startswith("Federation for active daemon: ") and contains(" instance ") and contains(" auth "))'
expect_tree "[data-id^='main.topbar.left.pages.']" '.ok and (.nodes | map(.text) | any(contains("Federation")))'
ctl key r
expect_tree "[data-id='main.columns.list.notice']" '.ok and .count == 1 and (.nodes[0].text == "no spoke federation enrollments")'
ctl key enter
expect_tree "[data-id='main.columns.list.notice']" '.ok and .count == 1'
ctl key escape
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
