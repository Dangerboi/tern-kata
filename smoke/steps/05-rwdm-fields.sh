ctl key q
expect_tree .sf-list '.count == 0'
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata --as smoke-reviewer"\n' "$KATA_HOME" "$tmp" >"$tmp/fields-launch.ctl"
ctl --file "$tmp/fields-launch.ctl"
expect_tree "[data-id$='.$hierarchy_parent']" '.count == 1'
ctl click "[data-id$='.$hierarchy_parent']"
ctl key +
expect_tree "[data-id='layer.fields.value']" '.ok and .count == 1'
ctl key t
ctl key r
ctl key i
ctl key a
ctl key g
ctl key e
ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.labels.label-triage']" '.ok and .count == 1'
kata --workspace "$tmp/workspace" show "$hierarchy_parent" --json >"$tmp/fields-written.json"
jq -e '.labels[] | select(.label == "triage") | .author == "smoke-reviewer"' "$tmp/fields-written.json"
ctl key '!'
ctl key 2
ctl key enter
expect_tree "[data-id='main.columns.list.issues']" '.ok and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("P2"))'
