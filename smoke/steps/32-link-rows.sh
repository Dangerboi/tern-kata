# A Links row reads `relation | #id - title` as the Pane draws it: relation and separator muted; the reference and the title
# both jump on a click. Own Project and Issues; it cleans up.
lr_dir="$tmp/linkrows-workspace"
mkdir -p "$lr_dir" "$tmp/linkrows-unbound"
kata --workspace "$lr_dir" init --project linkrows --json >/dev/null
lr_project=$(kata --workspace "$lr_dir" projects list --json | jq -er '.projects[] | select(.name == "linkrows") | .uid')
kata --workspace "$lr_dir" create "Linkrows peer" --json >"$tmp/linkrows-peer.json"
lr_short=$(jq -r '.issue.short_id' "$tmp/linkrows-peer.json")
kata --workspace "$lr_dir" create "Linkrows main" --blocked-by "$lr_short" --json >"$tmp/linkrows-main.json"
lr_main=$(jq -r '.issue.uid' "$tmp/linkrows-main.json")

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/linkrows-unbound" "$KATA_HOME" "$tmp" >"$tmp/linkrows-launch.ctl"
ctl --file "$tmp/linkrows-launch.ctl"
ctl click "[data-id='main.columns.collections.back']"
expect_tree "[data-id='main.columns.collections.projects']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$lr_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$lr_project']"
expect_tree "[data-id\$='.$lr_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$lr_main']"
ctl key enter
ctl key Tab
ctl key Tab
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.heading']" '.ok and (.nodes[0].text == "Blocked by")'
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.jump']" ".ok and (.nodes[0].text == \"#$lr_short Linkrows peer\")"
# The row reads `Relation #id title` as laid out, with no pipe or dash between the pieces. The relation's muted span must sit
# inside its box, left of the reference.
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row']" ".ok and (.nodes[0].text | test(\"^Blocked by\\\\s*#$lr_short Linkrows peer$\"))"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row'] *" '.ok and ([.nodes[] | select(.text == "Blocked by" and .class == "sf-t-muted")] | length == 1) and ([.nodes[] | select(.class == "sf-t-muted")][0].rect as $r | [.nodes[] | select(.class | startswith("sf sf-text"))][1].rect as $ref | $r[0] + $r[2] <= $ref[0] + 0.5)'

# Link ids are daemon-wide, so the row is found by suffix. A click on the reference and title opens the peer.
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.jump']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Linkrows peer"))'

quit_app
kata projects remove linkrows --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
