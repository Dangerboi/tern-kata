# The Page bar in the top bar switches between the Issues and Board Pages by key and by click; the Board Page opens the Lane view
# on Enter and Esc comes back to the Board, as Board mode did.
expect_tree "[data-id^='main.topbar.left.pages.']" '.ok and .count == 6 and (.nodes[0].text | contains("Issues")) and (.nodes[1].text | contains("Board")) and (.nodes[2].text | contains("Jobs")) and (.nodes[3].text | contains("Runs")) and (.nodes[4].text | contains("Workflows")) and (.nodes[5].text | contains("Attention"))'
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
ctl key 2
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
expect_tree "[data-id='main.columns.list.issues']" '.count == 0'
ctl key 1
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id='main.columns.list.board']" '.count == 0'
ctl click "[data-id='main.topbar.left.pages.board']"
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
ctl key enter
expect_tree "[data-id='main.columns.list.lane']" '.count == 1'
ctl key escape
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
ctl click "[data-id='main.topbar.left.pages.issues']"
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
ctl key b
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
ctl key b
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
