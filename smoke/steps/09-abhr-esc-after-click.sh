# esc backs out after any click exactly as it does after the same keys. Fixtures are this step's own.
kata --workspace "$tmp/workspace" create "Esc parent" --json >"$tmp/esc-parent.json"
esc_parent=$(jq -r '.issue.uid' "$tmp/esc-parent.json")
kata --workspace "$tmp/workspace" create "Esc child" --parent "$esc_parent" --json >"$tmp/esc-child.json"
esc_child=$(jq -r '.issue.uid' "$tmp/esc-child.json")
kata --workspace "$tmp/workspace" create "Esc blocker" --json >"$tmp/esc-blocker.json"
esc_blocker=$(jq -r '.issue.uid' "$tmp/esc-blocker.json")
kata --workspace "$tmp/workspace" create "Esc target" --blocked-by "$(jq -r '.issue.short_id' "$tmp/esc-blocker.json")" --json >"$tmp/esc-target.json"
esc_target=$(jq -r '.issue.uid' "$tmp/esc-target.json")
esc_project=$(kata --workspace "$tmp/workspace" projects list --json | jq -er '.projects[] | select(.name == "smoke") | .uid')
# An unbound directory keeps the sidebar, so every scenario below can leave it and return.
mkdir -p "$tmp/esc-unbound"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/esc-unbound" "$KATA_HOME" "$tmp" >"$tmp/esc-launch.ctl"
ctl plugins expect smoke
ctl key q
expect_tree ".sf-list" '.ok and .count == 0'
ctl --file "$tmp/esc-launch.ctl"
expect_tree "[data-id='main.columns.collections.all-open']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.all-open']"
expect_tree "[data-id$='.$esc_parent']" '.ok and .count == 1'
expect_tree "[data-id$='.$esc_child']" '.ok and .count == 0'

# A parent row clicked while the keys are in the sidebar takes them to the list, selects, expands.
ctl key esc
hint_expect_mode collections
ctl click "[data-id$='.$esc_parent']"
hint_expect_mode list
expect_tree "[data-id$='.$esc_parent']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id$='.$esc_child']" '.ok and .count == 1'
ctl key esc
hint_expect_mode collections

# A nested child clicked from the sidebar behaves the same, and esc goes back out.
ctl click "[data-id$='.$esc_child']"
hint_expect_mode list
expect_tree "[data-id$='.$esc_child']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Esc child"))'
ctl key esc
hint_expect_mode collections

# A referenced Issue opened by click unwinds one jump per esc, then leaves detail, then the list.
ctl click "[data-id$='.$esc_target']"
hint_expect_mode list
ctl key enter
hint_expect_mode detail
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']" '.ok and .count == 1'
# The reference is the styled span itself, no separate button: its text ends in the peer's short id, and a click opens it.
esc_blocker_short=$(jq -r '.issue.short_id' "$tmp/esc-blocker.json")
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']" ".ok and (.nodes[0].text | endswith(\"#$esc_blocker_short\"))"
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Esc blocker"))'
hint_expect_mode detail
ctl key esc
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Esc target"))'
hint_expect_mode detail
ctl key esc
hint_expect_mode list
ctl key esc
hint_expect_mode collections

# A Collection clicked mid-jump ends the jump: enter then esc leaves detail instead of replaying it.
ctl click "[data-id$='.$esc_target']"
ctl key enter
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']" '.ok and .count == 1'
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Esc blocker"))'
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$esc_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$esc_project']"
hint_expect_mode list
expect_tree "[data-id$='.$esc_blocker']" '.ok and .count == 1'
ctl key enter
hint_expect_mode detail
ctl key esc
hint_expect_mode list
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Esc blocker"))'

# An Activity tab clicked gives detail the keys, and esc leaves it.
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
hint_expect_mode detail
ctl key esc
hint_expect_mode list

# Layers are closed by esc after a click inside them.
ctl key f
expect_tree "[data-id='layer.filters']" '.ok and .count == 1'
ctl click "[data-id='layer.filters.owner']"
ctl key esc
expect_tree "[data-id='layer.filters']" '.ok and .count == 0'
hint_expect_mode list
ctl key n
expect_tree "[data-id='layer.create']" '.ok and .count == 1'
ctl click "[data-id='layer.create.labels']"
ctl key esc
expect_tree "[data-id='layer.create']" '.ok and .count == 0'
hint_expect_mode list
ctl key x
expect_tree "[data-id='layer.close']" '.ok and .count == 1'
ctl click "[data-id='layer.close.field2']"
ctl key esc
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
hint_expect_mode list
ctl key +
expect_tree "[data-id='layer.fields']" '.ok and .count == 1'
ctl click "[data-id='layer.fields.value']"
ctl key esc
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
hint_expect_mode list
ctl key '?'
expect_tree "[data-id='layer.help']" '.ok and .count == 1'
ctl click "[data-id='layer.help']"
ctl key esc
expect_tree "[data-id='layer.help']" '.ok and .count == 0'
hint_expect_mode list
ctl key /
expect_tree "[data-id='main.columns.list.search']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.search']"
ctl key esc
expect_tree "[data-id='main.columns.list.search']" '.ok and .count == 0'
hint_expect_mode list

for fixture in esc-target esc-blocker esc-child esc-parent; do
  uid=$(jq -r '.issue.uid' "$tmp/$fixture.json")
  short=$(jq -r '.issue.short_id' "$tmp/$fixture.json")
  kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
done
ctl key q
expect_tree ".sf-list" '.ok and .count == 0'
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
