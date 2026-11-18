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
expect_tree "[data-id='main.topbar.left.heading']" ".ok and .count == 1 and .nodes[0].text == \"$bar_name\""
bar_cells="[data-id='main.topbar'],[data-id='main.topbar.left'],[data-id='main.topbar.right'],[data-id='main.topbar.right.menu']"

# Wide: the cells share one line (the bar is no taller than its tallest single-line cell, so a wrapped title or button label
# makes it taller; the Menu button is shorter than 1.75 lines of the facts cell), each cell lies inside the bar's vertical
# range and the left cell and the button are centred on it, the Menu button ends where the bar does, and the left cell ends
# before the right one begins.
expect_tree "$bar_cells" '.ok and .count == 4
  and (.nodes[0].rect as $bar | .nodes[1].rect as $h | .nodes[2].rect as $r | .nodes[3].rect as $d
    | ($bar[1] + $bar[3] / 2) as $mid
    | $bar[3] <= ([$h[3], $d[3]] | max) + 1
      and ([.nodes[1:][].rect | .[1] >= $bar[1] - 1 and .[1] + .[3] <= $bar[1] + $bar[3] + 1] | all)
      and ([$h, $d | (.[1] + .[3] / 2 - $mid) | fabs <= 1] | all)
      and (($d[0] + $d[2] - $bar[0] - $bar[2]) | fabs) <= 1 and $h[0] + $h[2] <= $r[0])
'
# The long name and the five-Page Page bar squeeze the Daemon at the default width; a wider Pane shows it whole.
ctl resize 3200 "$bar_height"
expect_tree "[data-id^='main.topbar.right.facts.fact']" '[.nodes[].text] | join("") | startswith("local")'
ctl resize "$bar_full" "$bar_height"
expect_tree "[data-id='main.filters.icon']" '.ok and .count == 0'

# One column (80-139): the facts give way, this binary's version first; the title is cut last.
ctl resize 980 "$bar_height"
expect_tree "[data-id='main.topbar.right.menu']" '.ok and .count == 1'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"$bar_name\""
ctl resize 680 "$bar_height"
expect_tree "[data-id='main.topbar.right.menu']" '.ok and .count == 1'
# The Page bar is compact: only the current Page keeps its label, the other Pages are icons.
expect_tree "[data-id^='main.topbar.left.pages.'][data-id$='.label'],[data-id^='main.pagebar.'][data-id$='.label']" '.ok and .count == 1'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"$bar_name\""
ctl resize "$bar_full" "$bar_height"

# A filter adds its line; clearing it takes the line away. The title never carries filters.
ctl key s
expect_tree "[data-id^='main.filters.']" '.ok and ([.nodes[] | select(.text) | .text] | any(. == "Status open"))'
expect_tree "[data-id='main.topbar.left.heading']" ".nodes[0].text == \"$bar_name\""
ctl key s
ctl key s
expect_tree "[data-id='main.filters.icon']" '.ok and .count == 0'

# The Menu button sits after the Daemon at the default width and shows no keycap; a click opens the Menu, Close (esc) closes it.
expect_tree "[data-id='main.topbar.right.menu']" '.ok and .count == 1 and .nodes[0].text == "Menu"'
expect_tree "[data-id='main.topbar.left.heading'],[data-id='main.topbar.right.menu']" '.nodes[0].rect[0] < .nodes[1].rect[0]'
ctl click "[data-id='main.topbar.right.menu']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.foot.close']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

quit_app
kata projects remove "$bar_name" --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
