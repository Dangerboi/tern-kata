# The Federation Page (F) lists this daemon's spoke federations under a header naming the active daemon. The smoke daemon is
# unfederated, so the Page says "No spoke federations"; r reads again and esc returns to the Page shown before.
ctl key F
expect_tree "[data-id='main.columns.list.empty']" '.ok and .count == 1 and (.nodes[0] | tojson | contains("No spoke federations"))'
expect_tree "[data-id='main.columns.list.heading.facts']" '.ok and (.nodes[0].text | contains("auth "))'
expect_tree "[data-id^='main.topbar.left.pages.'][data-id$='.label']" '.ok and (.nodes | map(.text) | any(contains("Federation")))'
ctl key r
expect_tree "[data-id='main.columns.list.empty']" '.ok and .count == 1 and (.nodes[0] | tojson | contains("No spoke federations"))'
ctl key enter
expect_tree "[data-id='main.columns.list.empty']" '.ok and .count == 1'
ctl key escape
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
