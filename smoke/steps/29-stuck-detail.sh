# The detail keeps scrolling whatever is clicked, in any order: rows, an Activity tab, a comment row,
# a link jump and esc, Collection switches, a resize between tiers, and the Pane losing the window's
# focus and getting it back with a click. Tern keeps the node of the last reveal in view and ignores the
# wheel past it, so one reveal the app sent for a click (title, heading, row) froze the detail until the
# column was removed and drawn again. After every click the wheel must still move the detail (which ops the
# app may send is the Pane driver's tests). Own Project and Issues; it cleans up.
stuck_y() { ctl tree "[data-id='main.columns.detail.viewport.content.heading']" | jq -r '.nodes[0].rect[1] // empty | floor'; }
stuck_wheel() { for _ in $(seq "$2"); do ctl wheel 0 "$1" >/dev/null; done; }
# The wheel moves the detail down from its top and back up, whatever was clicked before.
stuck_scrolls() {
  local top y
  ctl hover "[data-id='main.columns.detail.viewport']" >/dev/null
  stuck_wheel -300 8
  sleep 0.5
  top=$(stuck_y)
  stuck_wheel 300 2
  for ((attempt = 0; attempt < 30; attempt++)); do
    y=$(stuck_y)
    [[ -z $y ]] || (( y > top - 100 )) || break
    sleep 0.1
  done
  [[ -n $y && $y -le $((top - 100)) ]] || { printf 'The detail did not scroll after %s (top %s, now %s)\n' "$1" "$top" "$y" >&2; return 1; }
  stuck_wheel -300 8
  sleep 0.5
}
# Wheels down until sel is inside the detail's list, so a click on it lands on it.
stuck_bring_into_view() {
  ctl hover "[data-id='main.columns.detail.viewport']" >/dev/null
  for ((attempt = 0; attempt < 20; attempt++)); do
    ctl tree "[data-id='main.columns.detail.viewport'],$1" | jq -e '.nodes as $n | ($n | length) == 2 and ($n[1].rect[1] > $n[0].rect[1]) and ($n[1].rect[1] + $n[1].rect[3] <= $n[0].rect[1] + $n[0].rect[3])' >/dev/null && return 0
    ctl wheel 0 300 >/dev/null
    sleep 0.2
  done
  printf '%s never scrolled into view\n' "$1" >&2
  return 1
}

stuck_dir="$tmp/stuck-workspace"
mkdir -p "$stuck_dir" "$tmp/stuck-unbound"
kata --workspace "$stuck_dir" init --project stuck-detail --json >/dev/null
stuck_project=$(kata --workspace "$stuck_dir" projects list --json | jq -er '.projects[] | select(.name == "stuck-detail") | .uid')
kata --workspace "$stuck_dir" create "Stuck peer" --json >"$tmp/stuck-peer.json"
stuck_body=$(for line in $(seq 40); do printf 'Body line %s\n\n' "$line"; done)
kata --workspace "$stuck_dir" create "Stuck long" --body "$stuck_body" --blocked-by "$(jq -r '.issue.short_id' "$tmp/stuck-peer.json")" --json >"$tmp/stuck-long.json"
stuck_long=$(jq -r '.issue.uid' "$tmp/stuck-long.json")
stuck_peer=$(jq -r '.issue.uid' "$tmp/stuck-peer.json")
kata --workspace "$stuck_dir" comment "$(jq -r '.issue.short_id' "$tmp/stuck-long.json")" --body "Stuck comment" --json >/dev/null

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/stuck-unbound" "$KATA_HOME" "$tmp" >"$tmp/stuck-launch.ctl"
ctl --file "$tmp/stuck-launch.ctl"
read -r stuck_width stuck_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$stuck_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$stuck_project']"
expect_tree "[data-id$='.$stuck_long']" '.ok and .count == 1'

# Rows, the detail title and an Activity tab.
ctl click "[data-id$='.$stuck_long']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck long"))'
stuck_scrolls "a click on an Issue row"
ctl click "[data-id='main.columns.detail.viewport.content.heading.title']"
stuck_scrolls "a click on the detail title"
stuck_bring_into_view "[data-id='main.columns.detail.viewport.content.activity.tabs.tab1']"
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab1']"
stuck_scrolls "a click on an Activity tab"

# A comment row and a link jump, then esc unwinds the jump.
stuck_bring_into_view "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
stuck_bring_into_view "[data-id^='main.columns.detail.viewport.content.activity.'].sf-card"
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'].sf-card"
stuck_scrolls "a click on a comment row"
stuck_bring_into_view "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
stuck_bring_into_view "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']"
ctl click "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck peer"))'
ctl key esc
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck long"))'
stuck_scrolls "a link jump and esc"
ctl key esc
ctl key esc

# Collection switches. Inside a Project the sidebar is gone and esc opens the popup; a fixed Collection picked there brings the sidebar back.
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.list.all-open']"
expect_tree "[data-id='main.columns.collections.all-open']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$stuck_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$stuck_project']"
expect_tree "[data-id$='.$stuck_long']" '.ok and .count == 1'
ctl click "[data-id$='.$stuck_long']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck long"))'
stuck_scrolls "a Collection switch"

# A resize between tiers: detail only, then side by side again.
ctl click "[data-id='main.columns.detail.viewport.content.heading.title']"
ctl resize 1000 "$stuck_height"
expect_tree "[data-id='main.columns.list']" '.ok and .count == 0'
ctl resize "$stuck_width" "$stuck_height"
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
stuck_scrolls "a resize between tiers"

# The window loses its focus and gets it back with a click.
ctl window-focus off
ctl window-focus on
ctl click "[data-id$='.$stuck_peer']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck peer"))'
ctl click "[data-id$='.$stuck_long']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Stuck long"))'
stuck_scrolls "the window regaining focus"

quit_app
kata projects remove stuck-detail --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
