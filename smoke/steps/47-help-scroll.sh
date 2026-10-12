# Closing the help sheet gives the columns back as they were: the detail and the Issue list
# stay scrolled where they were. Own Project and Issues; it cleans up.
hs_y() { ctl tree "$1" | jq -r '.nodes[0].rect[1] // empty | floor'; }
hs_title="[data-id='main.columns.detail.viewport.content.heading.title']"
hs_wheel() { ctl hover "[data-id='$1']" >/dev/null; for _ in $(seq "$3"); do ctl wheel 0 "$2" >/dev/null; done; sleep 0.5; }

hs_dir="$tmp/help-workspace"
mkdir -p "$hs_dir" "$tmp/help-unbound"
kata --workspace "$hs_dir" init --project help-scroll --json >/dev/null
hs_project=$(kata --workspace "$hs_dir" projects list --json | jq -er '.projects[] | select(.name == "help-scroll") | .uid')
for n in $(seq 40); do kata --workspace "$hs_dir" create "Help filler $n" --json >/dev/null; done
hs_body=$(for line in $(seq 40); do printf 'Body line %s\n\n' "$line"; done)
kata --workspace "$hs_dir" create "Help long" --body "$hs_body" --json >"$tmp/help-long.json"
hs_long=$(jq -r '.issue.uid' "$tmp/help-long.json")
hs_row="[data-id='main.columns.list.issues.$hs_long']"

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/help-unbound" "$KATA_HOME" "$tmp" >"$tmp/help-launch.ctl"
ctl --file "$tmp/help-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$hs_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$hs_project']"
expect_tree "$hs_row" '.ok and .count == 1'
ctl click "$hs_row"
expect_tree "$hs_title" '.ok and (.nodes[0].text | contains("Help long"))'

# Scroll the detail and the list away from their tops.
hs_title_top=$(hs_y "$hs_title")
hs_wheel main.columns.detail.viewport 300 4
hs_wheel main.columns.list.issues 300 4
hs_title_y=$(hs_y "$hs_title")
hs_row_y=$(hs_y "$hs_row")
(( hs_title_y < hs_title_top - 100 )) || { printf 'The detail did not scroll (%s, was %s)\n' "$hs_title_y" "$hs_title_top" >&2; return 1; }

ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
expect_tree '[data-id="main.topbar"],[data-id="main.columns"],[data-id="dock.hints"]' '.ok and .count == 0'
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 0'
expect_tree '[data-id="main.columns"]' '.ok and .count == 1'
sleep 0.5
[[ $(hs_y "$hs_title") == "$hs_title_y" && $(hs_y "$hs_row") == "$hs_row_y" ]] || { printf 'Help closed with the columns scrolled elsewhere: detail %s -> %s, list row %s -> %s\n' "$hs_title_y" "$(hs_y "$hs_title")" "$hs_row_y" "$(hs_y "$hs_row")" >&2; return 1; }

# A live update that changes the Issue shown in the detail while the sheet is open must not end the Pane.
ctl key '?'
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
kata --workspace "$hs_dir" delete "$hs_long" --force --confirm "DELETE help-scroll#$(jq -r '.issue.short_id' "$tmp/help-long.json")" --json >/dev/null
sleep 1
expect_tree '[data-id="main.help"]' '.ok and .count == 1'
ctl key '?'
expect_tree "$hs_row" '.ok and .count == 0'
expect_tree "$hs_title" '.ok and (.nodes[0].text | contains("Help filler"))'

quit_app
kata projects remove help-scroll --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
