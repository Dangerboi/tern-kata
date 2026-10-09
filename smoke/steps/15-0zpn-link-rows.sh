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
expect_tree "[data-id='main.columns.collections.project.$lr_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$lr_project']"
expect_tree "[data-id\$='.$lr_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$lr_main']"
ctl key enter
ctl key Tab
ctl key Tab
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.heading']" '.ok and (.nodes[0].text == "blocked_by |")'
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.jump']" ".ok and (.nodes[0].text == \"#$lr_short\")"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.title']" '.ok and (.nodes[0].text == "Linkrows peer")'
# The row reads `relation | #id - title` as laid out: Tern collapses the edge spaces of each piece, so the row's own text
# (innerText, what the Pane draws) carries the spaces only if the pieces keep them through layout. The relation's muted span
# must sit inside its box (its trailing space is laid out, not hanging over the reference).
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row']" ".ok and (.nodes[0].text == \"blocked_by | #$lr_short - Linkrows peer\")"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row'] *" '.ok and ([.nodes[] | select(.text == "blocked_by |" and .class == "sf-t-muted")] | length == 1) and ([.nodes[] | select(.class == "sf-t-muted")][0].rect as $r | [.nodes[] | select(.class | startswith("sf sf-text"))][1].rect as $ref | $r[0] + $r[2] <= $ref[0] + 0.5)'

# Link ids are daemon-wide, so the row is found by suffix. A click on the reference opens the peer; so does a click on the title, after esc backs out.
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.jump']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Linkrows peer"))'
ctl key esc
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Linkrows main"))'
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.row.title']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Linkrows peer"))'

quit_app
kata projects remove linkrows --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
