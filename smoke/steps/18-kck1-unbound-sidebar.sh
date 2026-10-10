# An unbound directory shows the sidebar on a fixed Collection and drops it once a Project is entered; esc then opens the popup, and a fixed Collection brings the sidebar back.
us_sidebar="[data-id='main.columns.collections']"
us_heading="[data-id='main.topbar.heading']"
us_smoke_uid=$(kata projects list --json | jq -er '.projects[] | select(.name == "smoke") | .uid')
quit_app
mkdir -p "$tmp/sidebar-unbound"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/sidebar-unbound" "$KATA_HOME" "$tmp" >"$tmp/sidebar-unbound.ctl"
ctl --file "$tmp/sidebar-unbound.ctl"
expect_tree "$us_sidebar" '.ok and .count == 1'
expect_tree "$us_heading" '.ok and .nodes[0].text == "Inbox"'

ctl click "[data-id='main.columns.collections.project.$us_smoke_uid']"
expect_tree "$us_heading" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "$us_sidebar" '.ok and .count == 0'

ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.list.inbox']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$us_heading" '.ok and .nodes[0].text == "Inbox"'
expect_tree "$us_sidebar" '.ok and .count == 1'

quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
