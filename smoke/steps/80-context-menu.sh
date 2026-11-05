# A right-click on an Issue row, a Lane row and a Board card opens a menu of the Issue's keys; choosing a row selects that Issue,
# then does what its key does (here x, the Close card). The menu is drawn in the page, so a row is chosen by a click on it. Own Issues; they stay open.
cm_dir="$tmp/workspace"
kata --workspace "$cm_dir" create "Smoke menu first" --json >"$tmp/cm-first.json"
kata --workspace "$cm_dir" create "Smoke menu second" --json >"$tmp/cm-second.json"
cm_first=$(jq -r '.issue.uid' "$tmp/cm-first.json")
cm_second=$(jq -r '.issue.uid' "$tmp/cm-second.json")
cm_row() { printf "[data-id='main.columns.list.issues.%s']" "$1"; }
cm_card() { printf "[data-id^='main.columns.list.board.lane'][data-id\$='.cards.%s']" "$1"; }
cm_selected() { expect_tree "$1" '.ok and .count == 1 and (.nodes[0]["aria-selected"] == "true" or (.nodes[0].class | test("(^| )selected( |$)")))'; }
# cm_pick right-clicks the row or card, checks the menu names the Issue's keys and chooses Close, which opens the Close card.
cm_pick() {
  ctl click "$1" right
  expect_tree "[role='menu']" '.ok and .count == 1 and (.nodes[0].text | startswith("Open↵Agent…W") and contains("CloseX") and contains("Move to Project⇧M"))'
  ctl tree "[role='menuitem']" | jq -er '[.nodes[] | select(.text | startswith("Close"))][0].rect | "\(.[0] + .[2] / 2) \(.[1] + .[3] / 2)"' >"$tmp/cm-close.xy"
  # shellcheck disable=SC2046
  ctl click $(<"$tmp/cm-close.xy")
  expect_tree "[data-id='layer.close']" '.ok and .count == 1'
  ctl key Escape
  expect_tree "[data-id='layer.close']" '.ok and .count == 0'
}

# The Issue list: the right-clicked row is not the selected one, and the pick makes it so.
expect_tree "$(cm_row "$cm_first")" '.ok and .count == 1'
expect_tree "$(cm_row "$cm_second")" '.ok and .count == 1'
ctl click "$(cm_row "$cm_first")"
cm_selected "$(cm_row "$cm_first")"
cm_pick "$(cm_row "$cm_second")"
cm_selected "$(cm_row "$cm_second")"

# The Board card.
ctl key 5
expect_tree "$(cm_card "$cm_first")" '.ok and .count == 1'
ctl click "$(cm_card "$cm_first")"
cm_selected "$(cm_card "$cm_first")"
cm_pick "$(cm_card "$cm_second")"
cm_selected "$(cm_card "$cm_second")"

# The Lane view's row.
ctl key Enter
expect_tree "[data-id='main.columns.list.lane']" '.ok and .count == 1'
ctl click "$(cm_row "$cm_first")"
cm_selected "$(cm_row "$cm_first")"
cm_pick "$(cm_row "$cm_second")"
cm_selected "$(cm_row "$cm_second")"

quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
