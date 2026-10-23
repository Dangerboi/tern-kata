# ctrl+d in the Filter card saves the filters as the Project's default; a Pane started afterwards has them applied and its
# filter line ends with "(default)". The step removes the default again so later steps start unfiltered.
df_workspace="$tmp/workspace"
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1'
ctl key tab
ctl key Right
expect_tree "[data-id='layer.filters.status']" '.ok and (.nodes[0].text | contains("[ open ]"))'
ctl key Ctrl+d
expect_tree "[data-id='layer.filters']" '.count == 0'
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("status=open") and endswith("(default)")'
[[ -f "$XDG_CONFIG_HOME/tern-kata/filters.toml" ]]
grep -q 'status = "open"' "$XDG_CONFIG_HOME/tern-kata/filters.toml"

quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id='main.filters']" '.ok and (.nodes[0].text | contains("status=open") and endswith("(default)"))'

# c clears the filters for this Pane only.
ctl key c
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

# An empty card's ctrl+d removes the default.
ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1'
ctl click "[data-id='layer.filters.actions.default']"
expect_tree "[data-id='layer.filters']" '.count == 0'
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
! grep -q 'status' "$XDG_CONFIG_HOME/tern-kata/filters.toml"
