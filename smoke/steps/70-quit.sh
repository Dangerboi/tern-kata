# q asks "Quit tern-kata? y/n" while kata's [tui] confirm_quit is on (the smoke home turns it off for the other steps): y quits,
# n and esc stay, and ctrl+c quits at once. The Pane is launched again after each quit.
ctl key q
expect_tree .sf-list '.count == 0'
printf '[tui]\nconfirm_quit = true\n' >"$KATA_HOME/config.toml"
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'
ctl key q
expect_tree "[data-id='main.quit']" '.ok and .count == 1 and (.nodes[0].text == "Quit tern-kata? y/n")'
ctl key n
expect_tree "[data-id='main.quit']" '.count == 0'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'
ctl key q
expect_tree "[data-id='main.quit']" '.ok and .count == 1'
ctl key escape
expect_tree "[data-id='main.quit']" '.count == 0'
ctl key q
ctl key y
expect_tree .sf-list '.count == 0'
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'
ctl key ctrl+c
expect_tree .sf-list '.count == 0'
printf '[tui]\nconfirm_quit = false\n' >"$KATA_HOME/config.toml"
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'
