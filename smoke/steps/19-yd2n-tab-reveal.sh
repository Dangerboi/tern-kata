# Switching Activity tab scrolls the new tab into view: with the detail scrolled to the bottom on the empty
# Links tab, tab to Comments (taller) and its last card is on screen; the same by a click on a tab name.
# Own Project and Issue; it cleans up.
tr_rects() { ctl tree "[data-id='main.columns.detail.viewport'],$1" | jq -c '[.nodes[].rect]'; }
# Whether the last node of selector $1 lies inside the detail viewport, in px.
tr_inside() { tr_rects "$1" | jq -e 'length >= 2 and .[-1][1] >= .[0][1] and (.[-1][1] + .[-1][3]) <= (.[0][1] + .[0][3])' >/dev/null; }
tr_wait_inside() { # <label> <node selector>
  for ((attempt = 0; attempt < 40; attempt++)); do
    tr_inside "$2" && return 0
    sleep 0.1
  done
  printf '%s: %s is not inside the detail viewport: %s\n' "$1" "$2" "$(tr_rects "$2")" >&2
  return 1
}
tr_scroll_to_bottom() {
  ctl hover "[data-id='main.columns.detail.viewport']" >/dev/null
  for _ in $(seq 12); do ctl wheel 0 300 >/dev/null; done
  for ((attempt = 0; attempt < 40; attempt++)); do
    ctl tree "[data-id='main.columns.detail.viewport'],[data-id='main.columns.detail.viewport.content']" | jq -e '.nodes as $n | ($n[1].rect[1] + $n[1].rect[3]) - ($n[0].rect[1] + $n[0].rect[3]) | floor <= 1' >/dev/null && return 0
    sleep 0.1
  done
  printf 'The detail never reached its bottom\n' >&2
  return 1
}

tr_dir="$tmp/reveal-workspace"
mkdir -p "$tr_dir" "$tmp/reveal-unbound"
kata --workspace "$tr_dir" init --project reveal-detail --json >/dev/null
tr_project=$(kata --workspace "$tr_dir" projects list --json | jq -er '.projects[] | select(.name == "reveal-detail") | .uid')
tr_body=$(for line in $(seq 40); do printf 'Body line %s\n\n' "$line"; done)
kata --workspace "$tr_dir" create "Reveal main" --body "$tr_body" --json >"$tmp/reveal-main.json"
tr_main=$(jq -r '.issue.uid' "$tmp/reveal-main.json")
tr_short=$(jq -r '.issue.short_id' "$tmp/reveal-main.json")
for n in 1 2 3 4; do kata --workspace "$tr_dir" comment "$tr_short" --body "Reveal comment $n" --json >/dev/null; done

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/reveal-unbound" "$KATA_HOME" "$tmp" >"$tmp/reveal-launch.ctl"
ctl --file "$tmp/reveal-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$tr_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$tr_project']"
expect_tree "[data-id\$='.$tr_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$tr_main']"
tr_tab="main.columns.detail.viewport.content.activity.tabs.tab"
tr_activity="main.columns.detail.viewport.content.activity"
tr_label() { expect_tree "[data-id='$tr_tab$1']" ".ok and (.nodes[0].text | $2)"; }
tr_cards="[data-id^='$tr_activity.'].sf-card"
ctl key enter
tr_label 0 'startswith("[ Comments (4) ]")'

# To the Links tab (empty), then to the bottom of the detail.
ctl key shift+Tab
tr_label 2 'startswith("[ Links (0) ]")'
tr_scroll_to_bottom
expect_tree "[data-id='$tr_activity.empty']" '.ok and .count == 1'

# tab wraps to Comments, taller than the empty line: its last card must be in view, the tab names too.
ctl key Tab
tr_label 0 'startswith("[ Comments (4) ]")'
expect_tree "$tr_cards" '.ok and .count == 4'
tr_wait_inside "tab to Comments" "$tr_cards"
tr_wait_inside "tab to Comments, tab names" "[data-id='$tr_activity.tabs']"

# The same by a click on a tab name: to the bottom on Links again, click Comments.
ctl key shift+Tab
tr_label 2 'startswith("[ Links (0) ]")'
tr_scroll_to_bottom
ctl click "[data-id='${tr_tab}0']"
tr_label 0 'startswith("[ Comments (4) ]")'
tr_wait_inside "click on Comments" "$tr_cards"

quit_app
kata projects remove reveal-detail --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
