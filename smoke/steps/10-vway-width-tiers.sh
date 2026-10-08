# Width tiers: list and detail side by side from 140 columns, one column from 80,
# one notice below, and the plugin lives at every width. `ctl resize` takes
# pixels; the headless Pane is about 8 pixels per column, so the pixel widths
# below are checked against the columns Tern reports. Uses the seeded Issues.
width_cols() { ctl state | jq -r '.panes[0].cols'; }
expect_alive() {
  ctl state | jq -e '.focused.running != null' >/dev/null || { printf 'tern-kata exited at %s\n' "$1" >&2; return 1; }
}
# Resizes to $1 pixels, then expects $2 list, $3 detail and $4 notice nodes.
expect_width() {
  ctl resize "$1" "$width_height"
  expect_tree "[data-id='main.notice']" ".ok and .count == $4"
  expect_tree "[data-id='main.columns.list']" ".ok and .count == $2"
  expect_tree "[data-id='main.columns.detail']" ".ok and .count == $3"
  expect_alive "$1 pixels"
}
read -r width_full width_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')

# Open the second Issue's detail in a wide Pane; it is what every resize must bring back.
expect_tree "[data-id='main.columns.list.issues']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke second Issue"))'
ctl key g
ctl key j
ctl key enter
expect_tree "[data-id='main.columns.detail.title']" '.ok and .count == 1'
width_title=$(ctl tree "[data-id='main.columns.detail.title']" | jq -r '.nodes[0].text')
[[ -n $width_title ]]
expect_width 1200 1 1 0
(( $(width_cols) >= 140 ))

# From 80 to 139 columns one column shows; detail has the keys, so it is the one.
expect_width 1000 0 1 0
(( $(width_cols) >= 80 && $(width_cols) < 140 ))
expect_width 700 0 1 0
expect_tree "[data-id='dock.hints']" '.ok and .count == 1'

# Below 80 columns one centred notice replaces everything, down to a sliver.
expect_width 620 0 0 1
(( $(width_cols) < 80 ))
expect_tree "[data-id='main.notice.text']" '.nodes[0].text | contains("80 columns")'
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
expect_tree "[data-id='main.columns.detail.title']" ".nodes[0].text == $(jq -Rn --arg t "$width_title" '$t')"
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("esc back"))'

# q still quits below 80 columns.
expect_width 300 0 0 1
quit_app
ctl resize "$width_full" "$width_height"
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
