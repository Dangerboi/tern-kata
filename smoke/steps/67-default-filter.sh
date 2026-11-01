# ctrl+d in the Filter card saves the filters as the Project's default; a Pane started afterwards has them applied and its
# filter line ends with the note "default". The step removes the default again so later steps start unfiltered.
df_workspace="$tmp/workspace"
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1'
ctl key tab
ctl key Right
expect_tree "[data-id='layer.filters.status.control'] .sf-tab[aria-selected='true']" '.ok and .count == 1 and .nodes[0].text == "open"'
ctl key Ctrl+d
expect_tree "[data-id='layer.filters']" '.count == 0'
expect_tree "[data-id='main.filters.status'], [data-id='main.filters.note']" '.ok and ([.nodes[].text] == ["status open", "default"])'
[[ -f "$XDG_CONFIG_HOME/tern-kata/filters.toml" ]]
grep -q 'status = "open"' "$XDG_CONFIG_HOME/tern-kata/filters.toml"

quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id='main.filters.status'], [data-id='main.filters.note']" '.ok and ([.nodes[].text] == ["status open", "default"])'

# c clears the filters for this Pane only.
ctl key c
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

# An empty card's ctrl+d removes the default.
ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1'
ctl click "[data-id='layer.filters.foot.default']"
expect_tree "[data-id='layer.filters']" '.count == 0'
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
if grep -q 'status' "$XDG_CONFIG_HOME/tern-kata/filters.toml"; then printf 'filters.toml still holds a status filter after Default\n' >&2; return 1; fi
