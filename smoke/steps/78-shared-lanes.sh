# The Lanes card shares the Project's Lane list through its tern.lanes metadata: a Pane with a config folder of its own (another
# machine, no lanes.toml) shows it, another client's change reaches a running Board live, and stopping sharing returns the Pane
# to its own lanes.toml, here none: the default list.
sl_source="[data-id='main.columns.list.lane-source']"
sl_lane() { printf "[data-id='main.columns.list.board.lane%s.head.title']" "$1"; }
sl_project() { kata projects show smoke --json | jq "$1"; }
quit_app
ctl --file "$tmp/launch.ctl"
ctl key 5
expect_tree "$sl_source" '.count == 1 and (.nodes[0].text | contains("default"))'

ctl key B
expect_tree "[data-id='layer.lanes']" '.count == 1'
ctl type '", shared-lane"'
for _ in 1 2 3; do ctl key Tab; done
ctl key Right
expect_tree "[data-id='layer.lanes.share.control'] .sf-tab[aria-selected='true']" '.count == 1 and .nodes[0].text == "The Project"'
ctl key Enter
expect_tree "[data-id='layer.lanes']" '.count == 0'
expect_tree "$sl_source" '.count == 1 and (.nodes[0].text | contains("shared with the Project"))'
expect_tree "$(sl_lane 5)" '.ok and .nodes[0].text == "Shared lane"'
sl_project '.project.metadata["tern.lanes"].labels | last == "shared-lane"' | grep -qx true
if grep -qs shared-lane "$XDG_CONFIG_HOME/tern-kata/lanes.toml"; then printf 'sharing wrote lanes.toml\n' >&2; return 1; fi
quit_app

sed "s|KATA_HOME=|XDG_CONFIG_HOME=$tmp/other-config KATA_HOME=|" "$tmp/launch.ctl" >"$tmp/other-config.ctl"
ctl --file "$tmp/other-config.ctl"
ctl key 5
expect_tree "$sl_source" '.count == 1 and (.nodes[0].text | contains("shared with the Project"))'
expect_tree "$(sl_lane 5)" '.ok and .nodes[0].text == "Shared lane"'
sl_socket=$(kata daemon status --json | jq -r '.selected.endpoint | sub("^unix://"; "")')
curl -fsS --unix-socket "$sl_socket" -X POST -H 'Content-Type: application/json' \
  "http://kata.invalid/api/v1/projects/$(sl_project '.project.id')/metadata" \
  -d '{"actor":"rival","patch":{"tern.lanes":{"labels":["triage","from-elsewhere"]}}}' >/dev/null
expect_tree "$(sl_lane 2)" '.ok and .nodes[0].text == "From elsewhere"'
ctl key B
expect_tree "[data-id='layer.lanes.foot.stop']" '.count == 1'
ctl key Ctrl+x
expect_tree "[data-id='layer.lanes']" '.count == 0'
expect_tree "$sl_source" '.count == 1 and (.nodes[0].text | contains("default"))'
sl_project '.project.metadata | has("tern.lanes")' | grep -qx false
quit_app
ctl --file "$tmp/launch.ctl"
