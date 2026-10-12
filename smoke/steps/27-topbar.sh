# Top bar: one row with the title left, the counts in the middle and the Daemon then the accent Menu button right-aligned;
# from 80 to 139 columns Tern shrinks the Daemon, then the counts, before the title; a second
# line lists filters only while one is on. Its own Project, with a long name to force the squeeze.
read -r bar_full bar_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')
bar_name="topbar-a-rather-long-project-name-for-the-bar"
bar_dir="$tmp/topbar-workspace"
mkdir -p "$bar_dir"
kata --workspace "$bar_dir" init --project "$bar_name" --json >/dev/null
kata --workspace "$bar_dir" create "Top bar Issue" --json >/dev/null
quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$bar_dir" "$KATA_HOME" "$tmp" >"$tmp/topbar-launch.ctl"
ctl --file "$tmp/topbar-launch.ctl"
expect_tree "[data-id='main.topbar.left.heading']" ".ok and .count == 1 and .nodes[0].text == \"Project: $bar_name\""
bar_cells="[data-id='main.topbar'],[data-id='main.topbar.left'],[data-id='main.topbar.counts'],[data-id='main.topbar.right'],[data-id='main.topbar.right.menu']"

# Wide: the three cells share one line, the Menu button ends where the bar does, the counts sit between.
expect_tree "$bar_cells" '.ok and .count == 5 and ([.nodes[].rect[1]] | unique | length) == 1
  and (.nodes[0].rect as $bar | .nodes[1].rect as $h | .nodes[2].rect as $c | .nodes[4].rect as $d
    | $d[0] + $d[2] == $bar[0] + $bar[2] and $h[0] + $h[2] < $c[0] and $c[0] + $c[2] < $d[0])
'
expect_tree "[data-id='main.topbar.right.daemon']" '.nodes[0].text | startswith("Daemon: local")'
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

# One column (80-139): the Daemon gives way first, then the counts, the title is cut last.
ctl resize 900 "$bar_height"
expect_tree "[data-id='main.topbar.right.daemon']" '.ok and .count == 1 and (.nodes[0].text | endswith("…"))'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"Project: $bar_name\""
ctl resize 680 "$bar_height"
expect_tree "[data-id='main.topbar.counts']" '.nodes[0].text | endswith("…")'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"Project: $bar_name\""
ctl resize "$bar_full" "$bar_height"

# A filter adds its line; clearing it takes the line away. The title never carries filters.
ctl key s
expect_tree "[data-id='main.filters']" '.ok and .count == 1 and .nodes[0].text == "status=open"'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"Project: $bar_name\""
ctl key s
ctl key s
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

# The title still opens the Menu.
ctl click "[data-id='main.topbar.left.heading']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

# The Menu button sits after the Daemon at the default width; a click opens the Menu, Close (esc) closes it.
expect_tree "[data-id='main.topbar.right.menu']" '.ok and .count == 1 and .nodes[0].text == "Menu"'
expect_tree "[data-id='main.topbar.left.heading'],[data-id='main.topbar.right.menu']" '.nodes[0].rect[0] < .nodes[1].rect[0]'
ctl click "[data-id='main.topbar.right.menu']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.close']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

quit_app
kata projects remove "$bar_name" --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
