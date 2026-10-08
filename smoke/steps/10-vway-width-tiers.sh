# Width tiers: list and detail side by side from 140 columns, one column from 80,
# one notice below, and the plugin lives at every width. `ctl resize` takes
# pixels; the headless Pane is about 8 pixels per column, so the pixel widths
# below are checked against the columns Tern reports. It makes its own Project.
width_cols() { ctl state | jq -r '.panes[0].cols'; }
expect_alive() {
  ctl state | jq -e '.focused.running != null' >/dev/null || { printf 'tern-kata exited at %s\n' "$1" >&2; return 1; }
}
# Resizes to $1 pixels, then expects $2 list and $3 detail nodes, and the notice
# when $4 is 1 (a sliver of a Pane keeps only part of its nodes) or none when 0.
expect_width() {
  ctl resize "$1" "$width_height"
  expect_tree "[data-id^='main.notice']" ".ok and (.count > 0) == ($4 == 1)"
  expect_tree "[data-id='main.columns.list']" ".ok and .count == $2"
  expect_tree "[data-id='main.columns.detail']" ".ok and .count == $3"
  expect_alive "$1 pixels"
}
read -r width_full width_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')

# Its own Project and Issues, so no earlier step's fixtures or selection matter.
width_dir="$tmp/width-workspace"
mkdir -p "$width_dir"
kata --workspace "$width_dir" init --project width-tiers --json >/dev/null
for name in first second third; do
  kata --workspace "$width_dir" create "Width $name" --body "Width $name body" --json >/dev/null
done
quit_app
rm -f "$tmp/width-frames.jsonl"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$width_dir" "$tmp/width-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/width-launch.ctl"
ctl --file "$tmp/width-launch.ctl"

# Open the second Issue's detail in a wide Pane; it is what every resize must bring back.
expect_tree "[data-id='main.columns.list.issues']" '.ok and .count == 1 and (.nodes[0].text | contains("Width third"))'
ctl key g
ctl key j
ctl key enter
expect_tree "[data-id='main.columns.detail.title']" '.ok and .count == 1 and (.nodes[0].text | contains("Width second") and (contains("loading") | not))'
width_title=$(ctl tree "[data-id='main.columns.detail.title']" | jq -r '.nodes[0].text')
[[ -n $width_title ]]
expect_width 1200 1 1 0
(( $(width_cols) >= 140 ))

# From 80 to 139 columns one column shows; detail has the keys, so it is the one.
expect_width 1000 0 1 0
(( $(width_cols) >= 80 && $(width_cols) < 140 ))
expect_width 700 0 1 0
expect_tree "[data-id='dock.hints']" '.ok and .count == 1'

# Moving through the list in that column loads each detail behind it; the
# detail column is not drawn, so nothing may be asked of it.
title_json=$(jq -Rn --arg t "$width_title" '$t')
ctl key esc
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
ctl key j
expect_tree "[data-id='dock.hints.position']" '.nodes[0].text | startswith("3 /")'
ctl key PageDown
ctl key enter
expect_tree "[data-id='main.columns.detail.title']" ".ok and .count == 1 and (.nodes[0].text != $title_json) and (.nodes[0].text | contains(\"loading\") | not)"
ctl key esc
ctl key k
ctl key enter
expect_tree "[data-id='main.columns.detail.title']" ".nodes[0].text == $title_json"

# j and tab in the detail column move its selection and reveal a section node.
ctl key j
ctl key tab
expect_alive "j and tab in the detail-only tier"

# Below 80 columns one centred notice replaces everything, down to a sliver.
expect_width 620 0 0 1
(( $(width_cols) < 80 ))
expect_tree "[data-id='main.notice.text']" '.nodes[0].text | contains("80 columns")'
# Neither the Issue rows nor the sections exist now, so no reveal or scroll may
# name one: j and tab with detail focus, then j with list focus.
width_mark=$(wc -l <"$tmp/width-frames.jsonl")
ctl key j
ctl key tab
ctl key esc
ctl key j
ctl key k
ctl key enter
expect_alive "j and tab below 80 columns"
tail -n "+$((width_mark + 1))" "$tmp/width-frames.jsonl" | jq -c 'select(.dir == "out" and .verb == "f") | .body.ops[] | select(.[0] == "reveal" or .[0] == "scroll")' >"$tmp/width-undrawn-ops.json"
[[ ! -s $tmp/width-undrawn-ops.json ]] || { printf 'Reveal or scroll below 80 columns: %s\n' "$(<"$tmp/width-undrawn-ops.json")" >&2; return 1; }
expect_tree "[data-id='dock.hints']" '.ok and .count == 0'
for width in 300 120 40 8; do
  expect_width "$width" 0 0 1
done

# The keys still act on the hidden screen without hurting the Pane: paging and a
# help layer opened blind both come back when it widens.
ctl key PageDown
ctl key PageUp
ctl key '?'
expect_alive "keys below 80 columns"
expect_tree "[data-id^='layer.']" '.ok and .count == 0'
expect_width 1200 1 1 0
expect_tree "[data-id^='layer.']" '.ok and .count > 0'
ctl key esc
expect_tree "[data-id^='layer.']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.detail.title']" ".nodes[0].text == $title_json"
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("esc back"))'

# q still quits below 80 columns.
expect_width 300 0 0 1
quit_app
kata projects remove width-tiers --force --json >/dev/null
ctl resize "$width_full" "$width_height"
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
