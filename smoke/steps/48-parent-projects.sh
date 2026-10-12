# p sets the selected Issue's parent by id, as in kata's TUI; P opens the Menu at its Projects level:
# the popup in a bound directory, the sidebar in an unbound one. Own Project and Issues; it cleans up.
sp_dir="$tmp/set-parent-workspace"
mkdir -p "$sp_dir" "$tmp/set-parent-unbound"
kata --workspace "$sp_dir" init --project set-parent --json >/dev/null
sp_project=$(kata --workspace "$sp_dir" projects list --json | jq -er '.projects[] | select(.name == "set-parent") | .uid')
kata --workspace "$sp_dir" create "Set parent epic" --json >"$tmp/sp-epic.json"
kata --workspace "$sp_dir" create "Set parent task" --json >"$tmp/sp-task.json"
sp_epic=$(jq -r '.issue.uid' "$tmp/sp-epic.json")
sp_task=$(jq -r '.issue.uid' "$tmp/sp-task.json")

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$sp_dir" "$KATA_HOME" "$tmp" >"$tmp/sp-launch.ctl"
ctl --file "$tmp/sp-launch.ctl"
expect_tree "[data-id\$='.$sp_task']" '.ok and .count == 1'
ctl click "[data-id\$='.$sp_task']"
ctl key p
expect_tree "[data-id='layer.fields']" '.ok and .count == 1'
ctl type "\"#$(jq -r '.issue.short_id' "$tmp/sp-epic.json")\""
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
# The task now sits under the epic: hidden while the epic is folded, under a connector once unfolded.
expect_tree "[data-id='main.columns.list.issues.$sp_task']" '.ok and .count == 0'
ctl click "[data-id='main.columns.list.issues.$sp_epic']"
ctl key Right
expect_tree "[data-id='main.columns.list.issues.$sp_task']" '.ok and .count == 1 and (.nodes[0].text | startswith("└─"))'

ctl key P
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
expect_tree "[data-id^='layer.collections.list.']" '.ok and .nodes[0].text == "Back"'
expect_tree "[data-id='layer.collections.list.project.$sp_project']" '.ok and .count == 1'
ctl key esc
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/set-parent-unbound" "$KATA_HOME" "$tmp" >"$tmp/sp-unbound.ctl"
ctl --file "$tmp/sp-unbound.ctl"
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 1'
ctl key P
expect_tree "[data-id='main.columns.collections.project.$sp_project']" '.ok and .count == 1'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

quit_app
kata projects remove set-parent --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
