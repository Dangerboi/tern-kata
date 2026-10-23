# Following a related id from a Board card into another Project switches the Pane to that Project, and esc goes back.
# Own Projects and Issues; it cleans up its Issues.
fp_a="$tmp/follow-a-workspace"
fp_b="$tmp/follow-b-workspace"
mkdir -p "$fp_a" "$fp_b"
kata --workspace "$fp_a" init --project follow-a --json >/dev/null
kata --workspace "$fp_b" init --project follow-b --json >/dev/null
kata --workspace "$fp_b" create "Follow there" --json >"$tmp/fp-there.json"
fp_there=$(jq -r '.issue.uid' "$tmp/fp-there.json")
fp_there_short=$(jq -r '.issue.short_id' "$tmp/fp-there.json")
kata --workspace "$fp_a" create "Follow here" --related "follow-b#$fp_there_short" --json >"$tmp/fp-here.json"
fp_here=$(jq -r '.issue.uid' "$tmp/fp-here.json")
fp_here_short=$(jq -r '.issue.short_id' "$tmp/fp-here.json")

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$fp_a" "$KATA_HOME" "$tmp" >"$tmp/fp-launch.ctl"
ctl --file "$tmp/fp-launch.ctl"
expect_tree "[data-id\$='.$fp_here']" '.ok and .count == 1'
ctl key 2
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
expect_tree "[data-id='main.topbar.left.heading']" '.ok and (.nodes[0].text | contains("Project: follow-a"))'
ctl click "[data-id\$='.cards.$fp_here']"
expect_tree "[data-id\$='.cards.$fp_here']" '.ok and (.nodes[0].class | contains("selected"))'

# The related id of the other Project is on the selected card; a click on it switches the Pane to that Project's Board.
expect_tree "[data-id*='.relationships.'][data-id\$='.$fp_there']" '.ok and .count == 1'
ctl click "[data-id*='.relationships.'][data-id\$='.$fp_there']"
expect_tree "[data-id='main.topbar.left.heading']" '.ok and (.nodes[0].text | contains("Project: follow-b"))'
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
expect_tree "[data-id\$='.cards.$fp_there']" '.ok and (.nodes[0].class | contains("selected"))'
expect_tree "[data-id\$='.cards.$fp_here']" '.ok and .count == 0'

# Esc returns to the first Project's Board with the card that was selected.
ctl key esc
expect_tree "[data-id='main.topbar.left.heading']" '.ok and (.nodes[0].text | contains("Project: follow-a"))'
expect_tree "[data-id='main.columns.list.board']" '.count == 1'
expect_tree "[data-id\$='.cards.$fp_here']" '.ok and (.nodes[0].class | contains("selected"))'
expect_tree "[data-id\$='.cards.$fp_there']" '.ok and .count == 0'

quit_app
kata --workspace "$fp_a" delete "$fp_here" --force --confirm "DELETE follow-a#$fp_here_short" --json >/dev/null
kata --workspace "$fp_b" delete "$fp_there" --force --confirm "DELETE follow-b#$fp_there_short" --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
