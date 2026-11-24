# A Board card dragged onto another Lane moves its Issue there as `>` does, a drop on its own Lane's header changes nothing, and a drop on Done opens the Close card.
# The selected card's … button opens the card's menu. Own Issues; they stay open.
bdr_dir="$tmp/workspace"
kata --workspace "$bdr_dir" create "Smoke drag first" --json >"$tmp/bdr-first.json"
kata --workspace "$bdr_dir" create "Smoke drag second" --label needs-triage --json >"$tmp/bdr-second.json"
bdr_first=$(jq -r '.issue.uid' "$tmp/bdr-first.json")
bdr_second=$(jq -r '.issue.uid' "$tmp/bdr-second.json")
bdr_lane() { printf "[data-id='main.columns.list.board.lane%s']" "$1"; }
bdr_card() { printf "[data-id='main.columns.list.board.lane%s.cards.%s']" "$1" "$2"; }
bdr_labels_have() { kata --workspace "$bdr_dir" show "$1" --json | jq -e --arg l "$2" '.issue.labels | index($l)' >/dev/null; }
# bdr_drag <card selector> <lane selector> <top|bottom>: drags the card into the Lane's column, near its top or its bottom edge.
bdr_drag() {
  local lx ly lw lh
  read -r lx ly lw lh < <(rect "$2")
  if [[ $3 == top ]]; then drag "$1" $((lx + lw / 2)) $((ly + 60)); else drag "$1" $((lx + lw / 2)) $((ly + lh - 20)); fi
}

ctl key 5
expect_tree "$(bdr_card 0 "$bdr_first")" '.ok and .count == 1'
expect_tree "$(bdr_card 1 "$bdr_second")" '.ok and .count == 1'
expect_tree "$(bdr_lane 1)" '.ok and .count == 1'

# A drop on another Lane moves the Issue there.
bdr_drag "$(bdr_card 0 "$bdr_first")" "$(bdr_lane 1)" top
expect_tree "$(bdr_card 1 "$bdr_first")" '.ok and .count == 1'
expect_tree "$(bdr_card 0 "$bdr_first")" '.ok and .count == 0'
wait_for bdr_labels_have "$bdr_first" needs-triage

# A drop on its own Lane's header row (outside the card list, which fills the Lane) moves nothing and leaves no custom order.
bdr_head="$(bdr_lane 1) [data-id='main.columns.list.board.lane1.head']"
read -r hx hy hw hh < <(rect "$bdr_head")
drag "$(bdr_card 1 "$bdr_first")" $((hx + hw / 2)) $((hy + hh / 2))
expect_tree "$(bdr_card 1 "$bdr_first")" '.ok and .count == 1'
expect_tree "$bdr_head" '.ok and .count == 1 and (.nodes[0].text | contains("custom order") | not)'
expect_tree "$(bdr_card 1 "$bdr_second")" '.ok and .count == 1'
kata --workspace "$bdr_dir" show "$bdr_first" --json | jq -e '.issue.labels == ["needs-triage"]' >/dev/null
kata --workspace "$bdr_dir" show "$bdr_second" --json | jq -e '.issue.labels == ["needs-triage"]' >/dev/null

# A drop on Done (the last column, with the default Lane list) opens the Close card; Escape leaves the Issue open.
bdr_first_rect() { rect "$(bdr_card 1 "$bdr_first")"; }
wait_settled bdr_first_rect
bdr_drag "$(bdr_card 1 "$bdr_first")" "$(bdr_lane 6)" top
expect_tree "[data-id='layer.close']" '.ok and .count == 1'
ctl key Escape
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
expect_tree "$(bdr_card 1 "$bdr_first")" '.ok and .count == 1'

wait_settled bdr_first_rect

# The selected card's … button opens the menu a right-click does.
ctl click "$(bdr_card 1 "$bdr_first")"
expect_tree "$(bdr_card 1 "$bdr_first")" '.ok and .count == 1 and (.nodes[0]["aria-selected"] == "true" or (.nodes[0].class | test("(^| )selected( |$)")))'
bdr_menu="[data-id^='main.columns.list.board.lane1.cards.$bdr_first.'] .sf-menu-trigger"
expect_tree "$bdr_menu" '.ok and .count == 1'
bdr_trigger_rect() { ctl tree "$bdr_menu" | jq -c '.nodes[0].rect'; }
wait_settled bdr_trigger_rect
ctl click "$bdr_menu"
expect_tree "[role='menu']" '.ok and .count == 1 and (.nodes[0].text | contains("Close…"))'
bdr_close_rect() { ctl tree "[role='menuitem']" | jq -c '[.nodes[] | select(.text | startswith("Close"))][0].rect'; }
wait_settled bdr_close_rect
ctl tree "[role='menuitem']" | jq -er '[.nodes[] | select(.text | startswith("Close"))][0].rect | "\(.[0] + .[2] / 2) \(.[1] + .[3] / 2)"' >"$tmp/bdr-close.xy"
# shellcheck disable=SC2046
ctl click $(<"$tmp/bdr-close.xy")
expect_tree "[data-id='layer.close']" '.ok and .count == 1'
ctl key Escape
expect_tree "[data-id='layer.close']" '.ok and .count == 0'

quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
